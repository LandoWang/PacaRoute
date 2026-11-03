package pacaroute

import "strings"

func parseGatewayPath(path string) (agent AgentProtocol, explicit, operation string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && (parts[0] == "sources" || parts[0] == "pools") {
		explicit = parts[1]
		if parts[0] == "pools" {
			explicit = poolTarget(explicit)
		}
		parts = parts[2:]
	}
	if len(parts) > 0 && parts[0] == "codex" {
		agent = AgentCodex
		parts = parts[1:]
	} else if len(parts) > 0 && parts[0] == "cc" {
		agent = AgentClaudeCode
		parts = parts[1:]
	}
	if len(parts) > 0 && parts[0] == "v1" {
		parts = parts[1:]
	}
	operation = strings.Join(parts, "/")
	if agent == "" {
		if operation == "messages" || operation == "messages/count_tokens" {
			agent = AgentClaudeCode
		} else {
			agent = AgentCodex
		}
	}
	return
}

// upstreamPath maps the Agent operation to the target's native endpoint path.
func upstreamPath(t Target, agent AgentProtocol, protocol, operation string) string {
	upstreamPath := "/responses"
	if protocol == "messages" {
		upstreamPath = "/v1/messages"
	}
	if operation == "messages/count_tokens" {
		upstreamPath += "/count_tokens"
	}
	if t.Source.Kind == SourcePeer {
		prefix := "/codex"
		if agent == AgentClaudeCode {
			prefix = "/cc"
		}
		upstreamPath = prefix + "/" + operation
	}
	return upstreamPath
}
