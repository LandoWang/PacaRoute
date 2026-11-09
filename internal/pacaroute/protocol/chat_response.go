package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

const maxChatResponse = 32 << 20

type chatResult struct {
	text   strings.Builder
	calls  map[int]object
	state  object
	finish string
	size   int
}

func newChatResult() *chatResult { return &chatResult{calls: map[int]object{}, state: object{}} }
func (c *chatResult) add(m object, stream bool) error {
	c.size += len(jsonBytes(m))
	if c.size > maxChatResponse {
		return errors.New("Chat response exceeds 32 MiB")
	}
	if text := rawString(m["content"]); text != "" {
		c.text.WriteString(text)
	}
	for k, v := range m {
		if k != "content" && k != "role" && k != "tool_calls" && string(v) != "null" {
			if stream && k == "reasoning_opaque" {
				c.state[k] = jsonBytes(rawString(c.state[k]) + rawString(v))
			} else {
				c.state[k] = v
			}
		}
	}
	for n, t := range array(m["tool_calls"]) {
		if stream {
			if json.Unmarshal(t["index"], &n) != nil || n < 0 || n > 1023 {
				return errors.New("invalid Chat tool index")
			}
		}
		old, ok := c.calls[n]
		if !ok {
			old = object{"id": jsonBytes(""), "name": jsonBytes(""), "arguments": jsonBytes("")}
			c.calls[n] = old
		}
		if id := rawString(t["id"]); id != "" {
			if prior := rawString(old["id"]); prior != "" && prior != id {
				return errors.New("Chat tool ID changed")
			}
			old["id"] = t["id"]
		}
		f := obj(t["function"])
		for _, k := range []string{"name", "arguments"} {
			if v := rawString(f[k]); v != "" {
				old[k] = jsonBytes(rawString(old[k]) + v)
			}
		}
		if len(rawString(old["arguments"])) > maxToolArguments {
			return errors.New("Chat tool arguments exceed 8 MiB")
		}
	}
	return nil
}
func (c *chatResult) validate() error {
	switch c.finish {
	case "stop", "tool_calls", "length":
	default:
		return fmt.Errorf("Chat response ended with unsupported finish reason %q", c.finish)
	}
	if c.finish == "length" {
		return nil
	}
	if c.finish == "tool_calls" && len(c.calls) == 0 {
		return errors.New("Chat tool finish without calls")
	}
	seen := map[string]bool{}
	for _, call := range c.calls {
		id := rawString(call["id"])
		if id == "" || seen[id] || rawString(call["name"]) == "" || !chatObject([]byte(rawString(call["arguments"]))) {
			return errors.New("invalid Chat tool call")
		}
		seen[id] = true
	}
	return nil
}
func chatUsage(u *usage.Usage, messages bool) any {
	if u.Input == nil || u.Output == nil {
		return nil
	}
	input := *u.Input
	if messages {
		v := map[string]any{"input_tokens": input, "output_tokens": *u.Output}
		if u.CacheRead != nil {
			v["input_tokens"] = max(int64(0), input-*u.CacheRead)
			v["cache_read_input_tokens"] = *u.CacheRead
		}
		return v
	}
	v := map[string]any{"input_tokens": input, "output_tokens": *u.Output, "total_tokens": input + *u.Output}
	if u.CacheRead != nil {
		v["input_tokens_details"] = map[string]any{"cached_tokens": *u.CacheRead}
	}
	return v
}
func (tr *Translation) chatBlocks(c *chatResult) ([]object, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	blocks := []object{}
	if len(c.state) > 0 {
		if tr.SealReplay == nil {
			return nil, errors.New("Chat replay is unavailable")
		}
		token, err := tr.SealReplay(jsonBytes(c.state))
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, object{"type": jsonBytes("redacted_thinking"), "data": jsonBytes(token)})
	}
	// Never expose a length-truncated call as executable, even if its prefix parses.
	if c.finish != "length" {
		indices := []int{}
		for n := range c.calls {
			indices = append(indices, n)
		}
		sort.Ints(indices)
		for _, n := range indices {
			call := c.calls[n]
			blocks = append(blocks, object{"type": jsonBytes("tool_use"), "id": call["id"], "name": call["name"], "input": json.RawMessage(rawString(call["arguments"]))})
		}
	}
	return blocks, nil
}
func (tr *Translation) chatItem(b object) (map[string]any, error) {
	if rawString(b["type"]) == "redacted_thinking" {
		return map[string]any{"id": "rs_" + domain.NewID(), "type": "reasoning", "summary": []any{}, "encrypted_content": rawString(b["data"])}, nil
	}
	item := tr.toolItem(b)
	if item["type"] == "custom_tool_call" {
		var input string
		if json.Unmarshal(obj(b["input"])["input"], &input) != nil {
			return nil, errors.New("custom tool requires a string input")
		}
		item["id"] = "ctc_" + rawString(b["id"])
	}
	return item, nil
}
func (tr *Translation) ChatJSON(data []byte) ([]byte, error) {
	m := obj(data)
	if m == nil {
		return nil, errors.New("invalid Chat JSON")
	}
	if m["error"] != nil {
		return nil, streamEventError(m, false)
	}
	choices := array(m["choices"])
	if len(choices) != 1 {
		return nil, errors.New("Chat response requires one choice")
	}
	c := newChatResult()
	if err := c.add(obj(choices[0]["message"]), false); err != nil {
		return nil, err
	}
	c.finish = rawString(choices[0]["finish_reason"])
	blocks, err := tr.chatBlocks(c)
	if err != nil {
		return nil, err
	}
	u := &usage.Usage{}
	u.Observe(data)
	if tr.From == "messages" {
		blocks = chatMessageBlocks(c, blocks)
		return json.Marshal(map[string]any{"id": "msg_" + domain.NewID(), "type": "message", "role": "assistant", "model": tr.Model, "content": blocks, "stop_reason": chatStop(c), "stop_sequence": nil, "usage": chatUsage(u, true)})
	}
	items := []any{}
	if c.text.Len() > 0 {
		items = append(items, map[string]any{"id": "msg_" + domain.NewID(), "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": c.text.String(), "annotations": []any{}}}})
	}
	for _, b := range blocks {
		item, err := tr.chatItem(b)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	v := map[string]any{"id": "resp_" + domain.NewID(), "object": "response", "model": tr.Model, "status": "completed", "output": items, "usage": chatUsage(u, false)}
	if c.finish == "length" {
		v["status"] = "incomplete"
		v["incomplete_details"] = map[string]string{"reason": "max_output_tokens"}
	}
	return json.Marshal(v)
}
func chatStop(c *chatResult) string {
	if c.finish == "length" {
		return "max_tokens"
	}
	if len(c.calls) > 0 {
		return "tool_use"
	}
	return "end_turn"
}

// StreamChat streams text immediately but releases tools only after a valid finish
// and [DONE]. Final usage and opaque state may arrive after the finish chunk.
func (tr *Translation) StreamChat(w io.Writer, r io.Reader, u *usage.Usage) error {
	c := newChatResult()
	rd := NewSSE(r)
	seq := 0
	id := "resp_" + domain.NewID()
	mid := "msg_" + domain.NewID()
	textOpen := false
	output := []any{}
	send := func(typ string, v map[string]any) error {
		v["type"] = typ
		if tr.From == "responses" {
			v["sequence_number"] = seq
			seq++
		}
		return emitSSE(w, typ, v)
	}
	response := func(status string) map[string]any {
		v := map[string]any{"id": id, "object": "response", "model": tr.Model, "status": status, "output": output, "usage": chatUsage(u, false)}
		if status == "incomplete" {
			v["incomplete_details"] = map[string]string{"reason": "max_output_tokens"}
		}
		return v
	}
	if tr.From == "responses" {
		if err := send("response.created", map[string]any{"response": response("in_progress")}); err != nil {
			return err
		}
	} else {
		if err := send("message_start", map[string]any{"message": map[string]any{"id": mid, "type": "message", "role": "assistant", "model": tr.Model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 0, "output_tokens": 0}}}); err != nil {
			return err
		}
	}
	for {
		_, data, err := rd.Next()
		if err != nil {
			return streamReadError(err, "[DONE]", c.finish)
		}
		if len(data) == 0 {
			continue
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}
		m := obj(data)
		if m == nil || !json.Valid(data) {
			return errors.New("invalid Chat stream JSON")
		}
		if m["error"] != nil {
			return streamEventError(m, false)
		}
		// Every Chat usage record is a snapshot, including an explicit zero cache count.
		if m["usage"] != nil && string(m["usage"]) != "null" {
			*u = usage.Usage{}
			u.Observe(data)
		}
		choices := array(m["choices"])
		if len(choices) == 0 {
			continue
		}
		if len(choices) != 1 {
			return errors.New("multiple Chat stream choices")
		}
		choice := choices[0]
		delta := obj(choice["delta"])
		text := rawString(delta["content"])
		if c.finish != "" && len(delta) > 0 {
			return errors.New("Chat content after finish")
		}
		if err = c.add(delta, true); err != nil {
			return err
		}
		if text != "" && tr.From == "responses" {
			if !textOpen {
				textOpen = true
				if tr.From == "responses" {
					if err = send("response.output_item.added", map[string]any{"output_index": 0, "item": map[string]any{"id": mid, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}}); err != nil {
						return err
					}
					err = send("response.content_part.added", map[string]any{"output_index": 0, "item_id": mid, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
				} else {
					err = send("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
				}
				if err != nil {
					return err
				}
			}
			if tr.From == "responses" {
				err = send("response.output_text.delta", map[string]any{"output_index": 0, "item_id": mid, "content_index": 0, "delta": text})
			} else {
				err = send("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": text}})
			}
			if err != nil {
				return err
			}
		}
		if finish := rawString(choice["finish_reason"]); finish != "" {
			c.finish = finish
		}
	}
	blocks, err := tr.chatBlocks(c)
	if err != nil {
		return err
	}
	if tr.From == "messages" {
		// Claude's final result follows the last content block. Provider state
		// arrives at finish, so emit it before buffered answer text and tools.
		for index, b := range chatMessageBlocks(c, blocks) {
			initial := object{}
			for k, v := range b {
				initial[k] = v
			}
			typ := rawString(b["type"])
			if typ == "text" {
				initial["text"] = jsonBytes("")
			}
			if typ == "tool_use" {
				initial["input"] = jsonBytes(map[string]any{})
			}
			if err = send("content_block_start", map[string]any{"index": index, "content_block": initial}); err != nil {
				return err
			}
			if typ == "text" {
				err = send("content_block_delta", map[string]any{"index": index, "delta": map[string]any{"type": "text_delta", "text": rawString(b["text"])}})
			}
			if typ == "tool_use" {
				err = send("content_block_delta", map[string]any{"index": index, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(b["input"])}})
			}
			if err != nil {
				return err
			}
			if err = send("content_block_stop", map[string]any{"index": index}); err != nil {
				return err
			}
		}
		if err = send("message_delta", map[string]any{"delta": map[string]any{"stop_reason": chatStop(c), "stop_sequence": nil}, "usage": chatUsage(u, true)}); err != nil {
			return err
		}
		return send("message_stop", map[string]any{})
	}
	if textOpen {
		if tr.From == "responses" {
			part := map[string]any{"type": "output_text", "text": c.text.String(), "annotations": []any{}}
			item := map[string]any{"id": mid, "type": "message", "role": "assistant", "status": "completed", "content": []any{part}}
			output = append(output, item)
			if err = send("response.output_text.done", map[string]any{"output_index": 0, "item_id": mid, "content_index": 0, "text": c.text.String()}); err != nil {
				return err
			}
			if err = send("response.content_part.done", map[string]any{"output_index": 0, "item_id": mid, "content_index": 0, "part": part}); err != nil {
				return err
			}
			err = send("response.output_item.done", map[string]any{"output_index": 0, "item": item})
		} else {
			err = send("content_block_stop", map[string]any{"index": 0})
		}
		if err != nil {
			return err
		}
	}
	for _, b := range blocks {
		item, e := tr.chatItem(b)
		if e != nil {
			return e
		}
		index := len(output)
		output = append(output, item)
		initial := map[string]any{}
		for k, v := range item {
			initial[k] = v
		}
		key := "arguments"
		event := "function_call_arguments"
		if item["type"] == "custom_tool_call" {
			key = "input"
			event = "custom_tool_call_input"
		}
		if item["type"] != "reasoning" {
			initial[key] = ""
			initial["status"] = "in_progress"
		}
		if err = send("response.output_item.added", map[string]any{"output_index": index, "item": initial}); err != nil {
			return err
		}
		if item["type"] != "reasoning" {
			if err = send("response."+event+".delta", map[string]any{"output_index": index, "item_id": item["id"], "delta": item[key]}); err != nil {
				return err
			}
			if err = send("response."+event+".done", map[string]any{"output_index": index, "item_id": item["id"], key: item[key]}); err != nil {
				return err
			}
		}
		if err = send("response.output_item.done", map[string]any{"output_index": index, "item": item}); err != nil {
			return err
		}
	}

	status := "completed"
	if c.finish == "length" {
		status = "incomplete"
	}
	return send("response."+status, map[string]any{"response": response(status)})
}

func chatMessageBlocks(c *chatResult, blocks []object) []object {
	out := []object{}
	for _, b := range blocks {
		if rawString(b["type"]) == "redacted_thinking" {
			out = append(out, b)
		}
	}
	if c.text.Len() > 0 {
		out = append(out, object{"type": jsonBytes("text"), "text": jsonBytes(c.text.String())})
	}
	for _, b := range blocks {
		if rawString(b["type"]) != "redacted_thinking" {
			out = append(out, b)
		}
	}
	return out
}
