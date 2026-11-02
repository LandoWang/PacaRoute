package pacaroute

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// Contract: migration imports once, preserves input files, and never resurrects
// an intentionally deleted local Client or accepts a partial invalid document.
func TestSQLiteMigrationDefaultDeletionAndInvalidSave(t *testing.T) {
	root := t.TempDir()
	original := []byte(`[{"id":"one","name":"One","kind":"github"}]`)
	os.WriteFile(filepath.Join(root, "accounts.json"), original, 0600)
	s, e := NewConfigStore(root)
	if e != nil {
		t.Fatal(e)
	}
	c := cloneConfig(s.Snapshot())
	local, ok := c.Client("default")
	if !ok || !local.Local || !local.LegacyDirect || !local.AgentEnabled("codex") || !local.AgentEnabled("claude-code") {
		t.Fatal("legacy default migration")
	}
	c.Clients = nil
	saveTest(t, s, c)
	revision := s.Snapshot().Revision
	invalid := cloneConfig(s.Snapshot())
	invalid.Sources = []Source{{ID: "bad", Name: "Bad", AccountID: "missing"}}
	if s.Save(invalid, revision) == nil || s.Snapshot().Revision != revision {
		t.Fatal("invalid write committed")
	}
	s.Close()
	os.WriteFile(filepath.Join(root, "clients.json"), []byte(`broken`), 0600)
	reopened, e := NewConfigStore(root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if len(reopened.Snapshot().Clients) != 0 || len(reopened.Snapshot().Accounts) != 1 {
		t.Fatal("migration repeated")
	}
	b, _ := os.ReadFile(filepath.Join(root, "accounts.json"))
	if string(b) != string(original) {
		t.Fatal("legacy file modified")
	}
	fresh := storeForTest(t)
	d, _ := fresh.Snapshot().Client("default")
	if d.LegacyDirect {
		t.Fatal("fresh Client has implicit routes")
	}
	if _, _, e = fresh.Snapshot().Resolve(d, "codex", "gpt-6-astra", ""); e == nil {
		t.Fatal("unconfigured default permitted")
	}
}
func TestSQLiteConcurrentWritersAndImmutableSnapshot(t *testing.T) {
	s := storeForTest(t)
	other, e := NewConfigStore(s.Root)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	old := s.Snapshot()
	one := cloneConfig(old)
	two := cloneConfig(old)
	one.Accounts = []Account{{ID: "one", Name: "One", Kind: "github"}}
	two.Accounts = []Account{{ID: "two", Name: "Two", Kind: "github"}}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, store := range []*ConfigStore{s, other} {
		wg.Add(1)
		go func(i int, store *ConfigStore) {
			defer wg.Done()
			c := one
			if i == 1 {
				c = two
			}
			results <- store.Save(c, old.Revision)
		}(i, store)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("concurrent writers did not conflict", success)
	}
	if len(old.Accounts) != 0 {
		t.Fatal("published snapshot mutated")
	}
	s.Reload()
	if len(s.Snapshot().Accounts) != 1 {
		t.Fatal("lost winning write")
	}
}
func TestClientAgentGateAndDefaultAuthorization(t *testing.T) {
	s := localTestServer(t)
	c := cloneConfig(s.Store.Snapshot())
	c.Clients[0].EnabledAgents = []string{"codex"}
	c.Clients[0].Routes = []Route{{ID: "main", Agent: "codex", MatchModel: "*", SourceID: "copilot", Model: "gpt-6-astra"}}
	c.Clients = append(c.Clients, Client{ID: "alice", Name: "Alice", KeyHash: keyHash("synthetic-key"), EnabledAgents: []string{}, Routes: []Route{{ID: "old", Agent: "codex", MatchModel: "*", SourceID: "copilot", Model: "gpt-6-astra"}}})
	saveTest(t, s.Store, c)
	for _, path := range []string{"/cc/messages", "/cc/models", "/sources/copilot/cc/models", "/pools/nope/cc/messages"} {
		method := "POST"
		if strings.HasSuffix(path, "models") {
			method = "GET"
		}
		w := localRequest(s, method, path, `{"model":"gpt-6-astra"}`, "127.0.0.1:42", "127.0.0.1:18766", false)
		if w.Code != 403 {
			t.Fatalf("disabled Agent %s: %d", path, w.Code)
		}
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host"} {
		r := httptest.NewRequest("GET", "http://localhost/codex/models", nil)
		r.RemoteAddr = "127.0.0.1:42"
		r.Header.Set(header, "evil")
		if _, ok := s.authenticate(r, s.Store.Snapshot()); ok {
			t.Fatal("forwarded local bypass", header)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost/codex/models", nil)
	r.RemoteAddr = "192.0.2.1:42"
	r.Header.Set("X-Client-ID", "default")
	if _, ok := s.authenticate(r, s.Store.Snapshot()); ok {
		t.Fatal("network local bypass")
	}
	r.RemoteAddr = "127.0.0.1:42"
	r.Header.Set("Authorization", "Bearer synthetic-key")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("empty Agent list ignored")
	}
	r.Header.Del("Authorization")
	client, ok := s.authenticate(r, s.Store.Snapshot())
	if !ok || client.ID != "default" {
		t.Fatal("default not used")
	}
	s.PublicURL = "https://gateway.example"
	if _, ok := s.authenticate(r, s.Store.Snapshot()); ok {
		t.Fatal("public service local bypass")
	}
	s.PublicURL = ""
	c = cloneConfig(s.Store.Snapshot())
	c.Clients = c.Clients[1:]
	saveTest(t, s.Store, c)
	if _, ok := s.authenticate(r, s.Store.Snapshot()); ok {
		t.Fatal("deleted default bypass")
	}
	if _, _, e := s.Store.Snapshot().Resolve(nil, "codex", "gpt-6-astra", ""); e == nil {
		t.Fatal("nil Client bypass")
	}
}
func TestSharedAzureIdentityAndIsolatedEnvironment(t *testing.T) {
	s := storeForTest(t)
	a := NewAuth(s, "")
	c := cloneConfig(s.Snapshot())
	c.Accounts = []Account{{ID: "shared", Name: "Shared", Kind: "azure", LoginMode: "azure-cli"}, {ID: "isolated", Name: "Isolated", Kind: "azure", LoginMode: "device"}}
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join(t.TempDir(), "cli"))
	if a.azureDirectory(&c, "shared") != os.Getenv("AZURE_CONFIG_DIR") {
		t.Fatal("CLI identity not reused")
	}
	if a.azureDirectory(&c, "isolated") == os.Getenv("AZURE_CONFIG_DIR") {
		t.Fatal("isolated account shares cache")
	}
	if !sharedAzure(&c, "shared") || sharedAzure(&c, "isolated") {
		t.Fatal("shared identity policy")
	}
	env := a.azureEnv(&c, "shared", Proxy{Mode: "direct"})
	count := 0
	for _, v := range env {
		if strings.HasPrefix(v, "AZURE_CONFIG_DIR=") {
			count++
			if v != "AZURE_CONFIG_DIR="+os.Getenv("AZURE_CONFIG_DIR") {
				t.Fatal(v)
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate cache directory")
	}
}
func TestConnectPreservesConfigurationAndKeepsKeyOffOutput(t *testing.T) {
	for _, agent := range []string{"codex", "claude-code"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("CODEX_HOME", root)
			t.Setenv("CLAUDE_CONFIG_DIR", root)
			path, _ := clientPath(agent)
			old := []byte("model=\"old\"\n[features]\nweb_search=false\n")
			if agent == "claude-code" {
				old = []byte(`{"permissions":{"allow":["Read"]},"env":{"CUSTOM":"keep","ANTHROPIC_API_KEY":"old"}}`)
			}
			os.WriteFile(path, old, 0600)
			args, _ := parseArgs([]string{"connect", "--agent", agent, "--url", "https://gateway.example/router", "--model", "coding"})
			key := "synthetic-client-key-for-connect"
			preview, e := connectClient(args, strings.NewReader(key))
			if e != nil {
				t.Fatal(e)
			}
			b, _ := os.ReadFile(path)
			if string(b) != string(old) {
				t.Fatal("preview writes")
			}
			out, _ := json.Marshal(preview)
			if strings.Contains(string(out), key) {
				t.Fatal("preview leaked key")
			}
			args.flags["apply"] = "true"
			result, e := connectClient(args, strings.NewReader(key))
			if e != nil {
				t.Fatal(e)
			}
			backup, _ := os.ReadFile(result["backup_path"].(string))
			if string(backup) != string(old) {
				t.Fatal("backup mismatch")
			}
			b, _ = os.ReadFile(path)
			if agent == "codex" {
				var m map[string]any
				if toml.Unmarshal(b, &m) != nil || m["model"] != "coding" || m["features"].(map[string]any)["web_search"] != false {
					t.Fatal("unrelated TOML altered")
				}
				provider := m["model_providers"].(map[string]any)["pacaroute"].(map[string]any)
				if provider["http_headers"].(map[string]any)["Authorization"] != "Bearer "+key {
					t.Fatal("Client authorization missing")
				}
			}
			if agent == "claude-code" {
				var m map[string]any
				json.Unmarshal(b, &m)
				env := m["env"].(map[string]any)
				if env["CUSTOM"] != "keep" || env["ANTHROPIC_AUTH_TOKEN"] != key || env["ANTHROPIC_API_KEY"] != nil || m["permissions"] == nil {
					t.Fatal("Claude config not preserved")
				}
			}
		})
	}
}

func TestConnectCreatesMissingClaudeConfiguration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new-home")
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	args, _ := parseArgs([]string{"connect", "--agent", "claude-code", "--url", "http://127.0.0.1:18766", "--model", "coding", "--local", "--apply"})
	result, e := connectClient(args, strings.NewReader(""))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(result["backup_path"].(string), ".missing") {
		t.Fatal("missing state not backed up")
	}
	b, e := os.ReadFile(filepath.Join(root, "settings.json"))
	if e != nil || !strings.Contains(string(b), "pacaroute-local") {
		t.Fatal("new Claude configuration not written")
	}
}

func TestDefaultAffinityCannotCollideWithNamedNetworkClient(t *testing.T) {
	local := &Client{ID: "default", Local: true}
	remote := &Client{ID: "local"}
	if affinityClient(local) == affinityClient(remote) {
		t.Fatal("local and network Clients share sticky scope")
	}
}
