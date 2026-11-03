package pacaroute

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

func maxTokensSSE(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: " + e + "\n\n")
	}
	return b.String()
}

func truncatedToolTurn(stopReason string) string {
	return maxTokensSSE(
		`{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"apply_patch","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"patch\":\"*** Begin"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"`+stopReason+`"},"usage":{"output_tokens":16384}}`,
		`{"type":"message_stop"}`,
	)
}

// A Messages turn that stops on max_tokens inside a tool_use is a well-formed,
// length-limited completion, not an upstream failure: the source is not cooled,
// the session stays on its bound source and Codex receives response.incomplete.
func TestTranslatedMaxTokensToolUseIsNotUpstreamFailure(t *testing.T) {
	s, bound, failover := streamPool(t, truncatedToolTurn("max_tokens"), true)
	c := cloneConfig(s.Store.Snapshot())
	for i := range c.Sources {
		if c.Sources[i].ID == "test" {
			c.Sources[i].Kind = "anthropic" // the session-bound member; "healthy" stays Responses
		}
	}
	saveTest(t, s.Store, c)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, streamRequest(t.Context(), fullHistoryRequest))
	records := s.Activity.Recent(false)
	if len(records) != 1 || records[0].Status != 200 || records[0].Error != "" {
		t.Fatalf("max_tokens turn recorded as failure: %+v", records)
	}
	if !strings.Contains(w.Body.String(), `"type":"response.incomplete"`) {
		t.Fatalf("missing response.incomplete terminal:\n%s", w.Body.String())
	}
	if cooldowns := s.Router.Health(s.Store.Snapshot()).Cooldowns; len(cooldowns) != 0 {
		t.Fatalf("length limit cooled the source: %+v", cooldowns)
	}
	s.Handler().ServeHTTP(httptest.NewRecorder(), streamRequest(t.Context(), fullHistoryRequest))
	if bound.Load() != 2 || failover.Load() != 0 {
		t.Fatalf("next request left the bound source: bound=%d failover=%d", bound.Load(), failover.Load())
	}
}

// Codex executes a function_call when it receives output_item.done. A call whose
// arguments were cut off by max_tokens must therefore never be completed; it is
// reported as an incomplete item inside response.incomplete (max_output_tokens).
func TestTranslatedTruncatedToolCallIsNotCompleted(t *testing.T) {
	tr := &translation{from: "responses", to: "messages", model: "coding", toolNames: map[string]toolIdentity{}}
	var out bytes.Buffer
	if err := tr.streamMessages(&out, strings.NewReader(truncatedToolTurn("max_tokens")), &usage.Usage{}); err != nil {
		t.Fatalf("max_tokens turn failed: %v", err)
	}
	body := out.String()
	for _, want := range []string{`"type":"response.output_item.added"`, `"type":"response.function_call_arguments.delta"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{`"type":"response.output_item.done"`, `"type":"response.function_call_arguments.done"`, `"type":"response.completed"`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("truncated call emitted %s:\n%s", unwanted, body)
		}
	}
	frames := strings.Split(strings.TrimSpace(body), "\n\n")
	final := frames[len(frames)-1]
	_, data, _ := newSSE(strings.NewReader(final + "\n\n")).next()
	response := obj(obj(data)["response"])
	if rawString(obj(data)["type"]) != "response.incomplete" || rawString(obj(response["incomplete_details"])["reason"]) != "max_output_tokens" {
		t.Fatalf("terminal is not response.incomplete(max_output_tokens): %s", final)
	}
	items := array(response["output"])
	if len(items) != 1 || rawString(items[0]["type"]) != "function_call" || rawString(items[0]["status"]) != "incomplete" {
		t.Fatalf("truncated call not reported as an incomplete item: %s", response["output"])
	}
}

// Invalid tool arguments without a max_tokens stop remain an upstream stream error,
// and complete tool calls are still completed.
func TestTranslatedToolCallStopReasonsUnchanged(t *testing.T) {
	tr := &translation{from: "responses", to: "messages", model: "coding", toolNames: map[string]toolIdentity{}}
	var out bytes.Buffer
	for _, reason := range []string{"end_turn", "tool_use"} {
		out.Reset()
		if err := tr.streamMessages(&out, strings.NewReader(truncatedToolTurn(reason)), &usage.Usage{}); err == nil || !strings.Contains(err.Error(), "invalid streamed tool arguments") {
			t.Fatalf("stop_reason %s with invalid arguments: err=%v", reason, err)
		}
		if strings.Contains(out.String(), `"type":"response.output_item.done"`) {
			t.Fatalf("stop_reason %s completed an invalid call", reason)
		}
	}
	out.Reset()
	complete := strings.Replace(truncatedToolTurn("tool_use"), `{\"patch\":\"*** Begin`, `{\"patch\":\"*** Begin\"}`, 1)
	if err := tr.streamMessages(&out, strings.NewReader(complete), &usage.Usage{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"type":"response.output_item.done"`) || !strings.Contains(out.String(), `"type":"response.completed"`) {
		t.Fatalf("complete tool call not completed:\n%s", out.String())
	}
}

// The non-streaming translation of the same truncation is not an error either.
func TestTranslatedNonStreamMaxTokensToolUse(t *testing.T) {
	tr := &translation{from: "responses", to: "messages", model: "coding", toolNames: map[string]toolIdentity{}}
	body, err := tr.responseJSON([]byte(`{"type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"apply_patch","input":{}}],"stop_reason":"max_tokens","usage":{"input_tokens":10,"output_tokens":16384}}`))
	if err != nil {
		t.Fatalf("non-stream max_tokens tool_use failed: %v", err)
	}
	m := obj(body)
	if rawString(m["status"]) != "incomplete" || rawString(obj(m["incomplete_details"])["reason"]) != "max_output_tokens" {
		t.Fatalf("non-stream truncation not incomplete: %s", body)
	}
}
