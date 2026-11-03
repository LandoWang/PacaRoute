package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func textContent(b json.RawMessage) string {
	if s := rawString(b); s != "" {
		return s
	}
	parts := []string{}
	for _, v := range array(b) {
		if t := rawString(v["text"]); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

// Tool output may contain screenshots. Preserve their order and reject unsupported
// blocks instead of silently turning a multimodal tool result into empty text.
func ToolOutput(b json.RawMessage, to string) (json.RawMessage, error) {
	var text string
	if json.Unmarshal(b, &text) == nil {
		return jsonBytes(text), nil
	}
	var blocks []object
	if json.Unmarshal(b, &blocks) != nil {
		return nil, errors.New("tool output must be text or a content array")
	}
	result := []object{}
	for _, block := range blocks {
		typ := rawString(block["type"])
		if typ == "text" || typ == "input_text" || typ == "output_text" {
			kind := "text"
			if to == "responses" {
				kind = "input_text"
			}
			result = append(result, object{"type": jsonBytes(kind), "text": block["text"]})
			continue
		}
		if to == "responses" && typ == "image" {
			source := obj(block["source"])
			url := rawString(source["url"])
			if rawString(source["type"]) == "base64" {
				url = "data:" + rawString(source["media_type"]) + ";base64," + rawString(source["data"])
			} else if rawString(source["type"]) != "url" {
				return nil, errors.New("unsupported tool image source")
			}
			result = append(result, object{"type": jsonBytes("input_image"), "image_url": jsonBytes(url)})
			continue
		}
		if to == "messages" && typ == "input_image" {
			url := rawString(block["image_url"])
			if url == "" {
				return nil, errors.New("tool image requires an image_url")
			}
			source := map[string]string{"type": "url", "url": url}
			if strings.HasPrefix(url, "data:") {
				media, data, ok := strings.Cut(strings.TrimPrefix(url, "data:"), ";base64,")
				if !ok {
					return nil, errors.New("unsupported tool data image")
				}
				source = map[string]string{"type": "base64", "media_type": media, "data": data}
			}
			result = append(result, object{"type": jsonBytes("image"), "source": jsonBytes(source)})
			continue
		}
		return nil, fmt.Errorf("tool output block %q cannot cross protocols", typ)
	}
	return jsonBytes(result), nil
}

type ToolIdentity struct {
	Name      string
	Namespace string
	Custom    bool
}
type Translation struct {
	From, To, Model string
	ToolNames       map[string]ToolIdentity
}

func MessagesToResponses(m object, tr *Translation) (object, error) {
	out := object{"model": m["model"], "stream": m["stream"], "store": jsonBytes(false)}
	if s := textContent(m["system"]); s != "" {
		out["instructions"] = jsonBytes(s)
	}
	input := []object{}
	for _, msg := range array(m["messages"]) {
		role := rawString(msg["role"])
		if text := rawString(msg["content"]); text != "" {
			input = append(input, object{"role": jsonBytes(role), "content": jsonBytes(text)})
			continue
		}
		content := []object{}
		flush := func() {
			if len(content) > 0 {
				input = append(input, object{"role": jsonBytes(role), "content": jsonBytes(content)})
				content = nil
			}
		}
		for _, b := range array(msg["content"]) {
			switch rawString(b["type"]) {
			case "text":
				typ := "input_text"
				if role == "assistant" {
					typ = "output_text"
				}
				content = append(content, object{"type": jsonBytes(typ), "text": b["text"]})
			case "image":
				src := obj(b["source"])
				imageURL := rawString(src["url"])
				if rawString(src["type"]) == "base64" {
					imageURL = "data:" + rawString(src["media_type"]) + ";base64," + rawString(src["data"])
				}
				content = append(content, object{"type": jsonBytes("input_image"), "image_url": jsonBytes(imageURL)})
			case "tool_use":
				flush()
				input = append(input, object{"type": jsonBytes("function_call"), "call_id": b["id"], "name": b["name"], "arguments": jsonBytes(string(b["input"]))})
			case "tool_result":
				flush()
				output, err := ToolOutput(b["content"], "responses")
				if err != nil {
					return nil, err
				}
				input = append(input, object{"type": jsonBytes("function_call_output"), "call_id": b["tool_use_id"], "output": output})
			case "thinking", "redacted_thinking": // Anthropic signed reasoning cannot be replayed to another provider.
			default:
				return nil, fmt.Errorf("unsupported Messages content block %q on Responses source", rawString(b["type"]))
			}
		}
		flush()
	}
	out["input"] = jsonBytes(input)
	tools := []object{}
	for _, t := range array(m["tools"]) {
		name := rawString(t["name"])
		if name == "" || t["input_schema"] == nil {
			return nil, errors.New("cross-protocol source requires function tools")
		}
		tools = append(tools, object{"type": jsonBytes("function"), "name": t["name"], "description": t["description"], "parameters": t["input_schema"], "strict": jsonBytes(false)})
	}
	if len(tools) > 0 {
		out["tools"] = jsonBytes(tools)
	}
	choice := obj(m["tool_choice"])
	switch rawString(choice["type"]) {
	case "auto":
		out["tool_choice"] = jsonBytes("auto")
	case "any":
		out["tool_choice"] = jsonBytes("required")
	case "none":
		out["tool_choice"] = jsonBytes("none")
	case "tool":
		out["tool_choice"] = jsonBytes(map[string]string{"type": "function", "name": rawString(choice["name"])})
	}
	if m["max_tokens"] != nil {
		out["max_output_tokens"] = m["max_tokens"]
	}
	if cfg := obj(m["output_config"]); cfg["effort"] != nil {
		out["reasoning"] = jsonBytes(object{"effort": cfg["effort"]})
	}
	return out, nil
}
func ResponsesToMessages(m object, tr *Translation) (object, error) {
	out := object{"model": m["model"], "stream": m["stream"], "max_tokens": jsonBytes(16384)}
	if m["max_output_tokens"] != nil {
		out["max_tokens"] = m["max_output_tokens"]
	}
	if m["previous_response_id"] != nil {
		return nil, errors.New("previous_response_id cannot cross provider protocols")
	}
	system := rawString(m["instructions"])
	tools := []object{}
	var addTools func([]object, string) error
	addTools = func(ts []object, ns string) error {
		for _, t := range ts {
			typ := rawString(t["type"])
			if typ == "namespace" {
				if e := addTools(array(t["tools"]), rawString(t["name"])); e != nil {
					return e
				}
				continue
			}
			if typ != "function" && typ != "custom" {
				return fmt.Errorf("tool %q cannot be translated to Messages", typ)
			}
			name := rawString(t["name"])
			upstream := name
			if ns != "" {
				upstream = ns + "__" + name
			}
			if _, exists := tr.ToolNames[upstream]; exists {
				return errors.New("translated tool name collision")
			}
			tr.ToolNames[upstream] = ToolIdentity{name, ns, typ == "custom"}
			schema := t["parameters"]
			if typ == "custom" {
				schema = jsonBytes(map[string]any{"type": "object", "properties": map[string]any{"input": map[string]string{"type": "string", "description": "Exact raw tool input"}}, "required": []string{"input"}})
			}
			if schema == nil {
				schema = jsonBytes(map[string]string{"type": "object"})
			}
			tools = append(tools, object{"name": jsonBytes(upstream), "description": t["description"], "input_schema": schema})
		}
		return nil
	}
	if e := addTools(array(m["tools"]), ""); e != nil {
		return nil, e
	}
	if len(tools) > 0 {
		out["tools"] = jsonBytes(tools)
	}
	messages := []object{}
	appendMessage := func(role string, content []object) {
		if len(messages) > 0 && rawString(messages[len(messages)-1]["role"]) == role {
			last := &messages[len(messages)-1]
			blocks := array((*last)["content"])
			(*last)["content"] = jsonBytes(append(blocks, content...))
		} else {
			messages = append(messages, object{"role": jsonBytes(role), "content": jsonBytes(content)})
		}
	}
	if s := rawString(m["input"]); s != "" {
		appendMessage("user", []object{{"type": jsonBytes("text"), "text": jsonBytes(s)}})
	}
	for _, item := range array(m["input"]) {
		switch rawString(item["type"]) {
		case "", "message":
			role := rawString(item["role"])
			if role == "system" || role == "developer" {
				system += "\n" + textContent(item["content"])
				continue
			}
			blocks := []object{}
			if s := rawString(item["content"]); s != "" {
				blocks = append(blocks, object{"type": jsonBytes("text"), "text": jsonBytes(s)})
			}
			for _, v := range array(item["content"]) {
				switch rawString(v["type"]) {
				case "input_text", "output_text", "text":
					blocks = append(blocks, object{"type": jsonBytes("text"), "text": v["text"]})
				case "input_image":
					u := rawString(v["image_url"])
					src := map[string]string{"type": "url", "url": u}
					if strings.HasPrefix(u, "data:") {
						meta, data, ok := strings.Cut(strings.TrimPrefix(u, "data:"), ";base64,")
						if !ok {
							return nil, errors.New("unsupported data image")
						}
						src = map[string]string{"type": "base64", "media_type": meta, "data": data}
					}
					blocks = append(blocks, object{"type": jsonBytes("image"), "source": jsonBytes(src)})
				default:
					return nil, errors.New("unsupported cross-protocol message content")
				}
			}
			if len(blocks) > 0 {
				appendMessage(role, blocks)
			}
		case "function_call", "custom_tool_call":
			name := rawString(item["name"])
			if ns := rawString(item["namespace"]); ns != "" {
				name = ns + "__" + name
			}
			args := json.RawMessage(rawString(item["arguments"]))
			if rawString(item["type"]) == "custom_tool_call" {
				args = jsonBytes(map[string]string{"input": rawString(item["input"])})
			}
			if !json.Valid(args) {
				return nil, errors.New("invalid tool arguments")
			}
			appendMessage("assistant", []object{{"type": jsonBytes("tool_use"), "id": item["call_id"], "name": jsonBytes(name), "input": args}})
		case "function_call_output", "custom_tool_call_output":
			content, err := ToolOutput(item["output"], "messages")
			if err != nil {
				return nil, err
			}
			appendMessage("user", []object{{"type": jsonBytes("tool_result"), "tool_use_id": item["call_id"], "content": content}})
		case "reasoning": // Provider-encrypted reasoning is opaque and cannot cross protocols.
		default:
			return nil, fmt.Errorf("input item %q cannot cross protocols", rawString(item["type"]))
		}
	}
	if system != "" {
		out["system"] = jsonBytes(system)
	}
	// Responses clients may finish replay with assistant output. Copilot Messages
	// rejects assistant prefill; use the documented continuation marker.
	if len(messages) > 0 && rawString(messages[len(messages)-1]["role"]) == "assistant" {
		appendMessage("user", []object{{"type": jsonBytes("text"), "text": jsonBytes("Please continue.")}})
	}
	out["messages"] = jsonBytes(messages)
	choice := rawString(m["tool_choice"])
	if choice == "required" {
		out["tool_choice"] = jsonBytes(map[string]string{"type": "any"})
	} else if choice == "auto" || choice == "none" {
		out["tool_choice"] = jsonBytes(map[string]string{"type": choice})
	}
	return out, nil
}
