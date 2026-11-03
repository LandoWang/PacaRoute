// Package clientconfig edits caller Agent configurations with exact backups.
package clientconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
	"github.com/pelletier/go-toml/v2"
)

// AgentConfigPath returns the configuration path for the selected Agent.
func AgentConfigPath(agent string) (string, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	switch agent {
	case "codex":
		root := os.Getenv("CODEX_HOME")
		if root == "" {
			root = filepath.Join(home, ".codex")
		}
		return filepath.Join(root, "config.toml"), nil
	case "claude-code":
		root := os.Getenv("CLAUDE_CONFIG_DIR")
		if root == "" {
			root = filepath.Join(home, ".claude")
		}
		return filepath.Join(root, "settings.json"), nil
	}
	return "", errors.New("select codex or claude-code")
}
func baseURL(port int, source, agent string) string {
	base := fmt.Sprintf("http://localhost:%d", port)
	if strings.HasPrefix(source, "pool:") {
		base += "/pools/" + strings.TrimPrefix(source, "pool:")
	} else if source != "" && source != "copilot" {
		base += "/sources/" + source
	}
	if agent == "codex" {
		return base + "/codex"
	}
	return base + "/cc"
}

// Plan holds a configuration preview and the original bytes required to apply it.
type Plan struct {
	Path        string
	old, next   []byte
	missing     bool
	Fingerprint string
	Changes     []string
}

// PlanLocal previews an Agent configuration for a local PacaRoute service.
func PlanLocal(agent, source, model string, port int) (Plan, error) {
	return PlanConnection(agent, baseURL(port, source, agent), model, "pacaroute-local")
}

// PlanConnection previews an Agent configuration without writing it.
func PlanConnection(agent, base, model, key string) (Plan, error) {
	path, e := AgentConfigPath(agent)
	if e != nil {
		return Plan{}, e
	}
	if f, e := os.Lstat(path); e == nil && f.Mode()&os.ModeSymlink != 0 {
		return Plan{}, errors.New("client config is a symlink; update its target explicitly")
	}
	old, e := os.ReadFile(path)
	missing := os.IsNotExist(e)
	if e != nil && !missing {
		return Plan{}, e
	}
	if missing {
		e = nil
	}
	if len(old) > 4<<20 {
		return Plan{}, errors.New("client configuration too large")
	}
	var next []byte
	if agent == "codex" {
		next, e = configureTOML(old, base, model, key)
	} else {
		m := map[string]any{}
		if len(old) > 0 {
			e = json.Unmarshal(old, &m)
		}
		if e == nil && m == nil {
			e = errors.New("Claude settings must be a JSON object")
		}
		if e == nil {
			env, valid := m["env"].(map[string]any)
			if _, exists := m["env"]; exists && !valid {
				return Plan{}, errors.New("Claude env must be an object; original file preserved")
			}
			if env == nil {
				env = map[string]any{}
			}
			env["ANTHROPIC_BASE_URL"] = base
			env["ANTHROPIC_AUTH_TOKEN"] = key
			delete(env, "ANTHROPIC_API_KEY")
			for _, k := range []string{"ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "CLAUDE_CODE_SUBAGENT_MODEL"} {
				delete(env, k)
			}
			if model != "" {
				env["ANTHROPIC_MODEL"] = model
			}
			m["env"] = env
			next, e = json.MarshalIndent(m, "", "  ")
			next = append(next, '\n')
		}
	}
	if e != nil {
		return Plan{}, e
	}
	fingerprint := fingerprint(string(old) + "\x00" + string(next) + fmt.Sprint(missing))
	return Plan{path, old, next, missing, fingerprint, []string{"Agent: " + agent, "Model: " + model, "Base URL: " + base, "Save a backup before applying"}}, nil
}
func configureTOML(old []byte, base, model, key string) ([]byte, error) {
	before := map[string]any{}
	if e := toml.Unmarshal(old, &before); e != nil {
		return nil, errors.New("invalid Codex TOML; original file preserved")
	}
	lines := strings.Split(strings.ReplaceAll(string(old), "\r\n", "\n"), "\n")
	out := []string{}
	root := true
	skip := false
	rootKey := regexp.MustCompile(`^\s*(model_provider|model)\s*=`)
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			root = false
			skip = trim == "[model_providers.pacaroute]" || strings.HasPrefix(trim, "[model_providers.pacaroute.")
		}
		if skip {
			continue
		}
		if root && rootKey.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	head := "model_provider = \"pacaroute\"\n"
	if model != "" {
		head += "model = " + fmt.Sprintf("%q", model) + "\n"
	} else if prev, ok := before["model"].(string); ok {
		head += "model = " + fmt.Sprintf("%q", prev) + "\n"
	}
	provider := fmt.Sprintf("\n[model_providers.pacaroute]\nname = \"PacaRoute\"\nbase_url = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\n", base)
	if key != "" {
		provider += fmt.Sprintf("http_headers = { Authorization = %q }\n", "Bearer "+key)
	}
	next := []byte(head + strings.Join(out, "\n") + provider)
	after := map[string]any{}
	if e := toml.Unmarshal(next, &after); e != nil {
		return nil, errors.New("could not safely edit Codex TOML")
	}
	strip := func(m map[string]any) {
		delete(m, "model_provider")
		delete(m, "model")
		if p, ok := m["model_providers"].(map[string]any); ok {
			delete(p, "pacaroute")
			if len(p) == 0 {
				delete(m, "model_providers")
			}
		}
	}
	strip(before)
	strip(after)
	if !reflect.DeepEqual(before, after) {
		return nil, errors.New("Codex config uses a complex TOML layout; automatic edit would alter unrelated settings")
	}
	return next, nil
}

// Apply saves an exact backup and writes the preview if the configuration is unchanged.
func (p Plan) Apply(expected string) (string, error) {
	unlock, e := atomicfile.Lock(context.Background(), p.Path+".pacaroute.lock", 5*time.Second)
	if e != nil {
		return "", e
	}
	defer unlock()
	b, e := os.ReadFile(p.Path)
	missing := os.IsNotExist(e)
	if e != nil && !missing {
		return "", e
	}
	if expected != p.Fingerprint || string(b) != string(p.old) || missing != p.missing {
		return "", errors.New("configuration changed; preview again before applying")
	}
	id := time.Now().UTC().Format("20060102T150405.000000000Z")
	dir := filepath.Join(filepath.Dir(p.Path), ".pacaroute-backups", filepath.Base(p.Path))
	backup := filepath.Join(dir, id+".bak")
	if p.missing {
		backup = filepath.Join(dir, id+".missing")
	}
	if e = atomicfile.Write(backup, p.old, 0600); e != nil {
		return "", e
	}
	return backup, atomicfile.Write(p.Path, p.next, 0600)
}

// State describes an Agent configuration and its connection to a local service.
type State struct {
	Agent     string
	Path      string
	Connected bool
	Drifted   bool
	Source    string
	// Model is nil when the configuration could not be read or parsed.
	Model *string
	Err   error
}

// AgentState reports the current Agent connection and local port drift.
func AgentState(agent string, port int) State {
	path, e := AgentConfigPath(agent)
	out := State{Agent: agent, Path: path, Source: "copilot"}
	if e != nil {
		out.Err = e
		return out
	}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return out
	}
	if e != nil {
		out.Err = e
		return out
	}
	base, model := "", ""
	if agent == "codex" {
		var m map[string]any
		if toml.Unmarshal(b, &m) != nil {
			out.Err = errors.New("Invalid TOML")
			return out
		}
		model, _ = m["model"].(string)
		provider, _ := m["model_provider"].(string)
		if providers, ok := m["model_providers"].(map[string]any); ok {
			if p, ok := providers[provider].(map[string]any); ok {
				base, _ = p["base_url"].(string)
			}
		}
	} else {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			out.Err = errors.New("Invalid JSON")
			return out
		}
		if env, ok := m["env"].(map[string]any); ok {
			base, _ = env["ANTHROPIC_BASE_URL"].(string)
			model, _ = env["ANTHROPIC_MODEL"].(string)
		}
	}
	out.Model = &model
	source := "copilot"
	parts := strings.Split(base, "/")
	for i, p := range parts {
		if (p == "sources" || p == "pools") && i+1 < len(parts) {
			source = parts[i+1]
			if p == "pools" {
				source = "pool:" + source
			}
		}
	}
	out.Source = source
	out.Connected = base == baseURL(port, source, agent)
	out.Drifted = strings.Contains(base, "localhost:") && base != baseURL(port, source, agent)
	return out
}
