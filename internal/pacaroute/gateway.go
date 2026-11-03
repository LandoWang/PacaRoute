package pacaroute

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

const (
	// maxRequestBody bounds an Agent request body.
	maxRequestBody = 32 << 20
	// maxUpstreamErrorBody bounds a relayed upstream error response.
	maxUpstreamErrorBody = 2 << 20
	// statusClientClosedRequest records an Agent that left before completion.
	statusClientClosedRequest = 499
	// statusLoopDetected answers a request that reached the peer hop limit.
	statusLoopDetected = 508
)

func (s *Server) gateway(w http.ResponseWriter, r *http.Request) { s.gatewayFor(w, r, false) }

// gatewayExchange is the execution state of one Agent request. It owns the
// configuration snapshot captured once at entry, the authenticated Client and
// grant, the selected targets, the activity being recorded and the usage that
// settlement observes. finishExchange is the single finalization owner.
type gatewayExchange struct {
	w                http.ResponseWriter
	r                *http.Request
	config           *domain.Config
	client           *domain.Client
	peer             bool
	hops             int
	agent            domain.AgentProtocol
	explicit         string
	operation        string
	affinityScope    string
	raw              []byte
	body             object
	session          string
	targets          []Target
	activity         usage.Activity
	usage            usage.Usage
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
		problem(w, statusLoopDetected, "PacaRoute forwarding loop or hop limit reached")
		return nil, false
	}
	agent, explicit, operation := parseGatewayPath(r.URL.Path)
	grant := agent.Grant()
	if peer {
		grant = domain.GrantPeer
	}
	if !client.AgentEnabled(grant) {
		problem(w, 404, "endpoint not found")
		return nil, false
	}
	affinityScope := affinityClient(client)
	if peer {
		client = peerClientView(c, client, agent)
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
	models, e := s.Catalog.Models(r.Context(), id)
	if e != nil {
		problem(w, 502, e.Error())
		return
	}
	// Management shows the full catalog; legacy Agent discovery retains
	// the compatible subset rather than advertising unsupported protocols.
	compatible := []domain.Model{}
	for _, model := range models {
		if agent == domain.AgentCodex && model.Codex || agent == domain.AgentClaudeCode && model.Claude {
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
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
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
	id, model, e := resolveRoute(x.config, x.client, x.agent, model, x.explicit)
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
	x.activity = usage.Activity{ID: strings.ReplaceAll(domain.NewID(), "-", ""), Timestamp: time.Now().UTC(), Kind: "request", Agent: string(x.agent), ClientID: clientID}
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
	activity, observed := &x.activity, &x.usage
	activity.Duration = time.Since(x.start).Milliseconds()
	activity.Input = observed.TotalInput(x.upstreamMessages)
	activity.Output = observed.Output
	activity.CacheRead = observed.CacheRead
	activity.CacheWrite = observed.CacheWrite
	var price *domain.ModelPrice
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
			activity.Status = statusClientClosedRequest
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
		for _, header := range retryHintHeaders {
			if value := res.Header.Get(header); value != "" {
				w.Header().Set(header, value)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(res.Body, maxUpstreamErrorBody))
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
	if agent == domain.AgentCodex && protocol == "responses" && (t.Source.Kind == domain.SourceCopilot || t.Source.Kind == domain.SourcePeer && res.Header.Get("X-Reasoning-Included") == "true") {
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

func retryable(status int) bool {
	return status == 401 || status == 403 || status == 408 || status == 429 || status >= 500
}

// Keep imported local affinity stable without merging any network Client scope.
func affinityClient(client *domain.Client) string {
	if client.Local && client.ID == "default" {
		return "local"
	}
	if client.ID == "local" {
		return "client:local"
	}
	return client.ID
}
