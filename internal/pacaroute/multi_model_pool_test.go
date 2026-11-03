package pacaroute

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func multiModelFixture(t *testing.T) (*Router, domain.Config) {
	t.Helper()
	r, err := NewRouter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := domain.Config{Accounts: []domain.Account{{ID: "gh", Name: "GitHub", Kind: "github"}}, Proxies: domain.Proxies{Bindings: map[string]string{}}}
	for _, id := range []string{"a", "b", "c"} {
		c.Sources = append(c.Sources, domain.Source{ID: id, Name: id, Kind: "copilot", AccountID: "gh"})
	}
	c.Pools = []domain.Pool{{ID: "big", Name: "Big Source", Agent: "codex", Model: "astra", MultiModel: true, Failover: true, Members: []domain.Member{
		{ID: "a-astra", SourceID: "a", Model: "gpt-6-astra", Alias: "astra", Weight: 50},
		{ID: "b-astra", SourceID: "b", Model: "gpt-6-astra", Alias: "astra", Weight: 30},
		{ID: "c-astra", SourceID: "c", Model: "gpt-6-astra", Alias: "astra", Weight: 20},
		{ID: "a-other", SourceID: "a", Model: "other-deployment", Alias: "other", Weight: 1},
	}}}
	if err = c.Validate(); err != nil {
		t.Fatal(err)
	}
	return r, c
}

// One model's throttle redistributes only its group's traffic by the surviving
// relative weights. Healthy sticky sessions stay put, and cannot cross models.
func TestMultiModelPoolThrottleWeightsAndStickyScope(t *testing.T) {
	r, c := multiModelFixture(t)
	first, err := r.Targets(&c, "alice", "codex", "pool:big", "other", "same-session", false)
	if err != nil || first[0].Source.ID != "a" || first[0].Model != "other-deployment" {
		t.Fatal("wrong model group", err)
	}
	if err = r.Bind(first[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Targets(&c, "alice", "codex", "pool:big", "astra", "same-session", true); err == nil {
		t.Fatal("source-bound reference crossed model groups")
	}
	distribution := func(n int) map[string]int {
		t.Helper()
		counts := map[string]int{}
		for i := 0; i < n; i++ {
			targets, err := r.Targets(&c, "alice", "codex", "pool:big", "astra", fmt.Sprintf("session-%d", i), false)
			if err != nil {
				t.Fatal(err)
			}
			for _, target := range targets {
				if target.Model != "gpt-6-astra" {
					t.Fatal("failover crossed public model groups")
				}
			}
			counts[targets[0].Source.ID]++
		}
		return counts
	}
	before := distribution(10000)
	if before["a"] < 4700 || before["a"] > 5300 {
		t.Fatal("incorrect initial 50:30:20 allocation", before)
	}
	pool := c.Pools[0]
	source, _ := c.Source("a")
	r.FailResponse(Target{Source: source, Model: "gpt-6-astra", Pool: &pool}, 429, http.Header{"Retry-After": []string{"120"}})
	after := distribution(10000)
	if after["a"] != 0 || after["b"] < 5700 || after["b"] > 6300 || after["b"]+after["c"] != 10000 {
		t.Fatal("remaining 30:20 weights did not become 60:40", after)
	}
	other, err := r.Targets(&c, "alice", "codex", "pool:big", "other", "same-session", true)
	if err != nil || !other[0].Sticky || other[0].Affinity != first[0].Affinity {
		t.Fatal("throttle leaked into another model or disturbed its session")
	}
	r.FailResponse(Target{Source: source, Model: "other-deployment", Pool: &pool}, 429, http.Header{"Retry-After": []string{"5"}})
	health := r.Health(&c)
	if len(health.Cooldowns) != 2 || health.Pools[0].Models[0].Available != 2 || health.Pools[0].Models[1].Available != 0 {
		t.Fatal("model-level exhaustion hidden", health)
	}
	if health.Cooldowns[0].Until.Sub(health.Cooldowns[1].Until) < 110*time.Second {
		t.Fatal("model throttle deadlines were shared")
	}
	// A late concurrent error cannot shorten the longer provider retry hint.
	until := health.Cooldowns[0].Until
	r.FailResponse(Target{Source: source, Model: "gpt-6-astra", Pool: &pool}, 429, http.Header{"Retry-After": []string{"1"}})
	if !r.Health(&c).Cooldowns[0].Until.Equal(until) {
		t.Fatal("parallel throttle shortened recovery time")
	}
	r.ExpireCooldown("a", "gpt-6-astra")
	restored := distribution(10000)
	if restored["a"] != before["a"] || restored["b"] != before["b"] {
		t.Fatal("expired throttle did not restore original allocation")
	}
	third, _ := c.Source("c")
	r.FailResponse(Target{Source: third, Model: "gpt-6-astra", Pool: &pool}, 429, http.Header{"Retry-After": []string{"20"}})
	fiveToThree := distribution(10000)
	if fiveToThree["c"] != 0 || fiveToThree["a"] < 5950 || fiveToThree["a"] > 6550 || fiveToThree["a"]+fiveToThree["b"] != 10000 {
		t.Fatal("remaining 50:30 weights did not become 62.5:37.5", fiveToThree)
	}
}

func TestMultiModelPoolAliasesValidationAndDirectThrottle(t *testing.T) {
	r, c := multiModelFixture(t)
	pool := c.Pools[0]
	pool.Members = append(pool.Members, domain.Member{ID: "a-alias", SourceID: "a", Model: "gpt-6-astra", Alias: "same-deployment", Weight: 1})
	c.Pools[0] = pool
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	source, _ := c.Source("a")
	r.FailResponse(Target{Source: source, Model: "gpt-6-astra"}, 429, http.Header{"X-Ms-Retry-After-Ms": []string{"2500"}})
	for _, alias := range []string{"same-deployment", "missing"} {
		if _, err := r.Targets(&c, "alice", "codex", "pool:big", alias, "session", false); err == nil {
			t.Fatal("unknown or throttled alias accepted", alias)
		}
	}
	if _, err := r.Targets(&c, "alice", "codex", "a", "gpt-6-astra", "", false); err == nil {
		t.Fatal("direct route bypassed model throttle")
	}
	if _, err := r.Targets(&c, "alice", "codex", "a", "other-deployment", "", false); err != nil {
		t.Fatal("direct route throttled whole source")
	}
	health := r.Health(&c)
	remaining := health.Cooldowns[0].Until.Sub(health.ObservedAt)
	if remaining <= time.Second || remaining > 2500*time.Millisecond {
		t.Fatal("millisecond Retry-After ignored", remaining)
	}
	c.Pools[0].Members[3].Weight = 0
	if c.Validate() == nil {
		t.Fatal("model group with no positive weight accepted")
	}
}

func TestModelThrottleRetryHintBounds(t *testing.T) {
	for _, test := range []struct {
		name, hint string
		expected   time.Duration
	}{
		{"seconds", "90", 90 * time.Second},
		{"http-date", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat), time.Minute},
		{"upper-bound", "999999", 24 * time.Hour},
		{"lower-bound", "0.001", time.Second},
		{"invalid", "not-a-time", 30 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, c := multiModelFixture(t)
			source, _ := c.Source("a")
			r.FailResponse(Target{Source: source, Model: "gpt-6-astra"}, 429, http.Header{"Retry-After": []string{test.hint}})
			h := r.Health(&c)
			remaining := h.Cooldowns[0].Until.Sub(h.ObservedAt)
			if remaining > test.expected || remaining < test.expected-2*time.Second {
				t.Fatal("incorrect bounded retry deadline", remaining)
			}
		})
	}
}
