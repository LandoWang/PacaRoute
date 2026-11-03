package pacaroute

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
)

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
