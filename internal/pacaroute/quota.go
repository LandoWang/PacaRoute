package pacaroute

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Amounts are decimal USD strings in configuration and integer micro-USD in the
// ledger. A price snapshot travels with every settlement; history is immutable.
type ModelPrice struct {
	Input      string `json:"input_usd"`
	Cached     string `json:"cached_usd"`
	Output     string `json:"output_usd"`
	CacheWrite string `json:"cache_write_usd,omitempty"`
}
type ClientQuota struct {
	WindowSeconds int64  `json:"window_seconds"`
	Requests      int64  `json:"requests,omitempty"`
	InputTokens   int64  `json:"input_tokens,omitempty"`
	Cost          string `json:"cost_usd,omitempty"`
}

var moneyPattern = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,6})?$`)

func moneyMicros(value string) (int64, error) {
	if !moneyPattern.MatchString(value) {
		return 0, errors.New("USD amounts need at most six decimal places")
	}
	parts := strings.SplitN(value, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	return strconv.ParseInt(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
}
func moneyText(micros int64) string { return fmt.Sprintf("%d.%06d", micros/1000000, micros%1000000) }
func (p ModelPrice) validate() error {
	for _, v := range []string{p.Input, p.Cached, p.Output} {
		if _, e := moneyMicros(v); e != nil {
			return e
		}
	}
	if p.CacheWrite != "" {
		_, e := moneyMicros(p.CacheWrite)
		return e
	}
	return nil
}
func (q *ClientQuota) validate() error {
	if q == nil {
		return nil
	}
	if q.WindowSeconds < 60 || q.WindowSeconds > 366*24*3600 || q.Requests < 0 || q.Requests > 1e12 || q.InputTokens < 0 || q.InputTokens > 1e15 {
		return errors.New("invalid Client quota window or limit")
	}
	if q.Cost != "" {
		_, e := moneyMicros(q.Cost)
		return e
	}
	return nil
}
func priceUsage(v Activity, p *ModelPrice, messages bool) (*int64, string) {
	if p == nil {
		return nil, "missing_price"
	}
	if v.Input == nil || v.Output == nil {
		return nil, "missing_usage"
	}
	inputRate, e := moneyMicros(p.Input)
	if e != nil {
		return nil, "invalid_price"
	}
	cachedRate, e := moneyMicros(p.Cached)
	if e != nil {
		return nil, "invalid_price"
	}
	outputRate, e := moneyMicros(p.Output)
	if e != nil {
		return nil, "invalid_price"
	}
	cached, write := int64(0), int64(0)
	if v.CacheRead != nil {
		cached = *v.CacheRead
	} else if inputRate != cachedRate {
		return nil, "missing_cache_usage"
	}
	writeRate := inputRate
	if messages {
		if v.CacheWrite == nil {
			return nil, "missing_cache_usage"
		}
		write = *v.CacheWrite
		if write > 0 {
			if p.CacheWrite == "" {
				return nil, "missing_cache_write_price"
			}
			writeRate, e = moneyMicros(p.CacheWrite)
			if e != nil {
				return nil, "invalid_price"
			}
		}
	}
	ordinary := *v.Input - cached - write
	if ordinary < 0 || cached < 0 || write < 0 || *v.Output < 0 {
		return nil, "invalid_usage"
	}
	sum := new(big.Int)
	for _, term := range [][2]int64{{ordinary, inputRate}, {cached, cachedRate}, {write, writeRate}, {*v.Output, outputRate}} {
		sum.Add(sum, new(big.Int).Mul(big.NewInt(term[0]), big.NewInt(term[1])))
	}
	// Round once per request, upwards to the micro-USD ledger unit.
	sum.Add(sum, big.NewInt(999999))
	sum.Div(sum, big.NewInt(1000000))
	if !sum.IsInt64() {
		return nil, "cost_overflow"
	}
	value := sum.Int64()
	return &value, ""
}

type QuotaStore struct{ db *sql.DB }
type QuotaState struct {
	WindowSeconds int64        `json:"window_seconds"`
	Requests      int64        `json:"requests"`
	InputTokens   int64        `json:"input_tokens"`
	CostMicros    int64        `json:"cost_microusd"`
	Cost          string       `json:"cost_usd"`
	Pending       int64        `json:"pending"`
	Unpriced      int64        `json:"unpriced"`
	MissingUsage  int64        `json:"missing_usage"`
	Blocked       string       `json:"blocked,omitempty"`
	RetryAfter    int64        `json:"retry_after_seconds,omitempty"`
	Limits        *ClientQuota `json:"limits"`
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

func quotaState(q quotaQuery, client string, limit *ClientQuota, now time.Time) (QuotaState, error) {
	window := int64(86400)
	if limit != nil {
		window = limit.WindowSeconds
	}
	v := QuotaState{WindowSeconds: window, Limits: limit}
	var first sql.NullInt64
	e := q.QueryRow(`SELECT COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(cost_microusd),0),COALESCE(SUM(state='reserved'),0),COALESCE(SUM(state='unresolved'),0),COALESCE(SUM(finished_ms IS NOT NULL AND input_tokens IS NULL),0),MIN(started_ms) FROM quota_requests WHERE client_id=? AND started_ms>?`, client, now.UnixMilli()-window*1000).Scan(&v.Requests, &v.InputTokens, &v.CostMicros, &v.Pending, &v.Unpriced, &v.MissingUsage, &first)
	v.Cost = moneyText(v.CostMicros)
	if limit != nil {
		cost, _ := moneyMicros(limit.Cost)
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
func (q *QuotaStore) State(client string, limit *ClientQuota, now time.Time) (QuotaState, error) {
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
func (q *QuotaStore) Admit(v Activity, limit *ClientQuota) (QuotaState, error) {
	tx, e := q.db.Begin()
	if e != nil {
		return QuotaState{}, e
	}
	defer tx.Rollback()
	state, e := quotaState(tx, v.ClientID, limit, v.Timestamp)
	if e != nil || state.Blocked != "" {
		return state, e
	}
	_, e = tx.Exec(`INSERT INTO quota_requests(request_id,client_id,agent,started_ms,state) VALUES(?,?,?,?,'reserved')`, v.ID, v.ClientID, v.Client, v.Timestamp.UnixMilli())
	if e != nil {
		return state, e
	}
	if e = billingEvent(tx, v.ID, "reserve", v.Timestamp, map[string]any{"client_id": v.ClientID, "requests": 1, "reserved_microusd": 0}); e != nil {
		return state, e
	}
	return state, tx.Commit()
}
func (q *QuotaStore) Settle(v Activity, p *ModelPrice, messages bool) error {
	cost, reason := priceUsage(v, p, messages)
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
