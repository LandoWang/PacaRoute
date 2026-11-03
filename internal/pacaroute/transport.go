package pacaroute

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const maxJSONBody = 8 << 20

type transports struct {
	mu     sync.Mutex
	values map[string]*http.Client
}

func (t *transports) client(p Proxy) *http.Client {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.values == nil {
		t.values = map[string]*http.Client{}
	}
	key := p.Mode + ":" + p.Address
	if c := t.values[key]; c != nil {
		return c
	}
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 64, MaxIdleConnsPerHost: 16, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 120 * time.Second, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if p.Mode == "direct" {
		tr.Proxy = nil
	}
	if p.Mode == "custom" {
		u, _ := url.Parse(p.Address)
		tr.Proxy = http.ProxyURL(u)
	}
	// Config edits must not grow the transport cache without bound.
	if len(t.values) >= 64 {
		for k, c := range t.values {
			c.CloseIdleConnections()
			delete(t.values, k)
		}
	}
	c := &http.Client{Transport: tr, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	t.values[key] = c
	return c
}
func proxyEnv(p Proxy) []string {
	env := os.Environ()
	keys := map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "http_proxy": true, "https_proxy": true, "all_proxy": true}
	if p.Mode == "system" {
		return env
	}
	out := []string{}
	for _, s := range env {
		k, _, _ := strings.Cut(s, "=")
		if !keys[k] {
			out = append(out, s)
		}
	}
	if p.Mode == "custom" {
		for k := range keys {
			out = append(out, k+"="+p.Address)
		}
	}
	return out
}
func readJSONBody(r io.Reader, out any) error {
	b, e := io.ReadAll(io.LimitReader(r, maxJSONBody+1))
	if e != nil {
		return e
	}
	if len(b) > maxJSONBody {
		return errors.New("response too large")
	}
	return json.Unmarshal(b, out)
}
