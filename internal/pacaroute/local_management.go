package pacaroute

import (
	"net/http"

	"github.com/LandoWang/pacaroute/internal/pacaroute/clientconfig"
	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func (s *Server) localAllowed(r *http.Request) bool {
	return s.LocalManagement && s.PublicURL == "" && loopbackRequest(r)
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
		if e := s.Store.Snapshot().ValidateTarget(v.Source, v.Model, domain.GrantKind(v.Client)); e != nil {
			problem(w, 400, e.Error())
			return
		}
		plan, e := clientconfig.PlanLocal(v.Client, v.Source, v.Model, s.servicePort())
		if e != nil {
			problem(w, 400, e.Error())
			return
		}
		result := map[string]any{"path": plan.Path, "fingerprint": plan.Fingerprint, "changes": plan.Changes}
		if v.Apply {
			backup, e := plan.Apply(v.Fingerprint)
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
