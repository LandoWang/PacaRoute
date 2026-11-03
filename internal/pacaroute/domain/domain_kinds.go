package domain

// GrantKind is what a Client grant or route permits. The two Agent protocols
// are grants too; GrantPeer is an access grant for the /pacaroute namespace,
// not a wire protocol. JSON keeps the plain string values.
type GrantKind string

const (
	GrantCodex      GrantKind = "codex"
	GrantClaudeCode GrantKind = "claude-code"
	GrantPeer       GrantKind = "pacaroute"
)

// AgentProtocol is the downstream wire protocol of an Agent request.
type AgentProtocol string

const (
	AgentCodex      AgentProtocol = "codex"
	AgentClaudeCode AgentProtocol = "claude-code"
)

// Grant is the Client grant that permits direct use of this protocol.
func (p AgentProtocol) Grant() GrantKind { return GrantKind(p) }

// SourceKind identifies the upstream resource type of a Source. The peer
// source kind shares the "pacaroute" spelling with GrantPeer but is unrelated.
type SourceKind string

const (
	SourceCopilot     SourceKind = "copilot"
	SourceAzureOpenAI SourceKind = "azure-openai"
	SourceOpenAI      SourceKind = "openai"
	SourceAnthropic   SourceKind = "anthropic"
	SourcePeer        SourceKind = "pacaroute"
)

// AccountKind identifies the provider identity type of an Account.
type AccountKind string

const (
	AccountGitHub    AccountKind = "github"
	AccountAzure     AccountKind = "azure"
	AccountOpenAI    AccountKind = "openai"
	AccountAnthropic AccountKind = "anthropic"
	AccountAPIKey    AccountKind = "api-key"
	AccountPeer      AccountKind = "pacaroute"
)

func AgentValid(a AgentProtocol) bool { return a == AgentCodex || a == AgentClaudeCode }
func clientAgentValid(a GrantKind) bool {
	return AgentValid(AgentProtocol(a)) || a == GrantPeer
}
func AccountUsesAPIKey(kind AccountKind) bool {
	return kind == AccountOpenAI || kind == AccountAnthropic || kind == AccountAPIKey || kind == AccountPeer
}
func sourceAcceptsAccount(source SourceKind, account AccountKind) bool {
	switch source {
	case SourceCopilot:
		return account == AccountGitHub
	case SourceAzureOpenAI:
		return account == AccountAzure
	case SourceOpenAI, SourceAnthropic:
		return string(account) == string(source) || account == AccountAPIKey
	case SourcePeer:
		return account == AccountPeer
	}
	return false
}
func (s Source) SupportsAgentModel(agent AgentProtocol, model string) bool {
	if s.Kind != SourcePeer {
		return true
	}
	for _, name := range s.AgentModels[string(agent)] {
		if name == model {
			return true
		}
	}
	return false
}
