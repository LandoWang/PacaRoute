package pacaroute

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func (c *Config) Validate() error {
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

	seen := map[string]bool{}
	for _, a := range c.Accounts {
		if !validID(a.ID) || seen[a.ID] || strings.TrimSpace(a.Name) == "" || (a.Kind != "github" && a.Kind != "azure" && !accountUsesAPIKey(a.Kind)) {
			return fmt.Errorf("invalid or duplicate account %q", a.ID)
		}
		if a.LoginMode != "" && !((a.Kind == "azure" && (a.LoginMode == "azure-cli" || a.LoginMode == "device")) || (a.Kind == "github" && (a.LoginMode == "device" || a.LoginMode == "oauth-app"))) {
			return errors.New("unsupported account login method")
		}
		if a.LoginMode == "oauth-app" && strings.TrimSpace(a.OAuthClientID) == "" {
			return errors.New("OAuth App client ID required")
		}
		if a.Kind == "pacaroute" && !validPeerEndpoint(a.Endpoint) {
			return errors.New("PacaRoute account requires an HTTP(S) service address without credentials")
		}
		seen[a.ID] = true
	}
	seen = map[string]bool{}
	for i := range c.Sources {
		s := &c.Sources[i]
		if len(s.Prices) > 512 {
			return errors.New("too many model prices")
		}
		for model, price := range s.Prices {
			if !modelIdentifier.MatchString(model) {
				return errors.New("invalid priced model")
			}
			if err := price.validate(); err != nil {
				return err
			}
		}
		if s.Kind == "" {
			s.Kind = "azure-openai"
		}
		if s.AuthMode == "" {
			if s.Kind == "openai" || s.Kind == "anthropic" || s.Kind == "pacaroute" {
				s.AuthMode = "api-key"
			} else {
				s.AuthMode = "azure-cli"
			}
		}
		if s.AccountID == "" {
			if s.Kind == "copilot" {
				if s.Deployments == nil {
					s.Deployments = []string{}
				}
				s.AccountID = "copilot"
			} else {
				s.AccountID = "azure-local"
			}
		}
		a, ok := c.Account(s.AccountID)
		if !validID(s.ID) || seen[s.ID] || s.Name == "" || !ok {
			return fmt.Errorf("invalid source/account %q", s.ID)
		}
		seen[s.ID] = true
		if s.Kind == "copilot" {
			if a.Kind != "github" {
				return errors.New("Copilot source requires GitHub account")
			}
			continue
		}
		if s.Kind != "azure-openai" && s.Kind != "openai" && s.Kind != "anthropic" && s.Kind != "pacaroute" {
			return fmt.Errorf("unsupported source kind %q", s.Kind)
		}
		if (s.Kind == "openai" || s.Kind == "anthropic" || s.Kind == "pacaroute") && (s.AuthMode != "api-key" || !sourceAcceptsAccount(s.Kind, a.Kind)) {
			return errors.New("API source requires API-key account and authentication")
		}
		if s.Deployments == nil {
			s.Deployments = []string{}
		}
		if s.Kind == "azure-openai" && (a.Kind != "azure" || (s.AuthMode != "azure-cli" && s.AuthMode != "api-key")) {
			return errors.New("invalid Azure authentication")
		}
		u, e := url.Parse(s.Endpoint)
		if e != nil || (u.Scheme != "https" && !(s.Kind == "pacaroute" && validPeerEndpoint(s.Endpoint))) || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("source %s requires an HTTPS endpoint without credentials", s.ID)
		}
		if s.Kind == "azure-openai" {
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
			if !modelIdentifier.MatchString(m) {
				return fmt.Errorf("invalid model %q", m)
			}
		}
		for agent, models := range s.AgentModels {
			if !agentValid(agent) {
				return errors.New("unknown Agent in discovered source models")
			}
			for _, model := range models {
				if !modelIdentifier.MatchString(model) {
					return errors.New("invalid discovered model")
				}
			}
		}
	}
	seen = map[string]bool{"system": true, "direct": true}
	for _, p := range c.Proxies.Profiles {
		if !validID(p.ID) || seen[p.ID] || p.Name == "" {
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
	seen = map[string]bool{}
	for _, p := range c.Pools {
		if !validID(p.ID) || seen[p.ID] || p.Name == "" || !agentValid(p.Agent) || !modelIdentifier.MatchString(p.Model) || len(p.Members) == 0 || len(p.Members) > 32 {
			return errors.New("invalid source pool")
		}
		seen[p.ID] = true
		ids := map[string]bool{}
		sum := 0
		weights := map[string]int{}
		for _, m := range p.Members {
			if strings.HasPrefix(m.SourceID, "pool:") {
				return errors.New("nested pools are not supported")
			}
			if !validID(m.ID) || ids[m.ID] || m.Weight < 0 || m.Weight > 1000000 {
				return errors.New("invalid pool member")
			}
			ids[m.ID] = true
			sum += m.Weight
			if !modelIdentifier.MatchString(p.MemberModel(m)) {
				return errors.New("invalid pool model alias")
			}
			weights[p.MemberModel(m)] += m.Weight
			if e := c.validateTarget(m.SourceID, m.Model, p.Agent); e != nil {
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
	seen = map[string]bool{}
	for i := range c.Clients {
		v := &c.Clients[i]
		if err := v.Quota.validate(); err != nil {
			return err
		}
		if v.EnabledAgents == nil {
			v.EnabledAgents = []string{"codex", "claude-code"}
		}
		if v.Routes == nil {
			v.Routes = []Route{}
		}
		if !validID(v.ID) || seen[v.ID] || v.Name == "" {
			return errors.New("invalid client")
		}
		seen[v.ID] = true
		agents := map[string]bool{}
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
			if !validID(route.ID) || ids[route.ID] || !clientAgentValid(route.Agent) || (route.MatchModel != "*" && !modelIdentifier.MatchString(route.MatchModel)) {
				return errors.New("invalid client route")
			}
			ids[route.ID] = true
			if e := c.validateTarget(route.SourceID, route.Model, route.Agent); e != nil {
				return e
			}
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
func (c *Config) validateTarget(id, model, agent string) error {
	if agent == peerGrant {
		for _, protocol := range []string{"codex", "claude-code"} {
			if c.validateTarget(id, model, protocol) == nil {
				return nil
			}
		}
		return errors.New("PacaRoute target must expose this model for Codex or Claude Code")
	}
	if strings.HasPrefix(id, "pool:") {
		p, ok := c.Pool(id)
		if !ok || p.Agent != agent || !p.HasModel(model) {
			return errors.New("route must match the pool Agent and model")
		}
		return nil
	}
	s, ok := c.Source(id)
	if !ok || !modelIdentifier.MatchString(model) {
		return errors.New("unknown source or invalid model")
	}
	if s.Kind == "pacaroute" && !s.SupportsAgentModel(agent, model) {
		return errors.New("remote PacaRoute does not expose this model for the selected Agent; refresh its model catalog")
	}
	if s.Kind != "copilot" {
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
