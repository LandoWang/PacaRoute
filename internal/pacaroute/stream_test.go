package pacaroute

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func streamPool(t *testing.T, failedWire string, failover bool) (*Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	bad, good := &atomic.Int32{}, &atomic.Int32{}
	s, _, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.HasPrefix(r.URL.Path, "/healthy/") {
			good.Add(1)
			fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":3}}}\n\n")
		} else {
			bad.Add(1)
			fmt.Fprint(w, failedWire)
		}
	})
	c := cloneConfig(store.Snapshot())
	healthy := c.Sources[0]
	healthy.ID = "healthy"
	healthy.Endpoint += "/healthy"
	c.Sources = append(c.Sources, healthy)
	c.Pools = []Pool{{ID: "pool", Name: "Pool", Agent: "codex", Model: "coding", Failover: failover, Members: []Member{{ID: "bad", SourceID: "test", Model: "deployed", Weight: 1}, {ID: "good", SourceID: "healthy", Model: "deployed", Weight: 1}}}}
	c.Clients[0].Routes[0].SourceID = "pool:pool"
	c.Clients[0].Routes[0].Model = "coding"
	saveTest(t, store, c)
	if err := s.Auth.SetKeys(map[string]string{"healthy": "synthetic-only"}); err != nil {
		t.Fatal(err)
	}
	targets, err := s.Router.Targets(store.Snapshot(), "alice", "codex", "pool:pool", "coding", "stream-session", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Source.ID == "test" {
			if err := s.Router.Bind(target); err != nil {
				t.Fatal(err)
			}
		}
	}
	return s, bad, good
}

func streamRequest(ctx context.Context, body string) *http.Request {
	r := httptest.NewRequest("POST", "/codex/responses", strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer alice-key")
	r.Header.Set("X-PacaRoute-Session", "stream-session")
	return r
}

const partialResponse = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
const fullHistoryRequest = `{"model":"coding","stream":true,"input":"hello"}`

// A started stream is never replayed. A later full-history client retry may
// escape its failed sticky source; opaque continuations and disabled failover may not.
func TestStreamFailureRecoversOnNextClientRetry(t *testing.T) {
	for _, failover := range []bool{true, false} {
		t.Run(fmt.Sprint(failover), func(t *testing.T) {
			s, bad, good := streamPool(t, partialResponse, failover)
			first := httptest.NewRecorder()
			s.Handler().ServeHTTP(first, streamRequest(context.Background(), fullHistoryRequest))
			if bad.Load() != 1 || good.Load() != 0 || !strings.HasPrefix(first.Body.String(), partialResponse) || strings.Contains(first.Body.String(), `"type":"response.completed"`) {
				t.Fatal("partial output was lost, replayed or fabricated", first.Body.String())
			}
			if !strings.Contains(first.Body.String(), `"code":"server_error"`) || !strings.Contains(first.Body.String(), "response.output_text.delta") {
				t.Fatal("Responses error is missing protocol or last-event detail", first.Body.String())
			}
			for _, body := range []string{`{"model":"coding","previous_response_id":"resp-bound"}`, `{"model":"coding","input":[{"type":"item_reference","id":"item-bound"}]}`} {
				bound := httptest.NewRecorder()
				s.Handler().ServeHTTP(bound, streamRequest(context.Background(), body))
				if bound.Code != 409 || good.Load() != 0 || bad.Load() != 1 {
					t.Fatal("source-bound continuation migrated", bound.Code, bound.Body.String())
				}
			}
			retry := httptest.NewRecorder()
			s.Handler().ServeHTTP(retry, streamRequest(context.Background(), fullHistoryRequest))
			if failover {
				if retry.Code != 200 || good.Load() != 1 || bad.Load() != 1 || retry.Header().Get("X-PacaRoute-Source") != "healthy" {
					t.Fatal("client retry did not leave the failed member", retry.Code, retry.Body.String(), bad.Load(), good.Load())
				}
			} else if retry.Code != 503 || good.Load() != 0 || bad.Load() != 1 {
				t.Fatal("disabled failover was ignored", retry.Code, retry.Body.String())
			}
			failed := s.Activity.Recent(true)
			if len(failed) != 1 || failed[0].Status != 502 || failed[0].Source != "test" || !strings.Contains(failed[0].Error, "EOF") || !strings.Contains(failed[0].Error, "response.output_text.delta") || failed[0].Input != nil {
				t.Fatalf("lost error or invented usage: %+v", failed)
			}
		})
	}
}

// Native failure terminals remain byte-for-byte intact. A bare Responses error
// needs a failure terminal so Codex can surface its reason rather than generic EOF.
func TestStreamUpstreamErrorFidelityAndCooldown(t *testing.T) {
	for _, data := range []string{
		`{"type":"response.failed","response":{"error":{"code":"server_error","message":"provider temporarily unavailable","unknown":42}}}`,
		`{"type":"error","code":"rate_limit_exceeded","message":"provider temporarily unavailable","unknown":42}`,
		`{"type":"error","error":{"type":"overloaded_error","message":"provider temporarily unavailable","unknown":42}}`,
	} {
		wire := "event: error\ndata: " + data + "\n\n"
		s, bad, good := streamPool(t, wire, true)
		first := httptest.NewRecorder()
		s.Handler().ServeHTTP(first, streamRequest(context.Background(), fullHistoryRequest))
		if !strings.HasPrefix(first.Body.String(), wire) {
			t.Fatal("upstream error changed", first.Body.String())
		}
		if strings.Contains(data, `"type":"response.failed"`) && first.Body.String() != wire {
			t.Fatal("native failure terminal changed", first.Body.String())
		}
		if !strings.Contains(first.Body.String(), `"type":"response.failed"`) {
			t.Fatal("Responses error lacks semantic failure terminal", first.Body.String())
		}
		records := s.Activity.Recent(true)
		if len(records) != 1 || !strings.Contains(records[0].Error, "provider temporarily unavailable") {
			t.Fatal("upstream error detail discarded", records)
		}
		s.Handler().ServeHTTP(httptest.NewRecorder(), streamRequest(context.Background(), fullHistoryRequest))
		if good.Load() != 1 || bad.Load() != 1 {
			t.Fatal("upstream error did not cool down sticky member")
		}
	}
}

type disconnectedWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	mode   string
}

func (w *disconnectedWriter) Write(b []byte) (int, error) {
	if w.mode == "write" {
		return 0, io.ErrClosedPipe
	}
	if w.mode == "cancel" {
		w.cancel()
	}
	return w.ResponseRecorder.Write(b)
}
func (w *disconnectedWriter) FlushError() error {
	if w.mode == "flush" {
		return io.ErrClosedPipe
	}
	return nil
}

func TestClientStreamDisconnectDoesNotCooldownSource(t *testing.T) {
	for _, mode := range []string{"write", "flush", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := streamPool(t, partialResponse, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &disconnectedWriter{httptest.NewRecorder(), cancel, mode}
			s.Handler().ServeHTTP(w, streamRequest(ctx, fullHistoryRequest))
			targets, err := s.Router.Targets(s.Store.Snapshot(), "alice", "codex", "pool:pool", "coding", "stream-session", false)
			if err != nil || targets[0].Source.ID != "test" {
				t.Fatal("downstream failure penalized source", err)
			}
			rows := s.Activity.Recent(true)
			if len(rows) != 1 || rows[0].Status != 499 {
				t.Fatal("client disconnect misclassified", rows)
			}
		})
	}
}

// Chat Completions' sentinel alone cannot complete a Responses/Messages turn.
func TestBareDoneIsNotSemanticCompletion(t *testing.T) {
	var out strings.Builder
	wire := partialResponse + "data: [DONE]\n\n"
	err := relaySSE(&out, strings.NewReader(wire), &Usage{}, "responses")
	if !errors.Is(err, io.EOF) || out.String() != wire {
		t.Fatal("sentinel hid missing terminal or native bytes changed", err)
	}
}

// Captured Azure shape: a nested error precedes the response.failed terminal.
// Both frames must reach Codex, which ignores the first one.
func TestAzureErrorDoesNotHideFollowingFailureTerminal(t *testing.T) {
	wire := "data: {\"type\":\"error\",\"error\":{\"code\":\"rate_limit_reached\",\"message\":\"deployment throughput exceeded\"}}\n\n" +
		"event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_qa\",\"status\":\"failed\",\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"deployment throughput exceeded\"},\"unknown\":42}}\n\n"
	var out strings.Builder
	err := relaySSE(&out, strings.NewReader(wire), &Usage{}, "responses")
	var failure *upstreamStreamError
	if !errors.As(err, &failure) || !failure.forwarded || out.String() != wire || !strings.Contains(err.Error(), "rate_limit_exceeded") {
		t.Fatal("Azure failure terminal was discarded", err, out.String())
	}
}

func TestErroredResponsesStreamDoesNotWaitIndefinitely(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	go func() { _, _ = io.WriteString(writer, "data: {\"type\":\"error\",\"message\":\"failed\"}\n\n") }()
	result := make(chan error, 1)
	go func() { result <- relaySSE(io.Discard, reader, &Usage{}, "responses") }()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "failed") {
			t.Fatal("upstream error detail lost", err)
		}
	case <-time.After(3 * time.Second):
		reader.Close()
		t.Fatal("waiting for transport EOF after error")
	}
}

func TestTranslatedStreamErrorsRetainDetails(t *testing.T) {
	for _, to := range []string{"responses", "messages"} {
		t.Run(to, func(t *testing.T) {
			s, _, store := gatewayTest(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"provider busy\"}}\n\n")
			})
			c := cloneConfig(store.Snapshot())
			agent, path, body := "codex", "/codex/responses", fullHistoryRequest
			if to == "responses" {
				agent, path, body = "claude-code", "/cc/v1/messages", `{"model":"coding","stream":true,"messages":[{"role":"user","content":"hello"}],"max_tokens":100}`
			} else {
				c.Sources[0].Kind = "anthropic"
			}
			c.Clients[0].Routes[0].Agent = agent
			saveTest(t, store, c)
			r := httptest.NewRequest("POST", path, strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer alice-key")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			text := w.Body.String()
			if !strings.Contains(text, "provider busy") || !strings.Contains(text, "overloaded_error") || strings.Contains(text, "message_stop") || strings.Contains(text, "response.completed") {
				t.Fatal("translation hid error or fabricated completion", text)
			}
			shape := `"code":"server_error"`
			if agent == "claude-code" {
				shape = `"type":"api_error"`
			}
			if !strings.Contains(text, shape) {
				t.Fatal("wrong downstream protocol", text)
			}
		})
	}
}
