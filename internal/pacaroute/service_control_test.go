package pacaroute

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"
)

// Stopping inference immediately closes admission, lets accepted streams finish,
// preserves management/configuration, and reuses routing/Auth state on restart.
func TestIndependentManagementAndDrainingService(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, _, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, partialResponse)
		w.(http.Flusher).Flush()
		<-release
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":3}}}\n\n")
	})
	d := &desktopControl{store: store, auth: s.Auth, gateway: s}
	if err := d.start(); err != nil {
		t.Fatal(err)
	}
	defer d.close()
	adminURL := fmt.Sprintf("http://127.0.0.1:%d", d.managementPort())
	serviceURL := fmt.Sprintf("http://127.0.0.1:%d", d.portValue())
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	call := func(base, method, path, body, key string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}
	for _, test := range []struct {
		base, path, key string
		want            int
	}{{adminURL, "/api/config", "", 401}, {adminURL, "/api/config", s.AdminKey, 200}, {serviceURL, "/api/config", s.AdminKey, 404}, {adminURL, "/codex/models", "alice-key", 404}, {serviceURL, "/", s.AdminKey, 404}} {
		status, _ := call(test.base, "GET", test.path, "", test.key)
		if status != test.want {
			t.Fatal("listener separation", test.path, status)
		}
	}
	status, _ := call(adminURL, "PUT", "/api/service", `{"serving":false}`, "")
	if status != 401 {
		t.Fatal("unauthorized service control")
	}
	req, _ := http.NewRequest("POST", serviceURL+"/codex/responses", strings.NewReader(fullHistoryRequest))
	req.Header.Set("Authorization", "Bearer alice-key")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	status, b := call(adminURL, "PUT", "/api/service", `{"serving":false}`, s.AdminKey)
	if status != 200 || !strings.Contains(string(b), `"serving":false`) {
		t.Fatal("service stop failed", status, string(b))
	}
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(serviceURL, "http://"), time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("stopped service still accepting connections")
	}
	code, b := call(adminURL, "GET", "/api/status", "", s.AdminKey)
	if code != 200 || !strings.Contains(string(b), `"serving":false`) {
		t.Fatal("management lost stopped status", string(b))
	}
	config := cloneConfig(store.Snapshot())
	config.Clients[0].Name = "Edited while stopped"
	encoded, _ := json.Marshal(config)
	status, b = call(adminURL, "PUT", "/api/config", string(encoded), s.AdminKey)
	if status != 200 {
		t.Fatal("offline edit failed", string(b))
	}
	close(release)
	wire, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || !strings.Contains(string(wire), "response.completed") {
		t.Fatal("stop truncated active stream", err, string(wire))
	}
	previousRouter, previousAuth, previousActivity := s.Router, s.Auth, s.Activity
	source, _ := store.Snapshot().Source("test")
	s.Router.FailResponse(routing.Target{Source: source, Model: "deployed"}, 429, http.Header{"Retry-After": []string{"123"}})
	status, b = call(adminURL, "PUT", "/api/service", `{"serving":true}`, s.AdminKey)
	if status != 200 {
		t.Fatal("restart failed", string(b))
	}
	if d.gateway != s || s.Router != previousRouter || s.Auth != previousAuth || s.Activity != previousActivity || len(s.Router.Health(store.Snapshot()).Cooldowns) != 1 {
		t.Fatal("restart discarded application state")
	}
	status, _ = call(serviceURL, "GET", "/codex/models", "", "alice-key")
	if status != 200 {
		t.Fatal("restarted inference unreachable", status)
	}
	status, b = call(adminURL, "GET", "/api/config", "", s.AdminKey)
	if status != 200 || !strings.Contains(string(b), "Edited while stopped") {
		t.Fatal("restart lost configuration")
	}
	if err = d.stop(); err != nil {
		t.Fatal(err)
	}
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	if _, err = d.changePort(port); err != nil {
		t.Fatal(err)
	}
	if running, _, _ := d.state(); running {
		t.Fatal("editing stopped port started inference")
	}
	saved := readDesktopSettings(store.Root)
	if saved.Serving == nil || *saved.Serving || saved.Port != port || saved.AdminPort != d.managementPort() || saved.CredentialRoot != s.Auth.vault.root {
		t.Fatal("stopped port preferences not persisted", saved)
	}
}
