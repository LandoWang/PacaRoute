package pacaroute

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type browserTickets struct {
	mu     sync.Mutex
	values map[string]time.Time
}

func (t *browserTickets) issue() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.values == nil {
		t.values = map[string]time.Time{}
	}
	for key, expiry := range t.values {
		if time.Now().After(expiry) {
			delete(t.values, key)
		}
	}
	if len(t.values) >= 128 {
		t.values = map[string]time.Time{}
	}
	key := newKey()
	t.values[keyHash(key)] = time.Now().Add(time.Minute)
	return key
}
func (t *browserTickets) consume(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	hash := keyHash(key)
	expiry, ok := t.values[hash]
	delete(t.values, hash)
	return ok && time.Now().Before(expiry)
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
func (s *Server) localAllowed(r *http.Request) bool {
	return s.LocalManagement && s.PublicURL == "" && loopbackRequest(r)
}
func (s *Server) ticketLogin(w http.ResponseWriter, r *http.Request) {
	if !s.localAllowed(r) || !s.sameOrigin(r) {
		problem(w, 403, "local browser login requires the loopback origin")
		return
	}
	var input struct {
		Ticket string `json:"ticket"`
	}
	if decodeRequest(w, r, &input) != nil || !s.tickets.consume(input.Ticket) {
		problem(w, 401, "browser link expired or already used; open management from the tray again")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "pacaroute_admin", Value: s.AdminKey, Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) localAPI(w http.ResponseWriter, r *http.Request, path string) {
	if !s.localAllowed(r) {
		problem(w, 403, "local management is available only on an explicitly enabled loopback service")
		return
	}
	switch {
	case path == "browser-ticket" && r.Method == "POST":
		writeJSON(w, 200, map[string]string{"ticket": s.tickets.issue()})
	case path == "agents" && r.Method == "GET":
		writeJSON(w, 200, map[string]any{"clients": []any{clientState("codex", s.servicePort()), clientState("claude-code", s.servicePort())}})
	case path == "configure" && r.Method == "POST":
		var v struct {
			Client      string `json:"client"`
			Source      string `json:"source"`
			Model       string `json:"model"`
			Fingerprint string `json:"fingerprint"`
			Apply       bool   `json:"apply"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid configuration request")
			return
		}
		if !s.AllowLocal {
			problem(w, 409, "local Agent configuration requires --allow-local")
			return
		}
		if e := s.Store.Snapshot().validateTarget(v.Source, v.Model, GrantKind(v.Client)); e != nil {
			problem(w, 400, e.Error())
			return
		}
		plan, e := planClient(v.Client, v.Source, v.Model, s.servicePort())
		if e != nil {
			problem(w, 400, e.Error())
			return
		}
		result := map[string]any{"path": plan.path, "fingerprint": plan.fingerprint, "changes": plan.changes}
		if v.Apply {
			backup, e := plan.apply(v.Fingerprint)
			if e != nil {
				problem(w, 409, e.Error())
				return
			}
			result["backup_path"] = backup
		}
		writeJSON(w, 200, result)
	case path == "backups" && r.Method == "GET":
		v, e := listBackups(r.URL.Query().Get("agent"))
		if e != nil {
			problem(w, 400, e.Error())
			return
		}
		writeJSON(w, 200, v)
	case path == "restore" && r.Method == "POST":
		var v struct {
			Agent       string `json:"agent"`
			Backup      string `json:"backup"`
			Fingerprint string `json:"fingerprint"`
			Apply       bool   `json:"apply"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid restore request")
			return
		}
		result, e := restoreBackup(v.Agent, v.Backup, v.Fingerprint, v.Apply)
		if e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, result)
	case path == "service" && r.Method == "PUT" && s.Desktop != nil:
		var v struct {
			Port int `json:"port"`
		}
		if decodeRequest(w, r, &v) != nil || v.Port < 1024 || v.Port > 65535 {
			problem(w, 400, "port must be 1024–65535")
			return
		}
		address, e := s.Desktop.changePort(v.Port)
		if e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"url": address})
	case path == "startup" && r.Method == "PUT" && s.Desktop != nil:
		var v struct {
			Enabled bool `json:"enabled"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid startup preference")
			return
		}
		if e := s.Desktop.startup(v.Enabled); e != nil {
			problem(w, 400, e.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"enabled": v.Enabled})
	default:
		problem(w, 404, "local endpoint not found")
	}
}
