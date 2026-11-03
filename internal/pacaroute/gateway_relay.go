package pacaroute

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/LandoWang/pacaroute/internal/pacaroute/protocol"

	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"
)

// maxUpstreamJSON bounds a non-stream upstream response.
const maxUpstreamJSON = 32 << 20

// relayStream relays or translates this one committed upstream stream. After
// the downstream status is written no other source may take over the response.
func (s *Server) relayStream(x *gatewayExchange, t routing.Target, tr *protocol.Translation, upstreamProto string, res *http.Response) {
	w, r, agent, activity := x.w, x.r, x.agent, &x.activity
	var e error
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(res.StatusCode)
	reader := io.Reader(res.Body)
	var trace bytes.Buffer
	if s.TraceDir != "" {
		reader = io.TeeReader(reader, &trace)
	}
	if tr != nil {
		if tr.To == "responses" {
			e = tr.StreamResponses(w, reader, &x.usage)
		} else {
			e = tr.StreamMessages(w, reader, &x.usage)
		}
	} else {
		e = protocol.RelaySSE(w, struct {
			io.Reader
			io.Closer
		}{reader, res.Body}, &x.usage, upstreamProto)
	}
	s.trace(activity.ID, "upstream-res.sse", trace.Bytes())
	if e != nil {
		var downstream *protocol.DownstreamStreamError
		var upstream *protocol.UpstreamStreamError
		upstreamFailure := errors.As(e, &upstream)
		if errors.As(e, &downstream) || (r.Context().Err() != nil && !upstreamFailure) {
			activity.Status = statusClientClosedRequest
			activity.Error = "client disconnected before stream completion"
		} else {
			activity.Status = 502
			activity.Error = "stream interrupted before completion: " + protocol.StreamDetail(e.Error())
			// Never replay a started response. Cool down this source/model so
			// the next client retry can recover under the existing pool rules.
			s.Router.Fail(t, activity.Error)
			if !upstreamFailure || !upstream.Forwarded {
				_ = protocol.EmitStreamError(w, agent, activity.Error)
			}
		}
	}
}

// relayJSON relays a bounded non-stream response, translating it when needed.
func (s *Server) relayJSON(x *gatewayExchange, tr *protocol.Translation, res *http.Response) {
	w, activity := x.w, &x.activity
	var e error
	data, err := io.ReadAll(io.LimitReader(res.Body, maxUpstreamJSON+1))
	if err != nil || len(data) > maxUpstreamJSON {
		e = errors.New("upstream response incomplete or too large")
	} else {
		x.usage.Observe(data)
		s.trace(activity.ID, "upstream-res.json", data)
		if tr != nil && x.operation != "messages/count_tokens" {
			data, e = tr.ResponseJSON(data)
		}
		if e == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(res.StatusCode)
			_, e = w.Write(data)
		}
	}
	if e != nil {
		activity.Status = 502
		activity.Error = e.Error()
		problem(w, 502, "invalid upstream response")
	}
}
func (s *Server) trace(id, name string, b []byte) {
	if s.TraceDir == "" || len(b) == 0 {
		return
	}
	dir := filepath.Join(s.TraceDir, id)
	if os.MkdirAll(dir, 0700) == nil {
		_ = atomicWrite(filepath.Join(dir, name), b, 0600)
	}
}
