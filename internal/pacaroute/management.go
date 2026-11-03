package pacaroute

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
func (s *Server) admin(r *http.Request) bool {
	if key := bearer(r); key != "" {
		return constantEqual([]byte(key), []byte(s.AdminKey))
	}
	cookie, e := r.Cookie("pacaroute_admin")
	if e != nil || !constantEqual([]byte(cookie.Value), []byte(s.AdminKey)) {
		return false
	}
	return r.Method == "GET" || s.sameOrigin(r)
}
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(v); e != nil {
		return errors.New("invalid JSON request")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
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
		service := map[string]any{"serving": true}
		if s.Desktop != nil {
			service = s.Desktop.status()
		}
		writeJSON(w, 200, map[string]any{"name": "PacaRoute", "version": Version, "uptime_seconds": int(time.Since(s.started).Seconds()), "config_error": s.Store.Error(), "routing": s.Router.Health(s.Store.Snapshot()), "local_access": s.AllowLocal, "local_management": s.localAllowed(r), "desktop": s.Desktop != nil && s.Desktop.nativeTray && s.localAllowed(r), "service_control": s.Desktop != nil, "service": service, "port": s.servicePort(), "api_version": 1, "gateway_url": s.gatewayOrigin(r), "startup": s.Desktop != nil && s.Desktop.nativeTray && startupEnabled()})
	case path == "config" && r.Method == "GET":
		writeJSON(w, 200, s.Store.Snapshot())
	case path == "config" && r.Method == "PUT":
		var c Config
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
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/quota") && r.Method == "GET":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "clients/"), "/quota")
		client, ok := s.Store.Snapshot().Client(id)
		if !ok {
			problem(w, 404, "Client not found")
			return
		}
		value, e := s.Quotas.State(id, client.Quota, time.Now().UTC())
		if e != nil {
			problem(w, 503, "Client quota ledger unavailable")
			return
		}
		writeJSON(w, 200, value)
	case path == "clients" && r.Method == "POST":
		var v Client
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid Client")
			return
		}
		key := newKey()
		if v.Local {
			key = ""
			v.KeyHash = ""
		} else {
			v.KeyHash = keyHash(key)
		}
		c := cloneConfig(s.Store.Snapshot())
		c.Clients = append(c.Clients, v)
		if e := c.Validate(); e != nil {
			problem(w, 409, e.Error())
			return
		}
		if !v.Local {
			if e := s.Auth.SaveClientKey(r.Context(), v, key); e != nil {
				problem(w, 500, "cannot save Client credential")
				return
			}
		}
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"client": v, "api_key": key})
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/rotate") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "clients/"), "/rotate")
		c := cloneConfig(s.Store.Snapshot())
		key := newKey()
		found := false
		for i := range c.Clients {
			if c.Clients[i].ID == id && !c.Clients[i].Local {
				c.Clients[i].KeyHash = keyHash(key)
				found = true
			}
		}
		if !found {
			problem(w, 404, "Client not found")
			return
		}
		client, _ := c.Client(id)
		if e := s.Auth.SaveClientKey(r.Context(), *client, key); e != nil {
			problem(w, 500, "cannot save Client credential")
			return
		}
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"api_key": key})
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/key") && (r.Method == "GET" || r.Method == "PUT"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "clients/"), "/key")
		client, ok := s.Store.Snapshot().Client(id)
		if !ok {
			problem(w, 404, "Client not found")
			return
		}
		if r.Method == "PUT" {
			var v struct {
				Key string `json:"key"`
			}
			if decodeRequest(w, r, &v) != nil {
				problem(w, 400, "invalid Client credential")
				return
			}
			if e := s.Auth.SaveClientKey(r.Context(), *client, v.Key); e != nil {
				problem(w, 400, e.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		key, e := s.Auth.ClientKey(*client)
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"api_key": key, "available": key != ""})
	case strings.HasPrefix(path, "sources/") && strings.HasSuffix(path, "/pause") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "sources/"), "/pause")
		var v struct {
			Paused bool `json:"paused"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid pause value")
			return
		}
		c := cloneConfig(s.Store.Snapshot())
		paused := []string{}
		for _, p := range c.SourceState.Paused {
			if p != id {
				paused = append(paused, p)
			}
		}
		if v.Paused {
			paused = append(paused, id)
		}
		c.SourceState.Paused = paused
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, s.Store.Snapshot())
	case strings.HasPrefix(path, "keys/") && r.Method == "PUT":
		id := strings.TrimPrefix(path, "keys/")
		if _, ok := s.Store.Snapshot().Source(id); !ok {
			problem(w, 404, "source not found")
			return
		}
		var v struct {
			Key string `json:"key"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid credential")
			return
		}
		if e := s.Auth.SaveKey(r.Context(), id, v.Key); e != nil {
			problem(w, 400, e.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case strings.HasPrefix(path, "sources/") && strings.HasSuffix(path, "/discover-models") && r.Method == "GET":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "sources/"), "/discover-models")
		source, ok := s.Store.Snapshot().Source(id)
		if !ok {
			problem(w, 404, "source not found")
			return
		}
		result, e := s.Auth.DiscoverModels(r.Context(), s.Store.Snapshot(), source)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, result)
	case path == "model-discovery" && r.Method == "POST":
		var input struct {
			Source
			ProxyID string `json:"proxy_id"`
		}
		if decodeRequest(w, r, &input) != nil {
			problem(w, 400, "invalid source")
			return
		}
		source := input.Source
		draft := cloneConfig(s.Store.Snapshot())
		if !validID(source.ID) || source.ID == "copilot" {
			source.ID = "discovery-" + newID()[:8]
		}
		source.Name = "Model discovery"
		source.Deployments = []string{"discovery"}
		// Validate the draft independently of saved routes. Keep its source ID so
		// an existing resource-specific credential can override the account key.
		draft.Sources = []Source{source}
		draft.Pools = nil
		draft.Clients = nil
		draft.SourceState.Paused = nil
		if input.ProxyID != "" {
			draft.Proxies.Bindings[source.ID] = input.ProxyID
		}
		if e := draft.Validate(); e != nil {
			problem(w, 400, e.Error())
			return
		}
		source = draft.Sources[len(draft.Sources)-1]
		result, e := s.Auth.DiscoverModels(r.Context(), &draft, source)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, result)
	case path == "models" && r.Method == "GET":
		models, e := s.Auth.Models(r.Context(), r.URL.Query().Get("source"))
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"models": models})
	case path == "usage" && r.Method == "GET":
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
	case (path == "errors" || path == "activity") && r.Method == "GET":
		writeJSON(w, 200, s.Activity.Recent(path == "errors"))
	case strings.HasPrefix(path, "accounts/"):
		s.accountAPI(w, r, strings.TrimPrefix(path, "accounts/"))
	case strings.HasPrefix(path, "logins/") && r.Method == "GET":
		s.loginJobs.mu.Lock()
		job := s.loginJobs.jobs[strings.TrimPrefix(path, "logins/")]
		var status map[string]any
		if job != nil {
			status = map[string]any{"done": job.done, "error": job.err}
		}
		s.loginJobs.mu.Unlock()
		if status == nil {
			problem(w, 404, "login not found")
			return
		}
		writeJSON(w, 200, status)
	default:
		problem(w, 404, "management endpoint not found")
	}
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
