package pacaroute

import (
	"net/http"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func (s *Server) clientQuota(w http.ResponseWriter, r *http.Request, path string) {
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
}

// createClient issues the Client key through Auth before committing the Client.
func (s *Server) createClient(w http.ResponseWriter, r *http.Request) {
	var v domain.Client
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
}

func (s *Server) rotateClient(w http.ResponseWriter, r *http.Request, path string) {
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
}

func (s *Server) clientKeyAPI(w http.ResponseWriter, r *http.Request, path string) {
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
}
