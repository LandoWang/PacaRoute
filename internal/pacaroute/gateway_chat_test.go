package pacaroute

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func TestChatGatewayUsesCopilotAuthAndNeverCompletesBrokenStream(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprint(broken), func(t *testing.T) {
			store := storeForTest(t)
			c := cloneConfig(store.Snapshot())
			c.Clients = []domain.Client{{ID: "alice", Name: "Alice", KeyHash: keyHash("alice-key"), Routes: []domain.Route{{ID: "argon", Agent: "codex", MatchModel: "argon", SourceID: "copilot", Model: "gemini-4-argon"}}}}
			saveTest(t, store, c)
			auth := NewAuth(store, "")
			if err := auth.vault.write(auth.vault.githubPath("copilot"), credential{Version: 1, AccessToken: "synthetic-github", CredentialID: "test"}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			auth.http.Values = map[string]*http.Client{"system:": {Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := ""
				contentType := "application/json"
				if r.URL.Host == "api.github.com" {
					body = fmt.Sprintf(`{"token":"synthetic-copilot","expires_at":%d,"endpoints":{"api":"https://api.githubcopilot.com"}}`, time.Now().Add(time.Hour).Unix())
				} else {
					calls++
					if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-copilot" {
						t.Errorf("incorrect upstream endpoint/auth: %s", r.URL.Path)
					}
					raw, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(raw), `"model":"gemini-4-argon"`) || strings.Contains(string(raw), `"input":`) {
						t.Errorf("incorrect request conversion: %s", raw)
					}
					contentType = "text/event-stream"
					body = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n"
					if !broken {
						body += "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":8,\"prompt_tokens_details\":{\"cached_tokens\":30}}}\n\ndata: [DONE]\n\n"
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			server, err := NewServer(store, auth)
			if err != nil {
				t.Fatal(err)
			}
			invoke := func(key string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(`{"model":"argon","input":"hello","stream":true}`))
				r.Header.Set("Authorization", "Bearer "+key)
				w := httptest.NewRecorder()
				server.Handler().ServeHTTP(w, r)
				return w
			}
			if w := invoke("wrong"); w.Code != 401 || calls != 0 {
				t.Fatal("unauthorized inference")
			}
			w := invoke("alice-key")
			if calls != 1 {
				t.Fatalf("unexpected attempts: %d; status %d: %s", calls, w.Code, w.Body.String())
			}
			terminal := "response.completed"
			status := 200
			if broken {
				terminal = "response.failed"
				status = 502
				if strings.Contains(w.Body.String(), "response.completed") {
					t.Fatal("broken stream completed")
				}
			}
			if !strings.Contains(w.Body.String(), terminal) {
				t.Fatalf("missing terminal %s: %s", terminal, w.Body.String())
			}
			rows := server.Activity.Recent(false)
			if len(rows) != 1 || rows[0].Status != status {
				t.Fatalf("wrong activity: %+v", rows)
			}
			if !broken && (rows[0].Input == nil || *rows[0].Input != 100 || *rows[0].CacheRead != 30) {
				t.Fatalf("cache counted twice: %+v", rows)
			}
		})
	}
}
