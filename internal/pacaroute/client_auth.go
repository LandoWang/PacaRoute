package pacaroute

import (
	"net"
	"net/http"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return r.Header.Get("x-api-key")
}
func (s *Server) authenticate(r *http.Request, c *domain.Config) (*domain.Client, bool) {
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

func loopbackRequest(r *http.Request) bool {
	peer, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(peer)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	ip = net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
