package pacaroute

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

func keyEqual(hash, key string) bool {
	expected, e := hex.DecodeString(hash)
	if e != nil {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	return len(expected) == 32 && constantEqual(expected, sum[:])
}

func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return r.Header.Get("x-api-key")
}
func (s *Server) authenticate(r *http.Request, c *Config) (*Client, bool) {
	key := bearer(r)
	for _, v := range c.Clients {
		if !v.Local && key != "" && keyEqual(v.KeyHash, key) {
			if v.Disabled {
				return nil, false
			}
			copy := v
			return &copy, true
		}
	}
	// AgentLink's auth provider-token emits a public discovery sentinel, not an
	// account credential. Existing Codex sessions retain that helper after migration.
	localKey := key == "" || key == "dummy" || key == "pacaroute-local" || key == "copilot-bridge-provider"
	if s.AllowLocal && s.PublicURL == "" && loopbackRequest(r) && !forwardedRequest(r) && localKey {
		if local, exists := c.Client("default"); exists && local.Local && !local.Disabled {
			return local, true
		}
	}
	return nil, false
}

// Any forwarding field marks a proxied request, even when empty or repeated:
// a proxy may append its own field after an empty caller-supplied one.
func forwardedRequest(r *http.Request) bool {
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host"} {
		if _, ok := r.Header[name]; ok {
			return true
		}
	}
	return false
}
