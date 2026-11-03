package pacaroute

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// Admission health distinguishes source/model cooldown from manual pause and
// zero weight. Snapshots and expiration must not change active session bindings.
func TestRoutingHealthMatchesAdmissionAndExpiry(t *testing.T) {
	s, _, _ := streamPool(t, partialResponse, true)
	s.Handler().ServeHTTP(httptest.NewRecorder(), streamRequest(context.Background(), fullHistoryRequest))
	c := cloneConfig(s.Store.Snapshot())
	h := s.Router.Health(&c)
	if len(h.Cooldowns) != 1 || h.Cooldowns[0].SourceID != "test" || h.Cooldowns[0].Model != "deployed" || h.Cooldowns[0].Error == "" || h.Pools[0].Available != 1 || h.Pools[0].Total != 2 {
		t.Fatalf("incorrect admission snapshot: %+v", h)
	}
	remaining := h.Cooldowns[0].Until.Sub(h.ObservedAt)
	if remaining <= 0 || remaining > 30*time.Second {
		t.Fatal("invalid cooldown deadline", remaining)
	}
	// A cooling model does not disable other deployments on the same source.
	c.Pools[0].Members = append(c.Pools[0].Members, domain.Member{ID: "other", SourceID: "test", Model: "other-model", Weight: 1})
	if s.Router.Health(&c).Pools[0].Available != 2 {
		t.Fatal("cooldown incorrectly applied to all source models")
	}
	c.Pools[0].Members[2].Weight = 0
	c.SourceState.Paused = []string{"healthy"}
	if s.Router.Health(&c).Pools[0].Available != 0 {
		t.Fatal("paused/zero-weight members hide exhausted pool")
	}
	h.Cooldowns[0].Error = "changed by reader"
	if s.Router.Health(&c).Cooldowns[0].Error == "changed by reader" {
		t.Fatal("mutable snapshot leaked into Router")
	}
	s.Router.ExpireCooldown("test", "deployed")
	if next := s.Router.Health(&c); len(next.Cooldowns) != 0 || next.Pools[0].Available != 1 {
		t.Fatal("expired cooldown still blocks admission", next)
	}
}

func TestRoutingHealthIsAuthenticatedManagementState(t *testing.T) {
	s, _, _ := streamPool(t, partialResponse, true)
	s.Handler().ServeHTTP(httptest.NewRecorder(), streamRequest(context.Background(), fullHistoryRequest))
	for _, authorized := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/api/status", nil)
		if authorized {
			r.Header.Set("Authorization", "Bearer "+s.AdminKey)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if !authorized {
			if w.Code != 401 {
				t.Fatal("routing health leaked without management key")
			}
			continue
		}
		var status struct{ Routing RoutingHealth }
		if json.Unmarshal(w.Body.Bytes(), &status) != nil || len(status.Routing.Cooldowns) != 1 || status.Routing.Pools[0].Available != 1 {
			t.Fatal("status API omits runtime health", w.Body.String())
		}
	}
}
