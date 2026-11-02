package pacaroute

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const Version = "0.8.4"

type Server struct {
	Store           *ConfigStore
	Auth            *Auth
	Router          *Router
	Activity        *ActivityStore
	AllowLocal      bool
	LocalManagement bool
	Port            int
	Desktop         *desktopControl
	tickets         browserTickets
	AdminKey        string
	PublicURL       string
	TraceDir        string
	started         time.Time
	loginJobs       *loginJobs
}

func NewServer(store *ConfigStore, auth *Auth) (*Server, error) {
	r, e := NewRouter(store.Root)
	if e != nil {
		return nil, e
	}
	adminPath := filepath.Join(store.Root, "admin.key")
	unlock, e := fileLock(context.Background(), adminPath+".lock", 5*time.Second)
	if e != nil {
		return nil, e
	}
	defer unlock()
	key, e := os.ReadFile(adminPath)
	if os.IsNotExist(e) {
		key = []byte(newKey())
		e = atomicWrite(adminPath, key, 0600)
	}
	if e != nil {
		return nil, e
	}
	if len(key) < 32 {
		return nil, errors.New("invalid management key")
	}
	return &Server{Store: store, Auth: auth, Router: r, Activity: NewActivityStore(store.Root), AdminKey: string(key), started: time.Now(), loginJobs: newLoginJobs()}, nil
}
func constantEqual(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"type": "error", "error": map[string]string{"type": "pacaroute_error", "message": message}})
}
func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return r.Header.Get("x-api-key")
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.management(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/mark.svg" || strings.HasPrefix(r.URL.Path, "/assets/") {
			s.web(w, r)
			return
		}
		if r.URL.Path == "/health" {
			writeJSON(w, 200, map[string]any{"name": "PacaRoute", "version": Version, "ready": true})
			return
		}
		s.gateway(w, r)
	})
}
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
func (s *Server) authenticate(r *http.Request, c *Config) (*Client, bool) {
	key := bearer(r)
	for _, v := range c.Clients {
		if !v.Local && key != "" && keyEqual(v.KeyHash, key) {
			if v.Disabled {
				return nil, false
			}
			copy := v
			return &copy, true
		}
	}
	if s.AllowLocal && s.PublicURL == "" && loopbackRequest(r) && r.Header.Get("Forwarded") == "" && r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Forwarded-Host") == "" && (key == "" || key == "dummy" || key == "pacaroute-local") {
		if local, exists := c.Client("default"); exists && local.Local && !local.Disabled {
			return local, true
		}
	}
	return nil, false
}
func (s *Server) gateway(w http.ResponseWriter, r *http.Request) {
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
	agent, explicit, operation := parseGatewayPath(r.URL.Path)
	if !client.AgentEnabled(agent) {
		problem(w, 403, "Agent is not enabled for this Client")
		return
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
		writeJSON(w, 200, map[string]any{"object": "list", "data": models})
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
	targets, e := s.Router.Targets(c, affinityClient(client), agent, id, model, session, hasContinuation(body))
	if e != nil {
		status := 503
		if hasContinuation(body) {
			status = 409
		}
		problem(w, status, e.Error())
		return
	}
	activity := Activity{ID: strings.ReplaceAll(newID(), "-", ""), Timestamp: time.Now().UTC(), Kind: "request", Client: agent, ClientID: clientID}
	start := time.Now()
	usage := Usage{}
	upstreamMessages := false
	defer func() {
		activity.Duration = time.Since(start).Milliseconds()
		activity.Input = usage.totalInput(upstreamMessages)
		activity.Output = usage.Output
		activity.CacheRead = usage.CacheRead
		activity.CacheWrite = usage.CacheWrite
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
		if operation == "messages/count_tokens" && t.upstreamProtocol() != "messages" {
			activity.Status = 422
			activity.Error = "token counting is unavailable on this Responses source"
			problem(w, 422, activity.Error)
			return
		}
		protocol := t.upstreamProtocol()
		upstreamMessages = protocol == "messages"
		upstreamPath := "/responses"
		if protocol == "messages" {
			upstreamPath = "/v1/messages"
		}
		if operation == "messages/count_tokens" {
			upstreamPath += "/count_tokens"
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
			s.Router.Fail(t)
			if i+1 < len(targets) {
				continue
			}
			problem(w, 502, activity.Error)
			return
		}
		activity.Status = res.StatusCode
		if retryable(res.StatusCode) && i+1 < len(targets) {
			s.Router.Fail(t)
			res.Body.Close()
			continue
		}
		if res.StatusCode >= 400 {
			s.Router.Fail(t)
			activity.Error = fmt.Sprintf("upstream returned HTTP %d", res.StatusCode)
			w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
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
		if agent == "codex" && t.Source.Kind == "copilot" && protocol == "responses" {
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
				e = relaySSE(w, reader, &usage)
			}
			s.trace(activity.ID, "upstream-res.sse", trace.Bytes())
			if e != nil {
				activity.Status = 502
				activity.Error = "stream interrupted before completion"
				_ = emitSSE(w, "error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": activity.Error}})
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
func relaySSE(w io.Writer, r io.Reader, u *Usage) error {
	rd := newSSE(r)
	complete := false
	for {
		frame, data, e := rd.next()
		if e != nil {
			if e == io.EOF && complete {
				return nil
			}
			return e
		}
		u.observe(data)
		m := obj(data)
		typ := rawString(m["type"])
		if typ == "response.completed" || typ == "response.incomplete" || typ == "message_stop" || bytes.Equal(data, []byte("[DONE]")) {
			complete = true
		}
		if _, e = w.Write(frame); e != nil {
			return e
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// The client may close as soon as the terminal frame is delivered. Do not
		// wait for transport EOF and misclassify its cancellation as a failed turn.
		if complete {
			return nil
		}
		if typ == "response.failed" || typ == "error" {
			return errors.New("upstream stream failed")
		}
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
