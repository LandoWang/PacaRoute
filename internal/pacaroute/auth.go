package pacaroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type credential struct {
	Version        int        `json:"version"`
	AccessToken    string     `json:"access_token"`
	AccessExpires  *time.Time `json:"access_token_expires_at,omitempty"`
	RefreshToken   string     `json:"refresh_token,omitempty"`
	RefreshExpires *time.Time `json:"refresh_token_expires_at,omitempty"`
	TokenType      string     `json:"token_type,omitempty"`
	Scope          string     `json:"scope,omitempty"`
	OAuthClientID  string     `json:"oauth_client_id,omitempty"`
	CredentialID   string     `json:"credential_id"`
	Generation     int64      `json:"generation"`
}
type lease struct {
	Header      string
	Value       string
	Endpoint    string
	Integration string
	Expires     time.Time
}
type cachedLease struct {
	lease lease
	stamp string
}
type exchangeCache struct {
	Token          string    `json:"token"`
	Endpoint       string    `json:"endpoint"`
	Expires        time.Time `json:"expires"`
	CredentialHash string    `json:"credential_hash"`
}

// Auth is the only runtime owner of credential records, persistence and rotation.
type Auth struct {
	store                    *ConfigStore
	systemKeys               bool
	vault                    vault
	http                     transports
	mu                       sync.Mutex
	leaseGates               [64]sync.Mutex
	keyEpoch                 uint64
	leases                   map[string]cachedLease
	keys                     map[string]string
	session, machine, device string
}

func NewAuth(s *ConfigStore, credentialRoot string) *Auth {
	if credentialRoot == "" {
		credentialRoot = s.Root
	}
	return &Auth{store: s, vault: vault{root: credentialRoot}, leases: map[string]cachedLease{}, keys: map[string]string{}, session: newID(), machine: newID(), device: newID()}
}
func (a *Auth) SetKeys(keys map[string]string) error {
	for k, v := range keys {
		if !validID(k) || !validSecret(v) {
			return errors.New("invalid source credential")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range keys {
		if v == "" {
			delete(a.keys, k)
		} else {
			a.keys[k] = v
		}
	}
	a.leases = map[string]cachedLease{}
	a.keyEpoch++
	return nil
}
func validSecret(v string) bool { return len(v) <= 16384 && !strings.ContainsAny(v, "\r\n\x00") }
func (a *Auth) SaveKey(ctx context.Context, id, key string) error {
	if !validID(id) || key == "" || !validSecret(key) {
		return errors.New("invalid API key")
	}
	unlock, e := fileLock(ctx, a.vault.secretPath(id)+".lock", 10*time.Second)
	if e != nil {
		return e
	}
	defer unlock()
	if a.systemKeys {
		if e = saveSystemSourceKey(id, key); e != nil {
			return e
		}
		if e = os.Remove(a.vault.secretPath(id)); e != nil && !os.IsNotExist(e) {
			return e
		}
		if e = atomicWrite(a.vault.secretPath(id)+".revision", []byte(newID()), 0600); e != nil {
			return e
		}
	} else if e = a.vault.write(a.vault.secretPath(id), map[string]string{"key": key}); e != nil {
		return e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.keys, id)
	a.leases = map[string]cachedLease{}
	a.keyEpoch++
	return nil
}
func statStamp(path string) string {
	f, e := os.Stat(path)
	if e != nil {
		return "missing"
	}
	return fmt.Sprint(f.Size(), ":", f.ModTime().UnixNano())
}
func (a *Auth) Lease(ctx context.Context, c *Config, s Source, force bool) (lease, error) {
	p := c.SourceProxy(s.ID)
	key := s.ID + ":" + s.AccountID + ":" + s.AuthMode + ":" + s.SubscriptionID + ":" + p.Mode + p.Address
	// Bound concurrent refresh coordination without holding the global cache lock
	// across network calls. Unrelated sources can keep using their own credentials.
	var bucket uint32
	for _, ch := range key {
		bucket = bucket*31 + uint32(ch)
	}
	gate := &a.leaseGates[bucket%uint32(len(a.leaseGates))]
	gate.Lock()
	defer gate.Unlock()
	stamp := ""
	if s.Kind == "copilot" {
		stamp = statStamp(a.vault.githubPath(s.AccountID))
	} else if s.AuthMode == "api-key" {
		stamp = statStamp(a.vault.secretPath(s.ID)) + statStamp(a.vault.accountKeyPath(s.AccountID)) + statStamp(a.vault.secretPath(s.ID)+".revision") + statStamp(a.vault.accountKeyPath(s.AccountID)+".revision")
	} else {
		dir := a.azureDirectory(c, s.AccountID)
		stamp = dir + statStamp(filepath.Join(dir, "azureProfile.json")) + statStamp(filepath.Join(dir, "msal_token_cache.json"))

	}
	a.mu.Lock()
	epoch := a.keyEpoch
	secret := a.keys[s.ID]
	if v, ok := a.leases[key]; ok && !force && v.stamp == stamp && v.lease.Expires.After(time.Now().Add(time.Minute)) {
		a.mu.Unlock()
		return v.lease, nil
	}
	a.mu.Unlock()
	var l lease
	var e error
	switch {
	case s.Kind == "copilot":
		l, e = a.copilotLocked(ctx, s.AccountID, p, force)
	case s.AuthMode == "api-key":
		if secret == "" {
			var record map[string]string
			if e = a.vault.read(a.vault.secretPath(s.ID), &record); e == nil {
				secret = record["key"]
			} else if !os.IsNotExist(e) {
				return l, e
			}
		}
		if secret == "" && a.systemKeys {
			secret, e = systemSourceKey(s.ID)
			if e != nil && !os.IsNotExist(e) {
				return l, e
			}
			e = nil
		}
		if secret == "" {
			if account, ok := c.Account(s.AccountID); ok && accountUsesAPIKey(account.Kind) {
				secret, e = a.accountKey(s.AccountID)
				if e != nil && !os.IsNotExist(e) {
					return l, e
				}
				e = nil
			}
		}
		if secret == "" {
			return l, errors.New("source API key is missing; save it in PacaRoute")
		}
		l = lease{Header: "Authorization", Value: "Bearer " + secret, Expires: time.Now().Add(24 * time.Hour)}
		if s.Kind == "azure-openai" {
			l.Header = "api-key"
			l.Value = secret
		}
		if s.Kind == "anthropic" {
			l.Header = "x-api-key"
			l.Value = secret
		}
	default:
		args := []string{"account", "get-access-token", "--resource", "https://cognitiveservices.azure.com", "-o", "json"}
		if s.SubscriptionID != "" {
			args = append(args, "--subscription", s.SubscriptionID)
		}
		b, err := commandJSON(ctx, a.azureEnv(c, s.AccountID, p), args...)
		if err != nil {
			return l, err
		}
		var v struct {
			AccessToken string      `json:"accessToken"`
			Expires     json.Number `json:"expires_on"`
		}
		if json.Unmarshal(b, &v) != nil || v.AccessToken == "" {
			return l, errors.New("Azure returned no access token")
		}
		exp := time.Now().Add(10 * time.Minute)
		if n, err := v.Expires.Int64(); err == nil {
			exp = time.Unix(n, 0)
		}
		l = lease{Header: "Authorization", Value: "Bearer " + v.AccessToken, Expires: exp.Add(-4 * time.Minute)}
	}
	if e != nil {
		return l, e
	}
	a.mu.Lock()
	if epoch == a.keyEpoch {
		a.leases[key] = cachedLease{l, stamp}
	}
	a.mu.Unlock()
	return l, nil
}
func sharedAzure(c *Config, id string) bool {
	account, _ := c.Account(id)
	return id == "" || id == "azure-local" || account.LoginMode == "azure-cli"
}
func (a *Auth) azureDirectory(c *Config, id string) string {
	if !sharedAzure(c, id) {
		return filepath.Join(a.vault.root, "accounts", id)
	}
	if dir := os.Getenv("AZURE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".azure")
}
func (a *Auth) azureEnv(c *Config, id string, p Proxy) []string {
	env := proxyEnv(p)
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], "AZURE_CONFIG_DIR=") {
			env = append(env[:i], env[i+1:]...)
		}
	}
	return append(env, "AZURE_CONFIG_DIR="+a.azureDirectory(c, id))
}
func (a *Auth) readCredential(id string) (credential, error) {
	var c credential
	e := a.vault.read(a.vault.githubPath(id), &c)
	if os.IsNotExist(e) {
		return c, errors.New("GitHub account is not logged in")
	}
	if e != nil {
		return c, e
	}
	if c.Version < 1 || c.Version > 4 || c.AccessToken == "" {
		return c, errors.New("unsupported GitHub credential; original record preserved")
	}
	return c, nil
}
func (a *Auth) credentialLocked(ctx context.Context, id string, p Proxy, force bool) (credential, error) {
	c, e := a.readCredential(id)
	if e != nil {
		return c, e
	}
	expired := c.AccessExpires != nil && !c.AccessExpires.After(time.Now().Add(2*time.Minute))
	if !expired && !(force && c.Version == 4 && c.RefreshToken != "") {
		return c, nil
	}
	if c.RefreshToken == "" || (c.RefreshExpires != nil && !c.RefreshExpires.After(time.Now())) {
		return c, errors.New("GitHub login expired; authorize this account again")
	}
	// The caller holds the stable lock and re-read the current generation, so rotated refresh tokens are never copied or raced.
	v, e := a.oauth(ctx, p, url.Values{"client_id": {c.OAuthClientID}, "grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}})
	if e != nil {
		return c, e
	}
	if v.AccessToken == "" {
		return c, errors.New("GitHub refresh rejected; authorize this account again")
	}
	c.AccessToken = v.AccessToken
	c.AccessExpires = expiry(v.ExpiresIn)
	if v.RefreshToken != "" {
		c.RefreshToken = v.RefreshToken
		c.RefreshExpires = expiry(v.RefreshExpiresIn)
	}
	c.Generation++
	if e = a.vault.write(a.vault.githubPath(id), c); e != nil {
		return c, e
	}
	return c, nil
}
func (a *Auth) copilotLocked(ctx context.Context, id string, p Proxy, force bool) (lease, error) {
	path := a.vault.githubPath(id)
	unlock, e := fileLock(ctx, path+".lock", 30*time.Second)
	if e != nil {
		return lease{}, e
	}
	defer unlock()
	c, e := a.credentialLocked(ctx, id, p, force)
	if e != nil {
		return lease{}, e
	}
	integration := "vscode-chat"
	if c.Version == 4 {
		integration = "copilot-developer-cli"
	}
	if c.Version == 2 || c.Version == 4 {
		exp := time.Now().Add(time.Hour)
		if c.AccessExpires != nil {
			exp = *c.AccessExpires
		}
		return lease{Header: "Authorization", Value: "Bearer " + c.AccessToken, Endpoint: "https://api.githubcopilot.com", Integration: integration, Expires: exp}, nil
	}
	cachedPath := filepath.Join(filepath.Dir(path), "pacaroute-exchange.dat")
	hash := keyHash(c.AccessToken)
	var cached exchangeCache
	if !force && a.vault.read(cachedPath, &cached) == nil && cached.CredentialHash == hash && cached.Expires.After(time.Now().Add(2*time.Minute)) {
		return lease{Header: "Authorization", Value: "Bearer " + cached.Token, Endpoint: cached.Endpoint, Integration: integration, Expires: cached.Expires}, nil
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/copilot_internal/v2/token", nil)
	req.Header.Set("Authorization", "token "+c.AccessToken)
	a.copilotHeaders(req, lease{}, false)
	req.Header.Set("Accept", "application/json")
	res, e := a.http.client(p).Do(req)
	if e != nil {
		return lease{}, errors.New("Copilot token exchange connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return lease{}, fmt.Errorf("Copilot token exchange rejected (%d); check account access", res.StatusCode)
	}
	var v struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
		Endpoints struct {
			API string `json:"api"`
		} `json:"endpoints"`
	}
	if e = readJSONBody(res.Body, &v); e != nil || v.Token == "" {
		return lease{}, errors.New("invalid Copilot token exchange")
	}
	endpoint := v.Endpoints.API
	if endpoint == "" {
		endpoint = "https://api.githubcopilot.com"
	}
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "https" || u.User != nil || !(u.Hostname() == "api.githubcopilot.com" || strings.HasSuffix(u.Hostname(), ".githubcopilot.com")) {
		return lease{}, errors.New("untrusted Copilot endpoint")
	}
	exp := time.Unix(v.ExpiresAt, 0)
	if !exp.After(time.Now()) {
		return lease{}, errors.New("Copilot returned an expired lease")
	}
	cached = exchangeCache{v.Token, endpoint, exp, hash}
	if e = a.vault.write(cachedPath, cached); e != nil {
		return lease{}, e
	}
	return lease{Header: "Authorization", Value: "Bearer " + v.Token, Endpoint: endpoint, Integration: integration, Expires: exp}, nil
}
func (a *Auth) copilotHeaders(r *http.Request, l lease, vision bool) {
	r.Header.Set("X-GitHub-Api-Version", "2026-01-09")
	integration := l.Integration
	if integration == "" {
		integration = "vscode-chat"
	}
	r.Header.Set("Copilot-Integration-Id", integration)
	r.Header.Set("Editor-Plugin-Version", "copilot-chat/0.46.0")
	r.Header.Set("Editor-Version", "vscode/1.95.0")
	r.Header.Set("VScode-SessionId", a.session)
	r.Header.Set("VScode-MachineId", a.machine)
	r.Header.Set("Editor-Device-Id", a.device)
	r.Header.Set("User-Agent", "PacaRoute/0.6.0")
	if l.Value != "" {
		r.Header.Set(l.Header, l.Value)
	}
	if vision {
		r.Header.Set("Copilot-Vision-Request", "true")
	}
}

type oauthReply struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_token_expires_in"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
}

func expiry(seconds int) *time.Time {
	if seconds <= 0 {
		return nil
	}
	v := time.Now().Add(time.Duration(seconds) * time.Second)
	return &v
}
func (a *Auth) oauth(ctx context.Context, p Proxy, form url.Values) (oauthReply, error) {
	var v oauthReply
	r, _ := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	res, e := a.http.client(p).Do(r)
	if e != nil {
		return v, errors.New("GitHub OAuth connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return v, fmt.Errorf("GitHub OAuth failed (%d)", res.StatusCode)
	}
	e = readJSONBody(res.Body, &v)
	return v, e
}

type DeviceLogin struct {
	DeviceCode      string `json:"-"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"-"`
	ClientID        string `json:"-"`
	AccountID       string `json:"-"`
	Version         int    `json:"-"`
}

func (a *Auth) BeginLogin(ctx context.Context, id string) (DeviceLogin, error) {
	c := a.store.Snapshot()
	account, ok := c.Account(id)
	if !ok || account.Kind != "github" {
		return DeviceLogin{}, errors.New("GitHub account not found")
	}
	clientID := account.OAuthClientID
	version := 1
	if clientID == "" {
		clientID = "Iv1.b507a08c87ecfe98"
	} else {
		version = 4
	}
	p, _ := c.Proxy(c.Proxies.AccountProxyID)
	form := url.Values{"client_id": {clientID}, "scope": {"read:user"}}
	r, _ := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/device/code", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	res, e := a.http.client(p).Do(r)
	if e != nil {
		return DeviceLogin{}, errors.New("GitHub device login connection failed")
	}
	defer res.Body.Close()
	var v struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri"`
		Expires    int    `json:"expires_in"`
		Interval   int    `json:"interval"`
	}
	if res.StatusCode != 200 {
		return DeviceLogin{}, fmt.Errorf("GitHub device login failed (%d)", res.StatusCode)
	}
	if e = readJSONBody(res.Body, &v); e != nil || v.DeviceCode == "" {
		return DeviceLogin{}, errors.New("invalid device login response")
	}
	return DeviceLogin{v.DeviceCode, v.UserCode, v.URI, v.Expires, v.Interval, clientID, id, version}, nil
}
func (a *Auth) CompleteLogin(ctx context.Context, d DeviceLogin) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(d.ExpiresIn)*time.Second)
	defer cancel()
	interval := time.Duration(max(d.Interval, 5)) * time.Second
	for {
		select {
		case <-ctx.Done():
			return errors.New("device login expired or cancelled")
		case <-time.After(interval):
		}
		c := a.store.Snapshot()
		if _, ok := c.Account(d.AccountID); !ok {
			return errors.New("account removed during login")
		}
		p, _ := c.Proxy(c.Proxies.AccountProxyID)
		v, e := a.oauth(ctx, p, url.Values{"client_id": {d.ClientID}, "device_code": {d.DeviceCode}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}})
		if e != nil {
			return e
		}
		if v.Error == "authorization_pending" {
			continue
		}
		if v.Error == "slow_down" {
			interval += 5 * time.Second
			continue
		}
		if v.AccessToken == "" {
			return errors.New("GitHub device login was not authorized")
		}
		record := credential{Version: d.Version, AccessToken: v.AccessToken, AccessExpires: expiry(v.ExpiresIn), RefreshToken: v.RefreshToken, RefreshExpires: expiry(v.RefreshExpiresIn), TokenType: v.TokenType, Scope: v.Scope, OAuthClientID: d.ClientID, CredentialID: strings.ReplaceAll(newID(), "-", "")}
		unlock, e := fileLock(ctx, a.vault.githubPath(d.AccountID)+".lock", 10*time.Second)
		if e != nil {
			return e
		}
		e = a.vault.write(a.vault.githubPath(d.AccountID), record)
		unlock()
		return e
	}
}
func (a *Auth) LoggedIn(id string) bool {
	c, e := a.readCredential(id)
	return e == nil && (c.AccessExpires == nil || c.AccessExpires.After(time.Now()) || c.RefreshToken != "")
}
func (a *Auth) Logout(ctx context.Context, id string) error {
	if !validID(id) {
		return errors.New("invalid account")
	}
	path := a.vault.githubPath(id)
	unlock, e := fileLock(ctx, path+".lock", 10*time.Second)
	if e != nil {
		return e
	}
	defer unlock()
	for _, name := range []string{"github_credentials.dat", "pacaroute-exchange.dat", "github_credentials.v2.dat", "github_token.dat"} {
		e = os.Remove(filepath.Join(filepath.Dir(path), name))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	a.mu.Lock()
	a.leases = map[string]cachedLease{}
	a.keyEpoch++
	a.mu.Unlock()
	return nil
}
func (a *Auth) AccountInfo(ctx context.Context, id string) (map[string]any, error) {
	c := a.store.Snapshot()
	p, _ := c.Proxy(c.Proxies.AccountProxyID)
	unlock, e := fileLock(ctx, a.vault.githubPath(id)+".lock", 30*time.Second)
	if e != nil {
		return nil, e
	}
	record, e := a.credentialLocked(ctx, id, p, false)
	unlock()
	if e != nil {
		return nil, e
	}
	out := map[string]any{"logged_in": true}
	for _, item := range []struct{ path, key string }{{"/user", "user"}, {"/copilot_internal/user", "quota"}} {
		r, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com"+item.path, nil)
		r.Header.Set("Authorization", "Bearer "+record.AccessToken)
		a.copilotHeaders(r, lease{}, false)
		res, e := a.http.client(p).Do(r)
		if e != nil {
			out[item.key+"_error"] = "GitHub connection failed"
			continue
		}
		var body map[string]any
		if res.StatusCode == 200 && readJSONBody(res.Body, &body) == nil {
			if item.key == "user" {
				out["login"] = body["login"]
			} else {
				out["quota"] = body
			}
		} else {
			out[item.key+"_error"] = "GitHub returned " + strconv.Itoa(res.StatusCode)
		}
		res.Body.Close()
	}
	return out, nil
}

func (a *Auth) accountKey(id string) (string, error) {
	var record map[string]string
	e := a.vault.read(a.vault.accountKeyPath(id), &record)
	if e == nil {
		return record["key"], nil
	}
	if !os.IsNotExist(e) {
		return "", e
	}
	if a.systemKeys {
		return systemSourceKey("account." + id)
	}
	return "", e
}
func (a *Auth) HasAccountKey(id string) bool {
	key, e := a.accountKey(id)
	return e == nil && key != ""
}
func (a *Auth) SaveAccountKey(ctx context.Context, id, key string) error {
	account, ok := a.store.Snapshot().Account(id)
	if !ok || !accountUsesAPIKey(account.Kind) || key == "" || !validSecret(key) {
		return errors.New("select a key-based account and a valid API key")
	}
	path := a.vault.accountKeyPath(id)
	unlock, e := fileLock(ctx, path+".lock", 10*time.Second)
	if e != nil {
		return e
	}
	defer unlock()
	if a.systemKeys {
		if e = saveSystemSourceKey("account."+id, key); e != nil {
			return e
		}
		if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
			return e
		}
		if e = atomicWrite(path+".revision", []byte(newID()), 0600); e != nil {
			return e
		}
	} else if e = a.vault.write(path, map[string]string{"key": key}); e != nil {
		return e
	}
	a.mu.Lock()
	a.leases = map[string]cachedLease{}
	a.keyEpoch++
	a.mu.Unlock()
	return nil
}
