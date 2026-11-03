package pacaroute

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The loopback local Client never authorizes a forwarded request. A proxy may
// append its own forwarding field after an empty caller-supplied one, so the
// presence of any forwarding field, not only its first value, is decisive.
func TestLocalClientRejectsDuplicateForwardedHeaders(t *testing.T) {
	s := localTestServer(t)
	front := httptest.NewServer(s.Handler())
	defer front.Close()
	call := func(header string, values []string) int {
		r, err := http.NewRequest(http.MethodGet, front.URL+"/codex/models", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer copilot-bridge-provider")
		if header != "" {
			r.Header[header] = values
		}
		res, err := front.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := call("", nil); code != http.StatusOK {
		t.Fatalf("direct loopback local request: status=%d, want 200", code)
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host"} {
		for _, values := range [][]string{{"", "192.0.2.42"}, {""}} {
			if code := call(header, values); code != http.StatusUnauthorized {
				t.Errorf("%s %q accepted for local placeholder: status=%d, want 401", header, values, code)
			}
		}
	}
}
