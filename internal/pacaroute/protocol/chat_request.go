package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// ChatReplay carries opaque upstream state without treating it as model text.
// The gateway supplies authenticated encryption and binds it to the route/session.
type ChatReplay struct {
	Seal func([]byte) (string, error)
	Open func(string) ([]byte, error)
}

func PrepareChat(body object, model string, agent domain.AgentProtocol, replay ChatReplay) ([]byte, *Translation, error) {
	tr := &Translation{From: domain.IncomingProtocol(agent), To: "chat", Model: rawString(body["model"]), ToolNames: map[string]ToolIdentity{}, SealReplay: replay.Seal, OpenReplay: replay.Open}
	m := body
	if tr.From == "responses" {
		var err error
		m, err = ResponsesToMessages(body, tr)
		if err != nil {
			return nil, nil, err
		}
	}
	out := object{"model": jsonBytes(model), "stream": body["stream"]}
	if string(body["stream"]) == "true" {
		out["stream_options"] = jsonBytes(map[string]bool{"include_usage": true})
	}
	messages := []object{}
	if system := textContent(m["system"]); system != "" {
		messages = append(messages, object{"role": jsonBytes("system"), "content": jsonBytes(system)})
	}
	for _, msg := range array(m["messages"]) {
		role := rawString(msg["role"])
		if role != "user" && role != "assistant" && role != "system" && role != "developer" {
			return nil, nil, fmt.Errorf("unsupported Chat role %q", role)
		}
		blocks := array(msg["content"])
		if text := rawString(msg["content"]); text != "" {
			blocks = []object{{"type": jsonBytes("text"), "text": jsonBytes(text)}}
		}
		current := object{"role": jsonBytes(role)}
		content := []object{}
		calls := []object{}
		results := []object{}
		for _, b := range blocks {
			switch rawString(b["type"]) {
			case "text":
				content = append(content, object{"type": jsonBytes("text"), "text": b["text"]})
			case "redacted_thinking":
				if err := tr.restoreChatState(current, rawString(b["data"])); err != nil {
					return nil, nil, err
				}
			case "thinking":
				return nil, nil, errors.New("foreign thinking/signature cannot be replayed to Copilot Chat")
			case "image":
				src := obj(b["source"])
				url := rawString(src["url"])
				if rawString(src["type"]) == "base64" {
					url = "data:" + rawString(src["media_type"]) + ";base64," + rawString(src["data"])
				}
				if url == "" {
					return nil, nil, errors.New("image requires a URL or base64 data")
				}
				content = append(content, object{"type": jsonBytes("image_url"), "image_url": jsonBytes(map[string]string{"url": url})})
			case "tool_use":
				if role != "assistant" || rawString(b["id"]) == "" || rawString(b["name"]) == "" {
					return nil, nil, errors.New("invalid Chat tool call")
				}
				if !chatObject(b["input"]) {
					return nil, nil, errors.New("tool input must be an object")
				}
				calls = append(calls, object{"id": b["id"], "type": jsonBytes("function"), "function": jsonBytes(object{"name": b["name"], "arguments": jsonBytes(string(b["input"]))})})
			case "tool_result":
				if role != "user" || rawString(b["tool_use_id"]) == "" {
					return nil, nil, errors.New("invalid Chat tool result")
				}
				result, err := chatToolResult(b["content"])
				if err != nil {
					return nil, nil, err
				}
				results = append(results, object{"role": jsonBytes("tool"), "tool_call_id": b["tool_use_id"], "content": result})
			default:
				return nil, nil, fmt.Errorf("content %q cannot cross to Chat Completions", rawString(b["type"]))
			}
		}
		// Tool results precede any new user instruction in the same Messages turn.
		messages = append(messages, results...)
		if len(content) > 0 || len(calls) > 0 || current["reasoning_opaque"] != nil {
			current["content"] = jsonBytes(content)
			if len(content) == 1 && rawString(content[0]["type"]) == "text" {
				current["content"] = content[0]["text"]
			}
			if len(content) == 0 {
				current["content"] = jsonBytes("")
			}
			if len(calls) > 0 {
				current["tool_calls"] = jsonBytes(calls)
			}
			messages = append(messages, current)
		}
	}
	out["messages"] = jsonBytes(messages)
	tools := []object{}
	for _, t := range array(m["tools"]) {
		if t["type"] != nil && rawString(t["type"]) != "custom" {
			return nil, nil, errors.New("server-side tools cannot cross to Chat Completions")
		}
		f := object{"name": t["name"], "parameters": t["input_schema"]}
		if t["description"] != nil {
			f["description"] = t["description"]
		}
		if rawString(f["name"]) == "" {
			return nil, nil, errors.New("tool requires a name")
		}
		tools = append(tools, object{"type": jsonBytes("function"), "function": jsonBytes(f)})
	}
	if len(tools) > 0 {
		out["tools"] = jsonBytes(tools)
	}
	choice := obj(m["tool_choice"])
	switch rawString(choice["type"]) {
	case "auto", "none":
		out["tool_choice"] = choice["type"]
	case "any":
		out["tool_choice"] = jsonBytes("required")
	case "tool":
		out["tool_choice"] = jsonBytes(map[string]any{"type": "function", "function": map[string]string{"name": rawString(choice["name"])}})
	}
	if tr.From == "responses" {
		if c := obj(body["tool_choice"]); rawString(c["type"]) == "function" {
			out["tool_choice"] = jsonBytes(map[string]any{"type": "function", "function": map[string]string{"name": rawString(c["name"])}})
		}
		if body["parallel_tool_calls"] != nil {
			out["parallel_tool_calls"] = body["parallel_tool_calls"]
		}
		if body["max_output_tokens"] != nil {
			out["max_tokens"] = body["max_output_tokens"]
		}
		if effort := obj(body["reasoning"])["effort"]; effort != nil {
			out["reasoning_effort"] = effort
		}
	} else {
		if m["max_tokens"] != nil {
			out["max_tokens"] = m["max_tokens"]
		}
		if effort := obj(m["output_config"])["effort"]; effort != nil {
			out["reasoning_effort"] = effort
		}
		if string(choice["disable_parallel_tool_use"]) == "true" {
			out["parallel_tool_calls"] = jsonBytes(false)
		}
	}
	for _, key := range []string{"temperature", "top_p"} {
		if body[key] != nil {
			out[key] = body[key]
		}
	}
	if body["stop_sequences"] != nil {
		out["stop"] = body["stop_sequences"]
	}
	data, err := json.Marshal(out)
	return data, tr, err
}

func (tr *Translation) restoreChatState(m object, token string) error {
	if tr.OpenReplay == nil {
		return errors.New("Chat replay is unavailable")
	}
	b, err := tr.OpenReplay(token)
	if err != nil {
		return err
	}
	state := obj(b)
	if state == nil {
		return errors.New("invalid Chat replay")
	}
	for k, v := range state {
		if k == "role" || k == "content" || k == "tool_calls" {
			return errors.New("invalid Chat replay field")
		}
		m[k] = v
	}
	return nil
}
func chatToolResult(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return raw, nil
	}
	parts := array(raw)
	if parts == nil {
		return nil, errors.New("tool result must be text or a content array")
	}
	texts := []string{}
	for _, b := range parts {
		if rawString(b["type"]) != "text" {
			return nil, errors.New("non-text tool results are not supported by Copilot Chat")
		}
		texts = append(texts, rawString(b["text"]))
	}
	return jsonBytes(strings.Join(texts, "\n")), nil
}

func chatObject(b []byte) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(b, &v) == nil && v != nil
}
