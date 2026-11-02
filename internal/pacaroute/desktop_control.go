package pacaroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type desktopControl struct {
	mu        sync.Mutex
	store     *ConfigStore
	auth      *Auth
	port      int
	server    *http.Server
	gateway   *Server
	lastError string
}
type desktopSettings struct {
	Port int `json:"port"`
}

func (d *desktopControl) portValue() int { d.mu.Lock(); defer d.mu.Unlock(); return d.port }
func (d *desktopControl) state() (bool, int, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.server != nil, d.port, d.lastError
}
func (d *desktopControl) start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.server != nil {
		return nil
	}
	return d.startLocked(d.port)
}
func (d *desktopControl) startLocked(port int) error {
	listener, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if e != nil {
		d.lastError = fmt.Sprintf("端口 %d 不可用", port)
		return e
	}
	gateway, e := NewServer(d.store, d.auth)
	if e != nil {
		listener.Close()
		d.lastError = e.Error()
		return e
	}
	if port == 0 {
		port = listener.Addr().(*net.TCPAddr).Port
	}
	gateway.LocalManagement = true
	gateway.AllowLocal = true
	gateway.Port = port
	gateway.Desktop = d
	server := &http.Server{Handler: gateway.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 64 << 10}
	d.server = server
	d.gateway = gateway
	d.port = port
	d.lastError = ""
	go func() {
		e := server.Serve(listener)
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			d.mu.Lock()
			if d.server == server {
				d.server = nil
				d.lastError = e.Error()
			}
			d.mu.Unlock()
		}
	}()
	return nil
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
func (d *desktopControl) stop() {
	d.mu.Lock()
	server := d.server
	d.server = nil
	d.gateway = nil
	d.mu.Unlock()
	stopHTTP(server)
}
func (d *desktopControl) changePort(port int) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if port == d.port && d.server != nil {
		return fmt.Sprintf("http://127.0.0.1:%d/", port), nil
	}
	previous, oldGateway, oldPort := d.server, d.gateway, d.port
	if e := d.startLocked(port); e != nil {
		return "", e
	}
	b, _ := json.Marshal(desktopSettings{port})
	if e := atomicWrite(filepath.Join(d.store.Root, "desktop-settings.json"), b, 0600); e != nil {
		_ = d.server.Close()
		d.server = previous
		d.gateway = oldGateway
		d.port = oldPort
		return "", e
	}
	address := fmt.Sprintf("http://127.0.0.1:%d/#/connect?ticket=%s", port, d.gateway.tickets.issue())
	go stopHTTP(previous)
	return address, nil
}
func desktopPort(a arguments, root string, fallback int) int {
	if _, ok := a.flags["port"]; ok {
		return fallback
	}
	var v desktopSettings
	b, _ := os.ReadFile(filepath.Join(root, "desktop-settings.json"))
	if json.Unmarshal(b, &v) == nil && v.Port >= 1024 && v.Port <= 65535 {
		return v.Port
	}
	return fallback
}

func saveDesktopSettings(root string, port int) error {
	b, _ := json.Marshal(desktopSettings{port})
	return atomicWrite(filepath.Join(root, "desktop-settings.json"), b, 0600)
}
