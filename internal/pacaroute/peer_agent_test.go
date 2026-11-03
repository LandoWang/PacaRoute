package pacaroute

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A downstream router has its own grant and model routes. Native Agent paths
// cannot inherit that grant, and revoking it must also hide model discovery.
func TestPacaRouteAgentGrantAndDisabledPaths(t *testing.T) {
	_, front, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "response", "output": []any{}, "usage": map[string]int{"input_tokens": 3, "output_tokens": 1}})
	})
	c := cloneConfig(store.Snapshot())
	c.Clients[0].EnabledAgents = []string{"pacaroute"}
	c.Clients[0].Routes = []Route{
		{ID: "peer", Agent: "pacaroute", MatchModel: "shared", SourceID: "test", Model: "deployed"},
		{ID: "direct", Agent: "codex", MatchModel: "direct-only", SourceID: "test", Model: "deployed"},
	}
	saveTest(t, store, c)
	request := func(path, key, model string) (int, []byte) {
		t.Helper()
		method, body := "GET", ""
		if !strings.HasSuffix(path, "models") {
			method = "POST"
			body = `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}],"input":"hi","max_tokens":32}`
		}
		r, _ := http.NewRequest(method, front.URL+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+key)
		res, err := front.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res.StatusCode, b
	}
	for _, path := range []string{"/codex/responses", "/cc/messages", "/codex/models", "/cc/models", "/sources/test/codex/responses"} {
		if code, _ := request(path, "alice-key", "direct-only"); code != 404 {
			t.Fatalf("disabled path %s: %d", path, code)
		}
	}
	for _, path := range []string{"/pacaroute/codex/responses", "/pacaroute/cc/messages"} {
		if code, b := request(path, "alice-key", "shared"); code != 200 {
			t.Fatalf("peer path %s: %d %s", path, code, b)
		}
		if code, _ := request(path, "wrong-key", "shared"); code != 401 {
			t.Fatal("peer path bypassed Client authentication")
		}
	}
	code, body := request("/pacaroute/v1/models", "alice-key", "")
	var catalog struct {
		Data []peerModel `json:"data"`
	}
	if json.Unmarshal(body, &catalog) != nil || code != 200 || len(catalog.Data) != 1 || catalog.Data[0].ID != "shared" || len(catalog.Data[0].Agents) != 2 {
		t.Fatalf("peer route catalog: %d %s", code, body)
	}
	c = cloneConfig(store.Snapshot())
	c.Clients[0].EnabledAgents = []string{"codex"}
	saveTest(t, store, c)
	for _, path := range []string{"/pacaroute/codex/responses", "/pacaroute/cc/messages", "/pacaroute/v1/models", "/v1/models"} {
		if code, _ := request(path, "alice-key", "shared"); code != 404 {
			t.Fatalf("revoked peer path %s: %d", path, code)
		}
	}
	if code, _ := request("/codex/responses", "alice-key", "direct-only"); code != 200 {
		t.Fatal("peer revocation disabled the direct Agent")
	}
	local := peerTestServer(t, front.URL)
	if _, err := local.Auth.DiscoverModels(context.Background(), local.Store.Snapshot(), local.Store.Snapshot().Sources[0]); err == nil {
		t.Fatal("peer discovery fell back into a native grant after 404")
	}
}

func TestKnownClientAgentKindsOnly(t *testing.T) {
	store := storeForTest(t)
	c := cloneConfig(store.Snapshot())
	c.Clients[0].EnabledAgents = []string{"codex", "claude-code", "pacaroute"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Clients[0].EnabledAgents = []string{"unknown-agent"}
	if c.Validate() == nil {
		t.Fatal("unimplemented Agent accepted")
	}
}
