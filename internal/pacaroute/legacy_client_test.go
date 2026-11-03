package pacaroute

import (
	"net/http/httptest"
	"testing"
)

// Existing AgentLink sessions may keep using their public discovery sentinel.
// It must only select an enabled local Client, never authorize network access.
func TestLegacyProviderSentinelLocalMigration(t *testing.T) {
	for _, test := range []struct {
		name, peer, host, forwarded, key   string
		public, noLocal, disabled, deleted bool
		want                               bool
	}{
		{name: "existing-session", want: true},
		{name: "ipv6", peer: "[::1]:42", host: "[::1]:8766", want: true},
		{name: "network", peer: "192.0.2.1:42"},
		{name: "foreign-host", host: "example.com"},
		{name: "forwarded", forwarded: "Forwarded"},
		{name: "forwarded-for", forwarded: "X-Forwarded-For"},
		{name: "forwarded-host", forwarded: "X-Forwarded-Host"},
		{name: "public-service", public: true},
		{name: "opt-in-required", noLocal: true},
		{name: "disabled-client", disabled: true},
		{name: "deleted-client", deleted: true},
		{name: "unrecognized-key", key: "invalid-network-key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := localTestServer(t)
			c := cloneConfig(s.Store.Snapshot())
			if test.deleted {
				c.Clients = nil
			} else {
				c.Clients[0].Disabled = test.disabled
			}
			saveTest(t, s.Store, c)
			s.AllowLocal = !test.noLocal
			if test.public {
				s.PublicURL = "https://example.com"
			}
			peer, host, key := test.peer, test.host, test.key
			if peer == "" {
				peer = "127.0.0.1:42"
			}
			if host == "" {
				host = "localhost:8766"
			}
			if key == "" {
				key = "copilot-bridge-provider"
			}
			r := httptest.NewRequest("GET", "http://"+host+"/codex/models", nil)
			r.RemoteAddr = peer
			r.Header.Set("Authorization", "Bearer "+key)
			if test.forwarded != "" {
				r.Header.Set(test.forwarded, "192.0.2.1")
			}
			client, ok := s.authenticate(r, s.Store.Snapshot())
			if ok != test.want {
				t.Fatalf("authorized = %v, want %v", ok, test.want)
			}
			if ok && (client.ID != "default" || !client.Local) {
				t.Fatal("sentinel selected a network Client")
			}
		})
	}
}
