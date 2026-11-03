package pacaroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

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
		preferred := filepath.Join(base, "PacaRoute")
		if f, err := os.Stat(filepath.Join(preferred, "pacaroute.sqlite")); err == nil && !f.IsDir() {
			return preferred
		}
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

// usageText is printed for `help` or when no command is given.
const usageText = `PacaRoute — Go agent request gateway

pacaroute serve [--bind 127.0.0.1] [--port 8766] [--allow-local]
                [--admin-port 8767] [--no-serve]  Separate local management listener
           [--public-url https://gateway.example]  Origin behind a TLS proxy
pacaroute desktop [--port 8766]    Native tray and local service (desktop build)
pacaroute ui [--port 8766]         Open a running local service in your browser
serve --local-management          Enable loopback-only Agent configuration
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
pacaroute config export --file PATH     Portable configuration without credentials
pacaroute config import --file PATH [--mode add|replace] [--apply]
                                  Preview by default; --apply commits the import
pacaroute configure --client codex|claude-code --source ID --model MODEL
                   [--port PORT] [--apply --fingerprint HASH]
pacaroute connect --agent codex|claude-code --url URL --model ALIAS [--local] [--apply]
                  Client key on stdin; no router service or account required
pacaroute models --source ID
pacaroute usage|errors

Global: --home PATH (metadata), --credential-root PATH (existing credential authority)
Web management uses <home>/admin.key. Network clients always use their own keys.
--allow-local permits unauthenticated loopback Agent requests only.
`

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
		fmt.Fprint(out, usageText)
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
	if a.words[0] == "connect" {
		v, e := connectClient(a, in)
		if e != nil {
			return fail(e)
		}
		emit(v)
		return 0
	}
	root := a.get("home", defaultRoot())
	s, e := NewConfigStore(root)
	if e != nil {
		return fail(e)
	}
	defer s.Close()
	authority := a.get("credential-root", "")
	if authority == "" {
		authority = readDesktopSettings(s.Root).CredentialRoot
	}
	auth := NewAuth(s, authority)
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
		if _, explicit := a.flags["port"]; !explicit {
			if saved := readDesktopSettings(root); saved.AdminPort > 0 {
				port = saved.AdminPort
			}
		}
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
		v, e := newCatalog(auth.config, auth, &auth.http).Models(ctx, a.get("source", "copilot"))
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
		if action == "export" || action == "import" {
			v, err := configTransferCommand(ctx, a, s, auth, action)
			if err != nil {
				return fail(err)
			}
			emit(v)
			return 0
		}
		if action == "show" {
			emit(s.Snapshot())
			return 0
		}
		if action != "apply" {
			return fail(errors.New("use config show or config apply"))
		}
		var c domain.Config
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
