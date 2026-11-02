package pacaroute

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type binding struct {
	Key      string    `json:"key"`
	Member   string    `json:"member"`
	Source   string    `json:"source"`
	Model    string    `json:"model"`
	Endpoint string    `json:"endpoint"`
	At       time.Time `json:"at"`
}
type Router struct {
	mu       sync.Mutex
	path     string
	legacy   map[string]binding
	bindings map[string]binding
	cooldown map[string]Cooldown
	writes   int
}

func NewRouter(root string) (*Router, error) {
	r := &Router{path: filepath.Join(root, "affinity-v2.jsonl"), bindings: map[string]binding{}, legacy: map[string]binding{}, cooldown: map[string]Cooldown{}}
	// Import the previous application's hashes lazily, only for the local Client. Other Clients cannot inherit them.
	legacyPath := filepath.Join(root, "runtime", "source-pool-sessions.jsonl")
	if f, e := os.Open(legacyPath); e == nil {
		defer f.Close()
		scan := bufio.NewScanner(io.LimitReader(f, 64<<20))
		scan.Buffer(make([]byte, 4096), 65536)
		for scan.Scan() {
			var v struct {
				Key    string `json:"key"`
				Member string `json:"member_id"`
				Source string `json:"source_id"`
				Model  string `json:"model"`
			}
			if json.Unmarshal(scan.Bytes(), &v) == nil && len(v.Key) == 64 {
				r.legacy[strings.ToUpper(v.Key)] = binding{Key: v.Key, Member: v.Member, Source: v.Source, Model: v.Model, At: time.Now().UTC()}
			}
		}
		if e = scan.Err(); e != nil {
			return nil, e
		}
	}
	f, e := os.Open(r.path)
	if os.IsNotExist(e) {
		return r, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if info.Size() > 96<<20 {
		return nil, errors.New("affinity journal exceeds limit")
	}
	reader := bufio.NewReader(f)
	committed := int64(0)
	for {
		line, e := reader.ReadBytes('\n')
		if e == io.EOF {
			if len(line) > 0 {
				if e = os.Truncate(r.path, committed); e != nil {
					return nil, e
				}
			}
			break
		}
		if e != nil {
			return nil, e
		}
		committed += int64(len(line))
		var v binding
		if len(line) > 65536 || json.Unmarshal(line, &v) != nil || len(v.Key) != 64 {
			return nil, errors.New("invalid affinity journal")
		}
		if time.Since(v.At) < 30*24*time.Hour {
			r.bindings[v.Key] = v
		}
		r.writes++
	}
	return r, nil
}

type Target struct {
	Source   Source
	Model    string
	Member   string
	Affinity string
	Pool     *Pool
	Sticky   bool
}

func endpointIdentity(s Source) string { return s.Kind + ":" + s.AccountID + ":" + s.Endpoint }
func (r *Router) Targets(c *Config, client, agent, id, model, session string, continuation bool) ([]Target, error) {
	if !strings.HasPrefix(id, "pool:") {
		s, ok := c.Source(id)
		if !ok {
			return nil, errors.New("source not found")
		}
		if c.Paused(id) {
			return nil, errors.New("source is paused")
		}
		return []Target{{Source: s, Model: model}}, nil
	}
	p, ok := c.Pool(id)
	if !ok || p.Agent != agent {
		return nil, errors.New("pool does not match this Agent")
	}
	key := ""
	if session != "" {
		key = keyHash(client + "\x00" + agent + "\x00" + p.ID + "\x00" + session)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	prior, has := r.bindings[key]
	if !has && client == "local" && session != "" {
		legacyKey := strings.ToUpper(keyHash(agent + "\n" + p.ID + "\n" + session))
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
		seed = newID()
	}
	type scored struct {
		t     Target
		score float64
	}
	candidates := []scored{}
	for _, m := range p.Members {
		if m.Weight == 0 {
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

// Bind appends once per assignment; existing sessions do not reread or rewrite history.
func (r *Router) Bind(t Target) error {
	if t.Affinity == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if old, ok := r.bindings[t.Affinity]; ok && old.Member == t.Member && old.Source == t.Source.ID && old.Model == t.Model && now.Sub(old.At) < 24*time.Hour {
		return nil
	}
	value := binding{t.Affinity, t.Member, t.Source.ID, t.Model, endpointIdentity(t.Source), now}
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return e
	}
	_, e = f.Write(append(b, '\n'))
	if e == nil {
		e = f.Sync()
	}
	if e != nil {
		_ = f.Truncate(info.Size())
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	r.bindings[t.Affinity] = value
	r.writes++
	if len(r.bindings) > 100000 || r.writes > 2*len(r.bindings)+1024 {
		return r.compact(now)
	}
	return nil
}
func (r *Router) compact(now time.Time) error {
	entries := []binding{}
	for _, v := range r.bindings {
		if now.Sub(v.At) < 30*24*time.Hour {
			entries = append(entries, v)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].At.After(entries[j].At) })
	if len(entries) > 100000 {
		entries = entries[:80000]
	}
	var b bytes.Buffer
	for _, v := range entries {
		if e := json.NewEncoder(&b).Encode(v); e != nil {
			return e
		}
	}
	if e := atomicWrite(r.path, b.Bytes(), 0600); e != nil {
		return e
	}
	r.bindings = map[string]binding{}
	for _, v := range entries {
		r.bindings[v.Key] = v
	}
	r.writes = len(entries)
	return nil
}

func (r *Router) Fail(t Target, reason ...string) {
	if t.Pool == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cooldown) > 20000 {
		clear(r.cooldown)
	}
	message := "upstream request failed"
	if len(reason) > 0 && reason[0] != "" {
		message = streamDetail(reason[0])
	}
	now := time.Now().UTC()
	r.cooldown[t.Source.ID+":"+t.Model] = Cooldown{SourceID: t.Source.ID, Model: t.Model, Error: message, FailedAt: now, Until: now.Add(30 * time.Second)}
}

var legacySession = regexp.MustCompile(`(?:^|_)session_([A-Za-z0-9-]{1,128})$`)

func sessionID(agent string, h http.Header, body map[string]json.RawMessage) string {
	names := []string{"X-PacaRoute-Session", "X-AgentLink-Session"}
	if agent == "codex" {
		names = append(names, "thread-id", "session-id", "session_id")
	} else {
		names = append(names, "X-Claude-Code-Session-Id")
	}
	valid := func(v string) bool {
		return v != "" && len(v) <= 512 && strings.IndexFunc(v, func(r rune) bool { return r < 32 || r == 127 }) < 0
	}
	for _, name := range names {
		if vs := h.Values(name); len(vs) == 1 && valid(vs[0]) {
			return vs[0]
		}
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(body["metadata"], &m)
	v := m["user_id"]
	var text string
	if json.Unmarshal(v, &text) == nil {
		if len(text) > 4096 {
			return ""
		}
		if match := legacySession.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
		v = []byte(text)
	}
	var user struct {
		Session string `json:"session_id"`
	}
	if json.Unmarshal(v, &user) == nil && valid(user.Session) {
		return user.Session
	}
	return ""
}
func hasContinuation(body map[string]json.RawMessage) bool {
	if rawString(body["previous_response_id"]) != "" {
		return true
	}
	var input []map[string]json.RawMessage
	_ = json.Unmarshal(body["input"], &input)
	for _, v := range input {
		if rawString(v["type"]) == "item_reference" {
			return true
		}
	}
	return false
}
func rawString(b json.RawMessage) string { var s string; _ = json.Unmarshal(b, &s); return s }
func jsonBytes(v any) json.RawMessage    { b, _ := json.Marshal(v); return b }
func (c *Config) Resolve(client *Client, agent, model, explicit string) (string, string, error) {
	if !client.AgentEnabled(agent) {
		return "", "", errors.New("Agent is not enabled for this Client")
	}
	if !client.LegacyDirect {
		for _, r := range client.Routes {
			if r.Agent == agent && (r.MatchModel == "*" || r.MatchModel == model) {
				if explicit != "" && explicit != r.SourceID {
					return "", "", errors.New("client route does not permit this source")
				}
				return r.SourceID, r.Model, nil
			}
		}
		return "", "", errors.New("no route for this Client, Agent and model")
	}
	if explicit == "" {
		explicit = "copilot"
	}
	if strings.HasPrefix(explicit, "pool:") {
		p, ok := c.Pool(explicit)
		if !ok || p.Agent != agent {
			return "", "", errors.New("pool not found for Agent")
		}
		return explicit, p.Model, nil
	}
	if e := c.validateTarget(explicit, model, agent); e != nil {
		return "", "", e
	}
	return explicit, model, nil
}
func keyEqual(hash, key string) bool {
	expected, e := hex.DecodeString(hash)
	if e != nil {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	return len(expected) == 32 && constantEqual(expected, sum[:])
}
