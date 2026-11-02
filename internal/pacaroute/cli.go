package pacaroute

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type arguments struct {
	words []string
	flags map[string]string
}

func parseArgs(args []string) (arguments, error) {
	v := arguments{flags: map[string]string{}}
	bools := map[string]bool{"apply": true, "allow-local": true, "local-management": true, "json": true, "no-open": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			v.words = append(v.words, arg)
			continue
		}
		name, value, has := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if name == "help" {
			v.words = []string{"help"}
			return v, nil
		}
		if bools[name] {
			if !has {
				value = "true"
			}
		} else if !has {
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
				return v, fmt.Errorf("missing --%s value", name)
			}
			i++
			value = args[i]
		}
		v.flags[name] = value
	}
	return v, nil
}
func (a arguments) get(k, d string) string {
	if v, ok := a.flags[k]; ok {
		return v
	}
	return d
}
func defaultRoot() string {
	if v := os.Getenv("PACAROUTE_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("COPILOT_BRIDGE_APP_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("COPILOT_BRIDGE_ACCOUNTS_FILE"); v != "" {
		return filepath.Dir(v)
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		base := filepath.Join(home, "Library", "Application Support")
		legacy := filepath.Join(base, "Copilot Bridge")
		if f, e := os.Stat(legacy); e == nil && f.IsDir() {
			return legacy
		}
		return filepath.Join(base, "PacaRoute")
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "PacaRoute")
		}
		return filepath.Join(home, "AppData", "Local", "PacaRoute")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "pacaroute")
	}
	return filepath.Join(home, ".local", "share", "pacaroute")
}
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	if len(args) == 0 && desktopEnabled {
		args = []string{"desktop"}
	}
	a, e := parseArgs(args)
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 2
	}
	if len(a.words) == 0 || a.words[0] == "help" {
		fmt.Fprint(out, `PacaRoute — Go agent request gateway

pacaroute serve [--bind 127.0.0.1] [--port 8766] [--allow-local]
           [--public-url https://gateway.example]  Origin behind a TLS proxy
pacaroute desktop [--port 8766]    Native tray and local service (desktop build)
pacaroute ui [--port 8766]         Open a running local service in your browser
serve --local-management          Enable loopback-only Agent config and sessions
pacaroute auth login --account ID  Device login (GitHub or isolated Azure)
pacaroute auth status|logout --account ID
pacaroute accounts|sources|pools|proxies|clients list
pacaroute accounts|sources|pools|proxies add|update   JSON record on stdin
pacaroute accounts|sources|pools|proxies|clients remove --id ID
pacaroute sources pause|resume --id ID
pacaroute clients add              JSON Client and routes on stdin; emits key once
pacaroute clients rotate --id ID
pacaroute key set --source ID      Resource API key on stdin (never argv)
pacaroute key set --account ID     OpenAI/Anthropic account API key on stdin
pacaroute config show|apply        Full config with revision; JSON on stdin
pacaroute configure --client codex|claude-code --source ID --model MODEL
                   [--port PORT] [--apply --fingerprint HASH]
pacaroute models --source ID
pacaroute usage|errors

Global: --home PATH (metadata), --credential-root PATH (existing credential authority)
Web management uses <home>/admin.key. Network clients always use their own keys.
--allow-local permits unauthenticated loopback Agent requests only.
`)
		return 0
	}
	if a.words[0] == "version" {
		fmt.Fprintln(out, "PacaRoute", Version)
		return 0
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	emit := func(v any) { _ = json.NewEncoder(out).Encode(v) }
	fail := func(e error) int { emit(map[string]any{"ok": false, "event": "error", "error": e.Error()}); return 1 }
	root := a.get("home", defaultRoot())
	s, e := NewConfigStore(root)
	if e != nil {
		return fail(e)
	}
	auth := NewAuth(s, a.get("credential-root", ""))
	auth.systemKeys = desktopEnabled && runtime.GOOS == "darwin"
	port, e := strconv.Atoi(a.get("port", "8766"))
	if e != nil || port < 1 || port > 65535 {
		return fail(errors.New("port must be 1–65535"))
	}
	cmd := a.words[0]
	action := ""
	if len(a.words) > 1 {
		action = a.words[1]
	}
	switch cmd {
	case "serve":
		return serve(ctx, a, s, auth, port, in, emit, fail)
	case "ui":
		if e := openManagement(root, port); e != nil {
			return fail(e)
		}
		emit(map[string]any{"url": fmt.Sprintf("http://127.0.0.1:%d", port)})
		return 0
	case "desktop":
		if action == "" {
			return runDesktop(ctx, a, s, auth, port, emit, fail)
		}
		v, e := desktop(ctx, a, s, auth, action, port, emit)
		if e != nil {
			return fail(e)
		}
		v["ok"] = true
		v["event"] = "result"
		emit(v)
		return 0
	case "auth":
		v, e := desktop(ctx, a, s, auth, map[string]string{"status": "account-status", "login": "login", "logout": "logout"}[action], port, emit)
		if e != nil {
			return fail(e)
		}
		emit(v)
		return 0
	case "configure":
		v, e := desktop(ctx, a, s, auth, "configure", port, emit)
		if e != nil {
			return fail(e)
		}
		emit(v)
		return 0
	case "models":
		v, e := auth.Models(ctx, a.get("source", "copilot"))
		if e != nil {
			return fail(e)
		}
		emit(v)
		return 0
	case "usage":
		to := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
		emit(NewActivityStore(root).Summary(to.Add(-24*time.Hour), to, a.get("source", ""), a.get("client", "")))
		return 0
	case "errors":
		emit(NewActivityStore(root).Recent(true))
		return 0
	case "key":
		if action != "set" {
			return fail(errors.New("use key set --source ID"))
		}
		b, e := io.ReadAll(io.LimitReader(in, 16385))
		if e != nil {
			return fail(e)
		}
		if account := a.get("account", ""); account != "" {
			e = auth.SaveAccountKey(ctx, account, strings.TrimSpace(string(b)))
		} else {
			e = auth.SaveKey(ctx, a.get("source", ""), strings.TrimSpace(string(b)))
		}
		if e != nil {
			return fail(e)
		}
		emit(map[string]bool{"ok": true})
		return 0
	case "metadata":
		c := cloneConfig(s.Snapshot())
		var target any
		kind := a.get("kind", "")
		switch kind {
		case "accounts":
			target = &c.Accounts
		case "sources":
			target = &c.Sources
		case "pools":
			target = &c.Pools
		case "proxies":
			target = &c.Proxies
		case "source-state":
			target = &c.SourceState
		default:
			return fail(errors.New("unknown metadata kind"))
		}
		if e = readJSONBody(in, target); e != nil {
			return fail(e)
		}
		if kind == "sources" {
			paused := []string{}
			for _, id := range c.SourceState.Paused {
				if _, ok := c.Source(id); ok {
					paused = append(paused, id)
				}
			}
			c.SourceState.Paused = paused
			for id := range c.Proxies.Bindings {
				if _, ok := c.Source(id); !ok {
					delete(c.Proxies.Bindings, id)
				}
			}
		}
		if e = s.Save(c, c.Revision); e != nil {
			return fail(e)
		}
		emit(map[string]any{"ok": true, "event": "result"})
		return 0
	case "config":
		if action == "show" {
			emit(s.Snapshot())
			return 0
		}
		if action != "apply" {
			return fail(errors.New("use config show or config apply"))
		}
		var c Config
		if e = readJSONBody(in, &c); e != nil {
			return fail(e)
		}
		if c.Revision == "" {
			return fail(errors.New("configuration revision required"))
		}
		if e = s.Save(c, c.Revision); e != nil {
			return fail(e)
		}
		emit(s.Snapshot())
		return 0
	default:
		if e = entityCommand(ctx, a, s, in, emit); e != nil {
			return fail(e)
		}
		return 0
	}
}
func serve(ctx context.Context, a arguments, s *ConfigStore, auth *Auth, port int, in io.Reader, emit func(any), fail func(error) int) int {
	unlock, e := fileLock(ctx, filepath.Join(s.Root, "pacaroute-service.lock"), 200*time.Millisecond)
	if e != nil {
		return fail(errors.New("PacaRoute already runs for this data directory"))
	}
	defer unlock()
	server, e := NewServer(s, auth)
	if e != nil {
		return fail(e)
	}
	server.AllowLocal = a.get("allow-local", "") == "true" || os.Getenv("COPILOT_BRIDGE_DESKTOP") == "1"
	server.TraceDir = a.get("trace-dir", "")
	server.LocalManagement = a.get("local-management", "") == "true"
	server.Port = port
	if server.LocalManagement && a.get("bind", "127.0.0.1") != "127.0.0.1" && a.get("bind", "") != "::1" {
		return fail(errors.New("local-management requires a loopback listener"))
	}
	if public := a.get("public-url", ""); public != "" {
		u, err := url.Parse(public)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fail(errors.New("public-url must be an HTTP(S) origin without a path"))
		}
		server.PublicURL = u.Scheme + "://" + u.Host
	}
	if os.Getenv("COPILOT_BRIDGE_SECRETS_STDIN") == "1" {
		reader := bufio.NewReader(in)
		line, e := reader.ReadBytes('\n')
		if e != nil && e != io.EOF {
			return fail(e)
		}
		var keys map[string]string
		if len(line) > 65536 || json.Unmarshal(line, &keys) != nil {
			return fail(errors.New("invalid desktop credential envelope"))
		}
		if e = auth.SetKeys(keys); e != nil {
			return fail(e)
		}
		go func() {
			for {
				line, e := reader.ReadBytes('\n')
				if e != nil {
					return
				}
				if len(line) > 65536 {
					continue
				}
				var keys map[string]string
				if json.Unmarshal(line, &keys) == nil {
					_ = auth.SetKeys(keys)
				}
			}
		}()
	}
	listener, e := net.Listen("tcp", net.JoinHostPort(a.get("bind", "127.0.0.1"), strconv.Itoa(port)))
	if e != nil {
		return fail(fmt.Errorf("listen failed: %w", e))
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 64 << 10}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	emit(map[string]any{"ok": true, "event": "ready", "name": "PacaRoute", "version": Version, "address": listener.Addr().String()})
	select {
	case e = <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return fail(e)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e = httpServer.Shutdown(shutdown); e != nil {
			_ = httpServer.Close()
		}
	}
	return 0
}
func desktop(ctx context.Context, a arguments, s *ConfigStore, auth *Auth, action string, port int, emit func(any)) (map[string]any, error) {
	source := a.get("source", "copilot")
	accountID := a.get("account", "")
	if accountID == "" {
		if src, ok := s.Snapshot().Source(source); ok {
			accountID = src.AccountID
		} else {
			accountID = "copilot"
		}
	}
	switch action {
	case "config-backups":
		return listBackups(a.get("client", ""))
	case "restore-config":
		return restoreBackup(a.get("client", ""), a.get("backup", ""), a.get("fingerprint", ""), a.get("apply", "") == "true")
	case "status":
		return map[string]any{"logged_in": auth.LoggedIn("copilot"), "clients": []any{clientState("codex", port), clientState("claude-code", port)}}, nil
	case "configure":
		client := a.get("client", "")
		model := a.get("model", "")
		if e := s.Snapshot().validateTarget(source, model, client); e != nil {
			return nil, e
		}
		p, e := planClient(client, source, model, port)
		if e != nil {
			return nil, e
		}
		v := map[string]any{"path": p.path, "fingerprint": p.fingerprint, "changes": p.changes}
		if a.get("apply", "") == "true" {
			backup, e := p.apply(a.get("fingerprint", ""))
			if e != nil {
				return nil, e
			}
			v["backup_path"] = backup
		}
		return v, nil
	case "models":
		models, e := auth.Models(ctx, source)
		return map[string]any{"models": models}, e
	}
	account, ok := s.Snapshot().Account(accountID)
	if !ok {
		return nil, errors.New("account not found")
	}
	if accountUsesAPIKey(account.Kind) {
		return map[string]any{"has_key": auth.HasAccountKey(accountID), "kind": account.Kind}, nil
	}
	if account.Kind == "azure" {
		if action == "login" || action == "logout" {
			e := auth.AzureLogin(ctx, accountID, action == "logout", emit)
			return map[string]any{"logged_in": action == "login"}, e
		}
		return auth.AzureAction(ctx, accountID, action, a.get("subscription", ""), a.get("resource", ""))
	}
	switch action {
	case "account-status":
		return map[string]any{"logged_in": auth.LoggedIn(accountID)}, nil
	case "account":
		return auth.AccountInfo(ctx, accountID)
	case "logout":
		e := auth.Logout(ctx, accountID)
		return map[string]any{"logged_in": false}, e
	case "login", "reauthorize":
		if action == "login" && auth.LoggedIn(accountID) {
			return map[string]any{"logged_in": true}, nil
		}
		d, e := auth.BeginLogin(ctx, accountID)
		if e != nil {
			return nil, e
		}
		emit(map[string]any{"ok": true, "event": "device_code", "user_code": d.UserCode, "verification_uri": d.VerificationURI, "expires_in": d.ExpiresIn})
		e = auth.CompleteLogin(ctx, d)
		return map[string]any{"logged_in": e == nil}, e
	}
	return nil, errors.New("unsupported action")
}
func entityCommand(ctx context.Context, a arguments, s *ConfigStore, in io.Reader, emit func(any)) error {
	if len(a.words) < 2 {
		return errors.New("use help to list commands")
	}
	kind, action := a.words[0], a.words[1]
	c := cloneConfig(s.Snapshot())
	var entries []json.RawMessage
	var value any
	switch kind {
	case "accounts":
		value = c.Accounts
	case "sources":
		value = c.Sources
	case "pools":
		value = c.Pools
	case "clients":
		value = c.Clients
	case "proxies":
		value = c.Proxies.Profiles
	default:
		return errors.New("unknown command; use help")
	}
	_ = json.Unmarshal(jsonBytes(value), &entries)
	if action == "list" {
		emit(value)
		return nil
	}
	id := a.get("id", "")
	extra := map[string]any{"ok": true}
	if kind == "sources" && (action == "pause" || action == "resume") {
		paused := []string{}
		for _, v := range c.SourceState.Paused {
			if v != id {
				paused = append(paused, v)
			}
		}
		if action == "pause" {
			paused = append(paused, id)
		}
		c.SourceState.Paused = paused
	} else {
		if action == "add" || action == "update" {
			var record object
			if e := readJSONBody(in, &record); e != nil {
				return e
			}
			id = rawString(record["id"])
			found := false
			for _, v := range entries {
				if rawString(obj(v)["id"]) == id {
					found = true
				}
			}
			if action == "add" && found {
				return errors.New("ID already exists")
			}
			if action == "update" && !found {
				return errors.New("ID not found")
			}
			if kind == "clients" {
				if action == "add" {
					key := newKey()
					record["key_hash"] = jsonBytes(keyHash(key))
					extra["api_key"] = key
				} else {
					for _, v := range entries {
						m := obj(v)
						if rawString(m["id"]) == id {
							record["key_hash"] = m["key_hash"]
						}
					}
				}
			}
			filtered := []json.RawMessage{}
			for _, v := range entries {
				if rawString(obj(v)["id"]) != id {
					filtered = append(filtered, v)
				}
			}
			entries = append(filtered, jsonBytes(record))
		} else if action == "remove" {
			filtered := []json.RawMessage{}
			found := false
			for _, v := range entries {
				if rawString(obj(v)["id"]) == id {
					found = true
				} else {
					filtered = append(filtered, v)
				}
			}
			if !found {
				return errors.New("ID not found")
			}
			entries = filtered
		} else if kind == "clients" && action == "rotate" {
			found := false
			for i, v := range entries {
				m := obj(v)
				if rawString(m["id"]) == id {
					key := newKey()
					m["key_hash"] = jsonBytes(keyHash(key))
					entries[i] = jsonBytes(m)
					extra["api_key"] = key
					found = true
				}
			}
			if !found {
				return errors.New("Client not found")
			}
		} else {
			return errors.New("unsupported entity operation")
		}
		data := jsonBytes(entries)
		switch kind {
		case "accounts":
			_ = json.Unmarshal(data, &c.Accounts)
		case "sources":
			_ = json.Unmarshal(data, &c.Sources)
		case "pools":
			_ = json.Unmarshal(data, &c.Pools)
		case "clients":
			_ = json.Unmarshal(data, &c.Clients)
		case "proxies":
			_ = json.Unmarshal(data, &c.Proxies.Profiles)
		}
	}
	if e := s.Save(c, c.Revision); e != nil {
		return e
	}
	emit(extra)
	return nil
}
