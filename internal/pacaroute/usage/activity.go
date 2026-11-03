package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Activity struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Kind       string    `json:"kind"`
	Source     string    `json:"source"`
	Model      string    `json:"model"`
	Agent      string    `json:"client"`
	ClientID   string    `json:"client_id"`
	Status     int       `json:"status"`
	Input      *int64    `json:"input_tokens"`
	Output     *int64    `json:"output_tokens"`
	CacheRead  *int64    `json:"cache_read_tokens"`
	CacheWrite *int64    `json:"cache_write_tokens"`
	Duration   int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
	Attempts   int       `json:"attempts"`
}

type Totals struct {
	Requests int64 `json:"requests"`
	Failures int64 `json:"failures"`
	Input    int64 `json:"input_tokens"`
	Output   int64 `json:"output_tokens"`
	Cached   int64 `json:"cached_tokens"`
	Missing  int64 `json:"missing_usage"`
}

func (t *Totals) add(a Activity) {
	t.Requests++
	if a.Status >= 400 || a.Status == 0 {
		t.Failures++
	}
	if a.Input != nil {
		t.Input += *a.Input
	}
	if a.Output != nil {
		t.Output += *a.Output
	}
	if a.CacheRead != nil {
		t.Cached += *a.CacheRead
	}
	if a.Input == nil || a.Output == nil {
		t.Missing++
	}
}

type ActivityStore struct {
	root    string
	mu      sync.Mutex
	buckets map[string]map[activityDimension]Totals
	recent  []Activity
	offsets map[string]int64
	err     string
}

type activityDimension struct{ Source, Agent, Model string }

func NewActivityStore(root string) *ActivityStore {
	return &ActivityStore{root: filepath.Join(root, "runtime", "desktop-activity"), buckets: map[string]map[activityDimension]Totals{}, offsets: map[string]int64{}}
}
func (a *ActivityStore) ingest(v Activity) {
	hour := v.Timestamp.UTC().Format("2006-01-02T15:00:00Z")
	if a.buckets[hour] == nil {
		a.buckets[hour] = map[activityDimension]Totals{}
	}
	key := activityDimension{v.Source, v.ClientID, v.Model}
	t := a.buckets[hour][key]
	t.add(v)
	a.buckets[hour][key] = t
	a.recent = append(a.recent, v)
	if len(a.recent) > 500 {
		a.recent = a.recent[len(a.recent)-500:]
	}
}

// Refresh reads only appended bytes. The native Swift UI keeps its own SQLite activity index.
func (a *ActivityStore) refresh() {
	files, _ := filepath.Glob(filepath.Join(a.root, "*.jsonl"))
	for _, p := range files {
		f, e := os.Open(p)
		if e != nil {
			continue
		}
		info, _ := f.Stat()
		if info == nil || info.Size() <= a.offsets[p] {
			f.Close()
			continue
		}
		_, _ = f.Seek(a.offsets[p], 0)
		rd := bufio.NewReader(f)
		for {
			line, e := rd.ReadBytes('\n')
			if e != nil {
				break
			}
			a.offsets[p] += int64(len(line))
			var v Activity
			if len(line) < 1<<20 && json.Unmarshal(line, &v) == nil && v.Kind == "request" {
				a.ingest(v)
			}
		}
		f.Close()
	}
}
func (a *ActivityStore) Record(v Activity) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if e := os.MkdirAll(a.root, 0700); e != nil {
		return e
	}
	p := filepath.Join(a.root, "activity-"+v.Timestamp.UTC().Format("2006-01-02")+".jsonl")
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(append(b, '\n'))
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func (a *ActivityStore) Summary(from, to time.Time, source, client string) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refresh()
	total := Totals{}
	sources := map[string]Totals{}
	sourceModels := map[string]map[string]Totals{}
	timeline := map[string]Totals{}
	for hour, entries := range a.buckets {
		t, _ := time.Parse(time.RFC3339, hour)
		if t.Before(from) || !t.Before(to) {
			continue
		}
		for key, v := range entries {
			s, c := key.Source, key.Agent
			if (source != "" && source != s) || (client != "" && client != c) {
				continue
			}
			combine := func(dst Totals) Totals {
				dst.Requests += v.Requests
				dst.Failures += v.Failures
				dst.Input += v.Input
				dst.Output += v.Output
				dst.Cached += v.Cached
				dst.Missing += v.Missing
				return dst
			}
			total = combine(total)
			sources[s] = combine(sources[s])
			if sourceModels[s] == nil {
				sourceModels[s] = map[string]Totals{}
			}
			sourceModels[s][key.Model] = combine(sourceModels[s][key.Model])
			timeline[hour] = combine(timeline[hour])
		}
	}
	return map[string]any{"total": total, "sources": sources, "source_models": sourceModels, "timeline": timeline, "from": from, "to": to}
}
func (a *ActivityStore) Recent(errorsOnly bool) []Activity {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refresh()
	out := []Activity{}
	for i := len(a.recent) - 1; i >= 0; i-- {
		v := a.recent[i]
		if !errorsOnly || v.Status == 0 || v.Status >= 400 {
			out = append(out, v)
		}
	}
	return out
}
