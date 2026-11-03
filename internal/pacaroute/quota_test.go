package pacaroute

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func tokenCount(v int64) *int64 { return &v }
func TestPricingConfirmedUsageAndSnapshots(t *testing.T) {
	p := domain.ModelPrice{Input: "2", Cached: "0.2", Output: "8", CacheWrite: "2.5"}
	v := Activity{Input: tokenCount(1000), CacheRead: tokenCount(200), CacheWrite: tokenCount(100), Output: tokenCount(100)}
	cost, reason := priceUsage(v, &p, false)
	if reason != "" || cost == nil || *cost != 2440 {
		t.Fatal(cost, reason)
	}
	cost, reason = priceUsage(v, &p, true)
	if reason != "" || cost == nil || *cost != 2490 {
		t.Fatal(cost, reason)
	}
	for _, value := range []string{"-1", "NaN", "1e3", "0.0000001", "1000000000"} {
		if _, e := domain.MoneyMicros(value); e == nil {
			t.Fatal("invalid amount accepted", value)
		}
	}
	v.CacheRead = nil
	if cost, reason = priceUsage(v, &p, false); cost != nil || reason != "missing_cache_usage" {
		t.Fatal("missing cache billed as zero")
	}
	v.Input = nil
	if cost, reason = priceUsage(v, &p, false); cost != nil || reason != "missing_usage" {
		t.Fatal("missing input billed as zero")
	}
	if cost, reason = priceUsage(v, nil, false); cost != nil || reason != "missing_price" {
		t.Fatal("unconfigured price billed as zero")
	}
}
func TestQuotaConcurrentAdmissionRestartAndIdempotentSettlement(t *testing.T) {
	store := storeForTest(t)
	q, e := newQuotaStore(store.db)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	limit := &domain.ClientQuota{WindowSeconds: 60, Requests: 5}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := q.Admit(Activity{ID: domain.NewID(), ClientID: "alice", Agent: "codex", Timestamp: now}, limit)
			if e != nil {
				t.Error(e)
				return
			}
		}()
	}
	wg.Wait()
	state, e := q.State("alice", limit, now)
	if e != nil || state.Requests != 5 || state.Pending != 5 || state.Blocked != "requests" {
		t.Fatal(state, e)
	}
	var id string
	if e = store.db.QueryRow("SELECT request_id FROM quota_requests LIMIT 1").Scan(&id); e != nil {
		t.Fatal(e)
	}
	p := domain.ModelPrice{Input: "2", Cached: "0.2", Output: "8"}
	v := Activity{ID: id, ClientID: "alice", Source: "actual-source", Model: "deployed", Input: tokenCount(1000), CacheRead: tokenCount(200), Output: tokenCount(100), Status: 200}
	if e = q.Settle(v, &p, false); e != nil {
		t.Fatal(e)
	}
	p.Input = "999"
	if e = q.Settle(v, &p, false); e != nil {
		t.Fatal(e)
	}
	var events int
	if e = store.db.QueryRow("SELECT count(*) FROM billing_events WHERE request_id=?", id).Scan(&events); e != nil || events != 3 {
		t.Fatal(events, e)
	}
	root := store.Root
	store.Close()
	reopened, e := NewConfigStore(root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	q, e = newQuotaStore(reopened.db)
	if e != nil {
		t.Fatal(e)
	}
	state, e = q.State("alice", limit, now)
	if e != nil || state.Requests != 5 || state.CostMicros != 2440 || state.Pending != 4 {
		t.Fatal(state, e)
	}
	state, e = q.State("alice", limit, now.Add(61*time.Second))
	if e != nil || state.Requests != 0 || state.Blocked != "" {
		t.Fatal(state, e)
	}
}
func TestGatewayClientQuotaUsesActualTargetAndConfirmedTotals(t *testing.T) {
	var count atomic.Int32
	_, front, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		writeJSON(w, 200, map[string]any{"output": []any{}, "usage": map[string]any{"input_tokens": 1000, "output_tokens": 100, "input_tokens_details": map[string]int{"cached_tokens": 200}}})
	})
	c := cloneConfig(store.Snapshot())
	c.Clients[0].Quota = &domain.ClientQuota{WindowSeconds: 60, InputTokens: 1000, Cost: "0.003"}
	c.Sources[0].Prices = map[string]domain.ModelPrice{"deployed": {Input: "2", Cached: "0.2", Output: "8"}, "coding": {Input: "999", Cached: "999", Output: "999"}}
	saveTest(t, store, c)
	res := requestTest(t, front, "/codex/responses", "alice-key", `{"model":"coding"}`)
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	res = requestTest(t, front, "/codex/responses", "alice-key", `{"model":"coding"}`)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" {
		t.Fatal(res.Status, string(b))
	}
	if count.Load() != 1 {
		t.Fatal("quota reached upstream")
	}
	q, e := newQuotaStore(store.db)
	if e != nil {
		t.Fatal(e)
	}
	v, e := q.State("alice", c.Clients[0].Quota, time.Now().UTC())
	if e != nil || v.CostMicros != 2440 || v.InputTokens != 1000 || v.Requests != 1 {
		t.Fatal(v, e)
	}
	c = cloneConfig(store.Snapshot())
	c.Clients[0].Quota = &domain.ClientQuota{WindowSeconds: 60, Cost: "0.002"}
	saveTest(t, store, c)
	res = requestTest(t, front, "/codex/responses", "alice-key", `{"model":"coding"}`)
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	var body map[string]any
	json.Unmarshal(b, &body)
	if res.StatusCode != 429 {
		t.Fatal(res.Status, body)
	}
	bundle := ExportConfig(store.Snapshot())
	if bundle.Config.Clients[0].Quota == nil || len(bundle.Config.Sources[0].Prices) != 2 {
		t.Fatal("portable limits/prices lost")
	}
}
