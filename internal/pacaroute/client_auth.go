package pacaroute

import (
	"net/http"
	"strings"
)

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
	if s.AllowLocal && s.PublicURL == "" && loopbackRequest(r) && r.Header.Get("Forwarded") == "" && r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Forwarded-Host") == "" && localKey {
		if local, exists := c.Client("default"); exists && local.Local && !local.Disabled {
			return local, true
		}
	}
	return nil, false
}
