package pacaroute

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Affinity keys are persisted in affinity-v2.jsonl and legacy imports are looked
// up by the previous application's hashes. Their exact values are a storage
// contract: a code move must not change them, or existing sessions lose their
// bindings after an upgrade. Expected values are independent SHA-256 digests of
// the documented key inputs.
func TestAffinityKeysArePersistentContract(t *testing.T) {
	r, c := multiModelFixture(t)
	c.Pools = append(c.Pools, domain.Pool{ID: "solo", Name: "Solo", Agent: domain.AgentCodex, Model: "gpt-6-astra", Failover: true, Members: []domain.Member{{ID: "only", SourceID: "a", Model: "gpt-6-astra", Weight: 1}}})
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ pool, model, want string }{
		{"pool:solo", "gpt-6-astra", "e0c9c9ecd8d823efbed13dc1d1e89b5bce560374c4e70472734701ac2b3a44a9"},
		{"pool:big", "astra", "f0499f9131c0d657790f947f57137e8df33b2dddb06ff22dcb3dd436ef9ba136"},
	} {
		targets, err := r.Targets(&c, "alice", domain.AgentCodex, test.pool, test.model, "session-1", false)
		if err != nil || len(targets) == 0 || targets[0].Affinity != test.want {
			t.Fatalf("%s affinity key changed: %v %+v", test.pool, err, targets)
		}
	}

	// A peer receives a derived, installation-keyed session identity.
	if got := peerSessionHash("admin-fixture", "alice\x00pacaroute", domain.AgentCodex, "remote", "session-1"); got != "97a7fdd11496e7488ab25de1c29caab4fc33fc0c4c113ace8c5e6887747eaf71" {
		t.Fatal("peer session identity changed", got)
	}

	// The local Client imports a previous application's binding by its
	// upper-case legacy hash of Agent, pool and session.
	root := t.TempDir()
	legacy := `{"key":"326058AE4B2854B6C41FED9DBE94112743E71874C9E3D665DCC7539AE50A0EAB","member_id":"only","source_id":"b","model":"gpt-6-astra"}` + "\n"
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime", "source-pool-sessions.jsonl"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := routing.NewRouter(root)
	if err != nil {
		t.Fatal(err)
	}
	c.Pools = append(c.Pools, domain.Pool{ID: "pool-legacy", Name: "Legacy", Agent: domain.AgentCodex, Model: "gpt-6-astra", Failover: true, Members: []domain.Member{{ID: "only", SourceID: "b", Model: "gpt-6-astra", Weight: 1}, {ID: "other", SourceID: "c", Model: "gpt-6-astra", Weight: 1000}}})
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	targets, err := imported.Targets(&c, "local", domain.AgentCodex, "pool:pool-legacy", "gpt-6-astra", "legacy-session", false)
	if err != nil || !targets[0].Sticky || targets[0].Source.ID != "b" {
		t.Fatalf("legacy binding key changed: %v %+v", err, targets)
	}
}
