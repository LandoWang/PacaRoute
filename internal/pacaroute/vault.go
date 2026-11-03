package pacaroute

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Compatible with the existing Unix encrypted store. Never exports plaintext records.
type vault struct {
	root     string
	once     sync.Once
	enc, mac []byte
	err      error
}

func (v *vault) init() {
	v.once.Do(func() {
		var machine string
		if runtime.GOOS == "darwin" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			b, e := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
			if e != nil {
				v.err = errors.New("machine identity unavailable")
				return
			}
			m := regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`).FindSubmatch(b)
			if len(m) > 1 {
				machine = string(m[1])
			}
		} else {
			for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
				b, e := os.ReadFile(p)
				if e == nil {
					machine = strings.TrimSpace(string(b))
					break
				}
			}
		}
		// Minimal containers may have no OS machine-id. Keep a stable per-install identity in the persistent data volume.
		if machine == "" && runtime.GOOS != "darwin" && v.root != "" {
			path := filepath.Join(v.root, "machine-id")
			unlock, e := fileLock(context.Background(), path+lockSuffix, 5*time.Second)
			if e != nil {
				v.err = e
				return
			}
			defer unlock()
			b, e := os.ReadFile(path)
			if os.IsNotExist(e) {
				b = []byte(newID())
				e = atomicWrite(path, b, 0600)
			}
			if e != nil {
				v.err = e
				return
			}
			machine = strings.TrimSpace(string(b))
		}
		u, e := user.Current()
		if e != nil || machine == "" {
			v.err = errors.New("machine/user identity unavailable")
			return
		}
		v.enc, v.mac = deriveKeys([]byte(machine + "\x1f" + u.Username + "\x1fcopilot-bridge.machinekey.v1"))
	})
}
func deriveKeys(ikm []byte) ([]byte, []byte) {
	salt := []byte("copilot-bridge.tokenstore.hkdf.salt.v1")
	e, _ := hkdf.Key(sha256.New, ikm, salt, "copilot-bridge:aes-256-cbc:v1", 32)
	m, _ := hkdf.Key(sha256.New, ikm, salt, "copilot-bridge:hmac-sha256:v1", 32)
	return e, m
}
func (v *vault) seal(b []byte) ([]byte, error) {
	if out, handled, err := platformSeal(b); handled {
		return out, err
	}
	v.init()
	if v.err != nil {
		return nil, v.err
	}
	block, e := aes.NewCipher(v.enc)
	if e != nil {
		return nil, e
	}
	pad := 16 - len(b)%16
	p := append(bytes.Clone(b), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, 17+len(p)+32)
	out[0] = 1
	if _, e = rand.Read(out[1:17]); e != nil {
		return nil, e
	}
	cipher.NewCBCEncrypter(block, out[1:17]).CryptBlocks(out[17:len(out)-32], p)
	h := hmac.New(sha256.New, v.mac)
	h.Write(out[:len(out)-32])
	copy(out[len(out)-32:], h.Sum(nil))
	return out, nil
}
func (v *vault) open(b []byte) ([]byte, error) {
	if out, handled, err := platformOpen(b); handled {
		return out, err
	}
	v.init()
	if v.err != nil {
		return nil, v.err
	}
	bad := errors.New("encrypted credential could not be read; original file preserved")
	if len(b) < 65 || b[0] != 1 || (len(b)-49)%16 != 0 {
		return nil, bad
	}
	h := hmac.New(sha256.New, v.mac)
	h.Write(b[:len(b)-32])
	if !hmac.Equal(h.Sum(nil), b[len(b)-32:]) {
		return nil, bad
	}
	block, _ := aes.NewCipher(v.enc)
	p := make([]byte, len(b)-49)
	cipher.NewCBCDecrypter(block, b[1:17]).CryptBlocks(p, b[17:len(b)-32])
	n := int(p[len(p)-1])
	if n < 1 || n > 16 || !bytes.Equal(p[len(p)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return nil, bad
	}
	return p[:len(p)-n], nil
}
func (v *vault) read(path string, out any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if len(b) > 1<<20 {
		return errors.New("credential file too large")
	}
	b, e = v.open(b)
	if e != nil {
		return e
	}
	defer clear(b)
	if e = json.Unmarshal(b, out); e != nil {
		return errors.New("invalid encrypted credential payload")
	}
	return nil
}
func (v *vault) write(path string, in any) error {
	b, e := json.Marshal(in)
	if e != nil {
		return e
	}
	defer clear(b)
	b, e = v.seal(b)
	if e != nil {
		return e
	}
	return atomicWrite(path, b, 0600)
}
func (v *vault) githubPath(id string) string {
	if id == "copilot" {
		return filepath.Join(v.root, "runtime", "github_credentials.dat")
	}
	return filepath.Join(v.root, "accounts", id, "github_credentials.dat")
}
func (v *vault) secretPath(id string) string { return filepath.Join(v.root, "secrets", id+".dat") }

func (v *vault) accountKeyPath(id string) string {
	return filepath.Join(v.root, "accounts", id, "api_key.dat")
}
