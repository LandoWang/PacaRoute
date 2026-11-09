package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

// This sanitized capture guarded the original C# fix. The Go migration must
// preserve the same contract: only message identity may change on this path.
func rollingMessageFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../tests/CopilotBridge.UnitTests/Fixtures/responses-sse-rolling-message-ids.txt")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func messageEvents(t *testing.T, b []byte) []object {
	t.Helper()
	rd := NewSSE(bytes.NewReader(b))
	var events []object
	for {
		_, data, err := rd.Next()
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatal(err)
		}
		if m := obj(data); m != nil {
			events = append(events, m)
		}
	}
}

func assertMessageIdentity(t *testing.T, events []object, index int, want string) {
	t.Helper()
	seen := 0
	for _, event := range events {
		if string(event["output_index"]) == string(jsonBytes(index)) {
			if id, ok := event["item_id"]; ok {
				seen++
				if rawString(id) != want {
					t.Fatalf("%s message %d: id = %q, want %q", rawString(event["type"]), index, rawString(id), want)
				}
			}
			if item := obj(event["item"]); rawString(item["type"]) == "message" {
				seen++
				if rawString(item["id"]) != want {
					t.Fatalf("%s message %d: item id = %q, want %q", rawString(event["type"]), index, rawString(item["id"]), want)
				}
			}
		}
		output := array(obj(event["response"])["output"])
		if index < len(output) && rawString(output[index]["type"]) == "message" {
			seen++
			if rawString(output[index]["id"]) != want {
				t.Fatalf("%s terminal message %d identity changed", rawString(event["type"]), index)
			}
		}
	}
	if seen < 3 {
		t.Fatalf("missing message lifecycle: only %d identity references", seen)
	}
}

func TestCopilotMessageIdentityCapturedLifecycle(t *testing.T) {
	source := rollingMessageFixture(t)
	var out bytes.Buffer
	var usage usage.Usage
	if err := RelayCopilotResponsesSSE(&out, bytes.NewReader(source), &usage); err != nil {
		t.Fatal(err)
	}
	before, after := messageEvents(t, source), messageEvents(t, out.Bytes())
	assertMessageIdentity(t, after, 1, "opaque-message-added")
	if len(before) != len(after) {
		t.Fatal("event count changed")
	}
	// Restore only the permitted identity paths, then compare every JSON value.
	for i, old := range before {
		got := after[i]
		if string(old["output_index"]) == "1" {
			if id, ok := old["item_id"]; ok {
				got["item_id"] = id
			}
			if item := obj(old["item"]); rawString(item["type"]) == "message" {
				updated := obj(got["item"])
				updated["id"] = item["id"]
				got["item"] = jsonBytes(updated)
			}
		}
		if rawString(old["type"]) == "response.completed" {
			response := obj(got["response"])
			output := array(response["output"])
			output[1]["id"] = array(obj(old["response"])["output"])[1]["id"]
			response["output"] = jsonBytes(output)
			got["response"] = jsonBytes(response)
		}
		// Decode recursively to ignore JSON whitespace without losing number precision.
		if !equalJSON(jsonBytes(old), jsonBytes(got)) {
			t.Fatalf("undeclared change to event %d (%s)", i, rawString(old["type"]))
		}
	}
}

func TestCopilotMessageIdentityStableAndOtherProtocolsStayByteExact(t *testing.T) {
	source := rollingMessageFixture(t)
	for _, id := range []string{"opaque-content-added", "opaque-text-delta-one", "opaque-annotation-added", "opaque-text-delta-two", "opaque-text-done", "opaque-content-done", "opaque-message-done", "opaque-future-message-event", "opaque-message-terminal"} {
		source = bytes.ReplaceAll(source, []byte(id), []byte("opaque-message-added"))
	}
	for _, normalize := range []bool{false, true} {
		var out bytes.Buffer
		var u usage.Usage
		var err error
		if normalize {
			err = RelayCopilotResponsesSSE(&out, bytes.NewReader(source), &u)
		} else {
			err = RelaySSE(&out, bytes.NewReader(source), &u, "responses")
		}
		if err != nil || !bytes.Equal(out.Bytes(), source) {
			t.Fatalf("conforming stream changed (normalize=%t): %v", normalize, err)
		}
	}
	var out bytes.Buffer
	var u usage.Usage
	original := rollingMessageFixture(t)
	if err := RelaySSE(&out, bytes.NewReader(original), &u, "responses"); err != nil || !bytes.Equal(out.Bytes(), original) {
		t.Fatal("ordinary passthrough must not normalize provider identities")
	}
}

func equalJSON(a, b []byte) bool {
	var left, right any
	da, db := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	da.UseNumber()
	db.UseNumber()
	return da.Decode(&left) == nil && db.Decode(&right) == nil && reflect.DeepEqual(left, right)
}

func TestCopilotMessageIdentityTerminals(t *testing.T) {
	for _, terminal := range []string{"response.completed", "response.incomplete", "response.failed"} {
		t.Run(terminal, func(t *testing.T) {
			source := bytes.ReplaceAll(rollingMessageFixture(t), []byte("response.completed"), []byte(terminal))
			var out bytes.Buffer
			var u usage.Usage
			err := RelayCopilotResponsesSSE(&out, bytes.NewReader(source), &u)
			var failed *UpstreamStreamError
			if terminal == "response.failed" {
				if !errors.As(err, &failed) || !failed.Forwarded {
					t.Fatalf("upstream failure lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			assertMessageIdentity(t, messageEvents(t, out.Bytes()), 1, "opaque-message-added")
		})
	}
}

func TestCopilotMessageIdentityMultipleMessagesAndTools(t *testing.T) {
	source := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"first","phase":"commentary"}}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"tool-added","call_id":"call-1"}}`,
		`data: {"type":"response.output_item.added","output_index":2,"item":{"type":"message","id":"second","phase":"final_answer"}}`,
		`data: {"type":"response.output_text.delta","output_index":2,"item_id":"second-delta","delta":"second"}`,
		`data: {"type":"response.output_text.delta","output_index":0,"item_id":"first-delta","delta":"first"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":1,"item_id":"tool-delta","delta":"{}"}`,
		`data: {"type":"response.output_item.done","output_index":2,"item":{"type":"message","id":"second-done","phase":"final_answer"}}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"first-done","phase":"commentary"}}`,
		`data: {"type":"response.completed","response":{"id":"resp-original","output":[{"type":"message","id":"first-terminal"},{"type":"function_call","id":"tool-terminal","call_id":"call-1"},{"type":"message","id":"second-terminal"}]}}`,
	}, "\n\n") + "\n\n"
	var out bytes.Buffer
	var u usage.Usage
	if err := RelayCopilotResponsesSSE(&out, strings.NewReader(source), &u); err != nil {
		t.Fatal(err)
	}
	events := messageEvents(t, out.Bytes())
	assertMessageIdentity(t, events, 0, "first")
	assertMessageIdentity(t, events, 2, "second")
	if !bytes.Contains(out.Bytes(), []byte(`"item_id":"tool-delta"`)) || !bytes.Contains(out.Bytes(), []byte(`"id":"tool-terminal"`)) || !bytes.Contains(out.Bytes(), []byte(`"id":"resp-original"`)) {
		t.Fatal("non-message identities changed")
	}
}

func TestCopilotMessageIdentityMissingIDAndFraming(t *testing.T) {
	source := "event: response.created\r\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-missing\"}}\r\n\r\n" +
		"id: cursor-1\r\nretry: 1000\r\n: keep-comment\r\nevent: response.output_item.added\r\ndata: {\"type\":\"response.output_item.added\",\r\ndata: \"output_index\":0,\"item\":{\"type\":\"message\",\"future\":9007199254740993}}\r\n\r\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"item_id\":\"rotating\",\"delta\":\"hello\"}\r\n\r\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"id\":\"terminal\"}]}}\r\n\r\n"
	var previous []byte
	for range 2 {
		var out bytes.Buffer
		var u usage.Usage
		if err := RelayCopilotResponsesSSE(&out, strings.NewReader(source), &u); err != nil {
			t.Fatal(err)
		}
		events := messageEvents(t, out.Bytes())
		id := rawString(obj(events[1]["item"])["id"])
		if !strings.HasPrefix(id, "item_") {
			t.Fatalf("missing fallback identity: %q", id)
		}
		assertMessageIdentity(t, events, 0, id)
		if !bytes.Contains(out.Bytes(), []byte("id: cursor-1\r\nretry: 1000\r\n: keep-comment\r\nevent: response.output_item.added\r\n")) || !bytes.Contains(out.Bytes(), []byte("9007199254740993")) {
			t.Fatal("SSE metadata or numeric precision changed")
		}
		if previous != nil && !bytes.Equal(previous, out.Bytes()) {
			t.Fatal("identity is not deterministic on replay")
		}
		previous = bytes.Clone(out.Bytes())
	}
}
