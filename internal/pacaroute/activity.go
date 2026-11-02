package pacaroute

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	Client     string    `json:"client"`
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
type Usage struct{ Input, Output, CacheRead, CacheWrite *int64 }

func number(m map[string]json.RawMessage, k string) *int64 {
	b, ok := m[k]
	if !ok || string(b) == "null" {
		return nil
	}
	var v int64
	if json.Unmarshal(b, &v) != nil || v < 0 {
		return nil
	}
	return &v
}
func (u *Usage) observe(b []byte) {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return
	}
	for _, k := range []string{"response", "message"} {
		if v, ok := m[k]; ok {
			u.observe(v)
		}
	}
	v, ok := m["usage"]
	if !ok {
		return
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(v, &values) != nil {
		return
	}
	input := number(values, "input_tokens")
	if input == nil {
		input = number(values, "prompt_tokens")
	}
	output := number(values, "output_tokens")
	if output == nil {
		output = number(values, "completion_tokens")
	}
	if input != nil {
		u.Input = input
	}
	if output != nil {
		u.Output = output
	}
	if v := number(values, "cache_read_input_tokens"); v != nil {
		u.CacheRead = v
	}
	if v := number(values, "cache_creation_input_tokens"); v != nil {
		u.CacheWrite = v
	}
	for _, k := range []string{"input_tokens_details", "prompt_tokens_details"} {
		var d map[string]json.RawMessage
		_ = json.Unmarshal(values[k], &d)
		if v := number(d, "cached_tokens"); v != nil {
			u.CacheRead = v
		}
	}
}

// Incremental, bounded SSE parser. Unknown frames remain byte-for-byte on the native path.
type sseReader struct{ r *bufio.Reader }

func newSSE(r io.Reader) *sseReader { return &sseReader{bufio.NewReaderSize(r, 32768)} }
func (s *sseReader) next() ([]byte, []byte, error) {
	var frame, data bytes.Buffer
	for {
		line, e := s.r.ReadSlice('\n')
		if frame.Len()+len(line) > 8<<20 {
			return nil, nil, fmt.Errorf("SSE event exceeds 8 MiB")
		}
		frame.Write(line)
		if e == bufio.ErrBufferFull {
			line = bytes.Clone(line) // Finish a long line without dropping its prefix.
			for e == bufio.ErrBufferFull {
				part, err := s.r.ReadSlice('\n')
				line = append(bytes.Clone(line), part...)
				frame.Write(part)
				e = err
				if len(line) > 8<<20 {
					return nil, nil, fmt.Errorf("SSE line too large")
				}
			}
		}
		trim := bytes.TrimRight(line, "\r\n")
		if bytes.HasPrefix(trim, []byte("data:")) {
			v := bytes.TrimPrefix(trim, []byte("data:"))
			v = bytes.TrimPrefix(v, []byte(" "))
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.Write(v)
		}
		if e != nil {
			if frame.Len() > 0 {
				return frame.Bytes(), data.Bytes(), nil
			}
			return nil, nil, e
		}
		if len(trim) == 0 {
			return frame.Bytes(), data.Bytes(), nil
		}
	}
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

type activityDimension struct{ Source, Client, Model string }

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
			s, c := key.Source, key.Client
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

func (u *Usage) totalInput(messages bool) *int64 {
	if !messages || u.Input == nil {
		return u.Input
	}
	total := *u.Input
	if u.CacheRead != nil {
		total += *u.CacheRead
	}
	if u.CacheWrite != nil {
		total += *u.CacheWrite
	}
	return &total
}

func (u *Usage) responsesUsage() any {
	input := u.totalInput(true)
	if input == nil || u.Output == nil {
		return nil
	}
	value := map[string]any{"input_tokens": *input, "output_tokens": *u.Output, "total_tokens": *input + *u.Output}
	if u.CacheRead != nil {
		value["input_tokens_details"] = map[string]any{"cached_tokens": *u.CacheRead}
	}
	return value
}
