package pacaroute

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Contract: only management can recover the current Client key; encrypted keys
// survive restart, rotation revokes the old key, and config never contains it.
func TestClientKeyLifecycle(t *testing.T) {
	s := localTestServer(t)
	request := func(method, path, body string, status int) string {
		t.Helper()
		w := localRequest(s, method, path, body, "127.0.0.1:1234", "localhost:18766", true)
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s: unexpected status/cache policy %d", method, path, w.Code)
		}
		return w.Body.String()
	}
	decodeKey := func(body string) string {
		t.Helper()
		var v struct {
			Key string `json:"api_key"`
		}
		if json.Unmarshal([]byte(body), &v) != nil || v.Key == "" {
			t.Fatal("missing key")
		}
		return v.Key
	}
	key := decodeKey(request("POST", "/api/clients", `{"id":"alice","name":"Alice","enabled_agents":["codex"],"routes":[]}`, 201))
	if got := decodeKey(request("GET", "/api/clients/alice/key", "", 200)); got != key {
		t.Fatal("created key unavailable")
	}
	if strings.Contains(request("GET", "/api/config", "", 200), key) {
		t.Fatal("plaintext in config")
	}
	client, _ := s.Store.Snapshot().Client("alice")
	path, _ := s.Auth.clientKeyPath(*client)
	b, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(b), key) {
		t.Fatal("key not encrypted")
	}
	s.Auth = NewAuth(s.Store, "")
	if got := decodeKey(request("GET", "/api/clients/alice/key", "", 200)); got != key {
		t.Fatal("key lost after restart")
	}
	// Unauthenticated and Client-authenticated callers cannot recover keys.
	for _, credential := range []string{"", key} {
		r := httptest.NewRequest("GET", "/api/clients/alice/key", nil)
		if credential != "" {
			r.Header.Set("Authorization", "Bearer "+credential)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 401 || strings.Contains(w.Body.String(), key) {
			t.Fatal("non-admin recovered key")
		}
	}
	request("POST", "/api/clients", `{"id":"alice","name":"Duplicate"}`, 409)
	if got := decodeKey(request("GET", "/api/clients/alice/key", "", 200)); got != key {
		t.Fatal("duplicate creation changed credential")
	}
	newKey := decodeKey(request("POST", "/api/clients/alice/rotate", `{}`, 200))
	if newKey == key {
		t.Fatal("rotation reused key")
	}
	if got := decodeKey(request("GET", "/api/clients/alice/key", "", 200)); got != newKey {
		t.Fatal("stale key recovered")
	}
	for _, test := range []struct {
		key string
		ok  bool
	}{{key, false}, {newKey, true}} {
		r := httptest.NewRequest("GET", "/codex/models", nil)
		r.Header.Set("Authorization", "Bearer "+test.key)
		_, ok := s.authenticate(r, s.Store.Snapshot())
		if ok != test.ok {
			t.Fatal("rotation did not update Client authorization")
		}
	}
	c := cloneConfig(s.Store.Snapshot())
	c.Clients = nil
	saveTest(t, s.Store, c)
	request("GET", "/api/clients/alice/key", "", 404)
}

// Contract: legacy keys can be imported only if they match the committed hash.
// A misplaced encrypted record must never yield a different or stale key.
func TestClientKeyImportAndIsolation(t *testing.T) {
	s := localTestServer(t)
	client := domain.Client{ID: "legacy", Name: "Legacy", KeyHash: keyHash("synthetic-current")}
	c := cloneConfig(s.Store.Snapshot())
	c.Clients = append(c.Clients, client)
	saveTest(t, s.Store, c)
	get := func() string {
		t.Helper()
		key, err := s.Auth.ClientKey(client)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	if get() != "" {
		t.Fatal("hash-only Client invented a key")
	}
	for _, test := range []struct {
		key    string
		status int
	}{{"wrong", 400}, {"synthetic-current", 200}} {
		body, _ := json.Marshal(map[string]string{"key": test.key})
		w := localRequest(s, "PUT", "/api/clients/legacy/key", string(body), "127.0.0.1:1234", "localhost:18766", true)
		if w.Code != test.status {
			t.Fatalf("import status %d", w.Code)
		}
	}
	if get() != "synthetic-current" {
		t.Fatal("import not persisted")
	}
	if err := s.Auth.SaveKey(context.Background(), client.ID, "synthetic-source-key"); err != nil {
		t.Fatal(err)
	}
	if get() != "synthetic-current" {
		t.Fatal("source and Client credential collision")
	}
	path, _ := s.Auth.clientKeyPath(client)
	if err := s.Auth.vault.write(path, map[string]string{"key": "synthetic-stale"}); err != nil {
		t.Fatal(err)
	}
	if key, err := s.Auth.ClientKey(client); err == nil || key != "" {
		t.Fatal("returned mismatched encrypted key")
	}
	local := domain.Client{ID: "default", Local: true}
	if key, err := s.Auth.ClientKey(local); err != nil || key != "" {
		t.Fatal("local Client needs a key")
	}
	if err := s.Auth.SaveClientKey(context.Background(), local, "synthetic-current"); err == nil {
		t.Fatal("local key accepted")
	}
	client.KeyHash = "../../outside"
	if _, err := s.Auth.ClientKey(client); err == nil {
		t.Fatal("invalid hash accepted")
	}
}
