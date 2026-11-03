package pacaroute

import "github.com/LandoWang/pacaroute/internal/pacaroute/clientconfig"

// clientState is the CLI/management JSON view of one Agent's configuration.
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
