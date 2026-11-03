package pacaroute

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

const completedStream = "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}}\n\n"

// A connection failure happens before any downstream output, so a failover
// pool may try its next same-model member; without failover the Agent sees 502.
func TestConnectionFailureFailsOverOnlyBeforeCommit(t *testing.T) {
	for _, failover := range []bool{true, false} {
		t.Run(fmt.Sprint(failover), func(t *testing.T) {
			var healthy atomic.Int32
			s, _, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
				healthy.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, completedStream)
			})
			c := cloneConfig(store.Snapshot())
			dead := c.Sources[0]
			dead.ID = "dead"
			dead.Endpoint = "https://127.0.0.1:1"
			c.Sources = append(c.Sources, dead)
			c.Pools = []domain.Pool{{ID: "pool", Name: "Pool", Agent: "codex", Model: "coding", Failover: failover, Members: []domain.Member{{ID: "dead", SourceID: "dead", Model: "deployed", Weight: 1}, {ID: "live", SourceID: "test", Model: "deployed", Weight: 1}}}}
			c.Clients[0].Routes[0].SourceID = "pool:pool"
			c.Clients[0].Routes[0].Model = "coding"
			saveTest(t, store, c)
			if err := s.Auth.SetKeys(map[string]string{"dead": "synthetic-only"}); err != nil {
				t.Fatal(err)
			}
			// Pin the session to the unreachable member so it is attempted first.
			targets, err := s.Router.Targets(store.Snapshot(), "alice", "codex", "pool:pool", "coding", "dead-session", false)
			if err != nil {
				t.Fatal(err)
			}
			for _, target := range targets {
				if target.Source.ID == "dead" {
					if err := s.Router.Bind(target); err != nil {
						t.Fatal(err)
					}
				}
			}
			r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(`{"model":"coding","stream":true,"input":"hello"}`))
			r.Header.Set("Authorization", "Bearer alice-key")
			r.Header.Set("X-PacaRoute-Session", "dead-session")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if failover {
				if w.Code != 200 || w.Header().Get("X-PacaRoute-Source") != "test" || healthy.Load() != 1 || !strings.Contains(w.Body.String(), "response.completed") {
					t.Fatalf("pre-commit connection failure did not fail over: %d %q %s", w.Code, w.Header().Get("X-PacaRoute-Source"), w.Body.String())
				}
				recent := s.Activity.Recent(false)
				if len(recent) != 1 || recent[0].Attempts != 2 || recent[0].Source != "test" || recent[0].Status != 200 {
					t.Fatalf("activity must attribute the final attempt: %+v", recent)
				}
			} else if w.Code != 502 || healthy.Load() != 0 {
				t.Fatalf("disabled failover must not try another member: %d, healthy calls %d", w.Code, healthy.Load())
			}
			if cooling := s.Router.Health(store.Snapshot()).Cooldowns; len(cooling) != 1 || cooling[0].SourceID != "dead" || cooling[0].Model != "deployed" {
				t.Fatalf("connection failure must cool only the failed source/model: %+v", cooling)
			}
		})
	}
}

// A successful gateway response persists the session binding. After a restart
// the session stays on its member even when that member stops taking new sessions.
func TestGatewayBindingSurvivesRestartAtZeroWeight(t *testing.T) {
	s, _, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, completedStream)
	})
	c := cloneConfig(store.Snapshot())
	second := c.Sources[0]
	second.ID = "second"
	c.Sources = append(c.Sources, second)
	c.Pools = []domain.Pool{{ID: "pool", Name: "Pool", Agent: "codex", Model: "coding", Failover: true, Members: []domain.Member{{ID: "one", SourceID: "test", Model: "deployed", Weight: 1}, {ID: "two", SourceID: "second", Model: "deployed", Weight: 1}}}}
	c.Clients[0].Routes[0].SourceID = "pool:pool"
	c.Clients[0].Routes[0].Model = "coding"
	saveTest(t, store, c)
	if err := s.Auth.SetKeys(map[string]string{"second": "synthetic-only"}); err != nil {
		t.Fatal(err)
	}
	send := func(session string) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(`{"model":"coding","stream":true,"input":"hello"}`))
		r.Header.Set("Authorization", "Bearer alice-key")
		r.Header.Set("X-PacaRoute-Session", session)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("request failed: %d %s", w.Code, w.Body.String())
		}
		return w.Header().Get("X-PacaRoute-Source")
	}
	pinned := send("pinned-session")
	c = cloneConfig(store.Snapshot())
	for i := range c.Pools[0].Members {
		if c.Pools[0].Members[i].SourceID == pinned {
			c.Pools[0].Members[i].Weight = 0
		}
	}
	saveTest(t, store, c)
	restarted, err := routing.NewRouter(store.Root)
	if err != nil {
		t.Fatal(err)
	}
	s.Router = restarted
	if got := send("pinned-session"); got != pinned {
		t.Fatalf("bound session moved from %s to %s after restart", pinned, got)
	}
	if got := send("new-session"); got == pinned {
		t.Fatal("zero-weight member accepted a new session")
	}
}

// A rejected Copilot lease is refreshed and retried exactly once before the
// response is relayed; the Agent never sees the first 401.
func TestCopilotRejectedLeaseRefreshesOnce(t *testing.T) {
	store := storeForTest(t)
	c := domain.Config{Clients: []domain.Client{{ID: "alice", Name: "Alice", KeyHash: keyHash("alice-key"), Routes: []domain.Route{{ID: "main", Agent: "codex", MatchModel: "coding", SourceID: "copilot", Model: "gpt-synthetic"}}}}}
	saveTest(t, store, c)
	auth := NewAuth(store, "")
	if err := auth.vault.write(auth.vault.githubPath("copilot"), credential{Version: 1, AccessToken: "synthetic-github", CredentialID: "test"}); err != nil {
		t.Fatal(err)
	}
	var exchanges, upstream atomic.Int32
	auth.http.Values = map[string]*http.Client{"system:": {Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		reply := func(status int, contentType, body string) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}
		if r.URL.Host == "api.github.com" {
			n := exchanges.Add(1)
			return reply(200, "application/json", fmt.Sprintf(`{"token":"synthetic-exchange-%d","expires_at":%d,"endpoints":{"api":"https://api.githubcopilot.com"}}`, n, time.Now().Add(time.Hour).Unix()))
		}
		upstream.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-exchange-2" {
			return reply(401, "application/json", `{"error":"stale lease"}`)
		}
		return reply(200, "application/json", `{"id":"resp-1","object":"response","output":[],"usage":{"input_tokens":3,"output_tokens":1}}`)
	})}}
	s, err := NewServer(store, auth)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(`{"model":"coding","input":"hello"}`)).WithContext(context.Background())
	r.Header.Set("Authorization", "Bearer alice-key")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "resp-1") {
		t.Fatalf("rejected lease was not refreshed: %d %s", w.Code, w.Body.String())
	}
	if exchanges.Load() != 2 || upstream.Load() != 2 {
		t.Fatalf("lease refresh must retry once: exchanges %d, upstream %d", exchanges.Load(), upstream.Load())
	}
}
