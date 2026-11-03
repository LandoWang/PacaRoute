package transport

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

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

const MaxJSONBody = 8 << 20

type Pools struct {
	mu     sync.Mutex
	Values map[string]*http.Client
}

func (t *Pools) Client(p domain.Proxy) *http.Client {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.Values == nil {
		t.Values = map[string]*http.Client{}
	}
	key := p.Mode + ":" + p.Address
	if c := t.Values[key]; c != nil {
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
	if len(t.Values) >= 64 {
		for k, c := range t.Values {
			c.CloseIdleConnections()
			delete(t.Values, k)
		}
	}
	c := &http.Client{Transport: tr, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	t.Values[key] = c
	return c
}
func ProxyEnv(p domain.Proxy) []string {
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
func ReadJSONBody(r io.Reader, out any) error {
	b, e := io.ReadAll(io.LimitReader(r, MaxJSONBody+1))
	if e != nil {
		return e
	}
	if len(b) > MaxJSONBody {
		return errors.New("response too large")
	}
	return json.Unmarshal(b, out)
}
