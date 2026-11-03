package pacaroute

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

// Model breakdowns must reconcile to their source and the overall totals,
// including filters, missing usage, append-only refresh and restart backfill.
func TestUsageSourceModelBreakdown(t *testing.T) {
	root := t.TempDir()
	store := usage.NewActivityStore(root)
	start := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	n := func(v int64) *int64 { return &v }
	records := []usage.Activity{
		{Source: "copilot", Model: "opus", ClientID: "a", Input: n(100), Output: n(10), CacheRead: n(70), Status: 200},
		{Source: "copilot", Model: "sonnet", ClientID: "a", Input: n(40), Output: n(5), CacheRead: n(20), Status: 200},
		{Source: "azure", Model: "astra", ClientID: "a", Input: n(60), Output: n(8), CacheRead: n(0), Status: 200},
		{Source: "copilot", Model: "sonnet", ClientID: "b", Input: n(20), Output: n(2), Status: 200},
		{Source: "copilot", Model: "sonnet", ClientID: "a", Status: 429},
		{Source: "copilot", Model: "", ClientID: "a", Status: 502},
	}
	for i, record := range records {
		record.Kind = "request"
		record.Timestamp = start.Add(time.Duration(i) * time.Minute)
		if err := store.Record(record); err != nil {
			t.Fatal(err)
		}
	}
	check := func(summary map[string]any, want usage.Totals) {
		t.Helper()
		if got := summary["total"].(usage.Totals); got != want {
			t.Fatalf("total = %+v, want %+v", got, want)
		}
		byModel := summary["source_models"].(map[string]map[string]usage.Totals)
		sources := summary["sources"].(map[string]usage.Totals)
		for source, expected := range sources {
			var sum usage.Totals
			for _, v := range byModel[source] {
				sum.Requests += v.Requests
				sum.Input += v.Input
				sum.Output += v.Output
				sum.Cached += v.Cached
				sum.Missing += v.Missing
				sum.Failures += v.Failures
			}
			if sum != expected {
				t.Fatalf("%s models = %+v, source = %+v", source, sum, expected)
			}
		}
	}
	all := store.Summary(start, start.Add(time.Hour), "", "")
	check(all, usage.Totals{Requests: 6, Input: 220, Output: 25, Cached: 90, Failures: 2, Missing: 2})
	scoped := store.Summary(start, start.Add(time.Hour), "copilot", "a")
	check(scoped, usage.Totals{Requests: 4, Input: 140, Output: 15, Cached: 90, Failures: 2, Missing: 2})
	models := scoped["source_models"].(map[string]map[string]usage.Totals)["copilot"]
	if models["sonnet"] != (usage.Totals{Requests: 2, Input: 40, Output: 5, Cached: 20, Failures: 1, Missing: 1}) {
		t.Fatalf("Sonnet=%+v", models["sonnet"])
	}
	if _, ok := models[""]; !ok {
		t.Fatal("legacy model-less record lost")
	}
	if !reflect.DeepEqual(all, store.Summary(start, start.Add(time.Hour), "", "")) {
		t.Fatal("refresh double-counted history")
	}
	check(store.Summary(start.Add(time.Hour), start.Add(2*time.Hour), "", ""), usage.Totals{})
	record := usage.Activity{Kind: "request", Timestamp: start.Add(time.Hour), Source: "azure", Model: "astra", ClientID: "a", Status: 200, Input: n(3), Output: n(1)}
	if err := store.Record(record); err != nil {
		t.Fatal(err)
	}
	updated := store.Summary(start, start.Add(2*time.Hour), "", "")
	check(updated, usage.Totals{Requests: 7, Input: 223, Output: 26, Cached: 90, Failures: 2, Missing: 2})
	reopened := usage.NewActivityStore(root).Summary(start, start.Add(2*time.Hour), "", "")
	a, _ := json.Marshal(updated)
	b, _ := json.Marshal(reopened)
	if string(a) != string(b) {
		t.Fatal("restart must reconstruct the same model breakdown from existing logs")
	}
}
