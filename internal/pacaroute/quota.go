package pacaroute

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

type QuotaStore struct{ db *sql.DB }
type QuotaState struct {
	WindowSeconds int64               `json:"window_seconds"`
	Requests      int64               `json:"requests"`
	InputTokens   int64               `json:"input_tokens"`
	CostMicros    int64               `json:"cost_microusd"`
	Cost          string              `json:"cost_usd"`
	Pending       int64               `json:"pending"`
	Unpriced      int64               `json:"unpriced"`
	MissingUsage  int64               `json:"missing_usage"`
	Blocked       string              `json:"blocked,omitempty"`
	RetryAfter    int64               `json:"retry_after_seconds,omitempty"`
	Limits        *domain.ClientQuota `json:"limits"`
}

func newQuotaStore(db *sql.DB) (*QuotaStore, error) {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS quota_requests (
 request_id TEXT PRIMARY KEY, client_id TEXT NOT NULL, agent TEXT NOT NULL,
 started_ms INTEGER NOT NULL, finished_ms INTEGER, state TEXT NOT NULL,
 reserved_microusd INTEGER NOT NULL DEFAULT 0, released_microusd INTEGER NOT NULL DEFAULT 0,
 source_id TEXT, model TEXT, input_tokens INTEGER, cached_tokens INTEGER,
 cache_write_tokens INTEGER, output_tokens INTEGER, cost_microusd INTEGER,
 price_json TEXT, status INTEGER, reason TEXT);
 CREATE INDEX IF NOT EXISTS quota_client_window ON quota_requests(client_id,started_ms);
 CREATE TABLE IF NOT EXISTS billing_events (event_id TEXT PRIMARY KEY,request_id TEXT NOT NULL,phase TEXT NOT NULL,at_ms INTEGER NOT NULL,payload TEXT NOT NULL);`)
	return &QuotaStore{db}, e
}

type quotaQuery interface{ QueryRow(string, ...any) *sql.Row }

func quotaState(q quotaQuery, client string, limit *domain.ClientQuota, now time.Time) (QuotaState, error) {
	window := int64(86400)
	if limit != nil {
		window = limit.WindowSeconds
	}
	v := QuotaState{WindowSeconds: window, Limits: limit}
	var first sql.NullInt64
	e := q.QueryRow(`SELECT COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(cost_microusd),0),COALESCE(SUM(state='reserved'),0),COALESCE(SUM(state='unresolved'),0),COALESCE(SUM(finished_ms IS NOT NULL AND input_tokens IS NULL),0),MIN(started_ms) FROM quota_requests WHERE client_id=? AND started_ms>?`, client, now.UnixMilli()-window*1000).Scan(&v.Requests, &v.InputTokens, &v.CostMicros, &v.Pending, &v.Unpriced, &v.MissingUsage, &first)
	v.Cost = domain.MoneyText(v.CostMicros)
	if limit != nil {
		cost, _ := domain.MoneyMicros(limit.Cost)
		switch {
		case limit.Requests > 0 && v.Requests >= limit.Requests:
			v.Blocked = "requests"
		case limit.InputTokens > 0 && v.InputTokens >= limit.InputTokens:
			v.Blocked = "input_tokens"
		case cost > 0 && v.CostMicros >= cost:
			v.Blocked = "cost"
		}
	}
	if v.Blocked != "" && first.Valid {
		v.RetryAfter = (first.Int64 + window*1000 - now.UnixMilli() + 999) / 1000
		if v.RetryAfter < 1 {
			v.RetryAfter = 1
		}
	}
	return v, e
}
func (q *QuotaStore) State(client string, limit *domain.ClientQuota, now time.Time) (QuotaState, error) {
	return quotaState(q.db, client, limit, now)
}
func billingEvent(tx *sql.Tx, id, phase string, at time.Time, payload any) error {
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT INTO billing_events(event_id,request_id,phase,at_ms,payload) VALUES(?,?,?,?,?)`, id+":"+phase, id, phase, at.UnixMilli(), string(b))
	return e
}

// Requests reserve a slot atomically. Token/cost admission uses confirmed usage;
// no monetary reservation is enforced in this first soft-quota version.
func (q *QuotaStore) Admit(v usage.Activity, limit *domain.ClientQuota) (QuotaState, error) {
	tx, e := q.db.Begin()
	if e != nil {
		return QuotaState{}, e
	}
	defer tx.Rollback()
	var state QuotaState
	if limit.Enforced() {
		state, e = quotaState(tx, v.ClientID, limit, v.Timestamp)
	} else {
		// Without an enforced limit nothing can block; skip aggregating history.
		state = QuotaState{WindowSeconds: 86400, Limits: limit}
		if limit != nil {
			state.WindowSeconds = limit.WindowSeconds
		}
	}
	if e != nil || state.Blocked != "" {
		return state, e
	}
	_, e = tx.Exec(`INSERT INTO quota_requests(request_id,client_id,agent,started_ms,state) VALUES(?,?,?,?,'reserved')`, v.ID, v.ClientID, v.Agent, v.Timestamp.UnixMilli())
	if e != nil {
		return state, e
	}
	if e = billingEvent(tx, v.ID, "reserve", v.Timestamp, map[string]any{"client_id": v.ClientID, "requests": 1, "reserved_microusd": 0}); e != nil {
		return state, e
	}
	return state, tx.Commit()
}
func (q *QuotaStore) Settle(v usage.Activity, p *domain.ModelPrice, messages bool) error {
	cost, reason := usage.PriceUsage(v, p, messages)
	state := "settled"
	if cost == nil {
		state = "unresolved"
	}
	price, _ := json.Marshal(p)
	now := time.Now().UTC()
	tx, e := q.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.Exec(`UPDATE quota_requests SET finished_ms=?,state=?,source_id=?,model=?,input_tokens=?,cached_tokens=?,cache_write_tokens=?,output_tokens=?,cost_microusd=?,price_json=?,status=?,reason=?,released_microusd=reserved_microusd WHERE request_id=? AND state='reserved'`, now.UnixMilli(), state, v.Source, v.Model, v.Input, v.CacheRead, v.CacheWrite, v.Output, cost, string(price), v.Status, reason, v.ID)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return nil
	}
	if e = billingEvent(tx, v.ID, "settle", now, map[string]any{"source_id": v.Source, "model": v.Model, "input_tokens": v.Input, "cached_tokens": v.CacheRead, "cache_write_tokens": v.CacheWrite, "output_tokens": v.Output, "cost_microusd": cost, "price": p, "state": state, "reason": reason}); e != nil {
		return e
	}
	if e = billingEvent(tx, v.ID, "release", now, map[string]int64{"released_microusd": 0}); e != nil {
		return e
	}
	return tx.Commit()
}
