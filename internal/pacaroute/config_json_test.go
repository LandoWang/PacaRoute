package pacaroute

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// configJSONFixture covers every account/source kind, pool Agent, route grant and
// the omitted, null, empty and explicit enabled_agents forms, whose difference has
// legacy significance.
const configJSONFixture = `{
 "accounts": [
  {"id": "copilot", "name": "GitHub", "kind": "github", "login_mode": "device"},
  {"id": "azure-local", "name": "Azure", "kind": "azure", "login_mode": "azure-cli"},
  {"id": "oa", "name": "OpenAI", "kind": "openai"},
  {"id": "an", "name": "Anthropic", "kind": "anthropic"},
  {"id": "key", "name": "Key", "kind": "api-key"},
  {"id": "peer", "name": "Peer", "kind": "pacaroute", "endpoint": "http://127.0.0.1:9000"}
 ],
 "sources": [
  {"id": "copilot", "name": "Copilot", "kind": "copilot", "account_id": "copilot", "deployments": []},
  {"id": "az", "name": "Azure", "kind": "azure-openai", "account_id": "azure-local", "endpoint": "https://example.openai.azure.com", "deployments": ["gpt-x"]},
  {"id": "oa", "name": "OpenAI", "kind": "openai", "account_id": "oa", "endpoint": "https://api.openai.com/v1", "deployments": ["gpt-x"]},
  {"id": "an", "name": "Anthropic", "kind": "anthropic", "account_id": "an", "endpoint": "https://api.anthropic.com", "deployments": ["claude-x"]},
  {"id": "peer", "name": "Peer", "kind": "pacaroute", "account_id": "peer", "endpoint": "http://127.0.0.1:9000/pacaroute", "deployments": ["gpt-x"], "agent_models": {"codex": ["gpt-x"], "claude-code": ["gpt-x"]}}
 ],
 "pools": [
  {"id": "cx", "name": "Codex pool", "agent": "codex", "model": "gpt-x", "failover": true, "members": [{"id": "m", "source_id": "oa", "model": "gpt-x", "weight": 1}]},
  {"id": "cc", "name": "Claude pool", "agent": "claude-code", "model": "claude-x", "failover": false, "members": [{"id": "m", "source_id": "an", "model": "claude-x", "weight": 1}]}
 ],
 "proxies": {"profiles": [], "bindings": {}, "account_proxy_id": "system"},
 "clients": [
  {"id": "default", "name": "Local", "local": true, "routes": []},
  {"id": "nulled", "name": "Null", "enabled_agents": null, "key_hash": "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d", "routes": []},
  {"id": "empty", "name": "Empty", "enabled_agents": [], "key_hash": "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d", "routes": []},
  {"id": "all", "name": "All", "enabled_agents": ["codex", "claude-code", "pacaroute"], "key_hash": "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d", "routes": [
   {"id": "a", "agent": "codex", "match_model": "*", "source_id": "pool:cx", "model": "gpt-x"},
   {"id": "b", "agent": "claude-code", "match_model": "claude-x", "source_id": "pool:cc", "model": "claude-x"},
   {"id": "c", "agent": "pacaroute", "match_model": "gpt-x", "source_id": "peer", "model": "gpt-x"}
  ]}
 ],
 "source_state": {"paused": []}
}`

// Configuration JSON is a storage and wire contract: decoding and re-encoding
// (raw, and after validation/normalization) must produce the recorded bytes.
func TestConfigJSONRoundTripIsStable(t *testing.T) {
	var raw domain.Config
	if err := json.Unmarshal([]byte(configJSONFixture), &raw); err != nil {
		t.Fatal(err)
	}
	first, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var again domain.Config
	if err := json.Unmarshal(first, &again); err != nil {
		t.Fatal(err)
	}
	second, _ := json.Marshal(again)
	validated := cloneConfig(&raw)
	if err := validated.Validate(); err != nil {
		t.Fatal(err)
	}
	normalized, _ := json.Marshal(validated)
	got := append(append(append(append(first, '\n'), second...), '\n'), normalized...)
	golden := filepath.Join("testdata", "config-roundtrip.golden")
	if os.Getenv("PACAROUTE_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("configuration JSON changed:\n got %s\nwant %s", got, want)
	}
}
