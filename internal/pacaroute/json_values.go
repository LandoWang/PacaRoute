package pacaroute

import "encoding/json"

type object = map[string]json.RawMessage

func obj(b json.RawMessage) object       { v := object{}; _ = json.Unmarshal(b, &v); return v }
func array(b json.RawMessage) []object   { var v []object; _ = json.Unmarshal(b, &v); return v }
func rawString(b json.RawMessage) string { var s string; _ = json.Unmarshal(b, &s); return s }
func jsonBytes(v any) json.RawMessage    { b, _ := json.Marshal(v); return b }
