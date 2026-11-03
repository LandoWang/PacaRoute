package pacaroute

import "testing"

// A successful full-history request after an endpoint edit must establish the
// new endpoint binding, so its own subsequent source-bound continuation works.
func TestEndpointEditRebindsSuccessfulSession(t *testing.T) {
	c := fixtureConfig()
	c.Pools[0].Members = c.Pools[0].Members[1:]
	c.Accounts = []Account{{ID: "api", Name: "API", Kind: "api-key"}}
	c.Sources[0].Kind = "openai"
	c.Sources[0].AuthMode = "api-key"
	c.Sources[0].AccountID = "api"
	c.Sources[0].Deployments = []string{"gpt-6-astra"}
	c.Sources[0].Endpoint = "https://old.example.invalid"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	router, err := NewRouter(root)
	if err != nil {
		t.Fatal(err)
	}
	selectTargets := func(r *Router, continuation bool) ([]Target, error) {
		return r.Targets(&c, "alice", "codex", "pool:balanced", "coding", "same-session", continuation)
	}
	before, err := selectTargets(router, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = router.Bind(before[0]); err != nil {
		t.Fatal(err)
	}
	c.Sources[0].Endpoint = "https://new.example.invalid"
	// An old opaque reference must remain rejected after the endpoint edit.
	if _, err = selectTargets(router, true); err == nil {
		t.Fatal("old endpoint continuation accepted")
	}
	after, err := selectTargets(router, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = router.Bind(after[0]); err != nil {
		t.Fatal(err)
	}
	// Bind is what the gateway calls after the new upstream accepts the request.
	if _, err = selectTargets(router, true); err != nil {
		t.Errorf("successful new-endpoint request did not rebind: %v", err)
	}
	reopened, err := NewRouter(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = selectTargets(reopened, true); err != nil {
		t.Errorf("new-endpoint binding missing after restart: %v", err)
	}
}
