package pacaroute

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Both listeners share one application, Auth authority, router and activity store.
// Only the inference listener is replaceable; management remains reachable.
type desktopControl struct {
	closed     bool
	accepting  *atomic.Bool
	mu         sync.Mutex
	store      *ConfigStore
	auth       *Auth
	port       int
	adminPort  int
	bind       string
	nativeTray bool
	server     *http.Server
	listener   net.Listener
	management *http.Server
	gateway    *Server
	draining   map[*http.Server]bool
	lastError  string
}

func (d *desktopControl) portValue() int      { d.mu.Lock(); defer d.mu.Unlock(); return d.port }
func (d *desktopControl) managementPort() int { d.mu.Lock(); defer d.mu.Unlock(); return d.adminPort }
func (d *desktopControl) state() (bool, int, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.server != nil, d.port, d.lastError
}
func (d *desktopControl) status() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return map[string]any{"serving": d.server != nil, "port": d.port, "admin_port": d.adminPort, "draining": len(d.draining), "error": d.lastError}
}
func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 64 << 10}
}
func managementPath(path string) bool {
	return path == "/" || path == "/index.html" || path == "/mark.svg" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/assets/")
}
func (s *Server) splitHandler(management bool) http.Handler {
	handler := s.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if managementPath(r.URL.Path) != management {
			http.NotFound(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
func (d *desktopControl) startManagement() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("application is shutting down")
	}
	if d.management != nil {
		return nil
	}
	if d.gateway == nil {
		var err error
		d.gateway, err = NewServer(d.store, d.auth)
		if err != nil {
			return err
		}
		d.gateway.LocalManagement = true
		d.gateway.AllowLocal = true
	}
	if d.bind == "" {
		d.bind = "127.0.0.1"
	}
	d.gateway.Desktop = d
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(d.adminPort)))
	if err != nil {
		return fmt.Errorf("management port unavailable: %w", err)
	}
	d.adminPort = listener.Addr().(*net.TCPAddr).Port
	serving := true
	if saved := readDesktopSettings(d.store.Root); saved.Serving != nil {
		serving = *saved.Serving
	}
	if err = d.persist(d.port, serving); err != nil {
		listener.Close()
		return err
	}
	server := newHTTPServer(d.gateway.splitHandler(true))
	d.management = server
	go func() { _ = server.Serve(listener) }()
	return nil
}
func (d *desktopControl) start() error {
	if err := d.startManagement(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("application is shutting down")
	}
	if d.server != nil {
		return nil
	}
	return d.startLocked(d.port)
}
func (d *desktopControl) startLocked(port int) error {
	if port != 0 && port == d.adminPort {
		return errors.New("service and management ports must differ")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(d.bind, strconv.Itoa(port)))
	if err != nil {
		d.lastError = fmt.Sprintf("Service port %d unavailable", port)
		return err
	}
	port = listener.Addr().(*net.TCPAddr).Port
	if err = d.persist(port, true); err != nil {
		listener.Close()
		return err
	}
	previous, oldListener := d.server, d.listener
	accepting := &atomic.Bool{}
	accepting.Store(true)
	handler := d.gateway.splitHandler(false)
	server := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !accepting.Load() {
			problem(w, 503, "service listener is draining")
			return
		}
		handler.ServeHTTP(w, r)
	}))
	if d.accepting != nil {
		d.accepting.Store(false)
	}
	d.accepting = accepting
	d.server, d.listener, d.port, d.lastError = server, listener, port, ""
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.mu.Lock()
			defer d.mu.Unlock()
			if d.server == server {
				d.server = nil
				d.listener = nil
				d.lastError = "Service listener stopped unexpectedly"
			}
		}
	}()
	d.drainLocked(previous, oldListener)
	return nil
}

// Close admission synchronously, then let accepted requests drain without a
// deadline. Application exit is the only operation with a forced shutdown bound.
func (d *desktopControl) drainLocked(server *http.Server, listener net.Listener) {
	if server == nil {
		return
	}
	if d.draining == nil {
		d.draining = map[*http.Server]bool{}
	}
	d.draining[server] = true
	if listener != nil {
		_ = listener.Close()
	}
	go func() {
		_ = server.Shutdown(context.Background())
		d.mu.Lock()
		delete(d.draining, server)
		d.mu.Unlock()
	}()
}
func (d *desktopControl) stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("application is shutting down")
	}
	if err := d.persist(d.port, false); err != nil {
		return err
	}
	server, listener := d.server, d.listener
	if d.accepting != nil {
		d.accepting.Store(false)
	}
	d.server, d.listener, d.lastError = nil, nil, ""
	d.drainLocked(server, listener)
	return nil
}
func (d *desktopControl) close() {
	d.mu.Lock()
	d.closed = true
	if d.accepting != nil {
		d.accepting.Store(false)
	}
	servers := []*http.Server{d.server, d.management}
	for server := range d.draining {
		servers = append(servers, server)
	}
	d.server, d.management = nil, nil
	if d.listener != nil {
		_ = d.listener.Close()
		d.listener = nil
	}
	d.mu.Unlock()
	var wg sync.WaitGroup
	for _, server := range servers {
		if server != nil {
			wg.Add(1)
			go func() { defer wg.Done(); stopHTTP(server) }()
		}
	}
	wg.Wait()
}
func stopHTTP(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if server.Shutdown(ctx) != nil {
		_ = server.Close()
	}
}
func (d *desktopControl) changePort(port int) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return "", errors.New("application is shutting down")
	}
	if port < 1024 || port > 65535 || port == d.adminPort {
		return "", errors.New("service port must be 1024–65535 and differ from management")
	}
	if port != d.port {
		if d.server != nil {
			if err := d.startLocked(port); err != nil {
				return "", err
			}
		} else {
			if err := d.persist(port, false); err != nil {
				return "", err
			}
			d.port = port
			d.lastError = ""
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", d.adminPort), nil
}

func (d *desktopControl) startup(enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return setStartup(enabled, d.store.Root, d.port, d.auth.vault.root)
}
