package pacaroute

import (
	"net/http"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func (s *Server) pauseSource(w http.ResponseWriter, r *http.Request, path string) {
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
}

func (s *Server) saveSourceKey(w http.ResponseWriter, r *http.Request, path string) {
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
}

func (s *Server) discoverSourceModels(w http.ResponseWriter, r *http.Request, path string) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "sources/"), "/discover-models")
	source, ok := s.Store.Snapshot().Source(id)
	if !ok {
		problem(w, 404, "source not found")
		return
	}
	result, e := s.Catalog.DiscoverModels(r.Context(), s.Store.Snapshot(), source)
	if e != nil {
		problem(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, result)
}

// discoverDraftModels probes an unsaved source draft without committing it.
func (s *Server) discoverDraftModels(w http.ResponseWriter, r *http.Request) {
	var input struct {
		domain.Source
		ProxyID string `json:"proxy_id"`
	}
	if decodeRequest(w, r, &input) != nil {
		problem(w, 400, "invalid source")
		return
	}
	source := input.Source
	draft := cloneConfig(s.Store.Snapshot())
	if !domain.ValidID(source.ID) || source.ID == "copilot" {
		source.ID = "discovery-" + domain.NewID()[:8]
	}
	source.Name = "Model discovery"
	source.Deployments = []string{"discovery"}
	// Validate the draft independently of saved routes. Keep its source ID so
	// an existing resource-specific credential can override the account key.
	draft.Sources = []domain.Source{source}
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
	result, e := s.Catalog.DiscoverModels(r.Context(), &draft, source)
	if e != nil {
		problem(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) sourceModels(w http.ResponseWriter, r *http.Request) {
	models, e := s.Catalog.Models(r.Context(), r.URL.Query().Get("source"))
	if e != nil {
		problem(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"models": models})
}
