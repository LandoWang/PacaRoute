package pacaroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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

// Auth is the only runtime owner of credential records, persistence and rotation.
type Auth struct {
	store                    *ConfigStore
	systemKeys               bool
	vault                    vault
	http                     transports
	mu                       sync.Mutex
	leaseGates               [64]chan struct{}
	keyEpoch                 uint64
	leases                   map[string]cachedLease
	keys                     map[string]string
	session, machine, device string
}

func NewAuth(s *ConfigStore, credentialRoot string) *Auth {
	if credentialRoot == "" {
		credentialRoot = s.Root
	}
	a := &Auth{store: s, vault: vault{root: credentialRoot}, leases: map[string]cachedLease{}, keys: map[string]string{}, session: newID(), machine: newID(), device: newID()}
	for i := range a.leaseGates {
		a.leaseGates[i] = make(chan struct{}, 1)
	}
	return a
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
	key := s.ID + ":" + s.Kind + ":" + s.AccountID + ":" + s.AuthMode + ":" + s.SubscriptionID + ":" + p.Mode + p.Address
	// Bound concurrent refresh coordination without holding the global cache lock
	// across network calls. Unrelated sources can keep using their own credentials.
	var bucket uint32
	for _, ch := range key {
		bucket = bucket*31 + uint32(ch)
	}
	// A waiter whose request is cancelled must not stay queued behind another
	// request's slow exchange or refresh.
	gate := a.leaseGates[bucket%uint32(len(a.leaseGates))]
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return lease{}, ctx.Err()
	}
	defer func() { <-gate }()
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
