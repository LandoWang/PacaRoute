package pacaroute

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/transport"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

type modelDiscovery struct {
	Endpoint       string         `json:"endpoint,omitempty"`
	Models         []domain.Model `json:"models"`
	ResourceID     string         `json:"resource_id,omitempty"`
	SubscriptionID string         `json:"subscription_id,omitempty"`
}

func (a *catalog) DiscoverModels(ctx context.Context, c *domain.Config, source domain.Source) (modelDiscovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out := modelDiscovery{Models: []domain.Model{}, ResourceID: source.ResourceID, SubscriptionID: source.SubscriptionID}
	if source.Kind == domain.SourcePeer {
		return a.discoverPeer(ctx, c, source)
	}
	if source.Kind == domain.SourceCopilot {
		models, e := a.Models(ctx, source.ID)
		out.Models = models
		return out, e
	}
	if source.Kind == domain.SourceAzureOpenAI {
		subscription := source.SubscriptionID
		if source.ResourceID != "" {
			parts := strings.Split(source.ResourceID, "/")
			if len(parts) != 9 {
				return out, errors.New("invalid Azure resource ID")
			}
			if subscription == "" {
				subscription = parts[2]
			}
		}
		if subscription == "" {
			return out, errors.New("choose an Azure subscription and resource from Accounts before discovering deployments")
		}
		resource := source.ResourceID
		if resource == "" {
			resources, e := a.leases.AzureAction(ctx, source.AccountID, "azure-resources", subscription, "")
			if e != nil {
				return out, e
			}
			endpoint, _ := url.Parse(source.Endpoint)
			for _, v := range resources["resources"].([]any) {
				record := v.(map[string]string)
				u, e := url.Parse(record["endpoint"])
				if e == nil && strings.EqualFold(u.Hostname(), endpoint.Hostname()) {
					resource = record["id"]
					break
				}
			}
		}
		if resource == "" {
			return out, errors.New("no Azure resource in this subscription matches the source endpoint")
		}
		result, e := a.leases.AzureAction(ctx, source.AccountID, "azure-deployments", subscription, resource)
		if e != nil {
			return out, e
		}
		out.Models = result["models"].([]domain.Model)
		out.ResourceID = resource
		out.SubscriptionID = subscription
		return out, nil
	}
	lease, e := a.leases.Lease(ctx, c, source, false)
	if e != nil {
		return out, e
	}
	endpoint := strings.TrimRight(source.Endpoint, "/")
	if source.Kind == domain.SourceAnthropic && !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	endpoint += "/models"
	for page := 0; page < 20; page++ {
		req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if e != nil {
			return out, e
		}
		req.Header.Set(lease.Header, lease.Value)
		req.Header.Set("Accept", "application/json")
		if source.Kind == domain.SourceAnthropic {
			req.Header.Set("anthropic-version", anthropicVersion)
		}
		response, e := a.http.Client(c.SourceProxy(source.ID)).Do(req)
		if e != nil {
			return out, errors.New("model discovery connection failed")
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			return out, errors.New("model discovery was rejected; check the account key and endpoint")
		}
		var body struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		e = transport.ReadJSONBody(response.Body, &body)
		response.Body.Close()
		if e != nil {
			return out, e
		}
		for _, m := range body.Data {
			if !domain.ModelIdentifier.MatchString(m.ID) {
				continue
			}
			name := m.Name
			if name == "" {
				name = m.ID
			}
			out.Models = append(out.Models, domain.Model{ID: m.ID, Name: name, Vendor: string(source.Kind), Codex: true, Claude: true})
		}
		if !body.HasMore || body.LastID == "" {
			return out, nil
		}
		u, _ := url.Parse(endpoint)
		q := u.Query()
		q.Set("after_id", body.LastID)
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}
	return out, errors.New("model catalog exceeds the discovery limit; configure model IDs manually")
}

func (a *catalog) discoverPeer(ctx context.Context, c *domain.Config, source domain.Source) (modelDiscovery, error) {
	out := modelDiscovery{Models: []domain.Model{}}
	base := strings.TrimRight(source.Endpoint, "/")
	namespaced := strings.HasSuffix(base, "/pacaroute")
	if !namespaced {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/.well-known/pacaroute", nil)
		res, err := a.http.Client(c.SourceProxy(source.ID)).Do(req)
		if err == nil {
			var descriptor struct {
				PeerPath string `json:"peer_path"`
			}
			if res.StatusCode == 200 && transport.ReadJSONBody(res.Body, &descriptor) == nil && descriptor.PeerPath == "pacaroute" {
				base += "/pacaroute"
				namespaced = true
			}
			res.Body.Close()
		}
	}
	source.Endpoint = base
	out.Endpoint = base
	l, err := a.leases.Lease(ctx, c, source, false)
	if err != nil {
		return out, err
	}
	get := func(path string) ([]peerModel, int, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(source.Endpoint, "/")+path, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set(l.Header, l.Value)
		res, err := a.http.Client(c.SourceProxy(source.ID)).Do(req)
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
		if err := transport.ReadJSONBody(res.Body, &body); err != nil {
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
	models := map[string]domain.Model{}
	for _, entry := range entries {
		if !domain.ModelIdentifier.MatchString(entry.ID) {
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
