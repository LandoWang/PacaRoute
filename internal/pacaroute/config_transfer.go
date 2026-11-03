package pacaroute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	base.normalize()
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
			id = stem + "-" + digestHex(oldID)[:8] + "-" + namespace
			for n := 1; ids[kind][id]; n++ {
				id = stem + "-" + digestHex(fmt.Sprint(oldID, n))[:8] + "-" + namespace
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
	p.Fingerprint = digestHex(string(jsonBytes(struct {
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

// digestHex is the SHA-256 hex digest used for imported ID suffixes and the
// preview fingerprint.
func digestHex(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
