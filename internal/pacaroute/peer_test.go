package pacaroute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func peerTestSource(endpoint string) domain.Source {
	return domain.Source{ID: "peer", Name: "Peer", Kind: "pacaroute", AccountID: "remote", AuthMode: "api-key", Endpoint: endpoint, Deployments: []string{"coding", "chat"}, AgentModels: map[string][]string{"codex": {"coding"}, "claude-code": {"chat"}}}
}
func peerTestServer(t *testing.T, endpoint string) *Server {
	t.Helper()
	store := storeForTest(t)
	c := domain.Config{Accounts: []domain.Account{{ID: "remote", Name: "Remote PacaRoute", Kind: "pacaroute", Endpoint: endpoint}}, Sources: []domain.Source{peerTestSource(endpoint)}, Clients: []domain.Client{{ID: "outer", Name: "Outer", KeyHash: keyHash("outer-fixture-key"), EnabledAgents: []domain.GrantKind{"codex", "claude-code"}, Routes: []domain.Route{{ID: "coding", Agent: "codex", MatchModel: "outer-coding", SourceID: "peer", Model: "coding"}, {ID: "chat", Agent: "claude-code", MatchModel: "outer-chat", SourceID: "peer", Model: "chat"}}}}}
	saveTest(t, store, c)
	auth := NewAuth(store, "")
	if err := auth.SaveAccountKey(context.Background(), "remote", "alice-key"); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(store, auth)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A peer sees only the authenticated Client's public model aliases and Agent
// permissions. Provider accounts, source IDs, deployment IDs and other Clients
// remain opaque, even with a valid management credential.
func TestPeerCatalogClientScopeAndCapability(t *testing.T) {
	remote, front, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) { t.Error("catalog reached inference") })
	c := cloneConfig(store.Snapshot())
	c.Clients[0].EnabledAgents = []domain.GrantKind{"pacaroute"}
	c.Pools = []domain.Pool{{ID: "codex-only", Name: "Private pool", Agent: "codex", Model: "deployed", Members: []domain.Member{{ID: "member", SourceID: "test", Model: "deployed", Weight: 1}}}}
	c.Clients[0].Routes[0].Agent = "pacaroute"
	c.Clients[0].Routes[0].SourceID = "pool:codex-only"
	c.Clients = append(c.Clients, domain.Client{ID: "bob", Name: "Bob", EnabledAgents: []domain.GrantKind{"pacaroute"}, KeyHash: keyHash("bob-key"), Routes: []domain.Route{{ID: "other", Agent: "pacaroute", MatchModel: "private-model", SourceID: "test", Model: "deployed"}}})
	saveTest(t, store, c)
	for _, test := range []struct {
		key  string
		want int
	}{{"", 401}, {remote.AdminKey, 401}, {"alice-key", 200}, {"bob-key", 200}} {
		req, _ := http.NewRequest("GET", front.URL+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+test.key)
		res, err := front.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != test.want {
			t.Fatalf("catalog status %d", res.StatusCode)
		}
		if test.key == "alice-key" {
			for _, secret := range []string{"deployed", "private-model", "test", "api", "claude-code"} {
				if strings.Contains(string(b), secret) {
					t.Fatal("catalog disclosed unauthorized details", string(b))
				}
			}
		}
	}
	local := peerTestServer(t, front.URL)
	discovered, err := local.Catalog.DiscoverModels(context.Background(), local.Store.Snapshot(), local.Store.Snapshot().Sources[0])
	if err != nil || discovered.Endpoint != front.URL+"/pacaroute" || len(discovered.Models) != 1 || discovered.Models[0].ID != "coding" || !discovered.Models[0].Codex || discovered.Models[0].Claude {
		t.Fatal("peer discovery lost capabilities", discovered, err)
	}
	draft := cloneConfig(local.Store.Snapshot())
	draft.Clients[0].Routes[1].Model = "coding"
	if draft.Validate() == nil {
		t.Fatal("unsupported Agent accepted")
	}
	bundle := ExportConfig(local.Store.Snapshot())
	b, _ := json.Marshal(bundle)
	if strings.Contains(string(b), "alice-key") || strings.Contains(string(b), "key_hash") || !strings.Contains(string(b), "agent_models") {
		t.Fatal("peer export credential/capability contract")
	}
	c = cloneConfig(store.Snapshot())
	c.Clients[0].Disabled = true
	saveTest(t, store, c)
	if _, err = local.Catalog.DiscoverModels(context.Background(), local.Store.Snapshot(), local.Store.Snapshot().Sources[0]); err == nil {
		t.Fatal("disabled remote Client discovered models")
	}
}

func TestPeerLegacyDiscovery(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/.well-known/pacaroute") {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer alice-key" {
			t.Error("missing peer key")
		}
		switch r.URL.Path {
		case "/prefix/v1/models":
			http.NotFound(w, r)
		case "/prefix/codex/models":
			writeJSON(w, 200, map[string]any{"data": []map[string]string{{"id": "coding"}}})
		default:
			problem(w, 403, "Agent disabled")
		}
	}))
	defer up.Close()
	s := peerTestServer(t, up.URL+"/prefix")
	result, err := s.Catalog.DiscoverModels(context.Background(), s.Store.Snapshot(), s.Store.Snapshot().Sources[0])
	if err != nil || len(result.Models) != 1 || !result.Models[0].Codex || result.Models[0].Claude {
		t.Fatal(result, err)
	}
}

// Requests, unknown fields, native streaming completion, token accounting,
// throttle hints and isolated sticky session IDs survive the extra router hop.
func TestPeerNativeProtocolsAndSessionIsolation(t *testing.T) {
	var mu sync.Mutex
	sessions := []string{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer alice-key" || r.Header.Get(peerHopHeader) != "1" {
			t.Error("peer authentication/hop missing")
		}
		mu.Lock()
		sessions = append(sessions, r.Header.Get("X-PacaRoute-Session"))
		mu.Unlock()
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		if body["opaque_vendor_field"] != "preserved" {
			t.Error("native field stripped")
		}
		if body["throttle"] == true {
			w.Header().Set("Retry-After", "123")
			problem(w, 429, "rate limited")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch r.URL.Path {
		case "/codex/responses":
			if body["model"] != "coding" {
				t.Error("remote alias missing")
			}
			fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":3}}}\n\n")
		case "/cc/messages":
			if body["model"] != "chat" {
				t.Error("remote alias missing")
			}
			fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		default:
			t.Error("wrong peer protocol endpoint", r.URL.Path)
		}
	}))
	defer remote.Close()
	s := peerTestServer(t, remote.URL)
	c := cloneConfig(s.Store.Snapshot())
	second := c.Clients[0]
	second.ID = "other"
	second.KeyHash = keyHash("other-fixture-key")
	c.Clients = append(c.Clients, second)
	saveTest(t, s.Store, c)
	call := func(key, path, model, extra string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":true,"input":"hello","messages":[],"opaque_vendor_field":"preserved"%s}`, model, extra)))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("X-PacaRoute-Session", "same-session")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	for _, key := range []string{"outer-fixture-key", "outer-fixture-key", "other-fixture-key"} {
		w := call(key, "/codex/responses", "outer-coding", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "response.completed") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := call("outer-fixture-key", "/cc/messages", "outer-chat", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "message_stop") {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(sessions) != 4 || sessions[0] == "" || sessions[0] != sessions[1] || sessions[0] == sessions[2] || sessions[0] == sessions[3] {
		t.Fatal("peer sessions collapsed Client/Agent scopes", sessions)
	}
	w = call("outer-fixture-key", "/codex/responses", "outer-coding", `,"throttle":true`)
	if w.Code != 429 || w.Header().Get("Retry-After") != "123" {
		t.Fatal("throttle hint lost", w.Code, w.Header())
	}
	records := s.Activity.Recent(false)
	successes := 0
	for _, record := range records {
		if record.Status == 200 {
			successes++
			if record.Input == nil || *record.Input != 12 || record.Output == nil || *record.Output != 3 {
				t.Fatal("peer usage lost", record)
			}
		}
	}
	if successes != 4 {
		t.Fatal("peer requests not recorded", successes)
	}
	if len(s.Router.Health(s.Store.Snapshot()).Cooldowns) != 1 {
		t.Fatal("remote throttle not recorded")
	}
}

func TestPeerForwardingCycleTerminates(t *testing.T) {
	s := peerTestServer(t, "http://127.0.0.1:1")
	front := httptest.NewServer(s.Handler())
	defer front.Close()
	c := cloneConfig(s.Store.Snapshot())
	c.Sources[0].Endpoint = front.URL
	c.Clients[0].Routes[0].MatchModel = "coding"
	saveTest(t, s.Store, c)
	if err := s.Auth.SaveAccountKey(context.Background(), "remote", "outer-fixture-key"); err != nil {
		t.Fatal(err)
	}
	res := requestTest(t, front, "/codex/responses", "outer-fixture-key", `{"model":"coding","input":"hello"}`)
	defer res.Body.Close()
	if res.StatusCode != 508 {
		t.Fatal("forwarding cycle not bounded", res.StatusCode)
	}
	for _, value := range []string{"-1", "wrong", "999999999999999999999999"} {
		r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer outer-fixture-key")
		r.Header.Set(peerHopHeader, value)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("invalid hop accepted", value, w.Code)
		}
	}
}
