package pacaroute

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"
	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

func TestNativeCopilotRelayKeepsMessageIdentityAndRawTrace(t *testing.T) {
	source, err := os.ReadFile("../../tests/CopilotBridge.UnitTests/Fixtures/responses-sse-rolling-message-ids.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []domain.SourceKind{domain.SourceCopilot, domain.SourceOpenAI, domain.SourcePeer} {
		t.Run(string(kind), func(t *testing.T) {
			s := &Server{TraceDir: t.TempDir()}
			w := httptest.NewRecorder()
			x := &gatewayExchange{w: w, r: httptest.NewRequest("POST", "/codex/responses", nil), agent: domain.AgentCodex, activity: usage.Activity{ID: "qa-message"}}
			target := routing.Target{Source: domain.Source{Kind: kind}}
			s.relayStream(x, target, nil, "responses", &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(source))})
			if w.Code != 200 {
				t.Fatalf("unexpected status: %d", w.Code)
			}
			if kind == domain.SourceCopilot {
				if bytes.Contains(w.Body.Bytes(), []byte("opaque-message-done")) || !bytes.Contains(w.Body.Bytes(), []byte(`"item_id":"opaque-message-added"`)) {
					t.Fatal("native Copilot gateway did not stabilize message identity")
				}
			} else if !bytes.Equal(w.Body.Bytes(), source) {
				t.Fatal("other source passthrough changed")
			}
			trace, err := os.ReadFile(filepath.Join(s.TraceDir, "qa-message", "upstream-res.sse"))
			if err != nil || !bytes.Equal(trace, source) {
				t.Fatalf("raw upstream evidence changed: %v", err)
			}
		})
	}
}
