package pacaroute

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func parseGatewayPath(path string) (agent AgentProtocol, explicit, operation string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && (parts[0] == "sources" || parts[0] == "pools") {
		explicit = parts[1]
		if parts[0] == "pools" {
			explicit = "pool:" + explicit
		}
		parts = parts[2:]
	}
	if len(parts) > 0 && parts[0] == "codex" {
		agent = AgentCodex
		parts = parts[1:]
	} else if len(parts) > 0 && parts[0] == "cc" {
		agent = AgentClaudeCode
		parts = parts[1:]
	}
	if len(parts) > 0 && parts[0] == "v1" {
		parts = parts[1:]
	}
	operation = strings.Join(parts, "/")
	if agent == "" {
		if operation == "messages" || operation == "messages/count_tokens" {
			agent = AgentClaudeCode
		} else {
			agent = AgentCodex
		}
	}
	return
}
func (s *Server) gateway(w http.ResponseWriter, r *http.Request) { s.gatewayFor(w, r, false) }

// gatewayExchange is the execution state of one Agent request. It owns the
// configuration snapshot captured once at entry, the authenticated Client and
// grant, the selected targets, the activity being recorded and the usage that
// settlement observes. finishExchange is the single finalization owner.
type gatewayExchange struct {
	w                http.ResponseWriter
	r                *http.Request
	config           *Config
	client           *Client
	peer             bool
	hops             int
	agent            AgentProtocol
	explicit         string
	operation        string
	affinityScope    string
	raw              []byte
	body             object
	session          string
	targets          []Target
	activity         Activity
	usage            Usage
	upstreamMessages bool
	start            time.Time
}

func (s *Server) gatewayFor(w http.ResponseWriter, r *http.Request, peer bool) {
	x, ok := s.openExchange(w, r, peer)
	if !ok {
		return
	}
	if x.operation == "models" && r.Method == "GET" {
		s.agentModels(x)
		return
	}
	if !x.decode() || !s.selectTargets(x) || !s.admitQuota(x) {
		return
	}
	defer s.finishExchange(x)
	s.trace(x.activity.ID, "client-req.json", x.raw)
	for i, t := range x.targets {
		if !s.attempt(x, i, t) {
			return
		}
	}
}

// openExchange captures one configuration snapshot, authenticates the Client
// before anything else is interpreted and checks the endpoint's grant.
func (s *Server) openExchange(w http.ResponseWriter, r *http.Request, peer bool) (*gatewayExchange, bool) {
	if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
		problem(w, 403, "cross-origin Agent requests are not permitted")
		return nil, false
	}
	c := s.Store.Snapshot()
	client, ok := s.authenticate(r, c)
	if !ok {
		problem(w, 401, "valid Client API key required")
		return nil, false
	}
	hops, err := peerHops(r)
	if err != nil {
		problem(w, 400, err.Error())
		return nil, false
	}
	if hops >= maxPeerHops {
		problem(w, 508, "PacaRoute forwarding loop or hop limit reached")
		return nil, false
	}
	agent, explicit, operation := parseGatewayPath(r.URL.Path)
	grant := agent.Grant()
	if peer {
		grant = GrantPeer
	}
	if !client.AgentEnabled(grant) {
		problem(w, 404, "endpoint not found")
		return nil, false
	}
	affinityScope := affinityClient(client)
	if peer {
		client = c.peerClient(client, agent)
		affinityScope += "\x00pacaroute"
	}
	return &gatewayExchange{w: w, r: r, config: c, client: client, peer: peer, hops: hops, agent: agent, explicit: explicit, operation: operation, affinityScope: affinityScope}, true
}

// agentModels answers Agent model discovery. It is an authenticated read of
// permitted aliases, not an inference request.
func (s *Server) agentModels(x *gatewayExchange) {
	w, r, client, agent, explicit := x.w, x.r, x.client, x.agent, x.explicit
	if !client.LegacyDirect {
		models := []map[string]string{}
		for _, route := range client.Routes {
			if route.Agent == agent.Grant() && (explicit == "" || explicit == route.SourceID) {
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
		if agent == AgentCodex && model.Codex || agent == AgentClaudeCode && model.Claude {
			compatible = append(compatible, model)
		}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": compatible})
}

// decode accepts only the inference operations and a bounded JSON object body.
func (x *gatewayExchange) decode() bool {
	w, r := x.w, x.r
	if r.Method != "POST" || (x.operation != "responses" && x.operation != "messages" && x.operation != "messages/count_tokens") {
		problem(w, 404, "endpoint not found")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		problem(w, 413, "request body exceeds 32 MiB")
		return false
	}
	var body object
	if json.Unmarshal(b, &body) != nil || body == nil {
		problem(w, 400, "JSON object required")
		return false
	}
	x.raw, x.body = b, body
	return true
}

// selectTargets resolves the Client route for the requested model and asks the
// router for eligible targets under this session's affinity scope.
func (s *Server) selectTargets(x *gatewayExchange) bool {
	w := x.w
	model := rawString(x.body["model"])
	id, model, e := x.config.Resolve(x.client, x.agent, model, x.explicit)
	if e != nil {
		problem(w, 403, e.Error())
		return false
	}
	x.session = sessionID(x.agent, x.r.Header, x.body)
	targets, e := s.Router.Targets(x.config, x.affinityScope, x.agent, id, model, x.session, hasContinuation(x.body))
	if e != nil {
		status := 503
		if hasContinuation(x.body) {
			status = 409
		}
		problem(w, status, e.Error())
		return false
	}
	x.targets = targets
	return true
}

// admitQuota atomically reserves a request slot before any upstream call.
// A blocked request is answered without settlement or activity.
func (s *Server) admitQuota(x *gatewayExchange) bool {
	w, client := x.w, x.client
	clientID := "local"
	if client != nil {
		clientID = client.ID
	}
	x.activity = Activity{ID: strings.ReplaceAll(newID(), "-", ""), Timestamp: time.Now().UTC(), Kind: "request", Agent: string(x.agent), ClientID: clientID}
	quota, err := s.Quotas.Admit(x.activity, client.Quota)
	if err != nil {
		problem(w, 503, "Client quota ledger unavailable")
		return false
	}
	if quota.Blocked != "" {
		w.Header().Set("Retry-After", strconv.FormatInt(quota.RetryAfter, 10))
		problem(w, 429, "Client quota exceeded: "+quota.Blocked)
		return false
	}
	x.start = time.Now()
	return true
}

// finishExchange settles the admitted request against the actual source/model
// price and records its activity exactly once.
func (s *Server) finishExchange(x *gatewayExchange) {
	activity, usage := &x.activity, &x.usage
	activity.Duration = time.Since(x.start).Milliseconds()
	activity.Input = usage.totalInput(x.upstreamMessages)
	activity.Output = usage.Output
	activity.CacheRead = usage.CacheRead
	activity.CacheWrite = usage.CacheWrite
	var price *ModelPrice
	if source, ok := x.config.Source(activity.Source); ok {
		if p, exists := source.Prices[activity.Model]; exists {
			price = &p
		}
	}
	if err := s.Quotas.Settle(*activity, price, x.upstreamMessages); err != nil {
		log.Printf("quota settlement failed: %v", err)
	}
	if e := s.Activity.Record(*activity); e != nil {
		log.Printf("activity write failed: %v", e)
	}
}

// attempt sends the request to target i and answers the Agent, unless the
// failure is eligible for the next target before any response was committed.
// It reports whether the caller should continue with the next target.
func (s *Server) attempt(x *gatewayExchange, i int, t Target) bool {
	w, agent, operation, activity := x.w, x.agent, x.operation, &x.activity
	activity.Error = ""
	activity.Source = t.Source.ID
	activity.Model = t.Model
	activity.Attempts = i + 1
	protocol := t.upstreamProtocol(agent)
	wire, tr, e := prepare(x.body, t.Source.Kind, t.Model, agent, protocol)
	if e != nil {
		activity.Status = 400
		activity.Error = e.Error()
		problem(w, 400, e.Error())
		return false
	}
	if operation == "messages/count_tokens" && protocol != "messages" {
		activity.Status = 422
		activity.Error = "token counting is unavailable on this Responses source"
		problem(w, 422, activity.Error)
		return false
	}
	x.upstreamMessages = protocol == "messages"
	s.trace(activity.ID, fmt.Sprintf("upstream-%d-req.json", i), wire)
	res, e := s.send(x, t, wire, protocol, upstreamPath(t, agent, protocol, operation))
	if e != nil || res == nil {
		if x.r.Context().Err() != nil {
			// The Client left before any upstream response. This is not a
			// source failure: do not cool the member or try another target.
			activity.Status = 499
			activity.Error = "client disconnected before upstream response"
			return false
		}
		activity.Status = 502
		activity.Error = "upstream connection or authentication failed"
		if e != nil && !strings.Contains(e.Error(), "https://") {
			activity.Error = e.Error()
		}
		s.Router.Fail(t, activity.Error)
		if i+1 < len(x.targets) {
			return true
		}
		problem(w, 502, activity.Error)
		return false
	}
	activity.Status = res.StatusCode
	if retryable(res.StatusCode) && i+1 < len(x.targets) {
		s.Router.FailResponse(t, res.StatusCode, res.Header)
		res.Body.Close()
		return true
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
		return false
	}
	if operation != "messages/count_tokens" {
		if e = s.Router.Bind(t); e != nil {
			res.Body.Close()
			activity.Status = 500
			activity.Error = "could not persist session affinity"
			problem(w, 500, activity.Error)
			return false
		}
	}
	if agent == AgentCodex && protocol == "responses" && (t.Source.Kind == SourceCopilot || t.Source.Kind == SourcePeer && res.Header.Get("X-Reasoning-Included") == "true") {
		w.Header().Set("X-Reasoning-Included", "true")
	}
	w.Header().Set("X-PacaRoute-Request", activity.ID)
	w.Header().Set("X-PacaRoute-Source", t.Source.ID)
	w.Header().Set("Cache-Control", "no-cache")
	if strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") {
		s.relayStream(x, t, tr, protocol, res)
	} else {
		s.relayJSON(x, tr, res)
	}
	res.Body.Close()
	return false
}

// upstreamPath maps the Agent operation to the target's native endpoint path.
func upstreamPath(t Target, agent AgentProtocol, protocol, operation string) string {
	upstreamPath := "/responses"
	if protocol == "messages" {
		upstreamPath = "/v1/messages"
	}
	if operation == "messages/count_tokens" {
		upstreamPath += "/count_tokens"
	}
	if t.Source.Kind == SourcePeer {
		prefix := "/codex"
		if agent == AgentClaudeCode {
			prefix = "/cc"
		}
		upstreamPath = prefix + "/" + operation
	}
	return upstreamPath
}

// send performs one upstream request with an immutable Auth lease. A rejected
// Copilot lease is refreshed and retried once; no other retry happens here.
func (s *Server) send(x *gatewayExchange, t Target, wire []byte, protocol, upstreamPath string) (*http.Response, error) {
	r, c := x.r, x.config
	var res *http.Response
	var e error
	for authAttempt := 0; authAttempt < 2; authAttempt++ {
		l, err := s.Auth.Lease(r.Context(), c, t.Source, authAttempt > 0)
		if err != nil {
			e = err
			break
		}
		base := t.Source.Endpoint
		if t.Source.Kind == SourceCopilot {
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
		if t.Source.Kind == SourcePeer {
			req.Header.Set(peerHopHeader, strconv.Itoa(x.hops+1))
			if x.session != "" {
				req.Header.Set("X-PacaRoute-Session", peerSessionHash(s.AdminKey, x.affinityScope, x.agent, t.Source.ID, x.session))
			}
		}
		if t.Source.Kind == SourceCopilot {
			s.Auth.copilotHeaders(req, l, bytes.Contains(wire, []byte("input_image")) || bytes.Contains(wire, []byte(`"image"`)))
			req.Header.Set("X-Initiator", "agent")
			req.Header.Set("X-Request-Id", newID())
			req.Header.Set("Openai-Intent", "conversation-panel")
		}
		res, e = s.Auth.http.client(c.SourceProxy(t.Source.ID)).Do(req)
		if e != nil {
			break
		}
		if authAttempt == 0 && (res.StatusCode == 401 || res.StatusCode == 403) && t.Source.Kind == SourceCopilot {
			io.Copy(io.Discard, io.LimitReader(res.Body, 65536))
			res.Body.Close()
			res = nil
			continue
		}
		break
	}
	return res, e
}

// relayStream relays or translates this one committed upstream stream. After
// the downstream status is written no other source may take over the response.
func (s *Server) relayStream(x *gatewayExchange, t Target, tr *translation, protocol string, res *http.Response) {
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
		if tr.to == "responses" {
			e = tr.streamResponses(w, reader, &x.usage)
		} else {
			e = tr.streamMessages(w, reader, &x.usage)
		}
	} else {
		e = relaySSE(w, struct {
			io.Reader
			io.Closer
		}{reader, res.Body}, &x.usage, protocol)
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
}

// relayJSON relays a bounded non-stream response, translating it when needed.
func (s *Server) relayJSON(x *gatewayExchange, tr *translation, res *http.Response) {
	w, activity := x.w, &x.activity
	var e error
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20+1))
	if err != nil || len(data) > 32<<20 {
		e = errors.New("upstream response incomplete or too large")
	} else {
		x.usage.observe(data)
		s.trace(activity.ID, "upstream-res.json", data)
		if tr != nil && x.operation != "messages/count_tokens" {
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

// peerSessionHash derives the session identity sent to a peer PacaRoute. It is
// keyed by this installation's admin key so the peer cannot correlate the
// downstream session, and is stable so peer affinity survives restarts.
func peerSessionHash(adminKey, scope string, agent AgentProtocol, sourceID, session string) string {
	h := sha256.Sum256([]byte(adminKey + "\x00" + scope + "\x00" + string(agent) + "\x00" + sourceID + "\x00" + session))
	return hex.EncodeToString(h[:])
}
