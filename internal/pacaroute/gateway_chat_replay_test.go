package pacaroute

import (
	"bytes"
	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"
	"testing"
)

func TestChatReplaySurvivesRestartAndRejectsScopeChanges(t *testing.T) {
	s := &Server{AdminKey: "test-installation-key"}
	x := &gatewayExchange{affinityScope: "client-a", agent: domain.AgentCodex, session: "session-a"}
	target := routing.Target{Source: domain.Source{ID: "copilot", Kind: domain.SourceCopilot, AccountID: "account-a", Endpoint: "https://upstream"}, Model: "gemini-4-argon"}
	plain := []byte(`{"reasoning_opaque":"opaque"}`)
	token, err := s.chatReplay(x, target).Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Server{AdminKey: s.AdminKey}
	got, err := restarted.chatReplay(x, target).Open(token)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("restart: %s %v", got, err)
	}
	for _, field := range []string{"client", "agent", "session", "source", "account", "endpoint", "model", "pool", "key"} {
		t.Run(field, func(t *testing.T) {
			next := *x
			to := target
			server := Server{AdminKey: s.AdminKey}
			switch field {
			case "client":
				next.affinityScope = "client-b"
			case "agent":
				next.agent = domain.AgentClaudeCode
			case "session":
				next.session = "session-b"
			case "source":
				to.Source.ID = "other"
			case "account":
				to.Source.AccountID = "other"
			case "endpoint":
				to.Source.Endpoint = "https://other"
			case "model":
				to.Model = "other"
			case "pool":
				to.Pool = &domain.Pool{ID: "other"}
			case "key":
				server.AdminKey = "other"
			}
			if _, err := server.chatReplay(&next, to).Open(token); err == nil {
				t.Fatal("cross-scope replay accepted")
			}
		})
	}
	if _, err := s.chatReplay(x, target).Open(token + "corrupt"); err == nil {
		t.Fatal("tampered replay accepted")
	}
}
