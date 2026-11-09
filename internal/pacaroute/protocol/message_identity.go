package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

// RelayCopilotResponsesSSE preserves one client identity per message lifecycle.
// Copilot can rotate opaque message IDs on every event. Codex merges its streamed
// text and completed snapshot by ID, so both must use the ID from the added item.
// State belongs to this stream; reasoning, tool and response IDs remain opaque.
func RelayCopilotResponsesSSE(w io.Writer, r io.Reader, u *usage.Usage) error {
	ids := messageIdentities{byOutput: make(map[int]string)}
	return relaySSE(w, r, u, "responses", ids.rewrite)
}

type messageIdentities struct {
	responseID string
	byOutput   map[int]string
}

func (s *messageIdentities) rewrite(frame, data []byte, event object) []byte {
	typ := rawString(event["type"])
	if typ == "response.created" {
		s.responseID = rawString(obj(event["response"])["id"])
	}
	changed := false
	index, err := strconv.Atoi(string(event["output_index"]))
	if err == nil && index >= 0 {
		item := obj(event["item"])
		if typ == "response.output_item.added" && rawString(item["type"]) == "message" {
			if s.byOutput[index] == "" {
				id := rawString(item["id"])
				if id == "" {
					// Use a non-provider ID for a malformed added item. The digest is
					// deterministic for replay, and includes the response and slot.
					digest := sha256.Sum256(append([]byte(s.responseID+"\x1f"+strconv.Itoa(index)+"\x1f"), data...))
					id = "item_" + hex.EncodeToString(digest[:])
				}
				s.byOutput[index] = id
			}
		}
		if id := s.byOutput[index]; id != "" {
			if (typ == "response.output_item.added" || typ == "response.output_item.done") && rawString(item["type"]) == "message" && setMessageID(item, "id", id) {
				event["item"] = jsonBytes(item)
				changed = true
			}
			// Unknown message event kinds retain every field except this already
			// established identity reference, matching the native fidelity contract.
			if _, present := event["item_id"]; present && setMessageID(event, "item_id", id) {
				changed = true
			}
		}
	}
	if typ == "response.completed" || typ == "response.incomplete" || typ == "response.failed" {
		response := obj(event["response"])
		var output []json.RawMessage
		outputChanged := false
		if json.Unmarshal(response["output"], &output) == nil {
			// Native terminal output retains its original positions, including
			// reasoning and unknown items. Never compact it before looking up IDs.
			for i, raw := range output {
				item := obj(raw)
				if id := s.byOutput[i]; id != "" && rawString(item["type"]) == "message" && setMessageID(item, "id", id) {
					output[i] = jsonBytes(item)
					outputChanged = true
				}
			}
		}
		if outputChanged {
			response["output"] = jsonBytes(output)
			event["response"] = jsonBytes(response)
			changed = true
		}
	}
	if !changed {
		return frame
	}
	return replaceSSEData(frame, jsonBytes(event))
}

func setMessageID(owner object, key, id string) bool {
	if rawString(owner[key]) == id {
		return false
	}
	owner[key] = jsonBytes(id)
	return true
}

// Replace only the JSON data field. Preserve event/id/retry fields, comments,
// line endings and the frame boundary, including multiline upstream JSON.
func replaceSSEData(frame, data []byte) []byte {
	var out bytes.Buffer
	written := false
	for _, line := range bytes.SplitAfter(frame, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("data:")) {
			out.Write(line)
			continue
		}
		if !written {
			out.WriteString("data: ")
			out.Write(data)
			out.Write(line[len(bytes.TrimRight(line, "\r\n")):])
			written = true
		}
	}
	return out.Bytes()
}
