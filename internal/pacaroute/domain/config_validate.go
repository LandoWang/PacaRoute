package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Validate normalizes empty collections and then checks the whole graph in a
// fixed order. Entity validators also apply their per-record defaults.
func (c *Config) Validate() error {
	c.Normalize()
	for _, validate := range []func() error{c.validateAccounts, c.validateSources, c.validateProxies, c.validatePools, c.validateClients} {
		if err := validate(); err != nil {
			return err
		}
	}
	for _, id := range c.SourceState.Paused {
		if _, ok := c.Source(id); !ok {
			return errors.New("paused source does not exist")
		}
	}
	if len(c.Sources) > 512 || len(c.Accounts) > 512 || len(c.Pools) > 128 || len(c.Clients) > 4096 {
		return errors.New("configuration limits exceeded")
	}
	return nil
}

// normalize replaces absent collections with empty ones so the stored and
// published document always carries arrays and maps, never null.
func (c *Config) Normalize() {
	if c.Accounts == nil {
		c.Accounts = []Account{}
	}
	if c.Sources == nil {
		c.Sources = []Source{}
	}
	if c.Pools == nil {
		c.Pools = []Pool{}
	}
	if c.Clients == nil {
		c.Clients = []Client{}
	}
	if c.Proxies.Profiles == nil {
		c.Proxies.Profiles = []Proxy{}
	}
	if c.Proxies.Bindings == nil {
		c.Proxies.Bindings = map[string]string{}
	}
	if c.SourceState.Paused == nil {
		c.SourceState.Paused = []string{}
	}
}

// validateAccounts checks accounts: unique valid IDs, supported kinds and login methods.
func (c *Config) validateAccounts() error {
	seen := map[string]bool{}
	for _, a := range c.Accounts {
		if !ValidID(a.ID) || seen[a.ID] || strings.TrimSpace(a.Name) == "" || (a.Kind != AccountGitHub && a.Kind != AccountAzure && !AccountUsesAPIKey(a.Kind)) {
			return fmt.Errorf("invalid or duplicate account %q", a.ID)
		}
		if a.LoginMode != "" && !((a.Kind == AccountAzure && (a.LoginMode == "azure-cli" || a.LoginMode == "device")) || (a.Kind == AccountGitHub && (a.LoginMode == "device" || a.LoginMode == "oauth-app"))) {
			return errors.New("unsupported account login method")
		}
		if a.LoginMode == "oauth-app" && strings.TrimSpace(a.OAuthClientID) == "" {
			return errors.New("OAuth App client ID required")
		}
		if a.Kind == AccountPeer && !validPeerEndpoint(a.Endpoint) {
			return errors.New("PacaRoute account requires an HTTP(S) service address without credentials")
		}
		seen[a.ID] = true
	}
	return nil
}

// validateSources checks sources: account compatibility, endpoints, models and per-model prices.
func (c *Config) validateSources() error {
	seen := map[string]bool{}
	for i := range c.Sources {
		s := &c.Sources[i]
		if len(s.Prices) > 512 {
			return errors.New("too many model prices")
		}
		for model, price := range s.Prices {
			if !ModelIdentifier.MatchString(model) {
				return errors.New("invalid priced model")
			}
			if err := price.validate(); err != nil {
				return err
			}
		}
		if s.Kind == "" {
			s.Kind = SourceAzureOpenAI
		}
		if s.AuthMode == "" {
			if s.Kind == SourceOpenAI || s.Kind == SourceAnthropic || s.Kind == SourcePeer {
				s.AuthMode = "api-key"
			} else {
				s.AuthMode = "azure-cli"
			}
		}
		if s.AccountID == "" {
			if s.Kind == SourceCopilot {
				if s.Deployments == nil {
					s.Deployments = []string{}
				}
				s.AccountID = "copilot"
			} else {
				s.AccountID = "azure-local"
			}
		}
		a, ok := c.Account(s.AccountID)
		if !ValidID(s.ID) || seen[s.ID] || s.Name == "" || !ok {
			return fmt.Errorf("invalid source/account %q", s.ID)
		}
		seen[s.ID] = true
		if s.Kind == SourceCopilot {
			if a.Kind != AccountGitHub {
				return errors.New("Copilot source requires GitHub account")
			}
			continue
		}
		if s.Kind != SourceAzureOpenAI && s.Kind != SourceOpenAI && s.Kind != SourceAnthropic && s.Kind != SourcePeer {
			return fmt.Errorf("unsupported source kind %q", s.Kind)
		}
		if (s.Kind == SourceOpenAI || s.Kind == SourceAnthropic || s.Kind == SourcePeer) && (s.AuthMode != "api-key" || !sourceAcceptsAccount(s.Kind, a.Kind)) {
			return errors.New("API source requires API-key account and authentication")
		}
		if s.Deployments == nil {
			s.Deployments = []string{}
		}
		if s.Kind == SourceAzureOpenAI && (a.Kind != AccountAzure || (s.AuthMode != "azure-cli" && s.AuthMode != "api-key")) {
			return errors.New("invalid Azure authentication")
		}
		u, e := url.Parse(s.Endpoint)
		if e != nil || (u.Scheme != "https" && !(s.Kind == SourcePeer && validPeerEndpoint(s.Endpoint))) || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("source %s requires an HTTPS endpoint without credentials", s.ID)
		}
		if s.Kind == SourceAzureOpenAI {
			if u.Path != "" && u.Path != "/" && strings.TrimSuffix(u.Path, "/") != "/openai/v1" {
				return errors.New("Azure endpoint must be a resource URL or /openai/v1")
			}
			u.Path = "/openai/v1"
			s.Endpoint = u.String()
		}
		if len(s.Deployments) == 0 {
			return errors.New("a source needs at least one model/deployment")
		}
		for _, m := range s.Deployments {
			if !ModelIdentifier.MatchString(m) {
				return fmt.Errorf("invalid model %q", m)
			}
		}
		for agent, models := range s.AgentModels {
			if !AgentValid(AgentProtocol(agent)) {
				return errors.New("unknown Agent in discovered source models")
			}
			for _, model := range models {
				if !ModelIdentifier.MatchString(model) {
					return errors.New("invalid discovered model")
				}
			}
		}
	}
	return nil
}

// validateProxies checks proxy profiles, source bindings and the account-login proxy.
func (c *Config) validateProxies() error {
	seen := map[string]bool{"system": true, "direct": true}
	for _, p := range c.Proxies.Profiles {
		if !ValidID(p.ID) || seen[p.ID] || p.Name == "" {
			return errors.New("invalid proxy")
		}
		seen[p.ID] = true
		if p.Mode == "custom" {
			u, e := url.Parse(p.Address)
			if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
				return errors.New("invalid proxy address")
			}
		} else if p.Mode != "direct" && p.Mode != "system" {
			return errors.New("invalid proxy mode")
		}
	}
	for _, id := range c.Proxies.Bindings {
		if _, ok := c.Proxy(id); !ok {
			return errors.New("unknown proxy binding")
		}
	}
	if _, ok := c.Proxy(c.Proxies.AccountProxyID); !ok {
		return errors.New("unknown account proxy")
	}
	return nil
}

// validatePools checks pools: members, weights and exposed models.
func (c *Config) validatePools() error {
	seen := map[string]bool{}
	for _, p := range c.Pools {
		if !ValidID(p.ID) || seen[p.ID] || p.Name == "" || !AgentValid(p.Agent) || !ModelIdentifier.MatchString(p.Model) || len(p.Members) == 0 || len(p.Members) > 32 {
			return errors.New("invalid source pool")
		}
		seen[p.ID] = true
		ids := map[string]bool{}
		sum := 0
		weights := map[string]int{}
		for _, m := range p.Members {
			if IsPoolTarget(m.SourceID) {
				return errors.New("nested pools are not supported")
			}
			if !ValidID(m.ID) || ids[m.ID] || m.Weight < 0 || m.Weight > 1000000 {
				return errors.New("invalid pool member")
			}
			ids[m.ID] = true
			sum += m.Weight
			if !ModelIdentifier.MatchString(p.MemberModel(m)) {
				return errors.New("invalid pool model alias")
			}
			weights[p.MemberModel(m)] += m.Weight
			if e := c.ValidateTarget(m.SourceID, m.Model, p.Agent.Grant()); e != nil {
				return e
			}
		}
		if sum == 0 {
			return errors.New("pool weights must have a positive total")
		}
		if !p.HasModel(p.Model) {
			return errors.New("default pool model must be exposed by a member")
		}
		for _, weight := range weights {
			if weight == 0 {
				return errors.New("each pool model needs a positive total weight")
			}
		}
	}
	return nil
}

// validateClients checks Clients: grants, keys, quota policy and route targets.
func (c *Config) validateClients() error {
	seen := map[string]bool{}
	for i := range c.Clients {
		v := &c.Clients[i]
		if err := v.Quota.validate(); err != nil {
			return err
		}
		if v.EnabledAgents == nil {
			v.EnabledAgents = []GrantKind{GrantCodex, GrantClaudeCode}
		}
		if v.Routes == nil {
			v.Routes = []Route{}
		}
		if !ValidID(v.ID) || seen[v.ID] || v.Name == "" {
			return errors.New("invalid client")
		}
		seen[v.ID] = true
		agents := map[GrantKind]bool{}
		for _, agent := range v.EnabledAgents {
			if !clientAgentValid(agent) || agents[agent] {
				return errors.New("invalid enabled Agent")
			}
			agents[agent] = true
		}
		if v.Local {
			if v.ID != "default" || v.KeyHash != "" {
				return errors.New("local Client must be default and cannot have an API key")
			}
		} else {
			if h, e := hex.DecodeString(v.KeyHash); e != nil || len(h) != 32 {
				return errors.New("client requires a hashed API key")
			}
		}
		if v.LegacyDirect && (!v.Local || len(v.Routes) != 0) {
			return errors.New("legacy local routing cannot coexist with explicit routes")
		}
		ids := map[string]bool{}
		for _, route := range v.Routes {
			if !ValidID(route.ID) || ids[route.ID] || !clientAgentValid(route.Agent) || (route.MatchModel != "*" && !ModelIdentifier.MatchString(route.MatchModel)) {
				return errors.New("invalid client route")
			}
			ids[route.ID] = true
			if e := c.ValidateTarget(route.SourceID, route.Model, route.Agent); e != nil {
				return e
			}
		}
	}
	return nil
}
func (c *Config) ValidateTarget(id, model string, agent GrantKind) error {
	if agent == GrantPeer {
		for _, protocol := range []GrantKind{GrantCodex, GrantClaudeCode} {
			if c.ValidateTarget(id, model, protocol) == nil {
				return nil
			}
		}
		return errors.New("PacaRoute target must expose this model for Codex or Claude Code")
	}
	if IsPoolTarget(id) {
		p, ok := c.Pool(id)
		if !ok || p.Agent.Grant() != agent || !p.HasModel(model) {
			return errors.New("route must match the pool Agent and model")
		}
		return nil
	}
	s, ok := c.Source(id)
	if !ok || !ModelIdentifier.MatchString(model) {
		return errors.New("unknown source or invalid model")
	}
	if s.Kind == SourcePeer && !s.SupportsAgentModel(AgentProtocol(agent), model) {
		return errors.New("remote PacaRoute does not expose this model for the selected Agent; refresh its model catalog")
	}
	if s.Kind != SourceCopilot {
		for _, v := range s.Deployments {
			if v == model {
				return nil
			}
		}
		return errors.New("model is not configured on source")
	}
	return nil
}
func validPeerEndpoint(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
