package pacaroute

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/protocol"
	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// forwardedRequestHeaders are copied from the Agent request to the upstream:
// Anthropic protocol options followed by the Agent session headers.
var forwardedRequestHeaders = append(append([]string{"anthropic-version", "anthropic-beta"}, codexSessionHeaders...), claudeSessionHeaders...)

// send performs one upstream request with an immutable Auth lease. A rejected
// Copilot lease is refreshed and retried once; no other retry happens here.
func (s *Server) send(x *gatewayExchange, t routing.Target, wire []byte, upstreamProto, upstreamPath string) (*http.Response, error) {
	r, c := x.r, x.config
	if upstreamProto == "chat" {
		upstreamPath = "/chat/completions"
	}
	var res *http.Response
	var e error
	for authAttempt := 0; authAttempt < 2; authAttempt++ {
		l, err := s.Auth.Lease(r.Context(), c, t.Source, authAttempt > 0)
		if err != nil {
			e = err
			break
		}
		base := t.Source.Endpoint
		if t.Source.Kind == domain.SourceCopilot {
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
		for _, k := range forwardedRequestHeaders {
			if v := r.Header.Get(k); v != "" {
				req.Header.Set(k, v)
			}
		}
		if upstreamProto == "messages" && req.Header.Get("anthropic-version") == "" {
			req.Header.Set("anthropic-version", protocol.AnthropicVersion)
		}
		req.Header.Set(l.Header, l.Value)
		if t.Source.Kind == domain.SourcePeer {
			req.Header.Set(peerHopHeader, strconv.Itoa(x.hops+1))
			if x.session != "" {
				req.Header.Set(peerSessionHeader, peerSessionHash(s.AdminKey, x.affinityScope, x.agent, t.Source.ID, x.session))
			}
		}
		if t.Source.Kind == domain.SourceCopilot {
			s.Auth.copilotHeaders(req, l, bytes.Contains(wire, []byte("input_image")) || bytes.Contains(wire, []byte(`"image"`)))
			req.Header.Set("X-Initiator", "agent")
			req.Header.Set("X-Request-Id", domain.NewID())
			req.Header.Set("Openai-Intent", "conversation-panel")
		}
		res, e = s.Auth.http.Client(c.SourceProxy(t.Source.ID)).Do(req)
		if e != nil {
			break
		}
		if authAttempt == 0 && (res.StatusCode == 401 || res.StatusCode == 403) && t.Source.Kind == domain.SourceCopilot {
			io.Copy(io.Discard, io.LimitReader(res.Body, 65536))
			res.Body.Close()
			res = nil
			continue
		}
		break
	}
	return res, e
}

// peerSessionHash derives the session identity sent to a peer PacaRoute. It is
// keyed by this installation's admin key so the peer cannot correlate the
// downstream session, and is stable so peer affinity survives restarts.
func peerSessionHash(adminKey, scope string, agent domain.AgentProtocol, sourceID, session string) string {
	h := sha256.Sum256([]byte(adminKey + "\x00" + scope + "\x00" + string(agent) + "\x00" + sourceID + "\x00" + session))
	return hex.EncodeToString(h[:])
}
