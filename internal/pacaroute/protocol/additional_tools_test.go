package protocol

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Replay the committed desktop registration, including its grammar custom tool.
func TestDesktopAdditionalToolsCrossProtocols(t *testing.T) {
	capture, err := os.ReadFile("../../../tests/CopilotBridge.UnitTests/Fixtures/codex-additional-tools-req.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"messages", "chat", "responses"} {
		t.Run(target, func(t *testing.T) {
			body := obj(obj(capture)["body"])
			var wire []byte
			var tr *Translation
			if target == "chat" {
				wire, tr, err = PrepareChat(body, "gemini-4-argon", domain.AgentCodex, replayFixture())
			} else {
				wire, tr, err = Prepare(body, domain.SourceCopilot, "coding", domain.AgentCodex, target)
			}
			if err != nil {
				t.Fatal(err)
			}
			if target == "responses" {
				if tr != nil || !equalJSON(obj(wire)["input"], body["input"]) {
					t.Fatal("native registration was rewritten")
				}
				return
			}
			var expected int
			var count func([]object)
			count = func(tools []object) {
				for _, tool := range tools {
					if rawString(tool["type"]) == "namespace" {
						count(array(tool["tools"]))
					} else {
						expected++
					}
				}
			}
			count(array(body["tools"]))
			for _, item := range array(body["input"]) {
				if rawString(item["type"]) == "additional_tools" {
					count(array(item["tools"]))
				}
			}
			if len(array(obj(wire)["tools"])) != expected || len(tr.ToolNames) != expected {
				t.Fatalf("lost registrations: %s", wire)
			}
			if tr.ToolNames["exec"] != (ToolIdentity{Name: "exec", Custom: true}) {
				t.Fatalf("lost custom exec identity: %+v", tr.ToolNames)
			}
			if bytes.Contains(obj(wire)["messages"], []byte("additional_tools")) {
				t.Fatal("registration became conversation text")
			}
		})
	}
}

func TestAdditionalToolsNamespacedCustomRoundtrip(t *testing.T) {
	for _, target := range []string{"messages", "chat"} {
		t.Run(target, func(t *testing.T) {
			body := obj([]byte(`{"model":"coding","tools":[{"type":"function","name":"other","parameters":{"type":"object"}}],"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec","description":"Execute JavaScript","format":{"type":"grammar","syntax":"lark","definition":"start: /[\\s\\S]+/"}},{"type":"function","name":"wait","parameters":{"type":"object","properties":{"cell_id":{"type":"string"}},"required":["cell_id"]}}]}]},{"role":"user","content":"Read the file"}]}`))
			var wire []byte
			var tr *Translation
			var err error
			if target == "chat" {
				wire, tr, err = PrepareChat(body, "gemini-4-argon", domain.AgentCodex, replayFixture())
			} else {
				wire, tr, err = Prepare(body, domain.SourceCopilot, "claude-opus-5.5", domain.AgentCodex, "messages")
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(array(obj(wire)["tools"])) != 3 || tr.ToolNames["functions__exec"] != (ToolIdentity{Name: "exec", Namespace: "functions", Custom: true}) {
				t.Fatalf("lost registrations: %s", wire)
			}
			code := "const result = await tools.exec_command({cmd: 'pwd'});\ntext(result);"
			block := object{"type": jsonBytes("tool_use"), "id": jsonBytes("call_exec"), "name": jsonBytes("functions__exec"), "input": jsonBytes(map[string]string{"input": code})}
			var response []byte
			if target == "chat" {
				response, err = tr.ChatJSON(jsonBytes(map[string]any{"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"tool_calls": []any{map[string]any{"id": "call_exec", "type": "function", "function": map[string]any{"name": "functions__exec", "arguments": string(block["input"])}}}}}}}))
			} else {
				response, err = tr.MessagesJSON(object{"content": jsonBytes([]object{block})})
			}
			if err != nil {
				t.Fatal(err)
			}
			var call object
			for _, item := range array(obj(response)["output"]) {
				if rawString(item["type"]) == "custom_tool_call" {
					call = item
				}
			}
			if rawString(call["input"]) != code || rawString(call["namespace"]) != "functions" || rawString(call["name"]) != "exec" || rawString(call["call_id"]) != "call_exec" {
				t.Fatalf("bad custom dispatch: %s", response)
			}
			items := array(body["input"])
			items = append(items, call, object{"type": jsonBytes("custom_tool_call_output"), "call_id": jsonBytes("call_exec"), "output": jsonBytes("/workspace")})
			body["input"] = jsonBytes(items)
			if target == "chat" {
				wire, _, err = PrepareChat(body, "gemini-4-argon", domain.AgentCodex, replayFixture())
			} else {
				wire, _, err = Prepare(body, domain.SourceCopilot, "claude-opus-5.5", domain.AgentCodex, "messages")
			}
			if err != nil || !bytes.Contains(wire, []byte("/workspace")) {
				t.Fatalf("tool result failed: %v %s", err, wire)
			}
		})
	}
}

func TestAdditionalToolsRejectsMalformedAndCollidingRegistrations(t *testing.T) {
	for _, tools := range []string{`null`, `{}`, `"tools"`, `[null]`, `[{"type":"namespace","name":"functions","tools":{}}]`, `[{"type":"function","name":"existing"}]`, `[{"type":"web_search"}]`} {
		body := obj([]byte(`{"tools":[{"type":"function","name":"existing"}],"input":[{"type":"additional_tools","tools":` + tools + `}]}`))
		if _, _, err := Prepare(body, domain.SourceCopilot, "claude-opus-5.5", domain.AgentCodex, "messages"); err == nil {
			t.Fatalf("accepted invalid registration %s", tools)
		}
	}
	// An explicitly empty declaration is valid.
	tr := &Translation{ToolNames: map[string]ToolIdentity{}}
	if _, err := ResponsesToMessages(obj(json.RawMessage(`{"input":[{"type":"additional_tools","tools":[]},{"role":"user","content":"hi"}]}`)), tr); err != nil {
		t.Fatal(err)
	}
}
