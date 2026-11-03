package pacaroute

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type leaseProvider interface {
	Lease(context.Context, *Config, Source, bool) (lease, error)
	ApplyProviderHeaders(*http.Request, lease, bool)
	AzureAction(context.Context, string, string, string, string) (map[string]any, error)
}

type catalog struct {
	config configReader
	leases leaseProvider
	http   *transports
}

func newCatalog(config configReader, leases leaseProvider, http *transports) *catalog {
	return &catalog{config: config, leases: leases, http: http}
}

func (a *catalog) Models(ctx context.Context, id string) ([]Model, error) {
	if id == "" {
		id = "copilot"
	}
	c := a.config.Snapshot()
	if strings.HasPrefix(id, "pool:") {
		p, ok := c.Pool(id)
		if !ok {
			return nil, errors.New("pool not found")
		}
		models := []Model{}
		for _, name := range p.ModelNames() {
			models = append(models, Model{name, name, "Source pool", p.Agent == AgentClaudeCode, p.Agent == AgentCodex})
		}
		return models, nil
	}
	s, ok := c.Source(id)
	if !ok {
		return nil, errors.New("source not found")
	}
	models := []Model{}
	if s.Kind != SourceCopilot {
		for _, m := range s.Deployments {
			models = append(models, Model{m, m, string(s.Kind), s.SupportsAgentModel(AgentClaudeCode, m), s.SupportsAgentModel(AgentCodex, m)})
		}
		return models, nil
	}
	l, e := a.leases.Lease(ctx, c, s, false)
	if e != nil {
		return nil, e
	}
	r, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(l.Endpoint, "/")+"/models", nil)
	a.leases.ApplyProviderHeaders(r, l, false)
	res, e := a.http.client(c.SourceProxy(id)).Do(r)
	if e != nil {
		return nil, errors.New("model catalog connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("model catalog rejected")
	}
	var data struct {
		Data []struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Vendor    string   `json:"vendor"`
			Endpoints []string `json:"supported_endpoints"`
			Policy    struct {
				State string `json:"state"`
			} `json:"policy"`
		} `json:"data"`
	}
	if e = readJSONBody(res.Body, &data); e != nil {
		return nil, e
	}
	for _, m := range data.Data {
		if m.Policy.State == "disabled" {
			continue
		}
		responses, messages := false, false
		for _, p := range m.Endpoints {
			responses = responses || p == "/responses"
			messages = messages || p == "/v1/messages"
		}
		if len(m.Endpoints) == 0 {
			responses = strings.HasPrefix(m.ID, "gpt-")
			messages = strings.HasPrefix(m.ID, "claude-")
		}
		_, known := copilotResponseEfforts[m.ID]
		codex := responses && known
		// Native Messages models are advertised by the upstream catalog.
		claude := codex || messages
		// Catalog visibility is separate from Agent compatibility. Keep every
		// enabled upstream model visible without promising unsupported protocols.
		name := m.Name
		if name == "" {
			name = m.ID
		}
		models = append(models, Model{m.ID, name, m.Vendor, claude, codex})
	}
	return models, nil
}
