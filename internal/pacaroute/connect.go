package pacaroute

import (
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
)

// connect configures a caller's machine without starting a gateway or opening
// the service database. Keys enter on stdin and are never emitted in previews.
func connectClient(a arguments, in io.Reader) (map[string]any, error) {
	agent, model := a.get("agent", ""), a.get("model", "")
	if !agentValid(agent) || !modelIdentifier.MatchString(model) {
		return nil, errors.New("valid Agent and model alias required")
	}
	base := strings.TrimRight(a.get("url", ""), "/")
	u, e := url.Parse(base)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("provide the PacaRoute HTTP(S) URL without credentials")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	local := a.get("local", "") == "true"
	if local && !loopback {
		return nil, errors.New("default local Client requires a loopback URL")
	}
	key := "pacaroute-local"
	if !local {
		b, e := io.ReadAll(io.LimitReader(in, 8193))
		if e != nil {
			return nil, e
		}
		defer clear(b)
		key = strings.TrimSpace(string(b))
		if len(key) < 16 || len(key) > 8192 || strings.ContainsAny(key, "\r\n\x00") {
			return nil, errors.New("provide the Client API key on stdin")
		}
	}
	suffix := "/codex"
	if agent == "claude-code" {
		suffix = "/cc"
	}
	p, e := planConnection(agent, base+suffix, model, key)
	if e != nil {
		return nil, e
	}
	result := map[string]any{"path": p.path, "changes": p.changes}
	if a.get("apply", "") == "true" {
		backup, e := p.apply(p.fingerprint)
		if e != nil {
			return nil, e
		}
		result["backup_path"] = backup
	}
	return result, nil
}
