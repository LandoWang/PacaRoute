package pacaroute

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Account struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	OAuthClientID string `json:"oauth_client_id,omitempty"`
	LoginMode     string `json:"login_mode,omitempty"`
}
type Source struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Endpoint       string   `json:"endpoint"`
	AuthMode       string   `json:"auth_mode"`
	AccountID      string   `json:"account_id,omitempty"`
	SubscriptionID string   `json:"subscription_id,omitempty"`
	ResourceID     string   `json:"resource_id,omitempty"`
	Deployments    []string `json:"deployments"`
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
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Model    string `json:"model"`
	Weight   int    `json:"weight"`
}
type Pool struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Agent    string   `json:"agent"`
	Model    string   `json:"model"`
	Failover bool     `json:"failover"`
	Members  []Member `json:"members"`
}
type Route struct {
	ID         string `json:"id"`
	Agent      string `json:"agent"`
	MatchModel string `json:"match_model"`
	SourceID   string `json:"source_id"`
	Model      string `json:"model"`
}
type Client struct {
	Local         bool     `json:"local,omitempty"`
	LegacyDirect  bool     `json:"legacy_direct,omitempty"`
	EnabledAgents []string `json:"enabled_agents"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Disabled      bool     `json:"disabled"`
	KeyHash       string   `json:"key_hash,omitempty"`
	Routes        []Route  `json:"routes"`
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
	if id == "copilot" {
		return Account{ID: id, Name: "GitHub Copilot", Kind: "github"}, true
	}
	if id == "azure-local" {
		return Account{ID: id, Name: "Local Azure CLI", Kind: "azure"}, true
	}
	for _, a := range c.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	return Account{}, false
}
func (c *Config) Source(id string) (Source, bool) {
	if id == "copilot" {
		return Source{ID: id, Name: "GitHub Copilot", Kind: "copilot", AccountID: "copilot"}, true
	}
	for _, s := range c.Sources {
		if s.ID == id {
			return s, true
		}
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

	seen := map[string]bool{"copilot": true, "azure-local": true}
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
		seen[a.ID] = true
	}
	seen = map[string]bool{"copilot": true}
	for i := range c.Sources {
		s := &c.Sources[i]
		if s.Kind == "" {
			s.Kind = "azure-openai"
		}
		if s.AuthMode == "" {
			if s.Kind == "openai" || s.Kind == "anthropic" {
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
		if s.Kind != "azure-openai" && s.Kind != "openai" && s.Kind != "anthropic" {
			return fmt.Errorf("unsupported source kind %q", s.Kind)
		}
		if (s.Kind == "openai" || s.Kind == "anthropic") && (s.AuthMode != "api-key" || !sourceAcceptsAccount(s.Kind, a.Kind)) {
			return errors.New("API source requires API-key account and authentication")
		}
		if s.Deployments == nil {
			s.Deployments = []string{}
		}
		if s.Kind == "azure-openai" && (a.Kind != "azure" || (s.AuthMode != "azure-cli" && s.AuthMode != "api-key")) {
			return errors.New("invalid Azure authentication")
		}
		u, e := url.Parse(s.Endpoint)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
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
		for _, m := range p.Members {
			if strings.HasPrefix(m.SourceID, "pool:") {
				return errors.New("nested pools are not supported")
			}
			if !validID(m.ID) || ids[m.ID] || m.Weight < 0 || m.Weight > 1000000 {
				return errors.New("invalid pool member")
			}
			ids[m.ID] = true
			sum += m.Weight
			if e := c.validateTarget(m.SourceID, m.Model, p.Agent); e != nil {
				return e
			}
		}
		if sum == 0 {
			return errors.New("pool weights must have a positive total")
		}
	}
	seen = map[string]bool{}
	for i := range c.Clients {
		v := &c.Clients[i]
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
			if !agentValid(agent) || agents[agent] {
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
			if !validID(route.ID) || ids[route.ID] || !agentValid(route.Agent) || (route.MatchModel != "*" && !modelIdentifier.MatchString(route.MatchModel)) {
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
func agentValid(a string) bool { return a == "codex" || a == "claude-code" }
func (c *Config) validateTarget(id, model, agent string) error {
	if strings.HasPrefix(id, "pool:") {
		p, ok := c.Pool(id)
		if !ok || p.Agent != agent || model != p.Model {
			return errors.New("route must match the pool Agent and model")
		}
		return nil
	}
	s, ok := c.Source(id)
	if !ok || !modelIdentifier.MatchString(model) {
		return errors.New("unknown source or invalid model")
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

func (c *Config) Client(id string) (*Client, bool) {
	for _, v := range c.Clients {
		if v.ID == id {
			copy := v
			return &copy, true
		}
	}
	return nil, false
}
func (c *Client) AgentEnabled(agent string) bool {
	if c == nil || c.Disabled {
		return false
	}
	if c.EnabledAgents == nil {
		return agentValid(agent)
	}
	for _, value := range c.EnabledAgents {
		if value == agent {
			return true
		}
	}
	return false
}

func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".pacaroute-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func cloneConfig(c *Config) Config {
	b, _ := json.Marshal(c)
	var v Config
	_ = json.Unmarshal(b, &v)
	return v
}

func accountUsesAPIKey(kind string) bool {
	return kind == "openai" || kind == "anthropic" || kind == "api-key"
}
func sourceAcceptsAccount(source, account string) bool {
	switch source {
	case "copilot":
		return account == "github"
	case "azure-openai":
		return account == "azure"
	case "openai", "anthropic":
		return account == source || account == "api-key"
	}
	return false
}
