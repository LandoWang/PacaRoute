package pacaroute

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Credentials are versioned by the committed hash. Failed config commits and
// concurrent rotations cannot overwrite the key for the active snapshot.
func (a *Auth) clientKeyPath(c domain.Client) (string, error) {
	h, err := hex.DecodeString(c.KeyHash)
	if !domain.ValidID(c.ID) || c.Local || err != nil || len(h) != 32 {
		return "", errors.New("invalid network Client credential")
	}
	return filepath.Join(a.vault.root, "client-keys", c.ID, c.KeyHash+".dat"), nil
}

func (a *Auth) SaveClientKey(ctx context.Context, c domain.Client, key string) error {
	path, err := a.clientKeyPath(c)
	if err != nil {
		return err
	}
	if key == "" || !validSecret(key) || !constantEqual([]byte(keyHash(key)), []byte(c.KeyHash)) {
		return errors.New("key does not match this Client")
	}
	unlock, err := fileLock(ctx, path+lockSuffix, credentialLockWait)
	if err != nil {
		return err
	}
	defer unlock()
	return a.vault.write(path, map[string]string{"key": key})
}

func (a *Auth) ClientKey(c domain.Client) (string, error) {
	if c.Local {
		return "", nil
	}
	path, err := a.clientKeyPath(c)
	if err != nil {
		return "", err
	}
	var record map[string]string
	if err = a.vault.read(path, &record); os.IsNotExist(err) {
		return "", nil // Legacy Clients only stored a one-way hash.
	} else if err != nil {
		return "", errors.New("cannot read Client credential")
	}
	key := record["key"]
	if key == "" || !validSecret(key) || !constantEqual([]byte(keyHash(key)), []byte(c.KeyHash)) {
		return "", errors.New("stored key does not match this Client")
	}
	return key, nil
}

func keyHash(key string) string { h := sha256.Sum256([]byte(key)); return hex.EncodeToString(h[:]) }
func newKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "pr_" + hex.EncodeToString(b)
}

func keyEqual(hash, key string) bool {
	expected, e := hex.DecodeString(hash)
	if e != nil {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	return len(expected) == 32 && constantEqual(expected, sum[:])
}

func constantEqual(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }
