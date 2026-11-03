package pacaroute

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/LandoWang/pacaroute/internal/pacaroute/clientconfig"
	"github.com/LandoWang/pacaroute/internal/pacaroute/transport"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

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
		if e := s.Snapshot().ValidateTarget(source, model, domain.GrantKind(client)); e != nil {
			return nil, e
		}
		p, e := clientconfig.PlanLocal(client, source, model, port)
		if e != nil {
			return nil, e
		}
		v := map[string]any{"path": p.Path, "fingerprint": p.Fingerprint, "changes": p.Changes}
		if a.get("apply", "") == "true" {
			backup, e := p.Apply(a.get("fingerprint", ""))
			if e != nil {
				return nil, e
			}
			v["backup_path"] = backup
		}
		return v, nil
	case "models":
		models, e := newCatalog(auth.config, auth, &auth.http).Models(ctx, source)
		return map[string]any{"models": models}, e
	}
	account, ok := s.Snapshot().Account(accountID)
	if !ok {
		return nil, errors.New("account not found")
	}
	if domain.AccountUsesAPIKey(account.Kind) {
		return map[string]any{"has_key": auth.HasAccountKey(accountID), "kind": account.Kind}, nil
	}
	if account.Kind == domain.AccountAzure {
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
			if e := transport.ReadJSONBody(in, &record); e != nil {
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
				if string(record["local"]) == "true" {
					delete(record, "key_hash")
				} else if action == "add" {
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
					if string(m["local"]) == "true" {
						return errors.New("local Client does not use a key")
					}
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

func configTransferCommand(ctx context.Context, args arguments, store *ConfigStore, auth *Auth, action string) (any, error) {
	path := args.get("file", "")
	if path == "" {
		return nil, errors.New("--file PATH required")
	}
	if action == "export" {
		b := ExportConfig(store.Snapshot())
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, errors.New("cannot create export file; choose a new file path")
		}
		err = json.NewEncoder(f).Encode(b)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return map[string]any{"ok": true, "file": path, "credentials_included": false}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open configuration file")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, configBundleLimit+1))
	if err != nil {
		return nil, err
	}
	b, err := decodeConfigBundle(data)
	if err != nil {
		return nil, err
	}
	p, err := planConfigImport(store.Snapshot(), b, args.get("mode", "add"), importNamespace())
	if err != nil {
		return nil, err
	}
	if args.get("apply", "") == "true" {
		if err := applyConfigImport(ctx, store, auth, p); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "applied": true, "plan": p}, nil
	}
	return map[string]any{"applied": false, "plan": p}, nil
}
