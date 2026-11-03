package pacaroute

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
		return Account{ID: id, Name: "GitHub Copilot", Kind: AccountGitHub}, true
	}
	if id == "azure-local" {
		return Account{ID: id, Name: "Local Azure CLI", Kind: AccountAzure}, true
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
		return Source{ID: id, Name: "GitHub Copilot", Kind: SourceCopilot, AccountID: "copilot"}, true
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

// Amounts are decimal USD strings in configuration and integer micro-USD in the
// ledger. A price snapshot travels with every settlement; history is immutable.
type ModelPrice struct {
	Input      string `json:"input_usd"`
	Cached     string `json:"cached_usd"`
	Output     string `json:"output_usd"`
	CacheWrite string `json:"cache_write_usd,omitempty"`
}

var moneyPattern = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,6})?$`)

func moneyMicros(value string) (int64, error) {
	if !moneyPattern.MatchString(value) {
		return 0, errors.New("USD amounts need at most six decimal places")
	}
	parts := strings.SplitN(value, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	return strconv.ParseInt(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
}
func moneyText(micros int64) string { return fmt.Sprintf("%d.%06d", micros/1000000, micros%1000000) }
func (p ModelPrice) validate() error {
	for _, v := range []string{p.Input, p.Cached, p.Output} {
		if _, e := moneyMicros(v); e != nil {
			return e
		}
	}
	if p.CacheWrite != "" {
		_, e := moneyMicros(p.CacheWrite)
		return e
	}
	return nil
}

type ClientQuota struct {
	WindowSeconds int64  `json:"window_seconds"`
	Requests      int64  `json:"requests,omitempty"`
	InputTokens   int64  `json:"input_tokens,omitempty"`
	Cost          string `json:"cost_usd,omitempty"`
}

func (q *ClientQuota) validate() error {
	if q == nil {
		return nil
	}
	if q.WindowSeconds < 60 || q.WindowSeconds > 366*24*3600 || q.Requests < 0 || q.Requests > 1e12 || q.InputTokens < 0 || q.InputTokens > 1e15 {
		return errors.New("invalid Client quota window or limit")
	}
	if q.Cost != "" {
		_, e := moneyMicros(q.Cost)
		return e
	}
	return nil
}
func (q *ClientQuota) enforced() bool {
	if q == nil {
		return false
	}
	cost, _ := moneyMicros(q.Cost)
	return q.Requests > 0 || q.InputTokens > 0 || cost > 0
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func sharedAzure(c *Config, id string) bool {
	account, _ := c.Account(id)
	return id == "" || id == "azure-local" || account.LoginMode == "azure-cli"
}
