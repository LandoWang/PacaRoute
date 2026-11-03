package pacaroute

import (
	"errors"
	"strings"
)

func resolveRoute(c *Config, client *Client, agent AgentProtocol, model, explicit string) (string, string, error) {
	if !client.AgentEnabled(agent.Grant()) {
		return "", "", errors.New("Agent is not enabled for this Client")
	}
	if !client.LegacyDirect {
		for _, r := range client.Routes {
			if r.Agent == agent.Grant() && (r.MatchModel == "*" || r.MatchModel == model) {
				if explicit != "" && explicit != r.SourceID {
					return "", "", errors.New("client route does not permit this source")
				}
				return r.SourceID, r.Model, nil
			}
		}
		return "", "", errors.New("no route for this Client, Agent and model")
	}
	if explicit == "" {
		explicit = "copilot"
	}
	if strings.HasPrefix(explicit, "pool:") {
		p, ok := c.Pool(explicit)
		if !ok || p.Agent != agent {
			return "", "", errors.New("pool not found for Agent")
		}
		if p.MultiModel {
			if !p.HasModel(model) {
				return "", "", errors.New("model not exposed by pool")
			}
			return explicit, model, nil
		}
		return explicit, p.Model, nil
	}
	if e := c.validateTarget(explicit, model, agent.Grant()); e != nil {
		return "", "", e
	}
	return explicit, model, nil
}

// Project only peer grants onto the wire protocol requested by a downstream router.
// Direct Agent grants remain independent, including their sticky-session scope.
func peerClientView(c *Config, client *Client, agent AgentProtocol) *Client {
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
