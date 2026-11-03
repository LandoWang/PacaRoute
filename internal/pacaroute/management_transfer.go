package pacaroute

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) configTransferAPI(w http.ResponseWriter, r *http.Request, action string) {
	if action == "export" && r.Method == "GET" {
		w.Header().Set("Content-Disposition", `attachment; filename="PacaRoute-config.json"`)
		writeJSON(w, 200, ExportConfig(s.Store.Snapshot()))
		return
	}
	if action != "import" || r.Method != "POST" {
		problem(w, 405, "unsupported configuration transfer operation")
		return
	}
	var input struct {
		Bundle      json.RawMessage `json:"bundle"`
		Mode        string          `json:"mode"`
		Revision    string          `json:"revision"`
		Namespace   string          `json:"namespace"`
		Fingerprint string          `json:"fingerprint"`
		Apply       bool            `json:"apply"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
		problem(w, 400, "invalid import request (maximum 32 MiB)")
		return
	}
	b, err := decodeConfigBundle(input.Bundle)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	current, err := s.Store.Reload()
	if err != nil {
		problem(w, 409, "cannot read current configuration")
		return
	}
	if input.Revision == "" {
		problem(w, 428, "configuration revision required")
		return
	}
	if input.Revision != current.Revision {
		problem(w, 409, "configuration changed; reload and preview again")
		return
	}
	if !input.Apply {
		input.Namespace = importNamespace()
	}
	p, err := planConfigImport(current, b, input.Mode, input.Namespace)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	// Bind the reviewed plan to this service as well as the file, mode and revision.
	p.Fingerprint = keyHash(s.AdminKey + ":config-import:" + p.Fingerprint)
	if !input.Apply {
		writeJSON(w, 200, p)
		return
	}
	if !constantEqual([]byte(input.Fingerprint), []byte(p.Fingerprint)) {
		problem(w, 409, "import preview changed; preview again")
		return
	}
	if err := applyConfigImport(r.Context(), s.Store, s.Auth, p); err != nil {
		problem(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "plan": p})
}
