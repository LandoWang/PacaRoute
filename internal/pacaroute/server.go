package pacaroute

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/quota"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

const Version = "0.9.0"

type Server struct {
	Store           *ConfigStore
	Auth            *Auth
	Catalog         *catalog
	Router          *routing.Router
	Activity        *usage.ActivityStore
	Quotas          *quota.QuotaStore
	AllowLocal      bool
	LocalManagement bool
	Port            int
	Desktop         serviceControl
	tickets         browserTickets
	AdminKey        string
	PublicURL       string
	TraceDir        string
	started         time.Time
	loginJobs       *loginJobs
}

func NewServer(store *ConfigStore, auth *Auth) (*Server, error) {
	r, e := routing.NewRouter(store.Root)
	if e != nil {
		return nil, e
	}
	adminPath := filepath.Join(store.Root, "admin.key")
	unlock, e := atomicfile.Lock(context.Background(), adminPath+".lock", 5*time.Second)
	if e != nil {
		return nil, e
	}
	defer unlock()
	key, e := os.ReadFile(adminPath)
	if os.IsNotExist(e) {
		key = []byte(newKey())
		e = atomicfile.Write(adminPath, key, 0600)
	}
	if e != nil {
		return nil, e
	}
	if len(key) < 32 {
		return nil, errors.New("invalid management key")
	}
	quotas, e := quota.NewQuotaStore(store.db)
	if e != nil {
		return nil, e
	}
	var models *catalog
	if auth != nil {
		models = newCatalog(auth.config, auth, &auth.http)
	}
	return &Server{Quotas: quotas, Store: store, Auth: auth, Catalog: models, Router: r, Activity: usage.NewActivityStore(store.Root), AdminKey: string(key), started: time.Now(), loginJobs: newLoginJobs()}, nil
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.management(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/mark.svg" || strings.HasPrefix(r.URL.Path, "/assets/") {
			s.web(w, r)
			return
		}
		if r.URL.Path == "/health" {
			writeJSON(w, 200, map[string]any{"name": "PacaRoute", "version": Version, "ready": true})
			return
		}
		if r.URL.Path == "/.well-known/pacaroute" && r.Method == "GET" {
			writeJSON(w, 200, map[string]any{"name": "PacaRoute", "version": Version, "discovery_version": 1, "peer_path": "pacaroute", "models": "pacaroute/v1/models", "agents": map[string]string{"codex": "codex", "claude-code": "cc"}})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/pacaroute/") {
			copy := r.Clone(r.Context())
			copy.URL.Path = strings.TrimPrefix(r.URL.Path, "/pacaroute")
			if copy.URL.Path == "/v1/models" || copy.URL.Path == "/models" {
				s.peerCatalog(w, copy)
			} else {
				s.gatewayFor(w, copy, true)
			}
			return
		}
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			s.peerCatalog(w, r)
			return
		}
		s.gateway(w, r)
	})
}

func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if s.PublicURL != "" && !(s.Desktop != nil && strings.HasPrefix(r.URL.Path, "/api/")) {
		return origin == s.PublicURL
	}
	if origin == "" {
		return false
	}
	u, e := url.Parse(origin)
	if e != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Host == r.Host && u.Scheme == scheme
}
func (s *Server) requestOrigin(r *http.Request) string {
	if s.PublicURL != "" {
		return s.PublicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
