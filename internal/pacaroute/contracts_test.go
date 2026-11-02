package pacaroute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func storeForTest(t *testing.T) *ConfigStore {
	t.Helper()
	s, e := NewConfigStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func saveTest(t *testing.T, s *ConfigStore, c Config) {
	t.Helper()
	if e := s.Save(c, c.Revision); e != nil {
		t.Fatal(e)
	}
}
func fixtureConfig() Config {
	return Config{Sources: []Source{{ID: "secondary", Name: "Secondary", Kind: "copilot", AccountID: "copilot"}}, Pools: []Pool{{ID: "balanced", Name: "Balanced", Agent: "codex", Model: "coding", Failover: true, Members: []Member{{ID: "primary", SourceID: "copilot", Model: "gpt-6-astra", Weight: 70}, {ID: "secondary", SourceID: "secondary", Model: "gpt-6-astra", Weight: 30}}}}, Proxies: Proxies{Bindings: map[string]string{}, AccountProxyID: "system"}}
}

// Contract: live readers accept atomic edits, keep the last valid snapshot on malformed data,
// and reject stale writers so a Client route cannot silently disappear.
func TestConfigurationHotReloadAndStaleWriter(t *testing.T) {
	s := storeForTest(t)
	c := fixtureConfig()
	saveTest(t, s, c)
	other, e := NewConfigStore(s.Root)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	stale := cloneConfig(other.Snapshot())
	c = cloneConfig(s.Snapshot())
	c.SourceState.Paused = []string{"copilot"}
	saveTest(t, s, c)
	other.Reload()
	if !other.Snapshot().Paused("copilot") {
		t.Fatal("pause not observed")
	}
	if other.Save(stale, stale.Revision) == nil {
		t.Fatal("stale writer accepted")
	}
	if _, e = s.db.Exec(`UPDATE configuration SET revision=revision+1,document='broken'`); e != nil {
		t.Fatal(e)
	}
	other.Reload()
	if !other.Snapshot().Paused("copilot") || other.Error() == "" {
		t.Fatal("invalid edit replaced valid routing")
	}
}
func TestConfigurationRejectsReferenceAndNestedPool(t *testing.T) {
	c := fixtureConfig()
	c.Pools[0].Members[0].SourceID = "pool:balanced"
	c.Pools[0].Members[0].Model = "coding"
	if c.Validate() == nil {
		t.Fatal("recursive pool accepted")
	}
	c = fixtureConfig()
	c.Sources = nil
	if c.Validate() == nil {
		t.Fatal("referenced source deleted")
	}
}
func TestConfigurationCrashTransactionRollsForward(t *testing.T) {
	root := t.TempDir()
	c := fixtureConfig()
	if e := atomicWrite(filepath.Join(root, "config.transaction.json"), jsonBytes(c), 0600); e != nil {
		t.Fatal(e)
	}
	reopened, e := NewConfigStore(root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if len(reopened.Snapshot().Pools) != 1 {
		t.Fatal("lost committed transaction")
	}
	if _, e = os.Stat(filepath.Join(root, "config.transaction.json")); e != nil {
		t.Fatal("legacy journal must remain available for rollback")
	}
}

// Contract: 70/30 means approximate session allocation, while an existing conversation keeps
// its member across weight edits and process restarts. Client identity is part of stickiness.
func TestWeightedSessionsStickyRestartAndClientIsolation(t *testing.T) {
	c := fixtureConfig()
	r, e := NewRouter(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		targets, e := r.Targets(&c, "alice", "codex", "pool:balanced", "coding", fmt.Sprint(i), false)
		if e != nil {
			t.Fatal(e)
		}
		counts[targets[0].Member]++
	}
	if counts["primary"] < 6750 || counts["primary"] > 7250 {
		t.Fatalf("allocation=%v", counts)
	}
	first, _ := r.Targets(&c, "alice", "codex", "pool:balanced", "coding", "shared-session", false)
	if e = r.Bind(first[0]); e != nil {
		t.Fatal(e)
	}
	for i := range c.Pools[0].Members {
		if c.Pools[0].Members[i].ID == first[0].Member {
			c.Pools[0].Members[i].Weight = 0
		} else {
			c.Pools[0].Members[i].Weight = 100
		}
	}
	r2, e := NewRouter(filepath.Dir(r.path))
	if e != nil {
		t.Fatal(e)
	}
	same, _ := r2.Targets(&c, "alice", "codex", "pool:balanced", "coding", "shared-session", false)
	other, _ := r2.Targets(&c, "bob", "codex", "pool:balanced", "coding", "shared-session", false)
	if same[0].Member != first[0].Member || !same[0].Sticky {
		t.Fatal("existing session lost its binding")
	}
	if other[0].Member == same[0].Member || other[0].Affinity == same[0].Affinity {
		t.Fatal("another Client inherited Alice's sticky binding")
	}
	c.SourceState.Paused = []string{same[0].Source.ID}
	if _, e = r2.Targets(&c, "alice", "codex", "pool:balanced", "coding", "shared-session", true); e == nil {
		t.Fatal("source-bound continuation migrated")
	}
	fallback, e := r2.Targets(&c, "alice", "codex", "pool:balanced", "coding", "shared-session", false)
	if e != nil || fallback[0].Member == first[0].Member {
		t.Fatal("full-history failover unavailable")
	}
}
func TestSessionIdentityClaudeAndCodex(t *testing.T) {
	h := http.Header{}
	h.Set("thread-id", "conversation")
	if sessionID("codex", h, nil) != "conversation" {
		t.Fatal("Codex thread missing")
	}
	for _, metadata := range []string{`{"user_id":"user_a_session_abc-123"}`, `{"user_id":"{\"session_id\":\"abc-123\"}"}`, `{"user_id":{"session_id":"abc-123"}}`} {
		if sessionID("claude-code", http.Header{}, object{"metadata": []byte(metadata)}) != "abc-123" {
			t.Fatal("Claude identity lost")
		}
	}
}

// Contract: final SSE usage snapshots replace earlier snapshots, never multiply them;
// forwarding preserves unknown frames and UTF-8 content exactly.
func TestNativeSSEUsageAndWireFidelity(t *testing.T) {
	input := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\nevent: future.event\ndata: {\"type\":\"future.event\",\"text\":\"羊驼\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":23,\"input_tokens_details\":{\"cached_tokens\":70}}}}\n\n"
	var out bytes.Buffer
	u := Usage{}
	if e := relaySSE(&out, strings.NewReader(input), &u, "responses"); e != nil {
		t.Fatal(e)
	}
	if out.String() != input {
		t.Fatal("native SSE altered")
	}
	if u.Input == nil || *u.Input != 100 || u.Output == nil || *u.Output != 23 || u.CacheRead == nil || *u.CacheRead != 70 {
		t.Fatalf("usage=%+v", u)
	}
	if relaySSE(io.Discard, strings.NewReader("data: {\"type\":\"response.created\"}\n\n"), &Usage{}, "responses") == nil {
		t.Fatal("truncated stream treated as success")
	}
}
func TestNativeToolEchoPreservesCustomNamespaceAndOpaqueFields(t *testing.T) {
	body := obj([]byte(`{"model":"alias","stream":true,"unknown":{"keep":"exact"},"tools":[{"type":"custom","name":"exec","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}],"input":[{"type":"custom_tool_call","call_id":"call-a","name":"exec","namespace":"functions","input":"await do_work()"},{"type":"custom_tool_call_output","call_id":"call-a","output":"done"}]}`))
	b, tr, e := prepare(body, Target{Source: Source{Kind: "copilot"}, Model: "gpt-6-astra"}, "codex")
	if e != nil || tr != nil {
		t.Fatal("native path translated")
	}
	got := obj(b)
	for _, key := range []string{"unknown", "tools", "input"} {
		if !bytes.Equal(jsonBytes(obj(jsonBytes(map[string]json.RawMessage{key: body[key]}))), jsonBytes(obj(jsonBytes(map[string]json.RawMessage{key: got[key]})))) {
			t.Fatalf("%s lost", key)
		}
	}
}

func gatewayTest(t *testing.T, handler http.HandlerFunc) (*Server, *httptest.Server, *ConfigStore) {
	t.Helper()
	up := httptest.NewTLSServer(handler)
	t.Cleanup(up.Close)
	store := storeForTest(t)
	c := Config{Accounts: []Account{{ID: "api", Name: "API", Kind: "api-key"}}, Sources: []Source{{ID: "test", Name: "Test", Kind: "openai", AuthMode: "api-key", AccountID: "api", Endpoint: up.URL, Deployments: []string{"deployed"}}}, Clients: []Client{{ID: "alice", Name: "Alice", KeyHash: keyHash("alice-key"), Routes: []Route{{ID: "main", Agent: "codex", MatchModel: "coding", SourceID: "test", Model: "deployed"}}}}}
	saveTest(t, store, c)
	auth := NewAuth(store, "")
	auth.http.values = map[string]*http.Client{"system:": up.Client()}
	if e := auth.SetKeys(map[string]string{"test": "upstream-secret"}); e != nil {
		t.Fatal(e)
	}
	srv, e := NewServer(store, auth)
	if e != nil {
		t.Fatal(e)
	}
	front := httptest.NewServer(srv.Handler())
	t.Cleanup(front.Close)
	return srv, front, store
}
func requestTest(t *testing.T, front *httptest.Server, path, key, body string) *http.Response {
	t.Helper()
	r, _ := http.NewRequest("POST", front.URL+path, strings.NewReader(body))
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	res, e := front.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	return res
}
func TestClientAuthorizationRouteAndHotPause(t *testing.T) {
	var count atomic.Int32
	s, front, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		var body object
		_ = json.NewDecoder(r.Body).Decode(&body)
		if rawString(body["model"]) != "deployed" || r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Error("client key/model not translated to source")
		}
		writeJSON(w, 200, map[string]any{"usage": map[string]int{"input_tokens": 50, "output_tokens": 9}, "output": []any{}})
	})
	for _, key := range []string{"", "wrong"} {
		res := requestTest(t, front, "/codex/responses", key, `{"model":"coding"}`)
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatal("unauthorized request accepted")
		}
	}
	res := requestTest(t, front, "/sources/copilot/codex/responses", "alice-key", `{"model":"coding"}`)
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("Client escaped route via URL")
	}
	res = requestTest(t, front, "/codex/responses", "alice-key", `{"model":"coding"}`)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	if count.Load() != 1 {
		t.Fatal("unexpected upstream requests")
	}
	c := cloneConfig(store.Snapshot())
	c.SourceState.Paused = []string{"test"}
	saveTest(t, store, c)
	res = requestTest(t, front, "/codex/responses", "alice-key", `{"model":"coding"}`)
	res.Body.Close()
	if res.StatusCode != 503 || count.Load() != 1 {
		t.Fatal("pause did not block next request")
	}
	summary := s.Activity.Summary(time.Now().UTC().Truncate(time.Hour), time.Now().Add(time.Hour), "", "")
	total := summary["total"].(Totals)
	if total.Requests != 1 || total.Input != 50 || total.Output != 9 {
		t.Fatalf("summary=%+v", total)
	}
}
func TestManagementRequiresKeyAndBlocksCSRF(t *testing.T) {
	s, front, _ := gatewayTest(t, func(http.ResponseWriter, *http.Request) {})
	r, _ := http.NewRequest("GET", front.URL+"/api/config", nil)
	res, err := front.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("management exposed")
	}
	r, _ = http.NewRequest("POST", front.URL+"/api/login", strings.NewReader(string(jsonBytes(map[string]string{"key": s.AdminKey}))))
	r.Header.Set("Origin", "https://evil.example")
	res, err = front.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross-site login accepted")
	}
	r, _ = http.NewRequest("GET", front.URL+"/api/config", nil)
	r.Header.Set("Authorization", "Bearer "+s.AdminKey)
	res, err = front.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("admin cannot read config")
	}
}
func TestResponseStartedNeverFailsOver(t *testing.T) {
	var calls atomic.Int32
	s, front, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
		w.(http.Flusher).Flush()
	})
	c := cloneConfig(store.Snapshot())
	c.Pools = []Pool{{ID: "pool", Name: "Pool", Agent: "codex", Model: "coding", Failover: true, Members: []Member{{ID: "first", SourceID: "test", Model: "deployed", Weight: 1}, {ID: "second", SourceID: "test", Model: "deployed", Weight: 1}}}}
	c.Clients[0].Routes[0].SourceID = "pool:pool"
	c.Clients[0].Routes[0].Model = "coding"
	saveTest(t, store, c)
	res := requestTest(t, front, "/codex/responses", "alice-key", `{"model":"coding","stream":true}`)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if calls.Load() != 1 || !bytes.Contains(b, []byte(`stream interrupted`)) {
		t.Fatalf("spliced or hid interrupted response: %d %s", calls.Load(), b)
	}
	if recent := s.Activity.Recent(true); len(recent) != 1 || recent[0].Status != 502 {
		t.Fatal("stream failure not counted")
	}
}
func TestConcurrentConfigAndRequests(t *testing.T) {
	s := storeForTest(t)
	saveTest(t, s, fixtureConfig())
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				c := s.Snapshot()
				if len(c.Pools) != 1 {
					t.Error("partial snapshot")
				}
				_ = s.Error()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		c := cloneConfig(s.Snapshot())
		c.Pools[0].Members[0].Weight = 1 + i
		saveTest(t, s, c)
	}
	wg.Wait()
}
func TestVaultAuthenticatedRoundTrip(t *testing.T) {
	v := vault{}
	original := []byte(`{"access_token":"synthetic-only"}`)
	blob, e := v.seal(original)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(blob, []byte("synthetic-only")) {
		t.Fatal("secret persisted plaintext")
	}
	got, e := v.open(blob)
	if e != nil || !bytes.Equal(got, original) {
		t.Fatal("vault roundtrip failed")
	}
	for _, index := range []int{0, 1, 20, len(blob) - 1} {
		copy := bytes.Clone(blob)
		copy[index] ^= 1
		if _, e = v.open(copy); e == nil {
			t.Fatal("tampering accepted")
		}
	}
}
func TestConfigPreviewBacksUpAndRejectsStaleApply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	path := filepath.Join(home, "config.toml")
	original := []byte("# keep comment\nmodel = \"old\"\nmodel_provider = \"azure\"\n[model_providers.azure]\nbase_url = \"https://example.openai.azure.com/openai/v1\"\n[features]\nplugins = false\n")
	os.WriteFile(path, original, 0600)
	p, e := planClient("codex", "copilot", "gpt-6-astra", 45678)
	if e != nil {
		t.Fatal(e)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, original) {
		t.Fatal("preview changed config")
	}
	os.WriteFile(path, append(original, []byte("# concurrent edit\n")...), 0600)
	if _, e = p.apply(p.fingerprint); e == nil {
		t.Fatal("stale preview overwrote concurrent edit")
	}
	os.WriteFile(path, original, 0600)
	backup, e := p.apply(p.fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(backup); !bytes.Equal(b, original) {
		t.Fatal("backup incomplete")
	}
	next, _ := os.ReadFile(path)
	if !bytes.Contains(next, []byte("# keep comment")) || !bytes.Contains(next, []byte("example.openai.azure.com")) {
		t.Fatal("unrelated config lost")
	}
}
func TestCrossProtocolToolPairs(t *testing.T) {
	body := obj([]byte(`{"model":"gpt-6-astra","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call-a","name":"Bash","input":{"command":"pwd"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-a","content":"/tmp"}]}],"tools":[{"name":"Bash","input_schema":{"type":"object","properties":{"command":{"type":"string"}}}}]}`))
	tr := &translation{to: "responses", model: "alias"}
	wire, e := messagesToResponses(body, tr)
	if e != nil {
		t.Fatal(e)
	}
	input := array(wire["input"])
	if len(input) != 2 || rawString(input[0]["call_id"]) != rawString(input[1]["call_id"]) || rawString(input[1]["output"]) != "/tmp" {
		t.Fatal("tool pairing lost")
	}
	upstream := `{"id":"resp-1","status":"completed","output":[{"type":"function_call","call_id":"call-b","name":"Bash","arguments":"{\"command\":\"ls\"}"}],"usage":{"input_tokens":10,"output_tokens":5}}`
	reply, e := tr.responseJSON([]byte(upstream))
	if e != nil {
		t.Fatal(e)
	}
	m := obj(reply)
	blocks := array(m["content"])
	if rawString(m["stop_reason"]) != "tool_use" || len(blocks) != 1 || rawString(blocks[0]["id"]) != "call-b" || rawString(obj(blocks[0]["input"])["command"]) != "ls" {
		t.Fatal("Claude cannot execute returned tool")
	}
}

var _ = context.Background

// Copilot encrypts item IDs separately on successive frames. output_index/content_index,
// not the opaque changing item_id, define a single Claude content block.
func TestCrossProtocolChangingOpaqueIDsKeepOneTextBlock(t *testing.T) {
	stream := `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"opaque-start","type":"message"}}

data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"item_id":"opaque-one","delta":"PACAROUTE_"}

data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"item_id":"opaque-two","delta":"OK"}

data: {"type":"response.output_item.done","output_index":0,"item":{"id":"opaque-done","type":"message"}}

data: {"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":4}}}

`
	var out bytes.Buffer
	tr := translation{from: "messages", to: "responses", model: "alias"}
	if e := tr.streamResponses(&out, strings.NewReader(stream), &Usage{}); e != nil {
		t.Fatal(e)
	}
	rd := newSSE(&out)
	starts, stops := 0, 0
	text := ""
	for {
		_, data, e := rd.next()
		if e != nil {
			break
		}
		m := obj(data)
		switch rawString(m["type"]) {
		case "content_block_start":
			starts++
		case "content_block_stop":
			stops++
		case "content_block_delta":
			text += rawString(obj(m["delta"])["text"])
		}
	}
	if starts != 1 || stops != 1 || text != "PACAROUTE_OK" {
		t.Fatalf("fragmented Claude message: starts=%d stops=%d text=%s", starts, stops, text)
	}
}

func TestRestoreBackupPreservesExactBytesAndRejectsTraversal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	original := []byte("{\n  \"env\": {\"CUSTOM\": \"keep\"}, \"theme\": \"dark\"\n}\n")
	path := filepath.Join(home, "settings.json")
	os.WriteFile(path, original, 0600)
	p, e := planClient("claude-code", "copilot", "claude-opus-5.5", 45678)
	if e != nil {
		t.Fatal(e)
	}
	backup, e := p.apply(p.fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	id := filepath.Base(backup)
	preview, e := restoreBackup("claude-code", id, "", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = restoreBackup("claude-code", "../../settings.json", "", false); e == nil {
		t.Fatal("path traversal accepted")
	}
	_, e = restoreBackup("claude-code", id, preview["fingerprint"].(string), true)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if !bytes.Equal(b, original) {
		t.Fatal("restore changed bytes")
	}
}

func TestMessagesUsageIncludesCachedPromptOnce(t *testing.T) {
	u := Usage{}
	u.observe([]byte(`{"message":{"usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":70,"cache_creation_input_tokens":5}}}`))
	u.observe([]byte(`{"usage":{"output_tokens":12}}`))
	if v := u.totalInput(true); v == nil || *v != 85 || *u.Output != 12 {
		t.Fatal("cross-provider total omitted cache or double-counted snapshots")
	}
	if *u.totalInput(false) != 10 {
		t.Fatal("Responses input was incorrectly inflated")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func syntheticAuthClient(fn func(*http.Request) string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fn(r)))}, nil
	})}
}

// An unexpired Copilot exchange lease survives a process restart. Logout invalidates it
// even in another running process; the encrypted lease must never resurrect a login.
func TestExchangeLeaseRestartAndCrossProcessLogout(t *testing.T) {
	s := storeForTest(t)
	a := NewAuth(s, "")
	path := a.vault.githubPath("copilot")
	if e := a.vault.write(path, credential{Version: 1, AccessToken: "synthetic-github", CredentialID: "test"}); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	client := syntheticAuthClient(func(r *http.Request) string {
		calls.Add(1)
		if r.Header.Get("Authorization") != "token synthetic-github" {
			t.Error("wrong exchange credential")
		}
		return fmt.Sprintf(`{"token":"synthetic-exchange","expires_at":%d,"endpoints":{"api":"https://api.githubcopilot.com"}}`, time.Now().Add(time.Hour).Unix())
	})
	a.http.values = map[string]*http.Client{"system:": client}
	source, _ := s.Snapshot().Source("copilot")
	l, e := a.Lease(context.Background(), s.Snapshot(), source, false)
	if e != nil {
		t.Fatal(e)
	}
	b := NewAuth(s, "")
	b.http.values = map[string]*http.Client{"system:": client}
	second, e := b.Lease(context.Background(), s.Snapshot(), source, false)
	if e != nil || second.Value != l.Value || calls.Load() != 1 {
		t.Fatal("restart exchanged a valid cached lease again")
	}
	if e = a.Logout(context.Background(), "copilot"); e != nil {
		t.Fatal(e)
	}
	if _, e = b.Lease(context.Background(), s.Snapshot(), source, false); e == nil {
		t.Fatal("logout resurrected cached credentials")
	}
}

func TestRotatingCredentialRefreshSerializesAcrossProcesses(t *testing.T) {
	s := storeForTest(t)
	a := NewAuth(s, "")
	b := NewAuth(s, "")
	expired := time.Now().Add(-time.Minute)
	record := credential{Version: 4, AccessToken: "synthetic-expired", RefreshToken: "synthetic-refresh", AccessExpires: &expired, OAuthClientID: "synthetic-client", CredentialID: "one-identity", Generation: 7}
	if e := a.vault.write(a.vault.githubPath("copilot"), record); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	client := syntheticAuthClient(func(r *http.Request) string {
		calls.Add(1)
		form, _ := io.ReadAll(r.Body)
		if !bytes.Contains(form, []byte("refresh_token=synthetic-refresh")) {
			t.Error("refresh token rotated twice")
		}
		return `{"access_token":"synthetic-current","refresh_token":"synthetic-next","expires_in":3600,"refresh_token_expires_in":7200}`
	})
	a.http.values = map[string]*http.Client{"system:": client}
	b.http.values = map[string]*http.Client{"system:": client}
	source, _ := s.Snapshot().Source("copilot")
	var wg sync.WaitGroup
	for _, auth := range []*Auth{a, b} {
		wg.Add(1)
		go func(auth *Auth) {
			defer wg.Done()
			l, e := auth.Lease(context.Background(), s.Snapshot(), source, false)
			if e != nil || l.Value != "Bearer synthetic-current" {
				t.Errorf("lease error: %v", e)
			}
		}(auth)
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("rotation occurred %d times", calls.Load())
	}
	saved, e := a.readCredential("copilot")
	if e != nil || saved.Generation != 8 || saved.CredentialID != "one-identity" || saved.RefreshToken != "synthetic-next" {
		t.Fatal("credential generation/identity lost")
	}
}

// Screenshot-bearing tool results retain text, image bytes and ordering in both directions.
func TestCrossProtocolToolImagesRoundTrip(t *testing.T) {
	original := json.RawMessage(`[{"type":"text","text":"screenshot"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"}}]`)
	wire, err := toolOutput(original, "responses")
	if err != nil {
		t.Fatal(err)
	}
	blocks := array(wire)
	if len(blocks) != 2 || rawString(blocks[1]["image_url"]) != "data:image/png;base64,AQID" {
		t.Fatalf("lost screenshot: %s", wire)
	}
	back, err := toolOutput(wire, "messages")
	if err != nil {
		t.Fatal(err)
	}
	blocks = array(back)
	if len(blocks) != 2 || rawString(blocks[0]["text"]) != "screenshot" || rawString(obj(blocks[1]["source"])["data"]) != "AQID" {
		t.Fatalf("lost image on return: %s", back)
	}
	if _, err = toolOutput(json.RawMessage(`[{"type":"audio","data":"opaque"}]`), "responses"); err == nil {
		t.Fatal("unsupported block silently dropped")
	}
}

func TestNonStreamingMessagesUsageNormalizesCache(t *testing.T) {
	tr := translation{to: "messages"}
	body, err := tr.messagesJSON(obj([]byte(`{"content":[],"usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}`)))
	if err != nil {
		t.Fatal(err)
	}
	u := Usage{}
	u.observe(body)
	if total := number(obj(obj(body)["usage"]), "total_tokens"); total == nil || *total != 134 {
		t.Fatalf("missing Responses total_tokens: %s", body)
	}
	if u.Input == nil || *u.Input != 130 || u.Output == nil || *u.Output != 4 || u.CacheRead == nil || *u.CacheRead != 100 {
		t.Fatalf("inconsistent normalized usage: %s", body)
	}
}

func TestPublicOriginBehindTLSProxy(t *testing.T) {
	s, _, _ := gatewayTest(t, func(http.ResponseWriter, *http.Request) {})
	s.PublicURL = "https://gateway.example"
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/login", strings.NewReader(string(jsonBytes(map[string]string{"key": s.AdminKey}))))
	r.Header.Set("Origin", "https://gateway.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].Secure {
		t.Fatal("TLS proxy login requires a secure cookie", w.Code)
	}
	r.Header.Set("Origin", "https://attacker.example")
	if s.sameOrigin(r) {
		t.Fatal("untrusted proxy origin accepted")
	}
}

func TestAPIKeyRotationAcrossAuthorities(t *testing.T) {
	store := storeForTest(t)
	a := NewAuth(store, "")
	other := NewAuth(store, "")
	source := Source{ID: "api", Kind: "openai", AuthMode: "api-key"}
	if err := a.SaveKey(context.Background(), source.ID, "fixture-before"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Lease(context.Background(), store.Snapshot(), source, false); err != nil {
		t.Fatal(err)
	}
	if err := other.SaveKey(context.Background(), source.ID, "fixture-after"); err != nil {
		t.Fatal(err)
	}
	l, err := a.Lease(context.Background(), store.Snapshot(), source, false)
	if err != nil || l.Value != "Bearer fixture-after" {
		t.Fatal("running server kept the old CLI-managed key", err)
	}
}

func TestResponsesAssistantReplayIsNotPrefill(t *testing.T) {
	tr := &translation{toolNames: map[string]toolIdentity{}}
	wire, err := responsesToMessages(obj([]byte(`{"model":"claude-opus-5.5","input":[{"role":"assistant","content":"Working on it."}]}`)), tr)
	if err != nil {
		t.Fatal(err)
	}
	messages := array(wire["messages"])
	if len(messages) != 2 || rawString(messages[1]["role"]) != "user" {
		t.Fatal("replay sent unsupported assistant prefill")
	}
}

type terminalReadFailure struct{ calls int }

func (r *terminalReadFailure) Read([]byte) (int, error) { r.calls++; return 0, context.Canceled }

// A complete semantic response succeeds even when its transport has not closed yet.
func TestTerminalFrameDoesNotWaitForTransportEOF(t *testing.T) {
	for _, frame := range []string{`{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":3}}}`, `{"type":"message_stop"}`} {
		tail := &terminalReadFailure{}
		wire := "data: " + frame + "\n\n"
		var output bytes.Buffer
		if err := relaySSE(&output, io.MultiReader(strings.NewReader(wire), tail), &Usage{}, "responses"); err != nil {
			t.Fatal("completed response counted as failure", err)
		}
		if tail.calls != 0 || output.String() != wire {
			t.Fatal("waited beyond terminal frame or changed its bytes")
		}
	}
}
