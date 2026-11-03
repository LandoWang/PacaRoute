package pacaroute

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Peer forwarding is bounded at eight hops: a request that has travelled seven
// hops is still forwarded (as hop eight); a request already at eight is refused
// before any upstream call.
func TestPeerHopBoundIsEight(t *testing.T) {
	var calls atomic.Int32
	var seen atomic.Value
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		seen.Store(r.Header.Get(peerHopHeader))
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	}))
	defer remote.Close()
	s := peerTestServer(t, remote.URL)
	call := func(hops string) int {
		r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(`{"model":"outer-coding","stream":true,"input":"hello"}`))
		r.Header.Set("Authorization", "Bearer outer-fixture-key")
		r.Header.Set(peerHopHeader, hops)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := call("7"); code != 200 || calls.Load() != 1 || seen.Load() != "8" {
		t.Fatalf("hop 7: status=%d upstream calls=%d forwarded hop=%v", code, calls.Load(), seen.Load())
	}
	if code := call("8"); code != 508 || calls.Load() != 1 {
		t.Fatalf("hop 8: status=%d upstream calls=%d", code, calls.Load())
	}
}

// Denied requests and model-catalog reads never consume a quota request slot:
// with a one-request quota, a catalog GET and a forbidden-route request are
// followed by an admitted request, and only then is the Client blocked.
func TestDenialAndCatalogDoNotReserveQuota(t *testing.T) {
	var calls atomic.Int32
	_, front, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		writeJSON(w, 200, map[string]any{"status": "completed", "output": []any{}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 1}})
	})
	c := cloneConfig(store.Snapshot())
	c.Clients[0].Quota = &domain.ClientQuota{WindowSeconds: 3600, Requests: 1}
	saveTest(t, store, c)
	catalog, _ := http.NewRequest("GET", front.URL+"/codex/v1/models", nil)
	catalog.Header.Set("Authorization", "Bearer alice-key")
	res, err := front.Client().Do(catalog)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("catalog", res.StatusCode)
	}
	for _, step := range []struct {
		model string
		want  int
	}{{"not-granted", 403}, {"coding", 200}, {"coding", 429}} {
		res := requestTest(t, front, "/codex/responses", "alice-key", `{"model":"`+step.model+`","input":"hello"}`)
		res.Body.Close()
		if res.StatusCode != step.want {
			t.Fatalf("model %q: status %d, want %d", step.model, res.StatusCode, step.want)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls.Load())
	}
}
