package pacaroute

import (
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func parseGatewayPath(path string) (agent domain.AgentProtocol, explicit, operation string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && (parts[0] == "sources" || parts[0] == "pools") {
		explicit = parts[1]
		if parts[0] == "pools" {
			explicit = domain.PoolTarget(explicit)
		}
		parts = parts[2:]
	}
	if len(parts) > 0 && parts[0] == "codex" {
		agent = domain.AgentCodex
		parts = parts[1:]
	} else if len(parts) > 0 && parts[0] == "cc" {
		agent = domain.AgentClaudeCode
		parts = parts[1:]
	}
	if len(parts) > 0 && parts[0] == "v1" {
		parts = parts[1:]
	}
	operation = strings.Join(parts, "/")
	if agent == "" {
		if operation == "messages" || operation == "messages/count_tokens" {
			agent = domain.AgentClaudeCode
		} else {
			agent = domain.AgentCodex
		}
	}
	return
}

// upstreamPath maps the Agent operation to the target's native endpoint path.
func upstreamPath(t routing.Target, agent domain.AgentProtocol, upstreamProto, operation string) string {
	upstreamPath := "/responses"
	if upstreamProto == "messages" {
		upstreamPath = "/v1/messages"
	}
	if operation == "messages/count_tokens" {
		upstreamPath += "/count_tokens"
	}
	if t.Source.Kind == domain.SourcePeer {
		prefix := "/codex"
		if agent == domain.AgentClaudeCode {
			prefix = "/cc"
		}
		upstreamPath = prefix + "/" + operation
	}
	return upstreamPath
}
