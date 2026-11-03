package pacaroute

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Contract: enabled native Messages models are selectable by Claude Code;
// disabled models stay excluded; unsupported profiles remain visible without Agent compatibility.
func TestCopilotCatalogIncludesNativeMessagesModels(t *testing.T) {
	store := storeForTest(t)
	auth := NewAuth(store, "")
	if err := auth.vault.write(auth.vault.githubPath("copilot"), credential{Version: 1, AccessToken: "synthetic-github", CredentialID: "catalog"}); err != nil {
		t.Fatal(err)
	}
	transport := syntheticAuthClient(func(r *http.Request) string {
		if r.URL.Path != "/models" {
			return fmt.Sprintf(`{"token":"synthetic-exchange","expires_at":%d,"endpoints":{"api":"https://api.githubcopilot.com"}}`, time.Now().Add(time.Hour).Unix())
		}
		return `{"data":[
   {"id":"claude-opus-5.5","supported_endpoints":["/v1/messages"],"policy":{"state":"enabled"}},
   {"id":"claude-sonnet-5.5","supported_endpoints":["/v1/messages","/chat/completions"],"policy":{"state":"enabled"}},
   {"id":"claude-sonnet-5","supported_endpoints":["/v1/messages"],"policy":{"state":"enabled"}},
   {"id":"claude-haiku-5.5","supported_endpoints":["/v1/messages"],"policy":{"state":"enabled"}},
   {"id":"future-native","supported_endpoints":["/v1/messages"]},
   {"id":"disabled-native","supported_endpoints":["/v1/messages"],"policy":{"state":"disabled"}},
   {"id":"chat-only","supported_endpoints":["/chat/completions"]},
   {"id":"unknown-responses","supported_endpoints":["/responses"]},
   {"id":"gpt-6-astra","supported_endpoints":["/responses"]}
  ]}`
	})
	auth.http.values = map[string]*http.Client{"system:": transport}
	models, err := newCatalog(auth.config, auth, &auth.http).Models(context.Background(), "copilot")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]Model{}
	for _, model := range models {
		found[model.ID] = model
	}
	for _, id := range []string{"claude-opus-5.5", "claude-sonnet-5.5", "claude-sonnet-5", "claude-haiku-5.5", "future-native"} {
		if model, ok := found[id]; !ok || !model.Claude || model.Codex {
			t.Errorf("native model %s not available to Claude Code", id)
		}
	}
	for _, id := range []string{"disabled-native"} {
		if _, ok := found[id]; ok {
			t.Errorf("unsupported model %s listed", id)
		}
	}
	for _, id := range []string{"chat-only", "unknown-responses"} {
		if model, ok := found[id]; !ok || model.Claude || model.Codex {
			t.Errorf("catalog model %s must be visible without claiming compatibility", id)
		}
	}
	if model := found["gpt-6-astra"]; !model.Codex || !model.Claude {
		t.Fatal("reviewed Responses model lost compatibility")
	}
}

// Contract: an ordered Sonnet rule retains the requested family while the
// existing Opus default and independent Codex pool remain unchanged.
func TestExplicitSonnetRoutePrecedesDefaultModel(t *testing.T) {
	c := fixtureConfig()
	client := Client{ID: "default", Local: true, EnabledAgents: []GrantKind{"claude-code", "codex"}, Routes: []Route{
		{ID: "sonnet-55", Agent: "claude-code", MatchModel: "claude-sonnet-5-5", SourceID: "copilot", Model: "claude-sonnet-5.5"},
		{ID: "sonnet", Agent: "claude-code", MatchModel: "claude-sonnet-5", SourceID: "copilot", Model: "claude-sonnet-5"},
		{ID: "opus-default", Agent: "claude-code", MatchModel: "*", SourceID: "copilot", Model: "claude-opus-5.5"},
		{ID: "codex", Agent: "codex", MatchModel: "*", SourceID: "pool:balanced", Model: "coding"},
	}}
	for _, test := range []struct{ agent, request, source, model string }{
		{"claude-code", "claude-sonnet-5-5", "copilot", "claude-sonnet-5.5"},
		{"claude-code", "claude-sonnet-5", "copilot", "claude-sonnet-5"},
		{"claude-code", "claude-opus-5-5", "copilot", "claude-opus-5.5"},
		{"codex", "gpt-6-astra", "pool:balanced", "coding"},
	} {
		source, model, err := resolveRoute(&c, &client, AgentProtocol(test.agent), test.request, "")
		if err != nil || source != test.source || model != test.model {
			t.Errorf("%s: wrong route (%s, %s, %v)", test.request, source, model, err)
		}
	}
}

func TestSelectedModelsRejectUnselectedRequests(t *testing.T) {
	c := fixtureConfig()
	client := Client{ID: "selected", EnabledAgents: []GrantKind{"claude-code", "codex"}, Routes: []Route{
		{ID: "opus", Agent: "claude-code", MatchModel: "claude-opus-5.5", SourceID: "copilot", Model: "claude-opus-5.5"},
		{ID: "sonnet", Agent: "claude-code", MatchModel: "claude-sonnet-5.5", SourceID: "copilot", Model: "claude-sonnet-5.5"},
		{ID: "sonnet-alias", Agent: "claude-code", MatchModel: "claude-sonnet-5-5", SourceID: "copilot", Model: "claude-sonnet-5.5"},
		{ID: "codex", Agent: "codex", MatchModel: "*", SourceID: "pool:balanced", Model: "coding"},
	}}
	for _, requested := range []string{"claude-sonnet-5.5", "claude-sonnet-5-5"} {
		source, model, err := resolveRoute(&c, &client, "claude-code", requested, "")
		if err != nil || source != "copilot" || model != "claude-sonnet-5.5" {
			t.Fatalf("selected Sonnet changed: %s %s %v", source, model, err)
		}
	}
	for _, requested := range []string{"claude-haiku-4.5", "gpt-6-astra", ""} {
		if _, _, err := resolveRoute(&c, &client, "claude-code", requested, ""); err == nil {
			t.Fatalf("unselected model %q accepted", requested)
		}
	}
}
