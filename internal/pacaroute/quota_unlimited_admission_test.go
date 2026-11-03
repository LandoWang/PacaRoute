package pacaroute

import (
	"fmt"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// A Client without enforced limits cannot be blocked, so admitting it must not
// cost time proportional to its settled history. The reservation is still kept.
func TestUnlimitedAdmissionDoesNotScanHistory(t *testing.T) {
	store, err := NewConfigStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	q, err := newQuotaStore(store.db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	const rows = 100000
	if _, err = store.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM seq WHERE n<?)
		INSERT INTO quota_requests(request_id,client_id,agent,started_ms,finished_ms,state,input_tokens,output_tokens,cost_microusd)
		SELECT 'seed-'||n,'history','codex',? - 3600000 + n*3500000/?,?,'settled',1000,100,200 FROM seq`, rows, now.UnixMilli(), rows, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	sequence := 0
	measure := func(client string, limit *domain.ClientQuota) time.Duration {
		start := time.Now()
		for i := 0; i < 16; i++ {
			sequence++
			state, err := q.Admit(usage.Activity{ID: fmt.Sprintf("admit-%d", sequence), ClientID: client, Agent: "codex", Timestamp: now}, limit)
			if err != nil || state.Blocked != "" {
				t.Fatalf("unlimited admission: %+v %v", state, err)
			}
		}
		return time.Since(start) / 16
	}
	for _, limit := range []*domain.ClientQuota{nil, {WindowSeconds: 3600}} {
		measure("empty", limit)
		measure("history", limit)
		empty, history := measure("empty", limit), measure("history", limit)
		if history > 10*empty && history > 10*time.Millisecond {
			t.Fatalf("unlimited admission (limit %+v) scanned history: %s versus %s", limit, history, empty)
		}
	}
	state, err := q.State("history", nil, now)
	if err != nil || state.Requests != rows+64 || state.Pending != 64 {
		t.Fatalf("reservations not recorded: %+v %v", state, err)
	}
}
