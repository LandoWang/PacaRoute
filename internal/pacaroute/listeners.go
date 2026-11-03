package pacaroute

import (
	"net/http"
	"strings"
	"time"
)

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 64 << 10}
}
func managementPath(path string) bool {
	return path == "/" || path == "/index.html" || path == "/mark.svg" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/assets/")
}
func (s *Server) splitHandler(management bool) http.Handler {
	handler := s.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managementPath(r.URL.Path) != management {
			http.NotFound(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
