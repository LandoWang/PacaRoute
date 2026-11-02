package pacaroute

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func localTestServer(t *testing.T) *Server {
	t.Helper()
	store := storeForTest(t)
	server, e := NewServer(store, NewAuth(store, ""))
	if e != nil {
		t.Fatal(e)
	}
	server.LocalManagement = true
	server.AllowLocal = true
	server.Port = 18766
	return server
}
func localRequest(s *Server, method, path, body, peer, host string, authorized bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.RemoteAddr = peer
	r.Header.Set("Origin", "http://"+host)
	if authorized {
		r.Header.Set("Authorization", "Bearer "+s.AdminKey)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

// Contract: remote administration must never expose local Agent files, even to an admin.
func TestLocalManagementRequiresOptInAndLoopbackAuthority(t *testing.T) {
	s := localTestServer(t)
	for _, path := range []string{"/api/local/agents", "/api/local/sessions?agent=codex"} {
		for _, test := range []struct {
			peer, host string
			enabled    bool
			want       int
		}{{"127.0.0.1:42", "127.0.0.1:18766", false, 403}, {"192.0.2.1:42", "127.0.0.1:18766", true, 403}, {"127.0.0.1:42", "attacker.example", true, 403}} {
			s.LocalManagement = test.enabled
			w := localRequest(s, "GET", path, "", test.peer, test.host, true)
			if w.Code != test.want {
				t.Fatalf("%s: %d", path, w.Code)
			}
		}
	}
	s.LocalManagement = true
	s.PublicURL = "https://gateway.example"
	if w := localRequest(s, "GET", "/api/local/agents", "", "127.0.0.1:42", "127.0.0.1:18766", true); w.Code != 403 {
		t.Fatal("public deployment exposed local files")
	}
}

// Contract: a desktop browser handoff is one-use, expires, and cannot be redeemed cross-origin.
func TestBrowserTicketIsSingleUseAndExpires(t *testing.T) {
	s := localTestServer(t)
	ticket := s.tickets.issue()
	body := fmt.Sprintf(`{"ticket":%q}`, ticket)
	req := httptest.NewRequest("POST", "http://127.0.0.1:18766/api/login/ticket", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:123"
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-origin ticket accepted")
	}
	w = localRequest(s, "POST", "/api/login/ticket", body, "127.0.0.1:123", "127.0.0.1:18766", false)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe browser cookie")
	}
	if localRequest(s, "POST", "/api/login/ticket", body, "127.0.0.1:123", "127.0.0.1:18766", false).Code != 401 {
		t.Fatal("ticket replay succeeded")
	}
	key := s.tickets.issue()
	s.tickets.values[keyHash(key)] = time.Now().Add(-time.Second)
	if s.tickets.consume(key) {
		t.Fatal("expired ticket accepted")
	}
	if localRequest(s, "POST", "/api/local/browser-ticket", "{}", "127.0.0.1:123", "127.0.0.1:18766", false).Code != 401 {
		t.Fatal("anonymous browser ticket creation")
	}
}

// Contract: all release assets are embedded, typed, and served without weakening the origin policy.
func TestReactAssetsAreEmbeddedAndTyped(t *testing.T) {
	s := localTestServer(t)
	html := localRequest(s, "GET", "/", "", "127.0.0.1:42", "127.0.0.1:18766", false)
	if html.Code != 200 || !strings.Contains(html.Body.String(), "/assets/") {
		t.Fatal("React entry missing")
	}
	entries, e := webFiles.ReadDir("web/assets")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		w := localRequest(s, "GET", "/assets/"+entry.Name(), "", "127.0.0.1:42", "127.0.0.1:18766", false)
		if w.Code != 200 || w.Header().Get("Content-Type") == "" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("asset contract", entry.Name(), w.Code)
		}
	}
}

// Contract: browser configuration requires the preview fingerprint and preserves unrelated fields.
func TestLocalAgentPreviewApplyAndBackup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	path := filepath.Join(root, "config.toml")
	original := []byte("model = \"old\"\n[features]\nweb_search = false\n")
	os.WriteFile(path, original, 0600)
	s := localTestServer(t)
	request := map[string]any{"client": "codex", "source": "copilot", "model": "gpt-6-astra"}
	data, _ := json.Marshal(request)
	w := localRequest(s, "POST", "/api/local/configure", string(data), "127.0.0.1:42", "127.0.0.1:18766", true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var plan map[string]any
	json.Unmarshal(w.Body.Bytes(), &plan)
	b, _ := os.ReadFile(path)
	if string(b) != string(original) {
		t.Fatal("preview modified config")
	}
	request["apply"] = true
	request["fingerprint"] = "wrong"
	data, _ = json.Marshal(request)
	if localRequest(s, "POST", "/api/local/configure", string(data), "127.0.0.1:42", "127.0.0.1:18766", true).Code != 409 {
		t.Fatal("stale preview applied")
	}
	request["fingerprint"] = plan["fingerprint"]
	data, _ = json.Marshal(request)
	w = localRequest(s, "POST", "/api/local/configure", string(data), "127.0.0.1:42", "127.0.0.1:18766", true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &plan)
	backup, _ := os.ReadFile(plan["backup_path"].(string))
	if string(backup) != string(original) {
		t.Fatal("original config was not backed up exactly")
	}
}

// Contract: a failed port change keeps the current service, while a successful one persists and serves a fresh login ticket.
func TestDesktopPortChangeKeepsServiceOnConflict(t *testing.T) {
	store := storeForTest(t)
	d := &desktopControl{store: store, auth: NewAuth(store, ""), port: 0}
	if e := d.start(); e != nil {
		t.Fatal(e)
	}
	defer d.stop()
	old := d.server
	occupied, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	if _, e = d.changePort(occupied.Addr().(*net.TCPAddr).Port); e == nil {
		t.Fatal("occupied port accepted")
	}
	if d.server != old {
		t.Fatal("failed change stopped current service")
	}
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	address, e := d.changePort(port)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(address, "ticket=") {
		t.Fatal("missing new-port login handoff")
	}
	response, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	saved, _ := os.ReadFile(filepath.Join(store.Root, "desktop-settings.json"))
	if !strings.Contains(string(saved), fmt.Sprint(port)) {
		t.Fatal("port was not persisted")
	}
}

// Contract: account providers and resource products are separate; credentials cannot cross providers.
func TestProviderAccountsAndResourceCompatibility(t *testing.T) {
	c := Config{Accounts: []Account{{ID: "gh", Name: "GitHub", Kind: "github"}, {ID: "oa", Name: "OpenAI", Kind: "openai"}, {ID: "an", Name: "Anthropic", Kind: "anthropic"}, {ID: "az", Name: "Azure", Kind: "azure"}}, Sources: []Source{{ID: "ghcp", Name: "Copilot", Kind: "copilot", AccountID: "gh"}, {ID: "oa-api", Name: "OpenAI API", Kind: "openai", AccountID: "oa", AuthMode: "api-key", Endpoint: "https://api.openai.com/v1", Deployments: []string{"test-model"}}, {ID: "an-api", Name: "Anthropic API", Kind: "anthropic", AccountID: "an", AuthMode: "api-key", Endpoint: "https://api.anthropic.com", Deployments: []string{"test-model"}}, {ID: "az-resource", Name: "Foundry", Kind: "azure-openai", AccountID: "az", AuthMode: "azure-cli", Endpoint: "https://example.openai.azure.com", Deployments: []string{"deployment"}}}}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.Sources[1].AccountID = "an"
	if c.Validate() == nil {
		t.Fatal("OpenAI resource accepted Anthropic account")
	}
	c.Sources[1].AccountID = "oa"
	c.Accounts[1].Kind = "api-key"
	if e := c.Validate(); e != nil {
		t.Fatal("legacy API account rejected", e)
	}
}

// Contract: resources inherit an account key; explicit resource credentials override it, including after rotation in another process.
func TestAccountKeyInheritanceAndRotation(t *testing.T) {
	store := storeForTest(t)
	c := cloneConfig(store.Snapshot())
	c.Accounts = []Account{{ID: "openai-work", Name: "Work", Kind: "openai"}}
	source := Source{ID: "work-api", Name: "API", Kind: "openai", AccountID: "openai-work", AuthMode: "api-key", Endpoint: "https://api.openai.com/v1", Deployments: []string{"test-model"}}
	c.Sources = []Source{source}
	saveTest(t, store, c)
	a := NewAuth(store, "")
	other := NewAuth(store, "")
	if e := a.SaveAccountKey(context.Background(), "openai-work", "fixture-one"); e != nil {
		t.Fatal(e)
	}
	lease, e := a.Lease(context.Background(), store.Snapshot(), source, false)
	if e != nil || lease.Value != "Bearer fixture-one" {
		t.Fatal("account key not inherited", e)
	}
	if e = other.SaveAccountKey(context.Background(), "openai-work", "fixture-two"); e != nil {
		t.Fatal(e)
	}
	lease, e = a.Lease(context.Background(), store.Snapshot(), source, false)
	if e != nil || lease.Value != "Bearer fixture-two" {
		t.Fatal("account key rotation not observed", e)
	}
	if e = other.SaveKey(context.Background(), source.ID, "fixture-override"); e != nil {
		t.Fatal(e)
	}
	lease, e = a.Lease(context.Background(), store.Snapshot(), source, false)
	if e != nil || lease.Value != "Bearer fixture-override" {
		t.Fatal("source override lost", e)
	}
}

// Contract: model discovery uses the selected account's authenticated API and supports Anthropic pagination.
func TestProviderModelDiscoveryUsesAccountAndPagination(t *testing.T) {
	calls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("x-api-key") != "fixture-anthropic" || r.Header.Get("anthropic-version") == "" {
			t.Error("discovery authentication missing")
		}
		if r.URL.Path != "/v1/models" {
			t.Error("wrong discovery endpoint", r.URL.Path)
		}
		if r.URL.Query().Get("after_id") == "first" {
			writeJSON(w, 200, map[string]any{"data": []any{map[string]string{"id": "second", "display_name": "Second"}}})
			return
		}
		writeJSON(w, 200, map[string]any{"data": []any{map[string]string{"id": "first", "display_name": "First"}}, "has_more": true, "last_id": "first"})
	}))
	defer upstream.Close()
	store := storeForTest(t)
	c := cloneConfig(store.Snapshot())
	c.Accounts = []Account{{ID: "anthropic-work", Name: "Anthropic", Kind: "anthropic"}}
	source := Source{ID: "anthropic-api", Name: "API", Kind: "anthropic", AccountID: "anthropic-work", AuthMode: "api-key", Endpoint: upstream.URL, Deployments: []string{"first"}}
	c.Sources = []Source{source}
	saveTest(t, store, c)
	a := NewAuth(store, "")
	a.http.values = map[string]*http.Client{"system:": upstream.Client()}
	if e := a.SaveAccountKey(context.Background(), source.AccountID, "fixture-anthropic"); e != nil {
		t.Fatal(e)
	}
	result, e := a.DiscoverModels(context.Background(), store.Snapshot(), source)
	if e != nil || len(result.Models) != 2 || calls != 2 {
		t.Fatal("model discovery pagination failed", e, calls)
	}
}

// Contract: discovering an edited resource honors its credential and selected proxy without changing live routes.
func TestDraftModelDiscoveryPreservesResourceCredentialAndConfig(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-resource" || r.URL.Path != "/v1/models" {
			t.Error("resource discovery lost authentication or endpoint")
		}
		writeJSON(w, 200, map[string]any{"data": []any{map[string]string{"id": "discovered-model"}}})
	}))
	defer upstream.Close()
	s := localTestServer(t)
	c := cloneConfig(s.Store.Snapshot())
	c.Accounts = []Account{{ID: "openai-work", Name: "OpenAI", Kind: "openai"}}
	source := Source{ID: "work-resource", Name: "Resource", Kind: "openai", AccountID: "openai-work", AuthMode: "api-key", Endpoint: upstream.URL + "/v1", Deployments: []string{"configured-model"}}
	c.Sources = []Source{source}
	saveTest(t, s.Store, c)
	if e := s.Auth.SaveKey(context.Background(), source.ID, "fixture-resource"); e != nil {
		t.Fatal(e)
	}
	s.Auth.http.values = map[string]*http.Client{"direct:": upstream.Client()}
	before := s.Store.Snapshot().Revision
	body, _ := json.Marshal(struct {
		Source
		ProxyID string `json:"proxy_id"`
	}{source, "direct"})
	w := localRequest(s, "POST", "/api/model-discovery", string(body), "127.0.0.1:42", "127.0.0.1:18766", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "discovered-model") {
		t.Fatal(w.Code, w.Body.String())
	}
	if s.Store.Snapshot().Revision != before || s.Store.Snapshot().Sources[0].Deployments[0] != "configured-model" {
		t.Fatal("discovery modified the live configuration")
	}
}
