package pacaroute

import (
	"encoding/json"
	"errors"
)

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
		// Map usage exactly as the translated stream's message_delta does, so
		// Messages observers do not add Responses cached tokens a second time.
		var usage any
		if m["usage"] != nil && string(m["usage"]) != "null" {
			u := Usage{}
			u.observe(jsonBytes(m))
			usage = map[string]any{"input_tokens": u.Input, "output_tokens": u.Output}
		}
		return json.Marshal(map[string]any{"id": "msg_" + newID(), "type": "message", "role": "assistant", "model": tr.model, "content": blocks, "stop_reason": reason, "stop_sequence": nil, "usage": usage})
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
