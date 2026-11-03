package pacaroute

import (
	"net"
	"net/http"
	"strconv"
)

func (s *Server) servicePort() int {
	if s.Desktop != nil {
		return s.Desktop.portValue()
	}
	return s.Port
}
func (s *Server) gatewayOrigin(r *http.Request) string {
	if s.Desktop == nil {
		return s.requestOrigin(r)
	}
	if s.PublicURL != "" {
		return s.PublicURL
	}
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(s.servicePort()))
}
func (s *Server) serviceAPI(w http.ResponseWriter, r *http.Request) {
	if s.Desktop == nil {
		problem(w, 409, "start desktop or serve with --admin-port to control the service listener")
		return
	}
	d := s.Desktop
	if r.Method == "GET" {
		writeJSON(w, 200, d.status())
		return
	}
	if r.Method != "PUT" {
		problem(w, 405, "GET or PUT required")
		return
	}
	var input struct {
		Port    *int  `json:"port"`
		Serving *bool `json:"serving"`
	}
	if decodeRequest(w, r, &input) != nil || (input.Port == nil) == (input.Serving == nil) {
		problem(w, 400, "provide either port or serving")
		return
	}
	var err error
	if input.Port != nil {
		_, err = d.changePort(*input.Port)
	} else if *input.Serving {
		err = d.start()
	} else {
		err = d.stop()
	}
	if err != nil {
		problem(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, d.status())
}
