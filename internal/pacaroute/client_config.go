package pacaroute

import "github.com/LandoWang/pacaroute/internal/pacaroute/clientconfig"

func clientPath(client string) (string, error) {
	return clientconfig.AgentConfigPath(client)
}

// configPlan preserves the local preview fields used by CLI and management callers.
type configPlan struct {
	path        string
	fingerprint string
	changes     []string
	plan        clientconfig.Plan
}

func planClient(client, source, model string, port int) (configPlan, error) {
	p, e := clientconfig.PlanLocal(client, source, model, port)
	return configPlan{p.Path, p.Fingerprint, p.Changes, p}, e
}

func planConnection(client, base, model, key string) (configPlan, error) {
	p, e := clientconfig.PlanConnection(client, base, model, key)
	return configPlan{p.Path, p.Fingerprint, p.Changes, p}, e
}

func (p configPlan) apply(expected string) (string, error) {
	return p.plan.Apply(expected)
}

func clientState(client string, port int) map[string]any {
	state := clientconfig.AgentState(client, port)
	out := map[string]any{"id": state.Agent, "path": state.Path, "connected": state.Connected, "drifted": state.Drifted, "source": state.Source}
	if state.Err != nil {
		out["error"] = state.Err.Error()
	}
	if state.Model != nil {
		out["model"] = *state.Model
	}
	return out
}
