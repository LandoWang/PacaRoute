package pacaroute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Client cancellation before headers is not a source failure, just as client
// cancellation after stream commit must not penalize a healthy pool member.
func TestCancelBeforeHeadersDoesNotCooldownPool(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var once sync.Once
	var calls atomic.Int32
	s, _, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		once.Do(func() { close(entered) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	c := cloneConfig(store.Snapshot())
	other := c.Sources[0]
	other.ID = "other"
	c.Sources = append(c.Sources, other)
	if err := s.Auth.SetKeys(map[string]string{"other": "synthetic-only"}); err != nil {
		t.Fatal(err)
	}
	c.Pools = []domain.Pool{{ID: "pool", Name: "Pool", Agent: "codex", Model: "coding", Failover: true,
		Members: []domain.Member{{ID: "member", SourceID: "test", Model: "deployed", Weight: 1}, {ID: "other-member", SourceID: "other", Model: "deployed", Weight: 1}}}}
	c.Clients[0].Routes[0].SourceID = "pool:pool"
	c.Clients[0].Routes[0].Model = "coding"
	saveTest(t, store, c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Handler().ServeHTTP(httptest.NewRecorder(), streamRequest(ctx, fullHistoryRequest))
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream was not reached")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled request did not finish")
	}
	if calls.Load() != 1 {
		t.Fatalf("expected exactly one actual upstream call, got %d", calls.Load())
	}
	if health := s.Router.Health(store.Snapshot()); len(health.Cooldowns) != 0 {
		t.Errorf("client cancellation penalized healthy source: cooldowns=%d available=%d", len(health.Cooldowns), health.Pools[0].Available)
	}
	records := s.Activity.Recent(true)
	if len(records) != 1 {
		t.Fatalf("expected one activity record, got %d", len(records))
	}
	if records[0].Status != 499 {
		t.Errorf("client cancellation recorded as provider failure: status=%d attempts=%d", records[0].Status, records[0].Attempts)
	}
	if _, err := s.Router.Targets(store.Snapshot(), "other-client", "codex", "pool:pool", "coding", "new-session", false); err != nil {
		t.Errorf("client cancellation blocked unrelated session: %v", err)
	}
}
