package pacaroute

import (
	"net/http"
	"sync"
	"time"
)

type browserTickets struct {
	mu     sync.Mutex
	values map[string]time.Time
}

func (t *browserTickets) issue() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.values == nil {
		t.values = map[string]time.Time{}
	}
	for key, expiry := range t.values {
		if time.Now().After(expiry) {
			delete(t.values, key)
		}
	}
	if len(t.values) >= 128 {
		t.values = map[string]time.Time{}
	}
	key := newKey()
	t.values[keyHash(key)] = time.Now().Add(time.Minute)
	return key
}
func (t *browserTickets) consume(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	hash := keyHash(key)
	expiry, ok := t.values[hash]
	delete(t.values, hash)
	return ok && time.Now().Before(expiry)
}
func (s *Server) ticketLogin(w http.ResponseWriter, r *http.Request) {
	if !s.localAllowed(r) || !s.sameOrigin(r) {
		problem(w, 403, "local browser login requires the loopback origin")
		return
	}
	var input struct {
		Ticket string `json:"ticket"`
	}
	if decodeRequest(w, r, &input) != nil || !s.tickets.consume(input.Ticket) {
		problem(w, 401, "browser link expired or already used; open management from the tray again")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "pacaroute_admin", Value: s.AdminKey, Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
