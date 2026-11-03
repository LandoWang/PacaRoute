package pacaroute

import (
	"encoding/json"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// anthropicVersion is sent on Messages requests that do not carry one.
const anthropicVersion = "2023-06-01"

// Native paths preserve opaque request and response fields. Only probed Copilot constraints are changed.
// prepare builds the upstream body for a source kind, actual model and chosen
// upstream protocol. Native paths keep unknown fields; a protocol mismatch
// selects an explicit translation.
func prepare(body object, kind domain.SourceKind, model string, agent domain.AgentProtocol, upstream string) ([]byte, *translation, error) {
	// RawMessage values are never modified in place; copying the top-level map
	// isolates edits from the body that later failover targets reuse.
	wire := make(object, len(body))
	for k, v := range body {
		wire[k] = v
	}
	wire["model"] = jsonBytes(model)
	var tr *translation
	var e error
	if domain.IncomingProtocol(agent) != upstream {
		tr = &translation{from: domain.IncomingProtocol(agent), to: upstream, model: rawString(body["model"]), toolNames: map[string]toolIdentity{}}
		if tr.to == "responses" {
			wire, e = messagesToResponses(wire, tr)
		} else {
			wire, e = responsesToMessages(wire, tr)
		}
		if e != nil {
			return nil, nil, e
		}
	}
	if kind == domain.SourceCopilot {
		if upstream == "responses" {
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
			clampEffort(wire, model)
		} else {
			sanitizeMessages(wire, model)
		}
	}
	b, e := json.Marshal(wire)
	return b, tr, e
}
