package pacaroute

import (
	"net/http"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func (s *Server) management(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == "login/ticket" && r.Method == "POST" {
		s.ticketLogin(w, r)
		return
	}
	if path == "login" && r.Method == "POST" {
		if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
			problem(w, 403, "same origin required")
			return
		}
		var v struct {
			Key string `json:"key"`
		}
		if decodeRequest(w, r, &v) != nil || !constantEqual([]byte(v.Key), []byte(s.AdminKey)) {
			problem(w, 401, "invalid management key")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "pacaroute_admin", Value: s.AdminKey, Path: "/api", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(s.PublicURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if !s.admin(r) {
		problem(w, 401, "management authentication required")
		return
	}
	if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
		problem(w, 403, "same origin required")
		return
	}
	switch {
	case path == "config/export" || path == "config/import":
		s.configTransferAPI(w, r, strings.TrimPrefix(path, "config/"))
	case strings.HasPrefix(path, "local/"):
		s.localAPI(w, r, strings.TrimPrefix(path, "local/"))
	case path == "logout" && r.Method == "POST":
		http.SetCookie(w, &http.Cookie{Name: "pacaroute_admin", Value: "", Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		writeJSON(w, 200, map[string]bool{"ok": true})
	case path == "service":
		s.serviceAPI(w, r)
	case path == "status" && r.Method == "GET":
		s.statusAPI(w, r)
	case path == "config" && r.Method == "GET":
		writeJSON(w, 200, s.Store.Snapshot())
	case path == "config" && r.Method == "PUT":
		s.saveConfig(w, r)
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/quota") && r.Method == "GET":
		s.clientQuota(w, r, path)
	case path == "clients" && r.Method == "POST":
		s.createClient(w, r)
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/rotate") && r.Method == "POST":
		s.rotateClient(w, r, path)
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/key") && (r.Method == "GET" || r.Method == "PUT"):
		s.clientKeyAPI(w, r, path)
	case strings.HasPrefix(path, "sources/") && strings.HasSuffix(path, "/pause") && r.Method == "POST":
		s.pauseSource(w, r, path)
	case strings.HasPrefix(path, "keys/") && r.Method == "PUT":
		s.saveSourceKey(w, r, path)
	case strings.HasPrefix(path, "sources/") && strings.HasSuffix(path, "/discover-models") && r.Method == "GET":
		s.discoverSourceModels(w, r, path)
	case path == "model-discovery" && r.Method == "POST":
		s.discoverDraftModels(w, r)
	case path == "models" && r.Method == "GET":
		s.sourceModels(w, r)
	case path == "usage" && r.Method == "GET":
		s.usageAPI(w, r)
	case (path == "errors" || path == "activity") && r.Method == "GET":
		writeJSON(w, 200, s.Activity.Recent(path == "errors"))
	case strings.HasPrefix(path, "accounts/"):
		s.accountAPI(w, r, strings.TrimPrefix(path, "accounts/"))
	case strings.HasPrefix(path, "logins/") && r.Method == "GET":
		s.loginStatus(w, r, path)
	default:
		problem(w, 404, "management endpoint not found")
	}
}

// statusAPI reports service, routing health and listener state.
func (s *Server) statusAPI(w http.ResponseWriter, r *http.Request) {
	service := map[string]any{"serving": true}
	if s.Desktop != nil {
		service = s.Desktop.status()
	}
	writeJSON(w, 200, map[string]any{"name": "PacaRoute", "version": Version, "uptime_seconds": int(time.Since(s.started).Seconds()), "config_error": s.Store.Error(), "routing": s.Router.Health(s.Store.Snapshot()), "local_access": s.AllowLocal, "local_management": s.localAllowed(r), "desktop": s.Desktop != nil && s.Desktop.hasNativeTray() && s.localAllowed(r), "service_control": s.Desktop != nil, "service": service, "port": s.servicePort(), "api_version": 1, "gateway_url": s.gatewayOrigin(r), "startup": s.Desktop != nil && s.Desktop.hasNativeTray() && s.Desktop.startupEnabled()})
}

// saveConfig commits a full graph edit under its expected revision.
func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	var c domain.Config
	if e := decodeRequest(w, r, &c); e != nil {
		problem(w, 400, e.Error())
		return
	}
	if c.Revision == "" {
		problem(w, 428, "configuration revision required")
		return
	}
	if e := s.Store.Save(c, c.Revision); e != nil {
		problem(w, 409, e.Error())
		return
	}
	writeJSON(w, 200, s.Store.Snapshot())
}

func (s *Server) usageAPI(w http.ResponseWriter, r *http.Request) {
	to := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
	from := to.Add(-24 * time.Hour)
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if v := r.URL.Query().Get(key); v != "" {
			t, e := time.Parse(time.RFC3339, v)
			if e != nil {
				problem(w, 400, "timestamps must be RFC3339")
				return
			}
			*dst = t.UTC().Truncate(time.Hour)
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		problem(w, 400, "choose a range up to one year")
		return
	}
	writeJSON(w, 200, s.Activity.Summary(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("client")))
}
