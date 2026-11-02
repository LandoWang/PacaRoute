package pacaroute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Account struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	OAuthClientID string `json:"oauth_client_id,omitempty"`
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
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Disabled bool    `json:"disabled"`
	KeyHash  string  `json:"key_hash,omitempty"`
	Routes   []Route `json:"routes"`
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
	for _, v := range c.Clients {
		if !validID(v.ID) || seen[v.ID] || v.Name == "" {
			return errors.New("invalid client")
		}
		seen[v.ID] = true
		if h, e := hex.DecodeString(v.KeyHash); e != nil || len(h) != 32 {
			return errors.New("client requires a hashed API key")
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

type ConfigStore struct {
	Root      string
	mu        sync.Mutex
	current   *Config
	stamp     string
	lastError string
}

var configFiles = []string{"accounts.json", "sources.json", "source-pools.json", "proxies.json", "clients.json", "source-state.json"}

func NewConfigStore(root string) (*ConfigStore, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	s := &ConfigStore{Root: root}
	_, e := s.Reload()
	return s, e
}
func (s *ConfigStore) fingerprint() string {
	h := sha256.New()
	for _, n := range configFiles {
		f, e := os.Stat(filepath.Join(s.Root, n))
		if e != nil {
			fmt.Fprint(h, n, "missing")
		} else {
			fmt.Fprint(h, n, f.Size(), f.ModTime().UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *ConfigStore) Snapshot() *Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.reloadLocked(); e != nil {
		s.lastError = e.Error()
	}
	return s.current
}
func (s *ConfigStore) Reload() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.reloadLocked()
	return s.current, e
}
func (s *ConfigStore) reloadLocked() error {
	unlock, e := fileLock(context.Background(), filepath.Join(s.Root, "config.lock"), 5*time.Second)
	if e != nil {
		return e
	}
	defer unlock()
	return s.readLocked()
}
func (s *ConfigStore) readLocked() error {
	if e := s.recoverLocked(); e != nil {
		return e
	}
	stamp := s.fingerprint()
	if s.current != nil && stamp == s.stamp {
		return nil
	}
	c := &Config{Proxies: Proxies{AccountProxyID: "system", Bindings: map[string]string{}}}
	parts := []any{&c.Accounts, &c.Sources, &c.Pools, &c.Proxies, &c.Clients, &c.SourceState}
	for i, n := range configFiles {
		b, e := os.ReadFile(filepath.Join(s.Root, n))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if len(b) > 4<<20 {
			return errors.New("configuration file too large")
		}
		if e = json.Unmarshal(b, parts[i]); e != nil {
			s.lastError = "Invalid " + n
			return errors.New(s.lastError)
		}
	}
	if e := c.Validate(); e != nil {
		s.lastError = e.Error()
		return e
	}
	if stamp != s.fingerprint() {
		return errors.New("configuration changed while reading; retry")
	}
	c.Revision = stamp
	s.current = c
	s.stamp = stamp
	s.lastError = ""
	return nil
}

// Save serializes CLI/API writers and commits a recoverable transaction before replacing split files.
func (s *ConfigStore) Save(c Config, expected string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := fileLock(context.Background(), filepath.Join(s.Root, "config.lock"), 5*time.Second)
	if e != nil {
		return e
	}
	defer unlock()
	if e = s.readLocked(); e != nil {
		return e
	}
	if expected != "" && expected != s.current.Revision {
		return errors.New("configuration changed; reload before saving")
	}
	if e = c.Validate(); e != nil {
		return e
	}
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	if e = atomicWrite(filepath.Join(s.Root, "config.transaction.json"), b, 0600); e != nil {
		return e
	}
	if e = s.recoverLocked(); e != nil {
		return e
	}
	s.stamp = ""
	return s.readLocked()
}
func (s *ConfigStore) recoverLocked() error {
	path := filepath.Join(s.Root, "config.transaction.json")
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var c Config
	if e = json.Unmarshal(b, &c); e != nil {
		return e
	}
	if e = c.Validate(); e != nil {
		return e
	}
	parts := []any{c.Accounts, c.Sources, c.Pools, c.Proxies, c.Clients, c.SourceState}
	for i, n := range configFiles {
		b, e := json.MarshalIndent(parts[i], "", "  ")
		if e != nil {
			return e
		}
		if e = atomicWrite(filepath.Join(s.Root, n), append(b, '\n'), 0600); e != nil {
			return e
		}
	}
	return os.Remove(path)
}
func (s *ConfigStore) Error() string { s.mu.Lock(); defer s.mu.Unlock(); return s.lastError }
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
