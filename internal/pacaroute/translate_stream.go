package pacaroute

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// maxToolArguments bounds the streamed arguments of one translated tool call.
const maxToolArguments = 8 << 20

func emitSSE(w io.Writer, event string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return writeStreamFrame(w, []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, b)))
}

// Responses->Messages state machine opens each content block exactly once and closes it before message_stop.
func (tr *translation) streamResponses(w io.Writer, r io.Reader, u *Usage) error {
	rd := newSSE(r)
	last := "none"
	id := "msg_" + strings.ReplaceAll(domain.NewID(), "-", "")
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
			return streamReadError(e, "response.completed", last)
		}
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		u.observe(data)
		m := obj(data)
		typ := rawString(m["type"])
		if typ != "" {
			last = typ
		}
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
			return streamEventError(m, false)
		}
		if e != nil {
			return e
		}
	}
}
func (tr *translation) streamMessages(w io.Writer, r io.Reader, u *Usage) error {
	rd := newSSE(r)
	last := "none"
	id := "resp_" + domain.NewID()
	sequence := 0
	output := []map[string]any{}
	blocks := map[int]object{}
	buffers := map[int]*strings.Builder{}
	itemIndices := map[int]int{}
	truncated := false
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
			return streamReadError(e, "message_stop", last)
		}
		if len(data) == 0 {
			continue
		}
		u.observe(data)
		m := obj(data)
		typ := rawString(m["type"])
		if typ != "" {
			last = typ
		}
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
				item := map[string]any{"id": "msg_" + domain.NewID(), "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
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
				if buf.Len() > maxToolArguments {
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
					// A tool_use cut off by max_tokens carries partial input. Codex
					// executes a call on output_item.done, so never complete it; the
					// stop reason decides between response.incomplete and an error.
					item["status"] = "incomplete"
					if item["type"] != "custom_tool_call" {
						item["arguments"] = args
					}
					truncated = true
					continue
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
			if truncated && reason != "max_tokens" {
				return errors.New("invalid streamed tool arguments")
			}
		case "message_stop":
			if truncated && reason != "max_tokens" {
				return errors.New("invalid streamed tool arguments")
			}
			status := "completed"
			if reason == "max_tokens" {
				status = "incomplete"
			}
			return send("response."+status, map[string]any{"response": response(status)})
		case "error":
			return streamEventError(m, false)
		}
		if e != nil {
			return e
		}
	}
}
