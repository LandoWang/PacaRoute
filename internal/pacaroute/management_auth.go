package pacaroute

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) admin(r *http.Request) bool {
	if key := bearer(r); key != "" {
		return constantEqual([]byte(key), []byte(s.AdminKey))
	}
	cookie, e := r.Cookie("pacaroute_admin")
	if e != nil || !constantEqual([]byte(cookie.Value), []byte(s.AdminKey)) {
		return false
	}
	return r.Method == "GET" || s.sameOrigin(r)
}
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(v); e != nil {
		return errors.New("invalid JSON request")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
