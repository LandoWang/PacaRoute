package protocol

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

func replayFixture() ChatReplay {
	return ChatReplay{Seal: func(b []byte) (string, error) { return base64.RawURLEncoding.EncodeToString(b), nil }, Open: func(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }}
}

const chatToolFixture = `{"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"Checking.","reasoning_opaque":"opaque-not-text","tool_calls":[{"id":"call_1","type":"function","function":{"name":"functions__exec","arguments":"{\"cmd\":\"pwd\"}"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":30}}}`

func TestChatResponsesToolAndOpaqueRoundtrip(t *testing.T) {
	body := obj([]byte(`{"model":"coding","instructions":"Keep scope","input":"Inspect the path","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"exec","parameters":{"type":"object","properties":{"cmd":{"type":"string"},"cache_control":{"type":"string"}}}}]}],"reasoning":{"effort":"low"}}`))
	wire, tr, err := PrepareChat(body, "gemini-4-argon", domain.AgentCodex, replayFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"cache_control"`)) || !bytes.Contains(wire, []byte(`"reasoning_effort":"low"`)) {
		t.Fatalf("lost schema or effort: %s", wire)
	}
	response, err := tr.ChatJSON([]byte(chatToolFixture))
	if err != nil {
		t.Fatal(err)
	}
	out := obj(response)
	items := array(out["output"])
	found := false
	for _, item := range items {
		if rawString(item["type"]) == "function_call" {
			found = rawString(item["namespace"]) == "functions" && rawString(item["name"]) == "exec" && rawString(item["call_id"]) == "call_1"
		}
	}
	if !found {
		t.Fatalf("lost tool identity: %s", response)
	}
	if string(obj(out["usage"])["input_tokens"]) != "100" {
		t.Fatalf("cache counted twice: %s", response)
	}
	input := []object{{"role": jsonBytes("user"), "content": jsonBytes("Inspect the path")}}
	input = append(input, items...)
	input = append(input, object{"type": jsonBytes("function_call_output"), "call_id": jsonBytes("call_1"), "output": jsonBytes("/tmp")})
	body["input"] = jsonBytes(input)
	wire, _, err = PrepareChat(body, "gemini-4-argon", domain.AgentCodex, replayFixture())
	if err != nil {
		t.Fatal(err)
	}
	msgs := array(obj(wire)["messages"])
	found = false
	for _, m := range msgs {
		if rawString(m["role"]) == "assistant" {
			found = rawString(m["reasoning_opaque"]) == "opaque-not-text" && len(array(m["tool_calls"])) == 1
		}
	}
	if !found {
		t.Fatalf("lost opaque state or tool: %s", wire)
	}
}
func TestChatMessagesOpaqueAndUsageRoundtrip(t *testing.T) {
	body := obj([]byte(`{"model":"coding","messages":[{"role":"user","content":"inspect"}],"tools":[{"name":"functions__exec","input_schema":{"type":"object"}}]}`))
	_, tr, err := PrepareChat(body, "gemini-4-argon", domain.AgentClaudeCode, replayFixture())
	if err != nil {
		t.Fatal(err)
	}
	response, err := tr.ChatJSON([]byte(chatToolFixture))
	if err != nil {
		t.Fatal(err)
	}
	m := obj(response)
	if string(obj(m["usage"])["input_tokens"]) != "70" || string(obj(m["usage"])["cache_read_input_tokens"]) != "30" {
		t.Fatalf("wrong Messages usage: %s", response)
	}
	body["messages"] = jsonBytes([]object{{"role": jsonBytes("user"), "content": jsonBytes("inspect")}, {"role": jsonBytes("assistant"), "content": m["content"]}, {"role": jsonBytes("user"), "content": jsonBytes([]object{{"type": jsonBytes("tool_result"), "tool_use_id": jsonBytes("call_1"), "content": jsonBytes("/tmp")}})}})
	wire, _, err := PrepareChat(body, "gemini-4-argon", domain.AgentClaudeCode, replayFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"reasoning_opaque":"opaque-not-text"`)) {
		t.Fatalf("lost opaque state: %s", wire)
	}
}
func TestChatStreamRequiresTerminalBeforeExecutingTools(t *testing.T) {
	chunks := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"exec","arguments":"{\"cmd\":"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"pwd\"}"}}],"reasoning_opaque":"opaque"},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":100,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":30}}}` + "\n\n"
	for _, from := range []string{"responses", "messages"} {
		for _, done := range []bool{false, true} {
			t.Run(from+map[bool]string{true: "-done", false: "-truncated"}[done], func(t *testing.T) {
				tr := &Translation{From: from, To: "chat", Model: "coding", SealReplay: replayFixture().Seal}
				u := usage.Usage{}
				var out bytes.Buffer
				input := chunks
				if done {
					input += "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":110,\"completion_tokens\":8,\"prompt_tokens_details\":{\"cached_tokens\":40}}}\n\ndata: [DONE]\n\n"
				}
				err := tr.StreamChat(&out, strings.NewReader(input), &u)
				if !done {
					if err == nil || strings.Contains(out.String(), "call_1") {
						t.Fatalf("truncated tool executed: %v %s", err, &out)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if *u.Input != 110 || *u.Output != 8 || *u.CacheRead != 40 {
					t.Fatalf("not final usage: %+v", u)
				}
				if !strings.Contains(out.String(), "call_1") || !strings.Contains(out.String(), map[string]string{"responses": "response.completed", "messages": "message_stop"}[from]) {
					t.Fatalf("missing tool/terminal: %s", &out)
				}
			})
		}
	}
}
func TestChatRejectsForeignReferencesAndUnrepresentableTools(t *testing.T) {
	for _, raw := range []string{
		`{"model":"x","previous_response_id":"resp_old","input":"continue"}`,
		`{"model":"x","input":[{"type":"reasoning","encrypted_content":"not-our-token"}]}`,
		`{"model":"x","input":"hi","tools":[{"type":"web_search"}]}`,
	} {
		if _, _, err := PrepareChat(obj([]byte(raw)), "gemini-4-argon", domain.AgentCodex, replayFixture()); err == nil {
			t.Fatalf("accepted unsupported input: %s", raw)
		}
	}
}
func TestChatCustomToolAndLengthTerminal(t *testing.T) {
	body := obj([]byte(`{"model":"x","input":"edit","tools":[{"type":"custom","name":"apply_patch","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}]}`))
	_, tr, err := PrepareChat(body, "gemini-4-argon", domain.AgentCodex, replayFixture())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"c1","function":{"name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\\n*** End Patch\"}"}}]}}]}`)
	response, err := tr.ChatJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	item := array(obj(response)["output"])[0]
	if rawString(item["type"]) != "custom_tool_call" || !strings.HasPrefix(rawString(item["id"]), "ctc_") || rawString(item["input"]) != "*** Begin Patch\n*** End Patch" {
		t.Fatalf("bad custom tool: %s", response)
	}
	data = bytes.Replace(data, []byte(`"tool_calls","message"`), []byte(`"length","message"`), 1)
	response, err = tr.ChatJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if rawString(obj(response)["status"]) != "incomplete" || len(array(obj(response)["output"])) != 0 {
		t.Fatalf("executed length-truncated tool: %s", response)
	}
	if string(obj(response)["usage"]) != "null" {
		t.Fatal("fabricated missing usage")
	}
}
func TestChatMalformedArgumentsAndErrorFail(t *testing.T) {
	tr := &Translation{From: "responses", To: "chat", Model: "x", SealReplay: replayFixture().Seal}
	for _, data := range []string{`{"choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"c1","function":{"name":"exec","arguments":"{"}}]}}]}`, `{"choices":[{"finish_reason":"content_filter","message":{"content":""}}]}`, `{"error":{"message":"overloaded"}}`} {
		if _, err := tr.ChatJSON([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestChatPreservesMidConversationSystemAndParallelCalls(t *testing.T) {
	body := obj([]byte(`{"model":"x","system":"global","messages":[{"role":"user","content":"task"},{"role":"system","content":"new constraint"},{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"first","input":{}},{"type":"tool_use","id":"b","name":"second","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"one"},{"type":"tool_result","tool_use_id":"b","content":"two"}]}]}`))
	wire, _, err := PrepareChat(body, "gemini-4-argon", domain.AgentClaudeCode, replayFixture())
	if err != nil {
		t.Fatal(err)
	}
	messages := array(obj(wire)["messages"])
	if len(messages) != 6 || rawString(messages[2]["role"]) != "system" || rawString(messages[2]["content"]) != "new constraint" || len(array(messages[3]["tool_calls"])) != 2 || rawString(messages[4]["tool_call_id"]) != "a" || rawString(messages[5]["tool_call_id"]) != "b" {
		t.Fatalf("lost ordering/pairing: %s", wire)
	}
}
func TestChatStreamParallelFragmentedArguments(t *testing.T) {
	tr := &Translation{From: "responses", To: "chat", Model: "x", SealReplay: replayFixture().Seal}
	stream := `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"second","arguments":"{"}},{"index":0,"id":"a","function":{"name":"first","arguments":"{}"}}]}}]}` + "\n\n" + `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	var out bytes.Buffer
	u := usage.Usage{}
	if err := tr.StreamChat(&out, strings.NewReader(stream), &u); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"call_id":"a"`) || !strings.Contains(out.String(), `"call_id":"b"`) {
		t.Fatalf("lost parallel tool: %s", &out)
	}
	if u.Input != nil || u.Output != nil {
		t.Fatal("invented usage")
	}
}

func TestChatClaudeFinalAnswerFollowsOpaqueState(t *testing.T) {
	tr := &Translation{From: "messages", To: "chat", Model: "x", SealReplay: replayFixture().Seal}
	stream := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_opaque\":\"opaque\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	var out bytes.Buffer
	u := usage.Usage{}
	if err := tr.StreamChat(&out, strings.NewReader(stream), &u); err != nil {
		t.Fatal(err)
	}
	state := strings.Index(out.String(), `"type":"redacted_thinking"`)
	answer := strings.Index(out.String(), `"type":"text"`)
	if state < 0 || answer < state || !strings.Contains(out.String(), `"text":"answer"`) {
		t.Fatalf("final answer hidden by opaque block: %s", &out)
	}
}
