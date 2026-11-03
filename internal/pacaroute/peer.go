package pacaroute

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const peerHopHeader = "X-PacaRoute-Hops"
const maxPeerHops = 8

type peerModel struct {
	ID     string   `json:"id"`
	Object string   `json:"object"`
	Agents []string `json:"supported_agents"`
}

// Public catalogs disclose only aliases this authenticated Client can request,
// never account identities, pool membership, concrete deployments or credentials.
func (s *Server) peerCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		problem(w, 405, "GET required")
		return
	}
	if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
		problem(w, 403, "same origin required")
		return
	}
	c := s.Store.Snapshot()
	client, ok := s.authenticate(r, c)
	if !ok {
		problem(w, 401, "valid Client API key required")
		return
	}
	if !client.AgentEnabled(GrantPeer) {
		problem(w, 404, "endpoint not found")
		return
	}
	models := map[string]*peerModel{}
	for _, agent := range []AgentProtocol{AgentCodex, AgentClaudeCode} {
		for _, route := range c.peerClient(client, agent).Routes {
			name := route.MatchModel
			if name == "*" {
				name = route.Model
			}
			entry := models[name]
			if entry == nil {
				entry = &peerModel{ID: name, Object: "model", Agents: []string{}}
				models[name] = entry
			}
			found := false
			for _, agent := range entry.Agents {
				found = found || agent == string(route.Agent)
			}
			if !found {
				entry.Agents = append(entry.Agents, string(route.Agent))
			}
		}
	}
	data := []peerModel{}
	for _, entry := range models {
		sort.Strings(entry.Agents)
		data = append(data, *entry)
	}
	sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
	writeJSON(w, 200, map[string]any{"object": "list", "data": data})
}

// Project only peer grants onto the wire protocol requested by a downstream router.
// Direct Agent grants remain independent, including their sticky-session scope.
func (c *Config) peerClient(client *Client, agent AgentProtocol) *Client {
	copy := *client
	copy.LegacyDirect = false
	copy.EnabledAgents = []GrantKind{agent.Grant()}
	copy.Routes = nil
	for _, route := range client.Routes {
		if route.Agent == GrantPeer && c.validateTarget(route.SourceID, route.Model, agent.Grant()) == nil {
			route.Agent = agent.Grant()
			copy.Routes = append(copy.Routes, route)
		}
	}
	return &copy
}

func (a *Auth) discoverPeer(ctx context.Context, c *Config, source Source) (modelDiscovery, error) {
	out := modelDiscovery{Models: []Model{}}
	base := strings.TrimRight(source.Endpoint, "/")
	namespaced := strings.HasSuffix(base, "/pacaroute")
	if !namespaced {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/.well-known/pacaroute", nil)
		res, err := a.http.client(c.SourceProxy(source.ID)).Do(req)
		if err == nil {
			var descriptor struct {
				PeerPath string `json:"peer_path"`
			}
			if res.StatusCode == 200 && readJSONBody(res.Body, &descriptor) == nil && descriptor.PeerPath == "pacaroute" {
				base += "/pacaroute"
				namespaced = true
			}
			res.Body.Close()
		}
	}
	source.Endpoint = base
	out.Endpoint = base
	l, err := a.Lease(ctx, c, source, false)
	if err != nil {
		return out, err
	}
	get := func(path string) ([]peerModel, int, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(source.Endpoint, "/")+path, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set(l.Header, l.Value)
		res, err := a.http.client(c.SourceProxy(source.ID)).Do(req)
		if err != nil {
			return nil, 0, errors.New("cannot reach remote PacaRoute")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, res.StatusCode, nil
		}
		var body struct {
			Data []peerModel `json:"data"`
		}
		if err := readJSONBody(res.Body, &body); err != nil {
			return nil, 200, errors.New("invalid remote PacaRoute model catalog")
		}
		return body.Data, 200, nil
	}
	entries, status, err := get("/v1/models")
	if err != nil {
		return out, err
	}
	if (status == 404 || status == 403) && !namespaced {
		// Older peers expose catalogs on their per-Agent endpoints. A 403 for
		// one Agent means that Client has no permission for that Agent.
		entries = nil
		for _, item := range []struct{ agent, path string }{{"codex", "/codex/models"}, {"claude-code", "/cc/models"}} {
			items, code, err := get(item.path)
			if err != nil {
				return out, err
			}
			if code == 403 || code == 404 {
				continue
			}
			if code != 200 {
				return out, errors.New("remote PacaRoute rejected discovery; use its Client key, not its management key")
			}
			for _, model := range items {
				model.Agents = []string{item.agent}
				entries = append(entries, model)
			}
		}
	} else if status != 200 {
		return out, errors.New("remote PacaRoute rejected discovery; use its Client key, not its management key")
	}
	models := map[string]Model{}
	for _, entry := range entries {
		if !modelIdentifier.MatchString(entry.ID) {
			continue
		}
		m := models[entry.ID]
		m.ID, m.Name, m.Vendor = entry.ID, entry.ID, "PacaRoute"
		for _, agent := range entry.Agents {
			m.Codex = m.Codex || agent == "codex"
			m.Claude = m.Claude || agent == "claude-code"
		}
		if m.Codex || m.Claude {
			models[m.ID] = m
		}
	}
	for _, m := range models {
		out.Models = append(out.Models, m)
	}
	sort.Slice(out.Models, func(i, j int) bool { return out.Models[i].ID < out.Models[j].ID })
	return out, nil
}

func peerHops(r *http.Request) (int, error) {
	if len(r.Header.Values(peerHopHeader)) > 1 {
		return 0, errors.New("invalid PacaRoute hop header")
	}
	v := r.Header.Get(peerHopHeader)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, errors.New("invalid PacaRoute hop count")
	}
	return n, nil
}
