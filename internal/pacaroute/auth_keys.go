package pacaroute

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

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
	account, ok := a.config.Snapshot().Account(id)
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
