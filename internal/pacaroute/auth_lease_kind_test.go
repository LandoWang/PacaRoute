package pacaroute

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

const leaseKindSyntheticKey = "synthetic-audit-placeholder-0001"

// Contract: an Anthropic source authenticates with x-api-key, an OpenAI source
// with a Bearer Authorization header. A cached lease must not survive a change
// of the source's provider kind; "api-key" accounts accept both kinds.
func TestLeaseCacheSeparatesSourceKind(t *testing.T) {
	s := storeForTest(t)
	auth := NewAuth(s, t.TempDir())
	if err := auth.SetKeys(map[string]string{"models": leaseKindSyntheticKey}); err != nil {
		t.Fatal(err)
	}
	c := domain.Config{Accounts: []domain.Account{{ID: "shared", Name: "Shared", Kind: "api-key"}}, Proxies: domain.Proxies{Bindings: map[string]string{}, AccountProxyID: "system"}}
	openai := domain.Source{ID: "models", Name: "Models", Kind: "openai", AccountID: "shared", AuthMode: "api-key", Endpoint: "https://example.invalid/v1", Deployments: []string{"m"}}
	c.Sources = []domain.Source{openai}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if l, err := auth.Lease(context.Background(), &c, openai, false); err != nil || l.Header != "Authorization" {
		t.Fatalf("openai lease: %v %q", err, l.Header)
	}
	anthropic := openai
	anthropic.Kind = "anthropic"
	edited := c
	edited.Sources = []domain.Source{anthropic}
	if err := edited.Validate(); err != nil {
		t.Fatal(err)
	}
	l, err := auth.Lease(context.Background(), &edited, anthropic, false)
	if err != nil {
		t.Fatal(err)
	}
	if l.Header != "x-api-key" {
		t.Fatalf("anthropic source received stale %q lease from the openai cache entry", l.Header)
	}
}

// End-to-end form of the above through admin model discovery: after gateway
// traffic primed the cache as openai, discovering the same source as anthropic
// sends the wrong header and is rejected (discovery never forces a new lease).
func TestDiscoveryAfterSourceKindChange(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != leaseKindSyntheticKey {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-test"}],"has_more":false}`))
	}))
	defer srv.Close()
	s := storeForTest(t)
	auth := NewAuth(s, t.TempDir())
	auth.http.Values = map[string]*http.Client{"system:": srv.Client()}
	if err := auth.SetKeys(map[string]string{"models": leaseKindSyntheticKey}); err != nil {
		t.Fatal(err)
	}
	c := domain.Config{Accounts: []domain.Account{{ID: "shared", Name: "Shared", Kind: "api-key"}}, Proxies: domain.Proxies{Bindings: map[string]string{}, AccountProxyID: "system"}}
	src := domain.Source{ID: "models", Name: "Models", Kind: "openai", AccountID: "shared", AuthMode: "api-key", Endpoint: srv.URL + "/v1", Deployments: []string{"m"}}
	c.Sources = []domain.Source{src}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Lease(context.Background(), &c, src, false); err != nil {
		t.Fatal(err)
	}
	src.Kind = "anthropic"
	c.Sources = []domain.Source{src}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := newCatalog(auth.config, auth, &auth.http).DiscoverModels(context.Background(), &c, src); err != nil {
		t.Fatalf("discovery after kind change failed: %v", err)
	}
}
