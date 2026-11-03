package pacaroute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const configBundleFormat = "pacaroute-config"
const configBundleLimit = 24 << 20

// A bundle contains portable configuration only. Credentials, credential hashes,
// runtime state and machine settings are deliberately outside this contract.
type ConfigBundle struct {
	Format     string  `json:"format"`
	Version    int     `json:"version"`
	ExportedAt string  `json:"exported_at"`
	Config     *Config `json:"config"`
}

func ExportConfig(current *Config) ConfigBundle {
	c := cloneConfig(current)
	c.Revision = ""
	for i := range c.Clients {
		c.Clients[i].KeyHash = ""
		c.Clients[i].LegacyDirect = false
	}
	return ConfigBundle{configBundleFormat, 1, time.Now().UTC().Format(time.RFC3339), &c}
}

func decodeConfigBundle(data []byte) (ConfigBundle, error) {
	var b ConfigBundle
	if len(data) > configBundleLimit {
		return b, errors.New("configuration file exceeds 24 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&b) != nil || d.Decode(new(any)) != io.EOF {
		return b, errors.New("invalid PacaRoute configuration file")
	}
	if b.Format != configBundleFormat || b.Version != 1 || b.Config == nil {
		return b, errors.New("unsupported PacaRoute configuration format or version")
	}
	return b, nil
}

type ImportChange struct {
	Kind     string `json:"kind"`
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	FromName string `json:"from_name"`
	ToName   string `json:"to_name"`
}
type ImportIssue struct {
	Code string `json:"code"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}
type ImportCounts struct {
	Accounts int `json:"accounts"`
	Sources  int `json:"sources"`
	Pools    int `json:"pools"`
	Proxies  int `json:"proxies"`
	Clients  int `json:"clients"`
}
type ConfigImportPlan struct {
	Mode        string         `json:"mode"`
	Revision    string         `json:"revision"`
	Namespace   string         `json:"namespace"`
	Fingerprint string         `json:"fingerprint"`
	Added       ImportCounts   `json:"added"`
	Removed     ImportCounts   `json:"removed"`
	Changes     []ImportChange `json:"changes"`
	Issues      []ImportIssue  `json:"issues"`
	config      Config
	clientIDs   []string
}

func configCounts(c *Config) ImportCounts {
	return ImportCounts{len(c.Accounts), len(c.Sources), len(c.Pools), len(c.Proxies.Profiles), len(c.Clients)}
}

// Each import gets a fresh namespace, including replace. Reusing an old account
// or source ID could silently attach a credential left in the destination vault.
// Members/routes are scoped by their newly allocated pool/Client and retain order.
func planConfigImport(current *Config, b ConfigBundle, mode, namespace string) (*ConfigImportPlan, error) {
	if b.Format != configBundleFormat || b.Version != 1 || b.Config == nil {
		return nil, errors.New("unsupported PacaRoute configuration format or version")
	}
	if mode != "add" && mode != "replace" {
		return nil, errors.New("import mode must be add or replace")
	}
	if !validID(namespace) || len(namespace) != 33 || !strings.HasPrefix(namespace, "i") {
		return nil, errors.New("invalid import namespace")
	}
	in := cloneConfig(b.Config)
	if in.Revision != "" {
		return nil, errors.New("exported configuration must not include a revision")
	}
	for i := range in.Clients {
		c := &in.Clients[i]
		if c.KeyHash != "" || c.LegacyDirect {
			return nil, errors.New("configuration imports cannot contain Client keys or legacy routing")
		}
		if !c.Local {
			c.KeyHash = keyHash("import-preview-placeholder")
		}
	}
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("invalid imported configuration: %w", err)
	}
	for id := range in.Proxies.Bindings {
		if _, ok := in.Source(id); !ok {
			return nil, errors.New("proxy binding references an unknown source")
		}
	}
	// Materialize only referenced implicit identities. Otherwise references to
	// copilot/azure-local would silently connect the two workspaces.
	materializeLegacyIdentities(&in)
	p := &ConfigImportPlan{Mode: mode, Revision: current.Revision, Namespace: namespace, Added: configCounts(&in), Changes: []ImportChange{}, Issues: []ImportIssue{}}
	base := Config{}
	_ = base.Validate()
	if mode == "add" {
		base = cloneConfig(current)
	} else {
		p.Removed = configCounts(current)
	}
	// Names are user-visible; compare trimmed, case-folded names and allocate a
	// suffix independently from IDs so two differently named IDs still coexist.
	ids, names := map[string]map[string]bool{}, map[string]map[string]bool{}
	reserve := func(kind, id, name string) {
		if ids[kind] == nil {
			ids[kind] = map[string]bool{}
			names[kind] = map[string]bool{}
		}
		ids[kind][id] = true
		names[kind][strings.ToLower(strings.TrimSpace(name))] = true
	}
	reserve("proxies", "system", "System")
	reserve("proxies", "direct", "Direct")
	for _, a := range base.Accounts {
		reserve("accounts", a.ID, a.Name)
	}
	for _, s := range base.Sources {
		reserve("sources", s.ID, s.Name)
	}
	for _, pool := range base.Pools {
		reserve("pools", pool.ID, pool.Name)
	}
	for _, proxy := range base.Proxies.Profiles {
		reserve("proxies", proxy.ID, proxy.Name)
	}
	for _, client := range base.Clients {
		reserve("clients", client.ID, client.Name)
	}
	allocate := func(kind, oldID, oldName string, local bool) (string, string) {
		id := oldID
		if !local {
			stem := oldID
			if len(stem) > 19 {
				stem = stem[:19]
			}
			id = stem + "-" + keyHash(oldID)[:8] + "-" + namespace
			for n := 1; ids[kind][id]; n++ {
				id = stem + "-" + keyHash(fmt.Sprint(oldID, n))[:8] + "-" + namespace
			}
		}
		name := strings.TrimSpace(oldName)
		for n := 2; names[kind][strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s (%d)", strings.TrimSpace(oldName), n)
		}
		reserve(kind, id, name)
		p.Changes = append(p.Changes, ImportChange{kind, oldID, id, oldName, name})
		return id, name
	}
	accountMap, sourceMap, poolMap := map[string]string{}, map[string]string{}, map[string]string{}
	proxyMap := map[string]string{"": "system", "system": "system", "direct": "direct"}
	for _, a := range in.Accounts {
		old := a.ID
		a.ID, a.Name = allocate("accounts", a.ID, a.Name, false)
		accountMap[old] = a.ID
		if sharedAzure(&in, old) {
			a.LoginMode = "device"
			p.Issues = append(p.Issues, ImportIssue{"azure_login_isolated", a.ID, a.Name})
		}
		p.Issues = append(p.Issues, ImportIssue{"account_credentials_required", a.ID, a.Name})
		base.Accounts = append(base.Accounts, a)
	}
	for _, proxy := range in.Proxies.Profiles {
		old := proxy.ID
		proxy.ID, proxy.Name = allocate("proxies", old, proxy.Name, false)
		proxyMap[old] = proxy.ID
		base.Proxies.Profiles = append(base.Proxies.Profiles, proxy)
	}
	for _, source := range in.Sources {
		old := source.ID
		source.ID, source.Name = allocate("sources", old, source.Name, false)
		sourceMap[old] = source.ID
		source.AccountID = accountMap[source.AccountID]
		base.Sources = append(base.Sources, source)
		base.Proxies.Bindings[source.ID] = proxyMap[in.Proxies.Bindings[old]]
		if source.AuthMode == "api-key" {
			p.Issues = append(p.Issues, ImportIssue{"source_key_required", source.ID, source.Name})
		}
	}
	if mode == "replace" {
		base.Proxies.AccountProxyID = proxyMap[in.Proxies.AccountProxyID]
	} else if proxyMap[in.Proxies.AccountProxyID] != base.Proxies.AccountProxyID {
		p.Issues = append(p.Issues, ImportIssue{Code: "account_proxy_preserved"})
	}
	for _, pool := range in.Pools {
		old := pool.ID
		pool.ID, pool.Name = allocate("pools", old, pool.Name, false)
		poolMap[old] = pool.ID
		for i := range pool.Members {
			pool.Members[i].SourceID = sourceMap[pool.Members[i].SourceID]
		}
		base.Pools = append(base.Pools, pool)
	}
	for _, client := range in.Clients {
		_, conflict := base.Client(client.ID)
		if client.Local && conflict {
			client.Local = false
			p.Issues = append(p.Issues, ImportIssue{"local_client_converted", client.ID, client.Name})
		}
		client.ID, client.Name = allocate("clients", client.ID, client.Name, client.Local)
		if !client.Local {
			client.KeyHash = keyHash("import-preview-placeholder")
			p.clientIDs = append(p.clientIDs, client.ID)
			p.Issues = append(p.Issues, ImportIssue{"client_key_generated", client.ID, client.Name})
		}
		for i := range client.Routes {
			route := &client.Routes[i]
			if strings.HasPrefix(route.SourceID, "pool:") {
				route.SourceID = "pool:" + poolMap[strings.TrimPrefix(route.SourceID, "pool:")]
			} else {
				route.SourceID = sourceMap[route.SourceID]
			}
		}
		base.Clients = append(base.Clients, client)
	}
	for _, id := range in.SourceState.Paused {
		base.SourceState.Paused = append(base.SourceState.Paused, sourceMap[id])
	}
	if err := base.Validate(); err != nil {
		return nil, err
	}
	p.config = base
	p.Fingerprint = keyHash(string(jsonBytes(struct {
		Bundle                    ConfigBundle
		Mode, Revision, Namespace string
		Config                    Config
	}{b, mode, current.Revision, namespace, base})))
	return p, nil
}

func importNamespace() string { return "i" + strings.ReplaceAll(newID(), "-", "") }

func applyConfigImport(ctx context.Context, store *ConfigStore, auth *Auth, p *ConfigImportPlan) error {
	current, err := store.Reload()
	if err != nil {
		return err
	}
	if current.Revision != p.Revision {
		return errors.New("configuration changed; preview the import again")
	}
	c := cloneConfig(&p.config)
	for i := range c.Clients {
		for _, id := range p.clientIDs {
			if c.Clients[i].ID != id {
				continue
			}
			key := newKey()
			c.Clients[i].KeyHash = keyHash(key)
			if err := auth.SaveClientKey(ctx, c.Clients[i], key); err != nil {
				return errors.New("cannot save imported Client credential; configuration unchanged")
			}
		}
	}
	// Client credentials are hash-versioned by Auth. A losing concurrent write
	// can leave an unused encrypted record but cannot replace a live credential.
	return store.Save(c, p.Revision)
}

func (s *Server) configTransferAPI(w http.ResponseWriter, r *http.Request, action string) {
	if action == "export" && r.Method == "GET" {
		w.Header().Set("Content-Disposition", `attachment; filename="PacaRoute-config.json"`)
		writeJSON(w, 200, ExportConfig(s.Store.Snapshot()))
		return
	}
	if action != "import" || r.Method != "POST" {
		problem(w, 405, "unsupported configuration transfer operation")
		return
	}
	var input struct {
		Bundle      json.RawMessage `json:"bundle"`
		Mode        string          `json:"mode"`
		Revision    string          `json:"revision"`
		Namespace   string          `json:"namespace"`
		Fingerprint string          `json:"fingerprint"`
		Apply       bool            `json:"apply"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
		problem(w, 400, "invalid import request (maximum 32 MiB)")
		return
	}
	b, err := decodeConfigBundle(input.Bundle)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	current, err := s.Store.Reload()
	if err != nil {
		problem(w, 409, "cannot read current configuration")
		return
	}
	if input.Revision == "" {
		problem(w, 428, "configuration revision required")
		return
	}
	if input.Revision != current.Revision {
		problem(w, 409, "configuration changed; reload and preview again")
		return
	}
	if !input.Apply {
		input.Namespace = importNamespace()
	}
	p, err := planConfigImport(current, b, input.Mode, input.Namespace)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	// Bind the reviewed plan to this service as well as the file, mode and revision.
	p.Fingerprint = keyHash(s.AdminKey + ":config-import:" + p.Fingerprint)
	if !input.Apply {
		writeJSON(w, 200, p)
		return
	}
	if !constantEqual([]byte(input.Fingerprint), []byte(p.Fingerprint)) {
		problem(w, 409, "import preview changed; preview again")
		return
	}
	if err := applyConfigImport(r.Context(), s.Store, s.Auth, p); err != nil {
		problem(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "plan": p})
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
