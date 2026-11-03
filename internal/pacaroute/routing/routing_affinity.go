package routing

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
)

const (
	// maxAffinityJournal bounds the affinity journal read at startup.
	maxAffinityJournal = 96 << 20
	// maxLegacyJournal bounds the previous application's session journal.
	maxLegacyJournal = 64 << 20
)

type binding struct {
	Key      string    `json:"key"`
	Member   string    `json:"member"`
	Source   string    `json:"source"`
	Model    string    `json:"model"`
	Endpoint string    `json:"endpoint"`
	At       time.Time `json:"at"`
}

func NewRouter(root string) (*Router, error) {
	r := &Router{path: filepath.Join(root, "affinity-v2.jsonl"), bindings: map[string]binding{}, legacy: map[string]binding{}, cooldown: map[string]Cooldown{}}
	// Import the previous application's hashes lazily, only for the local Client. Other Clients cannot inherit them.
	legacyPath := filepath.Join(root, "runtime", "source-pool-sessions.jsonl")
	if f, e := os.Open(legacyPath); e == nil {
		defer f.Close()
		scan := bufio.NewScanner(io.LimitReader(f, maxLegacyJournal))
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
	if info.Size() > maxAffinityJournal {
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

// Bind appends once per assignment; existing sessions do not reread or rewrite history.
func (r *Router) Bind(t Target) error {
	if t.Affinity == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if old, ok := r.bindings[t.Affinity]; ok && old.Member == t.Member && old.Source == t.Source.ID && old.Model == t.Model && old.Endpoint == endpointIdentity(t.Source) && now.Sub(old.At) < 24*time.Hour {
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
	if e := atomicfile.Write(r.path, b.Bytes(), 0600); e != nil {
		return e
	}
	r.bindings = map[string]binding{}
	for _, v := range entries {
		r.bindings[v.Key] = v
	}
	r.writes = len(entries)
	return nil
}
