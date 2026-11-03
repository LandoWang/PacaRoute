package pacaroute

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/protocol"

	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Native preparation replaces the model and must otherwise forward the request
// bytes faithfully, without cloning the whole payload through JSON first.
func TestNativePrepareDoesNotCloneLargePayload(t *testing.T) {
	content := strings.Repeat("x", 8<<20)
	body := object{
		"model":             jsonBytes("public-alias"),
		"input":             jsonBytes([]any{map[string]any{"role": "user", "content": content}}),
		"unknown_extension": json.RawMessage(`{"opaque":{"number":1234567890123456789}}`),
	}
	before := jsonBytes(body)
	target := routing.Target{Source: domain.Source{Kind: "azure-openai"}, Model: "deployment"}
	expected, err := json.Marshal(map[string]json.RawMessage{"model": jsonBytes("deployment"), "input": body["input"], "unknown_extension": body["unknown_extension"]})
	if err != nil {
		t.Fatal(err)
	}
	wire, tr, err := protocol.Prepare(body, target.Source.Kind, target.Model, "codex", target.UpstreamProtocol("codex"))
	if err != nil || tr != nil {
		t.Fatalf("native prepare: %v %v", tr, err)
	}
	if !bytes.Equal(wire, expected) || !bytes.Contains(wire, []byte(`"unknown_extension":{"opaque":{"number":1234567890123456789}}`)) {
		t.Fatal("native wire bytes changed")
	}
	if !bytes.Equal(before, jsonBytes(body)) {
		t.Fatal("prepare mutated the shared request body used for failover targets")
	}
	// Allocation counters are process-wide and the encoder's buffer pool may be
	// cold; both only add bytes, so the minimum of several samples bounds
	// prepare's own steady-state cost.
	allocated := uint64(1<<63 - 1)
	for i := 0; i < 9; i++ {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, _, _ = protocol.Prepare(body, target.Source.Kind, target.Model, "codex", target.UpstreamProtocol("codex"))
		runtime.ReadMemStats(&after)
		allocated = min(allocated, after.TotalAlloc-before.TotalAlloc)
	}
	if allocated > 2*uint64(len(wire)) {
		t.Fatalf("preparing a %d byte native request allocated %d bytes", len(wire), allocated)
	}
}
