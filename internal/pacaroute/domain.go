package pacaroute

import (
	"regexp"
	"strings"
)

type Account struct {
	Endpoint      string      `json:"endpoint,omitempty"`
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Kind          AccountKind `json:"kind"`
	OAuthClientID string      `json:"oauth_client_id,omitempty"`
	LoginMode     string      `json:"login_mode,omitempty"`
}
type Source struct {
	Prices         map[string]ModelPrice `json:"prices,omitempty"`
	AgentModels    map[string][]string   `json:"agent_models,omitempty"`
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	Kind           SourceKind            `json:"kind"`
	Endpoint       string                `json:"endpoint"`
	AuthMode       string                `json:"auth_mode"`
	AccountID      string                `json:"account_id,omitempty"`
	SubscriptionID string                `json:"subscription_id,omitempty"`
	ResourceID     string                `json:"resource_id,omitempty"`
	Deployments    []string              `json:"deployments"`
}
type Proxy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Address string `json:"address,omitempty"`
}
type Proxies struct {
	Profiles       []Proxy           `json:"profiles"`
	Bindings       map[string]string `json:"bindings"`
	AccountProxyID string            `json:"account_proxy_id"`
}
type Member struct {
	Alias    string `json:"alias,omitempty"`
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Model    string `json:"model"`
	Weight   int    `json:"weight"`
}
type Pool struct {
	MultiModel bool          `json:"multi_model,omitempty"`
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Agent      AgentProtocol `json:"agent"`
	Model      string        `json:"model"`
	Failover   bool          `json:"failover"`
	Members    []Member      `json:"members"`
}

// ModelNames is the pool's public catalog. A legacy pool exposes its single
// alias; a multi-model pool groups concrete deployments by their public alias.
func (p Pool) MemberModel(m Member) string {
	if !p.MultiModel {
		return p.Model
	}
	if m.Alias != "" {
		return m.Alias
	}
	return m.Model
}
func (p Pool) ModelNames() []string {
	if !p.MultiModel {
		return []string{p.Model}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, m := range p.Members {
		name := p.MemberModel(m)
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}
func (p Pool) HasModel(model string) bool {
	for _, name := range p.ModelNames() {
		if name == model {
			return true
		}
	}
	return false
}

type Route struct {
	ID         string    `json:"id"`
	Agent      GrantKind `json:"agent"`
	MatchModel string    `json:"match_model"`
	SourceID   string    `json:"source_id"`
	Model      string    `json:"model"`
}
type Client struct {
	Quota         *ClientQuota `json:"quota,omitempty"`
	Local         bool         `json:"local,omitempty"`
	LegacyDirect  bool         `json:"legacy_direct,omitempty"`
	EnabledAgents []GrantKind  `json:"enabled_agents"`
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Disabled      bool         `json:"disabled"`
	KeyHash       string       `json:"key_hash,omitempty"`
	Routes        []Route      `json:"routes"`
}
type SourceState struct {
	Paused []string `json:"paused"`
}
type Config struct {
	Accounts    []Account   `json:"accounts"`
	Sources     []Source    `json:"sources"`
	Pools       []Pool      `json:"pools"`
	Proxies     Proxies     `json:"proxies"`
	Clients     []Client    `json:"clients"`
	SourceState SourceState `json:"source_state"`
	Revision    string      `json:"revision,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var modelIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,159}$`)

func validID(v string) bool { return identifier.MatchString(v) }
func (c *Config) Account(id string) (Account, bool) {
	for _, a := range c.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	if id == "copilot" {
		return Account{ID: id, Name: "GitHub Copilot", Kind: "github"}, true
	}
	if id == "azure-local" {
		return Account{ID: id, Name: "Local Azure CLI", Kind: "azure"}, true
	}
	return Account{}, false
}
func (c *Config) Source(id string) (Source, bool) {
	for _, s := range c.Sources {
		if s.ID == id {
			return s, true
		}
	}
	if id == "copilot" {
		return Source{ID: id, Name: "GitHub Copilot", Kind: "copilot", AccountID: "copilot"}, true
	}
	return Source{}, false
}
func (c *Config) Pool(id string) (Pool, bool) {
	for _, p := range c.Pools {
		if p.ID == strings.TrimPrefix(id, "pool:") {
			return p, true
		}
	}
	return Pool{}, false
}
func (c *Config) Paused(id string) bool {
	for _, v := range c.SourceState.Paused {
		if v == id {
			return true
		}
	}
	return false
}
func (c *Config) Proxy(id string) (Proxy, bool) {
	if id == "" || id == "system" {
		return Proxy{ID: "system", Name: "System", Mode: "system"}, true
	}
	if id == "direct" {
		return Proxy{ID: id, Name: "Direct", Mode: "direct"}, true
	}
	for _, p := range c.Proxies.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Proxy{}, false
}
func (c *Config) SourceProxy(id string) Proxy { p, _ := c.Proxy(c.Proxies.Bindings[id]); return p }

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

func agentValid(a AgentProtocol) bool { return a == AgentCodex || a == AgentClaudeCode }
func clientAgentValid(a GrantKind) bool {
	return agentValid(AgentProtocol(a)) || a == GrantPeer
}
func (c *Config) Client(id string) (*Client, bool) {
	for _, v := range c.Clients {
		if v.ID == id {
			copy := v
			return &copy, true
		}
	}
	return nil, false
}
func (c *Client) AgentEnabled(agent GrantKind) bool {
	if c == nil || c.Disabled {
		return false
	}
	if c.EnabledAgents == nil {
		return agentValid(AgentProtocol(agent))
	}
	for _, value := range c.EnabledAgents {
		if value == agent {
			return true
		}
	}
	return false
}
func accountUsesAPIKey(kind AccountKind) bool {
	return kind == "openai" || kind == "anthropic" || kind == "api-key" || kind == "pacaroute"
}
func sourceAcceptsAccount(source SourceKind, account AccountKind) bool {
	switch source {
	case "copilot":
		return account == "github"
	case "azure-openai":
		return account == "azure"
	case "openai", "anthropic":
		return string(account) == string(source) || account == "api-key"
	case "pacaroute":
		return account == "pacaroute"
	}
	return false
}
func (s Source) SupportsAgentModel(agent AgentProtocol, model string) bool {
	if s.Kind != "pacaroute" {
		return true
	}
	for _, name := range s.AgentModels[string(agent)] {
		if name == model {
			return true
		}
	}
	return false
}
