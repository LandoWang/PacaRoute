package pacaroute

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// A downstream disconnect must not penalize the source selected by the pool.
type downstreamStreamError struct{ error }

func writeStreamFrame(w io.Writer, frame []byte) error {
	n, err := w.Write(frame)
	if err == nil && n != len(frame) {
		err = io.ErrShortWrite
	}
	if err == nil {
		if rw, ok := w.(http.ResponseWriter); ok {
			err = http.NewResponseController(rw).Flush()
			if errors.Is(err, http.ErrNotSupported) {
				err = nil
			}
		} else if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	if err != nil {
		return &downstreamStreamError{err}
	}
	return nil
}

type upstreamStreamError struct {
	event, code, message string
	forwarded            bool
}

func (e *upstreamStreamError) Error() string {
	detail := e.event
	if e.code != "" {
		detail += " (" + e.code + ")"
	}
	if e.message != "" {
		detail += ": " + e.message
	}
	return "upstream " + detail
}

func streamEventError(m object, forwarded bool) error {
	typ := rawString(m["type"])
	detail := m
	if typ == "response.failed" {
		detail = obj(obj(m["response"])["error"])
	} else if nested := obj(m["error"]); len(nested) > 0 {
		detail = nested
	}
	code := rawString(detail["code"])
	if code == "" && detail != nil && rawString(detail["type"]) != "error" {
		code = rawString(detail["type"])
	}
	return &upstreamStreamError{event: streamDetail(typ), code: streamDetail(code), message: streamDetail(rawString(detail["message"])), forwarded: forwarded}
}

// Keep diagnostic fields useful without persisting entire upstream payloads.
func streamDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 2048 {
		return string(r[:2048]) + "…"
	}
	return s
}

func streamReadError(err error, terminal, last string) error {
	return fmt.Errorf("upstream stream ended before %s (last event: %s): %w", terminal, streamDetail(last), err)
}

func emitStreamError(w io.Writer, agent, message string) error {
	if agent == "codex" {
		// Codex consumes response.failed, but ignores bare error events. This is
		// a failure terminal only; never synthesize a successful completion.
		return emitSSE(w, "response.failed", map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]string{"code": "server_error", "message": message}}})
	}
	return emitSSE(w, "error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": message}})
}
