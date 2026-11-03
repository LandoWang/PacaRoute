package usage

import "encoding/json"

type Usage struct{ Input, Output, CacheRead, CacheWrite *int64 }

func Number(m map[string]json.RawMessage, k string) *int64 {
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
func (u *Usage) Observe(b []byte) { u.observeLevel(b, true) }

// Events carry usage at the top level or in one response/message envelope.
// Do not recurse further: each level would re-parse the remaining frame.
func (u *Usage) observeLevel(b []byte, envelope bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return
	}
	if envelope {
		for _, k := range []string{"response", "message"} {
			if v, ok := m[k]; ok {
				u.observeLevel(v, false)
			}
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
	input := Number(values, "input_tokens")
	if input == nil {
		input = Number(values, "prompt_tokens")
	}
	output := Number(values, "output_tokens")
	if output == nil {
		output = Number(values, "completion_tokens")
	}
	if input != nil {
		u.Input = input
	}
	if output != nil {
		u.Output = output
	}
	if v := Number(values, "cache_read_input_tokens"); v != nil {
		u.CacheRead = v
	}
	if v := Number(values, "cache_creation_input_tokens"); v != nil {
		u.CacheWrite = v
	}
	for _, k := range []string{"input_tokens_details", "prompt_tokens_details"} {
		var d map[string]json.RawMessage
		_ = json.Unmarshal(values[k], &d)
		if v := Number(d, "cached_tokens"); v != nil {
			u.CacheRead = v
		}
	}
}
func (u *Usage) TotalInput(messages bool) *int64 {
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
func (u *Usage) ResponsesUsage() any {
	input := u.TotalInput(true)
	if input == nil || u.Output == nil {
		return nil
	}
	value := map[string]any{"input_tokens": *input, "output_tokens": *u.Output, "total_tokens": *input + *u.Output}
	if u.CacheRead != nil {
		value["input_tokens_details"] = map[string]any{"cached_tokens": *u.CacheRead}
	}
	return value
}
