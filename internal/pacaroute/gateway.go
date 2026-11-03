package pacaroute

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func parseGatewayPath(path string) (agent, explicit, operation string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && (parts[0] == "sources" || parts[0] == "pools") {
		explicit = parts[1]
		if parts[0] == "pools" {
			explicit = "pool:" + explicit
		}
		parts = parts[2:]
	}
	if len(parts) > 0 && parts[0] == "codex" {
		agent = "codex"
		parts = parts[1:]
	} else if len(parts) > 0 && parts[0] == "cc" {
		agent = "claude-code"
		parts = parts[1:]
	}
	if len(parts) > 0 && parts[0] == "v1" {
		parts = parts[1:]
	}
	operation = strings.Join(parts, "/")
	if agent == "" {
		if operation == "messages" || operation == "messages/count_tokens" {
			agent = "claude-code"
		} else {
			agent = "codex"
		}
	}
	return
}
func (s *Server) gateway(w http.ResponseWriter, r *http.Request) { s.gatewayFor(w, r, false) }
func (s *Server) gatewayFor(w http.ResponseWriter, r *http.Request, peer bool) {
	if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
		problem(w, 403, "cross-origin Agent requests are not permitted")
		return
	}
	c := s.Store.Snapshot()
	client, ok := s.authenticate(r, c)
	if !ok {
		problem(w, 401, "valid Client API key required")
		return
	}
	hops, err := peerHops(r)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	if hops >= maxPeerHops {
		problem(w, 508, "PacaRoute forwarding loop or hop limit reached")
		return
	}
	agent, explicit, operation := parseGatewayPath(r.URL.Path)
	grant := agent
	if peer {
		grant = "pacaroute"
	}
	if !client.AgentEnabled(grant) {
		problem(w, 404, "endpoint not found")
		return
	}
	affinityScope := affinityClient(client)
	if peer {
		client = c.peerClient(client, agent)
		affinityScope += "\x00pacaroute"
	}
	if operation == "models" && r.Method == "GET" {
		if !client.LegacyDirect {
			models := []map[string]string{}
			for _, route := range client.Routes {
				if route.Agent == agent && (explicit == "" || explicit == route.SourceID) {
					alias := route.MatchModel
					if alias == "*" {
						alias = route.Model
					}
					models = append(models, map[string]string{"id": alias, "object": "model"})
				}
			}
			writeJSON(w, 200, map[string]any{"object": "list", "data": models})
			return
		}
		id := explicit
		if id == "" {
			id = "copilot"
		}
		models, e := s.Auth.Models(r.Context(), id)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		// Management shows the full catalog; legacy Agent discovery retains
		// the compatible subset rather than advertising unsupported protocols.
		compatible := []Model{}
		for _, model := range models {
			if agent == "codex" && model.Codex || agent == "claude-code" && model.Claude {
				compatible = append(compatible, model)
			}
		}
		writeJSON(w, 200, map[string]any{"object": "list", "data": compatible})
		return
	}
	if r.Method != "POST" || (operation != "responses" && operation != "messages" && operation != "messages/count_tokens") {
		problem(w, 404, "endpoint not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		problem(w, 413, "request body exceeds 32 MiB")
		return
	}
	var body object
	if json.Unmarshal(b, &body) != nil || body == nil {
		problem(w, 400, "JSON object required")
		return
	}
	model := rawString(body["model"])
	id, model, e := c.Resolve(client, agent, model, explicit)
	if e != nil {
		problem(w, 403, e.Error())
		return
	}
	clientID := "local"
	if client != nil {
		clientID = client.ID
	}
	session := sessionID(agent, r.Header, body)
	targets, e := s.Router.Targets(c, affinityScope, agent, id, model, session, hasContinuation(body))
	if e != nil {
		status := 503
		if hasContinuation(body) {
			status = 409
		}
		problem(w, status, e.Error())
		return
	}
	activity := Activity{ID: strings.ReplaceAll(newID(), "-", ""), Timestamp: time.Now().UTC(), Kind: "request", Client: agent, ClientID: clientID}
	quota, err := s.Quotas.Admit(activity, client.Quota)
	if err != nil {
		problem(w, 503, "Client quota ledger unavailable")
		return
	}
	if quota.Blocked != "" {
		w.Header().Set("Retry-After", strconv.FormatInt(quota.RetryAfter, 10))
		problem(w, 429, "Client quota exceeded: "+quota.Blocked)
		return
	}
	start := time.Now()
	usage := Usage{}
	upstreamMessages := false
	defer func() {
		activity.Duration = time.Since(start).Milliseconds()
		activity.Input = usage.totalInput(upstreamMessages)
		activity.Output = usage.Output
		activity.CacheRead = usage.CacheRead
		activity.CacheWrite = usage.CacheWrite
		var price *ModelPrice
		if source, ok := c.Source(activity.Source); ok {
			if p, exists := source.Prices[activity.Model]; exists {
				price = &p
			}
		}
		if err := s.Quotas.Settle(activity, price, upstreamMessages); err != nil {
			log.Printf("quota settlement failed: %v", err)
		}
		if e := s.Activity.Record(activity); e != nil {
			log.Printf("activity write failed: %v", e)
		}
	}()
	s.trace(activity.ID, "client-req.json", b)
	for i, t := range targets {
		activity.Error = ""
		activity.Source = t.Source.ID
		activity.Model = t.Model
		activity.Attempts = i + 1
		wire, tr, e := prepare(body, t, agent)
		if e != nil {
			activity.Status = 400
			activity.Error = e.Error()
			problem(w, 400, e.Error())
			return
		}
		if operation == "messages/count_tokens" && t.upstreamProtocol(agent) != "messages" {
			activity.Status = 422
			activity.Error = "token counting is unavailable on this Responses source"
			problem(w, 422, activity.Error)
			return
		}
		protocol := t.upstreamProtocol(agent)
		upstreamMessages = protocol == "messages"
		upstreamPath := "/responses"
		if protocol == "messages" {
			upstreamPath = "/v1/messages"
		}
		if operation == "messages/count_tokens" {
			upstreamPath += "/count_tokens"
		}
		if t.Source.Kind == "pacaroute" {
			prefix := "/codex"
			if agent == "claude-code" {
				prefix = "/cc"
			}
			upstreamPath = prefix + "/" + operation
		}
		s.trace(activity.ID, fmt.Sprintf("upstream-%d-req.json", i), wire)
		var res *http.Response
		for authAttempt := 0; authAttempt < 2; authAttempt++ {
			l, err := s.Auth.Lease(r.Context(), c, t.Source, authAttempt > 0)
			if err != nil {
				e = err
				break
			}
			base := t.Source.Endpoint
			if t.Source.Kind == "copilot" {
				base = l.Endpoint
			}
			if strings.HasSuffix(strings.TrimRight(base, "/"), "/v1") && strings.HasPrefix(upstreamPath, "/v1/") {
				upstreamPath = strings.TrimPrefix(upstreamPath, "/v1")
			}
			url := strings.TrimRight(base, "/") + upstreamPath
			req, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(wire))
			if err != nil {
				e = err
				break
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", r.Header.Get("Accept"))
			for _, k := range []string{"anthropic-version", "anthropic-beta", "thread-id", "session-id", "session_id", "X-Claude-Code-Session-Id"} {
				if v := r.Header.Get(k); v != "" {
					req.Header.Set(k, v)
				}
			}
			if protocol == "messages" && req.Header.Get("anthropic-version") == "" {
				req.Header.Set("anthropic-version", "2023-06-01")
			}
			req.Header.Set(l.Header, l.Value)
			if t.Source.Kind == "pacaroute" {
				req.Header.Set(peerHopHeader, strconv.Itoa(hops+1))
				if session != "" {
					req.Header.Set("X-PacaRoute-Session", keyHash(s.AdminKey+"\x00"+affinityScope+"\x00"+agent+"\x00"+t.Source.ID+"\x00"+session))
				}
			}
			if t.Source.Kind == "copilot" {
				s.Auth.copilotHeaders(req, l, bytes.Contains(wire, []byte("input_image")) || bytes.Contains(wire, []byte(`"image"`)))
				req.Header.Set("X-Initiator", "agent")
				req.Header.Set("X-Request-Id", newID())
				req.Header.Set("Openai-Intent", "conversation-panel")
			}
			res, e = s.Auth.http.client(c.SourceProxy(t.Source.ID)).Do(req)
			if e != nil {
				break
			}
			if authAttempt == 0 && (res.StatusCode == 401 || res.StatusCode == 403) && t.Source.Kind == "copilot" {
				io.Copy(io.Discard, io.LimitReader(res.Body, 65536))
				res.Body.Close()
				res = nil
				continue
			}
			break
		}
		if e != nil || res == nil {
			activity.Status = 502
			activity.Error = "upstream connection or authentication failed"
			if e != nil && !strings.Contains(e.Error(), "https://") {
				activity.Error = e.Error()
			}
			s.Router.Fail(t, activity.Error)
			if i+1 < len(targets) {
				continue
			}
			problem(w, 502, activity.Error)
			return
		}
		activity.Status = res.StatusCode
		if retryable(res.StatusCode) && i+1 < len(targets) {
			s.Router.FailResponse(t, res.StatusCode, res.Header)
			res.Body.Close()
			continue
		}
		if res.StatusCode >= 400 {
			activity.Error = fmt.Sprintf("upstream returned HTTP %d", res.StatusCode)
			s.Router.FailResponse(t, res.StatusCode, res.Header)
			w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
			for _, header := range []string{"Retry-After", "Retry-After-Ms", "X-Ms-Retry-After-Ms"} {
				if value := res.Header.Get(header); value != "" {
					w.Header().Set(header, value)
				}
			}
			w.WriteHeader(res.StatusCode)
			_, _ = io.Copy(w, io.LimitReader(res.Body, 2<<20))
			res.Body.Close()
			return
		}
		if operation != "messages/count_tokens" {
			if e = s.Router.Bind(t); e != nil {
				res.Body.Close()
				activity.Status = 500
				activity.Error = "could not persist session affinity"
				problem(w, 500, activity.Error)
				return
			}
		}
		if agent == "codex" && protocol == "responses" && (t.Source.Kind == "copilot" || t.Source.Kind == "pacaroute" && res.Header.Get("X-Reasoning-Included") == "true") {
			w.Header().Set("X-Reasoning-Included", "true")
		}
		w.Header().Set("X-PacaRoute-Request", activity.ID)
		w.Header().Set("X-PacaRoute-Source", t.Source.ID)
		w.Header().Set("Cache-Control", "no-cache")
		stream := strings.Contains(res.Header.Get("Content-Type"), "text/event-stream")
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(res.StatusCode)
			reader := io.Reader(res.Body)
			var trace bytes.Buffer
			if s.TraceDir != "" {
				reader = io.TeeReader(reader, &trace)
			}
			if tr != nil {
				if tr.to == "responses" {
					e = tr.streamResponses(w, reader, &usage)
				} else {
					e = tr.streamMessages(w, reader, &usage)
				}
			} else {
				e = relaySSE(w, struct {
					io.Reader
					io.Closer
				}{reader, res.Body}, &usage, protocol)
			}
			s.trace(activity.ID, "upstream-res.sse", trace.Bytes())
			if e != nil {
				var downstream *downstreamStreamError
				var upstream *upstreamStreamError
				upstreamFailure := errors.As(e, &upstream)
				if errors.As(e, &downstream) || (r.Context().Err() != nil && !upstreamFailure) {
					activity.Status = 499
					activity.Error = "client disconnected before stream completion"
				} else {
					activity.Status = 502
					activity.Error = "stream interrupted before completion: " + streamDetail(e.Error())
					// Never replay a started response. Cool down this source/model so
					// the next client retry can recover under the existing pool rules.
					s.Router.Fail(t, activity.Error)
					if !upstreamFailure || !upstream.forwarded {
						_ = emitStreamError(w, agent, activity.Error)
					}
				}
			}
		} else {
			data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20+1))
			if err != nil || len(data) > 32<<20 {
				e = errors.New("upstream response incomplete or too large")
			} else {
				usage.observe(data)
				s.trace(activity.ID, "upstream-res.json", data)
				if tr != nil && operation != "messages/count_tokens" {
					data, e = tr.responseJSON(data)
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
		res.Body.Close()
		return
	}
}
func retryable(status int) bool {
	return status == 401 || status == 403 || status == 408 || status == 429 || status >= 500
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

// Keep imported local affinity stable without merging any network Client scope.
func affinityClient(client *Client) string {
	if client.Local && client.ID == "default" {
		return "local"
	}
	if client.ID == "local" {
		return "client:local"
	}
	return client.ID
}
