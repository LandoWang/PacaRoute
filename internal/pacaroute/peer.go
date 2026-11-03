package pacaroute

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
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
