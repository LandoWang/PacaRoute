package pacaroute

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// peerSessionHeader carries a router-derived session identity between PacaRoute
// installations. Agent session headers are listed in lookup order.
const peerSessionHeader = "X-PacaRoute-Session"

var (
	codexSessionHeaders  = []string{"thread-id", "session-id", "session_id"}
	claudeSessionHeaders = []string{"X-Claude-Code-Session-Id"}
)

var legacySession = regexp.MustCompile(`(?:^|_)session_([A-Za-z0-9-]{1,128})$`)

func sessionID(agent domain.AgentProtocol, h http.Header, body map[string]json.RawMessage) string {
	names := []string{peerSessionHeader, "X-AgentLink-Session"}
	if agent == domain.AgentCodex {
		names = append(names, codexSessionHeaders...)
	} else {
		names = append(names, claudeSessionHeaders...)
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
