package pacaroute

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceAPIWithoutControl(t *testing.T) {
	store := storeForTest(t)
	s, err := NewServer(store, NewAuth(store, ""))
	if err != nil {
		t.Fatal(err)
	}
	if s.Desktop != nil {
		t.Fatal("a server without split listeners must have no service control")
	}
	const want = "{\"error\":{\"message\":\"start desktop or serve with --admin-port to control the service listener\",\"type\":\"pacaroute_error\"},\"type\":\"error\"}\n"
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.serviceAPI(w, httptest.NewRequest(method, "/api/service", nil))
			if w.Code != http.StatusConflict || w.Body.String() != want {
				t.Fatalf("missing service control: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
