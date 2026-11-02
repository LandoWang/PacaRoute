package pacaroute

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type modelDiscovery struct {
	Models         []Model `json:"models"`
	ResourceID     string  `json:"resource_id,omitempty"`
	SubscriptionID string  `json:"subscription_id,omitempty"`
}

func (a *Auth) DiscoverModels(ctx context.Context, c *Config, source Source) (modelDiscovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out := modelDiscovery{Models: []Model{}, ResourceID: source.ResourceID, SubscriptionID: source.SubscriptionID}
	if source.Kind == "copilot" {
		models, e := a.Models(ctx, source.ID)
		out.Models = models
		return out, e
	}
	if source.Kind == "azure-openai" {
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
			resources, e := a.AzureAction(ctx, source.AccountID, "azure-resources", subscription, "")
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
		result, e := a.AzureAction(ctx, source.AccountID, "azure-deployments", subscription, resource)
		if e != nil {
			return out, e
		}
		out.Models = result["models"].([]Model)
		out.ResourceID = resource
		out.SubscriptionID = subscription
		return out, nil
	}
	lease, e := a.Lease(ctx, c, source, false)
	if e != nil {
		return out, e
	}
	endpoint := strings.TrimRight(source.Endpoint, "/")
	if source.Kind == "anthropic" && !strings.HasSuffix(endpoint, "/v1") {
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
		if source.Kind == "anthropic" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		response, e := a.http.client(c.SourceProxy(source.ID)).Do(req)
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
		e = readJSONBody(response.Body, &body)
		response.Body.Close()
		if e != nil {
			return out, e
		}
		for _, m := range body.Data {
			if !modelIdentifier.MatchString(m.ID) {
				continue
			}
			name := m.Name
			if name == "" {
				name = m.ID
			}
			out.Models = append(out.Models, Model{ID: m.ID, Name: name, Vendor: source.Kind, Codex: true, Claude: true})
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
