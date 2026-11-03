package pacaroute

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// retryHintHeaders are the upstream retry hints relayed to the Agent and read
// for throttle cooldowns.
var retryHintHeaders = []string{"Retry-After", "Retry-After-Ms", "X-Ms-Retry-After-Ms"}

type Cooldown struct {
	Kind     string    `json:"kind"`
	SourceID string    `json:"source_id"`
	Model    string    `json:"model"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
	Until    time.Time `json:"until"`
}

type PoolHealth struct {
	ID        string            `json:"id"`
	Available int               `json:"available"`
	Total     int               `json:"total"`
	Models    []PoolModelHealth `json:"models"`
}
type PoolModelHealth struct {
	Model     string `json:"model"`
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
func (r *Router) Health(c *domain.Config) RoutingHealth {
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
		modelHealth := map[string]*PoolModelHealth{}
		for _, name := range pool.ModelNames() {
			modelHealth[name] = &PoolModelHealth{Model: name}
		}
		for _, m := range pool.Members {
			group := modelHealth[pool.MemberModel(m)]
			group.Total++
			_, exists := c.Source(m.SourceID)
			if exists && m.Weight > 0 && !c.Paused(m.SourceID) && !r.cooldown[m.SourceID+":"+m.Model].Until.After(now) {
				p.Available++
				group.Available++
			}
		}
		for _, name := range pool.ModelNames() {
			p.Models = append(p.Models, *modelHealth[name])
		}
		h.Pools = append(h.Pools, p)
	}
	return h
}

func (r *Router) Fail(t Target, reason ...string) {
	if t.Pool == nil {
		return
	}
	message := "upstream request failed"
	if len(reason) > 0 && reason[0] != "" {
		message = cooldownDetail(reason[0])
	}
	kind := "error"
	if strings.Contains(message, "rate_limit_") {
		kind = "throttle"
	}
	r.failCooldown(t, message, 30*time.Second, kind)
}

// Retry hints apply to the concrete source/model, never the whole account or
// public alias. Parallel failures must not shorten a previously announced wait.
func (r *Router) FailResponse(t Target, status int, headers http.Header) {
	message := fmt.Sprintf("upstream returned HTTP %d", status)
	if status != http.StatusTooManyRequests {
		r.Fail(t, message)
		return
	}
	delay := 30 * time.Second
	if value, err := strconv.ParseFloat(headers.Get("retry-after-ms"), 64); err == nil && value > 0 {
		delay = time.Duration(math.Min(value, 86400000) * float64(time.Millisecond))
	} else if value, err := strconv.ParseFloat(headers.Get("x-ms-retry-after-ms"), 64); err == nil && value > 0 {
		delay = time.Duration(math.Min(value, 86400000) * float64(time.Millisecond))
	} else if value, err := strconv.ParseFloat(headers.Get("Retry-After"), 64); err == nil && value > 0 {
		delay = time.Duration(math.Min(value, 86400) * float64(time.Second))
	} else if at, err := http.ParseTime(headers.Get("Retry-After")); err == nil && at.After(time.Now()) {
		delay = time.Until(at)
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	if delay < time.Second {
		delay = time.Second
	}
	r.failCooldown(t, message, delay, "throttle")
}
func (r *Router) failCooldown(t Target, message string, delay time.Duration, kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	key := t.Source.ID + ":" + t.Model
	until := now.Add(delay)
	if prior := r.cooldown[key]; prior.Until.After(until) {
		until = prior.Until
	}
	if len(r.cooldown) >= 20000 {
		oldestKey := ""
		var oldest time.Time
		for k, entry := range r.cooldown {
			if !entry.Until.After(now) {
				delete(r.cooldown, k)
				continue
			}
			if oldestKey == "" || entry.Until.Before(oldest) {
				oldestKey, oldest = k, entry.Until
			}
		}
		if len(r.cooldown) >= 20000 {
			delete(r.cooldown, oldestKey)
		}
	}
	r.cooldown[key] = Cooldown{SourceID: t.Source.ID, Model: t.Model, Error: cooldownDetail(message), FailedAt: now, Until: until, Kind: kind}
}

// cooldownDetail bounds the failure text kept in a cooldown entry: whitespace is
// collapsed and long text is cut at 2048 runes.
func cooldownDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 2048 {
		return string(r[:2048]) + "…"
	}
	return s
}
