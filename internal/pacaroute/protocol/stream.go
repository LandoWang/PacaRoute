package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// maxSSEFrame bounds one SSE event and each of its lines.
const maxSSEFrame = 8 << 20

// A downstream disconnect must not penalize the source selected by the pool.
type DownstreamStreamError struct{ error }

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
		return &DownstreamStreamError{err}
	}
	return nil
}

type UpstreamStreamError struct {
	event, code, message string
	Forwarded            bool
}

func (e *UpstreamStreamError) Error() string {
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
	return &UpstreamStreamError{event: StreamDetail(typ), code: StreamDetail(code), message: StreamDetail(rawString(detail["message"])), Forwarded: forwarded}
}

// Keep diagnostic fields useful without persisting entire upstream payloads.
func StreamDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 2048 {
		return string(r[:2048]) + "…"
	}
	return s
}

func streamReadError(err error, terminal, last string) error {
	return fmt.Errorf("upstream stream ended before %s (last event: %s): %w", terminal, StreamDetail(last), err)
}

func EmitStreamError(w io.Writer, agent domain.AgentProtocol, message string) error {
	if agent == domain.AgentCodex {
		// Codex consumes response.failed, but ignores bare error events. This is
		// a failure terminal only; never synthesize a successful completion.
		return emitSSE(w, "response.failed", map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]string{"code": "server_error", "message": message}}})
	}
	return emitSSE(w, "error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": message}})
}

// Incremental, bounded SSE parser. Unknown frames remain byte-for-byte on the native path.
type sseReader struct{ r *bufio.Reader }

func NewSSE(r io.Reader) *sseReader { return &sseReader{bufio.NewReaderSize(r, 32768)} }
func (s *sseReader) Next() ([]byte, []byte, error) {
	var frame, data bytes.Buffer
	for {
		line, e := s.r.ReadSlice('\n')
		if frame.Len()+len(line) > maxSSEFrame {
			return nil, nil, fmt.Errorf("SSE event exceeds 8 MiB")
		}
		frame.Write(line)
		if e == bufio.ErrBufferFull {
			line = bytes.Clone(line) // Finish a long line without dropping its prefix.
			for e == bufio.ErrBufferFull {
				var part []byte
				part, e = s.r.ReadSlice('\n')
				line = append(line, part...)
				frame.Write(part)
				if len(line) > maxSSEFrame {
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

func RelaySSE(w io.Writer, r io.Reader, u *usage.Usage, protocol string) error {
	rd := NewSSE(r)
	last := "none"
	var pending error
	var errorDeadline *time.Timer
	defer func() {
		if errorDeadline != nil {
			errorDeadline.Stop()
		}
	}()
	for {
		frame, data, e := rd.Next()
		if e != nil {
			if pending != nil {
				return pending
			}
			return streamReadError(e, "response.completed or message_stop", last)
		}
		u.Observe(data)
		m := obj(data)
		typ := rawString(m["type"])
		if typ != "" {
			last = typ
		}
		if e = writeStreamFrame(w, frame); e != nil {
			return e
		}
		// The client may close as soon as the terminal frame is delivered. Do not
		// wait for transport EOF and misclassify its cancellation as a failed turn.
		if typ == "response.completed" || typ == "response.incomplete" || typ == "message_stop" {
			return nil
		}
		if typ == "response.failed" || (typ == "error" && protocol == "messages") {
			return streamEventError(m, true)
		}
		if typ == "error" {
			pending = streamEventError(m, false)
			// Azure can emit error followed by response.failed. Codex ignores the
			// first event, so preserve the actual terminal failure when present.
			// Bound the wait for providers that leave the errored connection open.
			if closer, ok := r.(io.Closer); ok && errorDeadline == nil {
				errorDeadline = time.AfterFunc(time.Second, func() { _ = closer.Close() })
			}
		}
	}
}
