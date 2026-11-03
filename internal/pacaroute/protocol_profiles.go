package pacaroute

import "strings"

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
