package pacaroute

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func serve(ctx context.Context, a arguments, s *ConfigStore, auth *Auth, port int, in io.Reader, emit func(any), fail func(error) int) int {
	unlock, e := fileLock(ctx, filepath.Join(s.Root, "pacaroute-service.lock"), 200*time.Millisecond)
	if e != nil {
		return fail(errors.New("PacaRoute already runs for this data directory"))
	}
	defer unlock()
	server, e := NewServer(s, auth)
	if e != nil {
		return fail(e)
	}
	server.AllowLocal = a.get("allow-local", "") == "true" || os.Getenv("COPILOT_BRIDGE_DESKTOP") == "1"
	server.TraceDir = a.get("trace-dir", "")
	server.LocalManagement = a.get("local-management", "") == "true"
	server.Port = port
	if server.LocalManagement && a.get("admin-port", "") == "" && a.get("bind", "127.0.0.1") != "127.0.0.1" && a.get("bind", "") != "::1" {
		return fail(errors.New("local-management requires a loopback listener"))
	}
	if public := a.get("public-url", ""); public != "" {
		u, err := url.Parse(public)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fail(errors.New("public-url must be an HTTP(S) origin without a path"))
		}
		server.PublicURL = u.Scheme + "://" + u.Host
	}
	if os.Getenv("COPILOT_BRIDGE_SECRETS_STDIN") == "1" {
		reader := bufio.NewReader(in)
		line, e := reader.ReadBytes('\n')
		if e != nil && e != io.EOF {
			return fail(e)
		}
		var keys map[string]string
		if len(line) > 65536 || json.Unmarshal(line, &keys) != nil {
			return fail(errors.New("invalid desktop credential envelope"))
		}
		if e = auth.SetKeys(keys); e != nil {
			return fail(e)
		}
		go func() {
			for {
				line, e := reader.ReadBytes('\n')
				if e != nil {
					return
				}
				if len(line) > 65536 {
					continue
				}
				var keys map[string]string
				if json.Unmarshal(line, &keys) == nil {
					_ = auth.SetKeys(keys)
				}
			}
		}()
	}
	if value := a.get("admin-port", ""); value != "" {
		adminPort, err := strconv.Atoi(value)
		if err != nil || adminPort < 1024 || adminPort > 65535 || adminPort == port {
			return fail(errors.New("admin-port must be 1024–65535 and differ from port"))
		}
		control := &desktopControl{store: s, auth: auth, port: port, adminPort: adminPort, bind: a.get("bind", "127.0.0.1"), gateway: server}
		if err = control.startManagement(); err != nil {
			return fail(err)
		}
		defer control.close()
		if a.get("no-serve", "") == "true" {
			err = control.stop()
		} else {
			err = control.start()
		}
		// A gateway bind failure must not prevent repairing configuration in management.
		emit(map[string]any{"ok": true, "event": "ready", "name": "PacaRoute", "version": Version, "address": fmt.Sprintf("127.0.0.1:%d", adminPort), "service": control.status()})
		<-ctx.Done()
		return 0
	}
	if a.get("no-serve", "") == "true" {
		return fail(errors.New("no-serve requires --admin-port"))
	}
	listener, e := net.Listen("tcp", net.JoinHostPort(a.get("bind", "127.0.0.1"), strconv.Itoa(port)))
	if e != nil {
		return fail(fmt.Errorf("listen failed: %w", e))
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 64 << 10}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	emit(map[string]any{"ok": true, "event": "ready", "name": "PacaRoute", "version": Version, "address": listener.Addr().String()})
	select {
	case e = <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return fail(e)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e = httpServer.Shutdown(shutdown); e != nil {
			_ = httpServer.Close()
		}
	}
	return 0
}
