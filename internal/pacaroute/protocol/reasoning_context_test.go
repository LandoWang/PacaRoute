package protocol

import (
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func TestReasoningContextExactCopilotProfile(t *testing.T) {
	for _, kind := range []domain.SourceKind{domain.SourceCopilot, domain.SourceOpenAI} {
		for _, model := range []string{"gpt-5.3-codex", "gpt-5.4", "gpt-6.1-sol", "unknown"} {
			for _, context := range []string{`"all_turns"`, `"auto"`, `"current_turn"`, `null`, `"future"`} {
				body := obj([]byte(`{"model":"alias","input":"hi","reasoning":{"context":` + context + `,"effort":"low","future":{"value":1234567890123456789}}}`))
				original := string(body["reasoning"])
				wire, _, err := Prepare(body, kind, model, domain.AgentCodex, "responses")
				if err != nil {
					t.Fatal(err)
				}
				expected := context
				if kind == domain.SourceCopilot && model == "gpt-5.3-codex" && context == `"all_turns"` {
					expected = `"auto"`
				}
				r := obj(obj(wire)["reasoning"])
				if string(r["context"]) != expected || rawString(r["effort"]) != "low" || string(obj(r["future"])["value"]) != "1234567890123456789" {
					t.Fatalf("unexpected context profile: %s %s %s", kind, model, wire)
				}
				if string(body["reasoning"]) != original {
					t.Fatal("profile mutated request shared with failover")
				}
			}
		}
	}
}
