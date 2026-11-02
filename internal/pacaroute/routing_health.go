package pacaroute

import (
	"sort"
	"time"
)

type Cooldown struct {
	SourceID string    `json:"source_id"`
	Model    string    `json:"model"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
	Until    time.Time `json:"until"`
}

type PoolHealth struct {
	ID        string `json:"id"`
	Available int    `json:"available"`
	Total     int    `json:"total"`
}

type RoutingHealth struct {
	ObservedAt time.Time    `json:"observed_at"`
	Cooldowns  []Cooldown   `json:"cooldowns"`
	Pools      []PoolHealth `json:"pools"`
}

// Health reports admission eligibility, not a successful upstream probe. It
// reads bounded in-memory state; polling never scans request or session logs.
func (r *Router) Health(c *Config) RoutingHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	h := RoutingHealth{ObservedAt: now, Cooldowns: []Cooldown{}, Pools: []PoolHealth{}}
	for key, entry := range r.cooldown {
		if !entry.Until.After(now) {
			delete(r.cooldown, key)
			continue
		}
		if _, exists := c.Source(entry.SourceID); exists {
			h.Cooldowns = append(h.Cooldowns, entry)
		}
	}
	sort.Slice(h.Cooldowns, func(i, j int) bool {
		a, b := h.Cooldowns[i], h.Cooldowns[j]
		if a.SourceID == b.SourceID {
			return a.Model < b.Model
		}
		return a.SourceID < b.SourceID
	})
	for _, pool := range c.Pools {
		p := PoolHealth{ID: pool.ID, Total: len(pool.Members)}
		for _, m := range pool.Members {
			_, exists := c.Source(m.SourceID)
			if exists && m.Weight > 0 && !c.Paused(m.SourceID) && !r.cooldown[m.SourceID+":"+m.Model].Until.After(now) {
				p.Available++
			}
		}
		h.Pools = append(h.Pools, p)
	}
	return h
}
