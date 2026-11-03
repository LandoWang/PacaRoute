package pacaroute

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

type Router struct {
	mu       sync.Mutex
	path     string
	legacy   map[string]binding
	bindings map[string]binding
	cooldown map[string]Cooldown
	writes   int
}

type Target struct {
	Source   domain.Source
	Model    string
	Member   string
	Affinity string
	Pool     *domain.Pool
	Sticky   bool
}

func endpointIdentity(s domain.Source) string {
	return string(s.Kind) + ":" + s.AccountID + ":" + s.Endpoint
}
func (r *Router) Targets(c *domain.Config, scope string, agent domain.AgentProtocol, id, model, session string, continuation bool) ([]Target, error) {
	if !domain.IsPoolTarget(id) {
		s, ok := c.Source(id)
		if !ok {
			return nil, errors.New("source not found")
		}
		if c.Paused(id) {
			return nil, errors.New("source is paused")
		}
		r.mu.Lock()
		cooldown := r.cooldown[s.ID+":"+model]
		r.mu.Unlock()
		if cooldown.Until.After(time.Now()) {
			return nil, errors.New("source model is cooling until " + cooldown.Until.Format(time.RFC3339))
		}
		return []Target{{Source: s, Model: model}}, nil
	}
	p, ok := c.Pool(id)
	if !ok || p.Agent != agent || !p.HasModel(model) {
		return nil, errors.New("pool does not match this Agent")
	}
	key := ""
	if session != "" {
		key = affinityHash(scope + "\x00" + string(agent) + "\x00" + p.ID + "\x00" + session)
		if p.MultiModel {
			key = affinityHash(scope + "\x00" + string(agent) + "\x00" + p.ID + "\x00" + model + "\x00" + session)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	prior, has := r.bindings[key]
	if !has && !p.MultiModel && scope == "local" && session != "" {
		legacyKey := strings.ToUpper(affinityHash(string(agent) + "\n" + p.ID + "\n" + session))
		if v, ok := r.legacy[legacyKey]; ok {
			if source, found := c.Source(v.Source); found {
				v.Key = key
				v.Endpoint = endpointIdentity(source)
				prior = v
				has = true
			}
		}
	}
	result := []Target{}
	if has {
		for _, m := range p.Members {
			if p.MemberModel(m) != model {
				continue
			}
			if m.ID == prior.Member && m.SourceID == prior.Source && m.Model == prior.Model {
				if s, ok := c.Source(m.SourceID); ok && !c.Paused(s.ID) && prior.Endpoint == endpointIdentity(s) {
					if !r.cooldown[s.ID+":"+m.Model].Until.After(time.Now()) {
						result = append(result, Target{s, m.Model, m.ID, key, &p, true})
					}
				}
			}
		}
	}
	if continuation && (len(result) == 0) {
		return nil, errors.New("source-bound continuation cannot move; start a new session or replay full history")
	}
	if has && len(result) == 0 && !p.Failover {
		return nil, errors.New("sticky source unavailable and pool failover is disabled")
	}
	seed := key
	if seed == "" {
		seed = domain.NewID()
	}
	type scored struct {
		t     Target
		score float64
	}
	candidates := []scored{}
	for _, m := range p.Members {
		if m.Weight == 0 || p.MemberModel(m) != model {
			continue
		}
		s, ok := c.Source(m.SourceID)
		if !ok || c.Paused(s.ID) || r.cooldown[s.ID+":"+m.Model].Until.After(time.Now()) {
			continue
		}
		if len(result) > 0 && result[0].Member == m.ID {
			continue
		}
		sum := sha256.Sum256([]byte(seed + "\x00" + m.ID))
		u := (float64(binary.BigEndian.Uint64(sum[:8])>>11) + 1) / (1<<53 + 1)
		score := -math.Log(u) / float64(m.Weight)
		candidates = append(candidates, scored{Target{s, m.Model, m.ID, key, &p, false}, score})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })
	for _, v := range candidates {
		result = append(result, v.t)
	}
	if len(result) == 0 {
		return nil, errors.New("no healthy active source in pool")
	}
	if !p.Failover || continuation {
		return result[:1], nil
	}
	if len(result) > 3 {
		result = result[:3]
	}
	return result, nil
}

// affinityHash derives persisted session-binding and legacy import keys. Its
// SHA-256 hex output is a storage contract shared with existing journals.
func affinityHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func (t Target) upstreamProtocol(agent ...domain.AgentProtocol) string {
	if t.Source.Kind == domain.SourcePeer && len(agent) > 0 {
		return incomingProtocol(agent[0])
	}
	if t.Source.Kind == domain.SourceAnthropic || t.Source.Kind == domain.SourceCopilot && strings.HasPrefix(t.Model, "claude-") {
		return "messages"
	}
	return "responses"
}
