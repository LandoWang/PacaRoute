package pacaroute

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func transferFixture(t *testing.T) domain.Config {
	t.Helper()
	c := domain.Config{
		Accounts:    []domain.Account{{ID: "api", Name: "Work", Kind: "openai"}},
		Sources:     []domain.Source{{ID: "resource", Name: "Models", Kind: "openai", AccountID: "api", AuthMode: "api-key", Endpoint: "https://example.com/v1", Deployments: []string{"astra", "sonnet"}}},
		Proxies:     domain.Proxies{Profiles: []domain.Proxy{{ID: "office", Name: "Office", Mode: "custom", Address: "http://localhost:8888"}}, Bindings: map[string]string{"resource": "office", "copilot": "direct"}, AccountProxyID: "office"},
		Pools:       []domain.Pool{{ID: "combined", Name: "Models", Agent: "codex", Model: "astra", MultiModel: true, Failover: true, Members: []domain.Member{{ID: "first", SourceID: "resource", Model: "astra", Weight: 3}, {ID: "second", SourceID: "copilot", Model: "gpt-6-astra", Alias: "astra", Weight: 2}, {ID: "third", SourceID: "resource", Model: "sonnet", Weight: 1}}}},
		Clients:     []domain.Client{{ID: "default", Name: "Local", Local: true, Routes: []domain.Route{{ID: "main", Agent: "codex", MatchModel: "*", SourceID: "pool:combined", Model: "astra"}}}, {ID: "team", Name: "Team", KeyHash: keyHash("synthetic-old-client"), Routes: []domain.Route{{ID: "sonnet", Agent: "codex", MatchModel: "sonnet", SourceID: "pool:combined", Model: "sonnet"}}}},
		SourceState: domain.SourceState{Paused: []string{"resource"}},
	}
	materializeLegacyIdentities(&c)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	return c
}

// Export must not transfer any authority. Adding a bundle twice must create
// disjoint graphs, not merely rename labels while retaining old references.
func TestConfigTransferExportAndAddIsolation(t *testing.T) {
	s := storeForTest(t)
	original := transferFixture(t)
	saveTest(t, s, original)
	old := cloneConfig(s.Snapshot())
	b := ExportConfig(&old)
	raw := string(jsonBytes(b))
	for _, forbidden := range []string{"key_hash", "synthetic-old-client", "legacy_direct", `"revision"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatal("export includes authority or runtime metadata", forbidden)
		}
	}
	if !reflect.DeepEqual(old, *s.Snapshot()) {
		t.Fatal("export mutated live snapshot")
	}
	auth := NewAuth(s, "")
	for run := 0; run < 2; run++ {
		before := cloneConfig(s.Snapshot())
		plan, err := planConfigImport(&before, b, "add", importNamespace())
		if err != nil {
			t.Fatal(err)
		}
		if err = applyConfigImport(context.Background(), s, auth, plan); err != nil {
			t.Fatal(err)
		}
		after := s.Snapshot()
		if !reflect.DeepEqual(before.Accounts, after.Accounts[:len(before.Accounts)]) || !reflect.DeepEqual(before.Clients, after.Clients[:len(before.Clients)]) {
			t.Fatal("add changed existing identities or Clients")
		}
		for _, kind := range []string{"accounts", "sources", "pools", "proxies", "clients"} {
			ids, names := map[string]bool{}, map[string]bool{}
			for _, change := range plan.Changes {
				if change.Kind != kind {
					continue
				}
				if ids[change.ToID] || names[change.ToName] || change.FromID == change.ToID {
					t.Fatal("identity collision", change)
				}
				ids[change.ToID], names[change.ToName] = true, true
			}
		}
		pool := after.Pools[len(after.Pools)-1]
		if !pool.MultiModel || !reflect.DeepEqual(pool.ModelNames(), []string{"astra", "sonnet"}) {
			t.Fatal("pool model catalog changed")
		}
		for _, member := range pool.Members {
			if member.SourceID == "resource" || member.SourceID == "copilot" {
				t.Fatal("imported pool crossed into original sources")
			}
			source, ok := after.Source(member.SourceID)
			if !ok || source.AccountID == "api" || source.AccountID == "copilot" {
				t.Fatal("imported source crossed into original account")
			}
		}
		for _, client := range after.Clients[len(before.Clients):] {
			if client.Local {
				t.Fatal("second local Client bypasses authentication")
			}
			key, err := auth.ClientKey(client)
			if err != nil || key == "" || key == "synthetic-old-client" || !keyEqual(client.KeyHash, key) {
				t.Fatal("missing independent Client credential")
			}
			if client.Routes[0].SourceID != "pool:"+pool.ID {
				t.Fatal("Client route references old pool")
			}
		}
		newSource := after.Sources[len(before.Sources)]
		if !after.Paused(newSource.ID) || after.Proxies.Bindings[newSource.ID] == "office" {
			t.Fatal("lost pause state or proxy isolation")
		}
		if after.Proxies.AccountProxyID != before.Proxies.AccountProxyID {
			t.Fatal("add replaced shared account proxy")
		}
	}
}

func TestConfigTransferReplaceAndRevisionGuard(t *testing.T) {
	s := storeForTest(t)
	saveTest(t, s, transferFixture(t))
	auth := NewAuth(s, "")
	b := ExportConfig(s.Snapshot())
	p, err := planConfigImport(s.Snapshot(), b, "replace", importNamespace())
	if err != nil {
		t.Fatal(err)
	}
	if p.Removed.Clients != 2 || p.Added.Clients != 2 {
		t.Fatal("incorrect destructive preview counts")
	}
	old := s.Snapshot()
	if err = applyConfigImport(context.Background(), s, auth, p); err != nil {
		t.Fatal(err)
	}
	if len(s.Snapshot().Clients) != 2 || len(s.Snapshot().Pools) != 1 {
		t.Fatal("replace appended instead of replacing")
	}
	local, ok := s.Snapshot().Client("default")
	if !ok || !local.Local || local.KeyHash != "" {
		t.Fatal("replace lost local Client")
	}
	if reflect.DeepEqual(old.Accounts, s.Snapshot().Accounts) {
		t.Fatal("replace reused credential-bearing identities")
	}
	before := cloneConfig(s.Snapshot())
	if applyConfigImport(context.Background(), s, auth, p) == nil || !reflect.DeepEqual(before, *s.Snapshot()) {
		t.Fatal("stale import committed")
	}
	// Credentials cannot partially publish a config if Auth storage fails.
	p, err = planConfigImport(s.Snapshot(), b, "replace", importNamespace())
	if err != nil {
		t.Fatal(err)
	}
	badRoot := filepath.Join(t.TempDir(), "file")
	if err = os.WriteFile(badRoot, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if applyConfigImport(context.Background(), s, NewAuth(s, badRoot), p) == nil || !reflect.DeepEqual(before, *s.Snapshot()) {
		t.Fatal("partial config published after credential storage failure")
	}
}

func TestConfigTransferAPIRequiresReviewedFileAndAdmin(t *testing.T) {
	s := localTestServer(t)
	bundle := ExportConfig(func() *domain.Config { c := transferFixture(t); return &c }())
	request := func(value any, authorized bool) (int, []byte) {
		w := localRequest(s, "POST", "/api/config/import", string(jsonBytes(value)), "127.0.0.1:42", "localhost:18766", authorized)
		return w.Code, w.Body.Bytes()
	}
	v := map[string]any{"bundle": bundle, "mode": "add", "revision": s.Store.Snapshot().Revision}
	if code, _ := request(v, false); code != 401 {
		t.Fatal("unauthenticated import accepted")
	}
	code, raw := request(v, true)
	if code != 200 {
		t.Fatalf("preview: %s", raw)
	}
	var p ConfigImportPlan
	if json.Unmarshal(raw, &p) != nil {
		t.Fatal("invalid preview")
	}
	if len(s.Store.Snapshot().Pools) != 0 {
		t.Fatal("preview committed")
	}
	v["apply"], v["namespace"], v["fingerprint"] = true, p.Namespace, p.Fingerprint
	v["mode"] = "replace"
	if code, _ = request(v, true); code != 409 {
		t.Fatal("reviewed mode bypassed")
	}
	v["mode"] = "add"
	if code, raw = request(v, true); code != 200 {
		t.Fatalf("apply: %s", raw)
	}
	if code, _ = request(v, true); code != 409 {
		t.Fatal("preview replay accepted")
	}
	for _, invalid := range []string{`{}`, `{"format":"pacaroute-config","version":99,"config":{}}`, `{"format":"pacaroute-config","version":1,"config":{},"api_key":"secret"}`} {
		if _, err := decodeConfigBundle([]byte(invalid)); err == nil {
			t.Fatal("invalid bundle accepted")
		}
	}
	bundle.Config.Clients[1].KeyHash = keyHash("old")
	if _, err := planConfigImport(s.Store.Snapshot(), bundle, "add", importNamespace()); err == nil {
		t.Fatal("credential hash was importable")
	}
}

func TestFreshAccountsEmptyAndLegacyIdentityMigration(t *testing.T) {
	s := localTestServer(t)
	if len(s.Store.Snapshot().Accounts) != 0 || len(s.Store.Snapshot().Sources) != 0 {
		t.Fatal("fresh installation pre-created an account")
	}
	w := localRequest(s, "GET", "/api/accounts/copilot/status", "", "127.0.0.1:42", "localhost:18766", true)
	if w.Code != 404 {
		t.Fatal("fresh management exposed a phantom account")
	}
	c := cloneConfig(s.Store.Snapshot())
	c.Clients[0].Routes = []domain.Route{{ID: "main", Agent: "codex", MatchModel: "*", SourceID: "copilot", Model: "gpt-6-astra"}}
	saveTest(t, s.Store, c)
	c = cloneConfig(s.Store.Snapshot())
	if len(c.Accounts) != 1 || c.Accounts[0].ID != "copilot" || len(c.Sources) != 1 {
		t.Fatal("legacy identity not made explicit")
	}
	c.Accounts = nil
	if s.Store.Save(c, c.Revision) == nil {
		t.Fatal("removed account with live references")
	}
	c = cloneConfig(s.Store.Snapshot())
	c.Clients[0].Routes = nil
	c.Sources = nil
	c.Accounts = nil
	saveTest(t, s.Store, c)
	if len(s.Store.Snapshot().Accounts) != 0 {
		t.Fatal("removed account resurrected")
	}
}
