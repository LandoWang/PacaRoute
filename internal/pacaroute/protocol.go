package pacaroute

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type object = map[string]json.RawMessage

func obj(b json.RawMessage) object     { v := object{}; _ = json.Unmarshal(b, &v); return v }
func array(b json.RawMessage) []object { var v []object; _ = json.Unmarshal(b, &v); return v }
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
func toolOutput(b json.RawMessage, to string) (json.RawMessage, error) {
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
func (t Target) upstreamProtocol() string {
	if t.Source.Kind == "anthropic" || t.Source.Kind == "copilot" && strings.HasPrefix(t.Model, "claude-") {
		return "messages"
	}
	return "responses"
}
func incomingProtocol(agent string) string {
	if agent == "claude-code" {
		return "messages"
	}
	return "responses"
}

// Native paths preserve opaque request and response fields. Only probed Copilot constraints are changed.
func prepare(body object, t Target, agent string) ([]byte, *translation, error) {
	wire := obj(jsonBytes(body))
	wire["model"] = jsonBytes(t.Model)
	var tr *translation
	var e error
	if incomingProtocol(agent) != t.upstreamProtocol() {
		tr = &translation{from: incomingProtocol(agent), to: t.upstreamProtocol(), model: rawString(body["model"]), toolNames: map[string]toolIdentity{}}
		if tr.to == "responses" {
			wire, e = messagesToResponses(wire, tr)
		} else {
			wire, e = responsesToMessages(wire, tr)
		}
		if e != nil {
			return nil, nil, e
		}
	}
	if t.Source.Kind == "copilot" {
		if t.upstreamProtocol() == "responses" {
			delete(wire, "service_tier")
			if string(wire["store"]) == "true" {
				wire["store"] = jsonBytes(false)
			}
			tools := array(wire["tools"])
			if tools != nil {
				kept := []object{}
				for _, v := range tools {
					if rawString(v["type"]) != "image_generation" {
						kept = append(kept, v)
					}
				}
				if len(kept) != len(tools) {
					wire["tools"] = jsonBytes(kept)
				}
			}
			clampEffort(wire, t.Model)
		} else {
			sanitizeMessages(wire, t.Model)
		}
	}
	b, e := json.Marshal(wire)
	return b, tr, e
}

// Probe-backed model profiles carried forward from the previous gateway.
var copilotResponseEfforts = map[string]string{
	"gpt-5.3-codex": "none low medium high xhigh", "gpt-5.4": "none low medium high xhigh", "gpt-5.4-mini": "none low medium high xhigh", "gpt-5.4-nano": "none low medium high xhigh", "gpt-5.5": "none low medium high xhigh",
	"gpt-5.6-luna": "none low medium high xhigh max", "gpt-5.6-sol": "none low medium high xhigh max", "gpt-5.6-sol-fast": "none low medium high xhigh max", "gpt-5.6-terra": "none low medium high xhigh max", "gpt-6-luna": "none low medium high xhigh max", "gpt-6-sol": "none low medium high xhigh max", "gpt-6.1-terra": "none low medium high xhigh max",
	"gpt-6-astra": "low medium high xhigh max", "gpt-6.1-sol": "low medium high xhigh max", "gpt-5-mini": "minimal low medium high", "mai-code-1.1-flash": "minimal low medium high", "mai-code-2-flash": "minimal none low medium high xhigh max"}

func clampEffort(wire object, model string) {
	accepted, ok := copilotResponseEfforts[model]
	if !ok {
		return
	}
	r := obj(wire["reasoning"])
	effort := rawString(r["effort"])
	if effort == "" {
		return
	}
	if !strings.Contains(" "+accepted+" ", " "+effort+" ") {
		fallback := "xhigh"
		if model == "gpt-6-astra" || model == "gpt-6.1-sol" || model == "gpt-6.1-terra" || model == "gpt-5.4-nano" || model == "mai-code-2-flash" {
			fallback = "low"
		}
		if model == "gpt-5-mini" || model == "mai-code-1.1-flash" {
			fallback = "high"
		}
		r["effort"] = jsonBytes(fallback)
	}
	if effort == "minimal" && (model == "gpt-5-mini" || model == "mai-code-1.1-flash" || model == "mai-code-2-flash") {
		for _, tool := range array(wire["tools"]) {
			if strings.HasPrefix(rawString(tool["type"]), "web_search") {
				r["effort"] = jsonBytes("low")
			}
		}
	}
	wire["reasoning"] = jsonBytes(r)
}
func sanitizeMessages(wire object, model string) { // Keep unknown blocks and model-specific options; only remove provider-owned top-level metadata.
	delete(wire, "service_tier")
	// Copilot rejects Claude Code's local attribution header in system blocks.
	if blocks := array(wire["system"]); blocks != nil {
		kept := []object{}
		for _, b := range blocks {
			if !strings.HasPrefix(rawString(b["text"]), "x-anthropic-billing-header:") {
				kept = append(kept, b)
			}
		}
		wire["system"] = jsonBytes(kept)
	}
}

type toolIdentity struct {
	Name      string
	Namespace string
	Custom    bool
}
type translation struct {
	from, to, model string
	toolNames       map[string]toolIdentity
}

func messagesToResponses(m object, tr *translation) (object, error) {
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
				output, err := toolOutput(b["content"], "responses")
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
func responsesToMessages(m object, tr *translation) (object, error) {
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
			if _, exists := tr.toolNames[upstream]; exists {
				return errors.New("translated tool name collision")
			}
			tr.toolNames[upstream] = toolIdentity{name, ns, typ == "custom"}
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
			content, err := toolOutput(item["output"], "messages")
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
func emitSSE(w io.Writer, event string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return e
}

// Responses->Messages state machine opens each content block exactly once and closes it before message_stop.
func (tr *translation) streamResponses(w io.Writer, r io.Reader, u *Usage) error {
	rd := newSSE(r)
	id := "msg_" + strings.ReplaceAll(newID(), "-", "")
	started := false
	next := 0
	indices := map[string]int{}
	closed := map[int]bool{}
	hasTool := false
	start := func() error {
		if started {
			return nil
		}
		started = true
		return emitSSE(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": id, "type": "message", "role": "assistant", "model": tr.model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 0, "output_tokens": 0}}})
	}
	open := func(key string, block any) (int, error) {
		if n, ok := indices[key]; ok {
			return n, nil
		}
		n := next
		next++
		indices[key] = n
		return n, emitSSE(w, "content_block_start", map[string]any{"type": "content_block_start", "index": n, "content_block": block})
	}
	closeBlock := func(n int) error {
		if closed[n] {
			return nil
		}
		closed[n] = true
		return emitSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": n})
	}
	for {
		_, data, e := rd.next()
		if e != nil {
			if e == io.EOF {
				return errors.New("upstream ended before response.completed")
			}
			return e
		}
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		u.observe(data)
		m := obj(data)
		typ := rawString(m["type"])
		if e = start(); e != nil {
			return e
		}
		key := "output:" + string(m["output_index"])
		switch typ {
		case "response.output_item.added":
			item := obj(m["item"])
			if rawString(item["type"]) == "function_call" {
				hasTool = true
				_, e = open(key, map[string]any{"type": "tool_use", "id": rawString(item["call_id"]), "name": rawString(item["name"]), "input": map[string]any{}})
			}
		case "response.output_text.delta":
			n, err := open(key+":text:"+string(m["content_index"]), map[string]string{"type": "text", "text": ""})
			if err != nil {
				return err
			}
			e = emitSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": n, "delta": map[string]string{"type": "text_delta", "text": rawString(m["delta"])}})
		case "response.function_call_arguments.delta":
			n, ok := indices[key]
			if !ok {
				return errors.New("arguments arrived before tool start")
			}
			e = emitSSE(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": n, "delta": map[string]string{"type": "input_json_delta", "partial_json": rawString(m["delta"])}})
		case "response.output_item.done":
			for k, n := range indices {
				if k == key || strings.HasPrefix(k, key+":text:") {
					if e = closeBlock(n); e != nil {
						return e
					}
				}
			}
		case "response.completed", "response.incomplete":
			for n := 0; n < next; n++ {
				if e = closeBlock(n); e != nil {
					return e
				}
			}
			reason := "end_turn"
			if hasTool {
				reason = "tool_use"
			}
			if typ == "response.incomplete" {
				reason = "max_tokens"
			}
			usage := map[string]any{"input_tokens": u.Input, "output_tokens": u.Output}
			e = emitSSE(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": reason, "stop_sequence": nil}, "usage": usage})
			if e != nil {
				return e
			}
			return emitSSE(w, "message_stop", map[string]string{"type": "message_stop"})
		case "error", "response.failed":
			return errors.New("upstream Responses stream failed")
		}
		if e != nil {
			return e
		}
	}
}
func (tr *translation) responseJSON(body []byte) ([]byte, error) {
	m := obj(body)
	if tr.to == "responses" {
		blocks := []any{}
		hasTool := false
		for _, v := range array(m["output"]) {
			switch rawString(v["type"]) {
			case "message":
				for _, b := range array(v["content"]) {
					if rawString(b["type"]) == "output_text" {
						blocks = append(blocks, map[string]any{"type": "text", "text": rawString(b["text"])})
					}
				}
			case "function_call":
				hasTool = true
				args := json.RawMessage(rawString(v["arguments"]))
				if !json.Valid(args) {
					return nil, errors.New("invalid upstream tool arguments")
				}
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": rawString(v["call_id"]), "name": rawString(v["name"]), "input": args})
			}
		}
		reason := "end_turn"
		if hasTool {
			reason = "tool_use"
		}
		if rawString(m["status"]) == "incomplete" {
			reason = "max_tokens"
		}
		return json.Marshal(map[string]any{"id": "msg_" + newID(), "type": "message", "role": "assistant", "model": tr.model, "content": blocks, "stop_reason": reason, "stop_sequence": nil, "usage": m["usage"]})
	}
	return tr.messagesJSON(m)
}
func (tr *translation) messagesJSON(m object) ([]byte, error) {
	output := []any{}
	for _, b := range array(m["content"]) {
		switch rawString(b["type"]) {
		case "text":
			output = append(output, map[string]any{"id": "msg_" + newID(), "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": rawString(b["text"]), "annotations": []any{}}}})
		case "tool_use":
			item := tr.toolItem(b)
			output = append(output, item)
		}
	}
	status := "completed"
	var incomplete any
	if rawString(m["stop_reason"]) == "max_tokens" {
		status = "incomplete"
		incomplete = map[string]string{"reason": "max_output_tokens"}
	}
	u := Usage{}
	u.observe(jsonBytes(m))
	usage := u.responsesUsage()
	return json.Marshal(map[string]any{"id": "resp_" + newID(), "object": "response", "status": status, "model": tr.model, "output": output, "usage": usage, "incomplete_details": incomplete})
}
func (tr *translation) toolItem(b object) map[string]any {
	name := rawString(b["name"])
	identity, ok := tr.toolNames[name]
	if !ok {
		identity = toolIdentity{Name: name}
	}
	v := map[string]any{"id": "fc_" + rawString(b["id"]), "type": "function_call", "call_id": rawString(b["id"]), "name": identity.Name, "arguments": string(b["input"]), "status": "completed"}
	if identity.Namespace != "" {
		v["namespace"] = identity.Namespace
	}
	if identity.Custom {
		delete(v, "arguments")
		v["type"] = "custom_tool_call"
		v["input"] = rawString(obj(b["input"])["input"])
	}
	return v
}
func (tr *translation) streamMessages(w io.Writer, r io.Reader, u *Usage) error {
	rd := newSSE(r)
	id := "resp_" + newID()
	sequence := 0
	output := []map[string]any{}
	blocks := map[int]object{}
	buffers := map[int]*strings.Builder{}
	itemIndices := map[int]int{}
	started := false
	reason := "end_turn"
	send := func(typ string, v map[string]any) error {
		v["type"] = typ
		v["sequence_number"] = sequence
		sequence++
		return emitSSE(w, typ, v)
	}
	response := func(status string) map[string]any {
		v := map[string]any{"id": id, "object": "response", "status": status, "model": tr.model, "output": output, "usage": u.responsesUsage()}
		if status == "incomplete" {
			v["incomplete_details"] = map[string]string{"reason": "max_output_tokens"}
		}
		return v
	}
	for {
		_, data, e := rd.next()
		if e != nil {
			if e == io.EOF {
				return errors.New("upstream ended before message_stop")
			}
			return e
		}
		if len(data) == 0 {
			continue
		}
		u.observe(data)
		m := obj(data)
		typ := rawString(m["type"])
		if !started {
			started = true
			if e = send("response.created", map[string]any{"response": response("in_progress")}); e != nil {
				return e
			}
		}
		n := 0
		_ = json.Unmarshal(m["index"], &n)
		switch typ {
		case "content_block_start":
			b := obj(m["content_block"])
			blocks[n] = b
			buffers[n] = &strings.Builder{}
			switch rawString(b["type"]) {
			case "text":
				i := len(output)
				itemIndices[n] = i
				item := map[string]any{"id": "msg_" + newID(), "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
				output = append(output, item)
				if e = send("response.output_item.added", map[string]any{"output_index": i, "item": item}); e != nil {
					return e
				}
				e = send("response.content_part.added", map[string]any{"output_index": i, "item_id": item["id"], "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
			case "tool_use":
				i := len(output)
				itemIndices[n] = i
				item := tr.toolItem(b)
				item["status"] = "in_progress"
				if item["type"] == "custom_tool_call" {
					item["input"] = ""
				} else {
					item["arguments"] = ""
				}
				output = append(output, item)
				e = send("response.output_item.added", map[string]any{"output_index": i, "item": item})
			}
		case "content_block_delta":
			d := obj(m["delta"])
			buf := buffers[n]
			if buf == nil {
				continue
			}
			i, ok := itemIndices[n]
			if !ok {
				continue
			}
			item := output[i]
			switch rawString(d["type"]) {
			case "text_delta":
				s := rawString(d["text"])
				buf.WriteString(s)
				e = send("response.output_text.delta", map[string]any{"output_index": i, "item_id": item["id"], "content_index": 0, "delta": s})
			case "input_json_delta":
				s := rawString(d["partial_json"])
				buf.WriteString(s)
				if buf.Len() > 8<<20 {
					return errors.New("tool arguments too large")
				}
				if item["type"] != "custom_tool_call" {
					e = send("response.function_call_arguments.delta", map[string]any{"output_index": i, "item_id": item["id"], "delta": s})
				}
			}
		case "content_block_stop":
			i, ok := itemIndices[n]
			if !ok {
				continue
			}
			item := output[i]
			buf := buffers[n]
			item["status"] = "completed"
			if item["type"] == "message" {
				part := map[string]any{"type": "output_text", "text": buf.String(), "annotations": []any{}}
				item["content"] = []any{part}
				if e = send("response.output_text.done", map[string]any{"output_index": i, "item_id": item["id"], "content_index": 0, "text": buf.String()}); e != nil {
					return e
				}
				if e = send("response.content_part.done", map[string]any{"output_index": i, "item_id": item["id"], "content_index": 0, "part": part}); e != nil {
					return e
				}
			} else {
				args := buf.String()
				if args == "" {
					args = string(blocks[n]["input"])
				}
				if !json.Valid([]byte(args)) {
					return errors.New("invalid streamed tool arguments")
				}
				if item["type"] == "custom_tool_call" {
					input := rawString(obj([]byte(args))["input"])
					item["input"] = input
					if e = send("response.custom_tool_call_input.delta", map[string]any{"output_index": i, "item_id": item["id"], "delta": input}); e != nil {
						return e
					}
					e = send("response.custom_tool_call_input.done", map[string]any{"output_index": i, "item_id": item["id"], "input": input})
				} else {
					item["arguments"] = args
					e = send("response.function_call_arguments.done", map[string]any{"output_index": i, "item_id": item["id"], "arguments": args})
				}
				if e != nil {
					return e
				}
			}
			e = send("response.output_item.done", map[string]any{"output_index": i, "item": item})
		case "message_delta":
			reason = rawString(obj(m["delta"])["stop_reason"])
		case "message_stop":
			status := "completed"
			if reason == "max_tokens" {
				status = "incomplete"
			}
			return send("response."+status, map[string]any{"response": response(status)})
		case "error":
			return errors.New("upstream Messages stream failed")
		}
		if e != nil {
			return e
		}
	}
}
