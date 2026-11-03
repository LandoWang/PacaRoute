package pacaroute

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFiles embed.FS

func (s *Server) web(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		problem(w, 405, "GET required")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	b, e := webFiles.ReadFile("web/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}
func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if s.PublicURL != "" && !(s.Desktop != nil && strings.HasPrefix(r.URL.Path, "/api/")) {
		return origin == s.PublicURL
	}
	if origin == "" {
		return false
	}
	u, e := url.Parse(origin)
	if e != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Host == r.Host && u.Scheme == scheme
}
func (s *Server) admin(r *http.Request) bool {
	if key := bearer(r); key != "" {
		return constantEqual([]byte(key), []byte(s.AdminKey))
	}
	cookie, e := r.Cookie("pacaroute_admin")
	if e != nil || !constantEqual([]byte(cookie.Value), []byte(s.AdminKey)) {
		return false
	}
	return r.Method == "GET" || s.sameOrigin(r)
}
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(v); e != nil {
		return errors.New("invalid JSON request")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
func (s *Server) management(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == "login/ticket" && r.Method == "POST" {
		s.ticketLogin(w, r)
		return
	}
	if path == "login" && r.Method == "POST" {
		if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
			problem(w, 403, "same origin required")
			return
		}
		var v struct {
			Key string `json:"key"`
		}
		if decodeRequest(w, r, &v) != nil || !constantEqual([]byte(v.Key), []byte(s.AdminKey)) {
			problem(w, 401, "invalid management key")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "pacaroute_admin", Value: s.AdminKey, Path: "/api", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(s.PublicURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if !s.admin(r) {
		problem(w, 401, "management authentication required")
		return
	}
	if r.Header.Get("Origin") != "" && !s.sameOrigin(r) {
		problem(w, 403, "same origin required")
		return
	}
	switch {
	case path == "config/export" || path == "config/import":
		s.configTransferAPI(w, r, strings.TrimPrefix(path, "config/"))
	case strings.HasPrefix(path, "local/"):
		s.localAPI(w, r, strings.TrimPrefix(path, "local/"))
	case path == "logout" && r.Method == "POST":
		http.SetCookie(w, &http.Cookie{Name: "pacaroute_admin", Value: "", Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		writeJSON(w, 200, map[string]bool{"ok": true})
	case path == "service":
		s.serviceAPI(w, r)
	case path == "status" && r.Method == "GET":
		service := map[string]any{"serving": true}
		if s.Desktop != nil {
			service = s.Desktop.status()
		}
		writeJSON(w, 200, map[string]any{"name": "PacaRoute", "version": Version, "uptime_seconds": int(time.Since(s.started).Seconds()), "config_error": s.Store.Error(), "routing": s.Router.Health(s.Store.Snapshot()), "local_access": s.AllowLocal, "local_management": s.localAllowed(r), "desktop": s.Desktop != nil && s.Desktop.nativeTray && s.localAllowed(r), "service_control": s.Desktop != nil, "service": service, "port": s.servicePort(), "api_version": 1, "gateway_url": s.gatewayOrigin(r), "startup": s.Desktop != nil && s.Desktop.nativeTray && startupEnabled()})
	case path == "config" && r.Method == "GET":
		writeJSON(w, 200, s.Store.Snapshot())
	case path == "config" && r.Method == "PUT":
		var c Config
		if e := decodeRequest(w, r, &c); e != nil {
			problem(w, 400, e.Error())
			return
		}
		if c.Revision == "" {
			problem(w, 428, "configuration revision required")
			return
		}
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, s.Store.Snapshot())
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/quota") && r.Method == "GET":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "clients/"), "/quota")
		client, ok := s.Store.Snapshot().Client(id)
		if !ok {
			problem(w, 404, "Client not found")
			return
		}
		value, e := s.Quotas.State(id, client.Quota, time.Now().UTC())
		if e != nil {
			problem(w, 503, "Client quota ledger unavailable")
			return
		}
		writeJSON(w, 200, value)
	case path == "clients" && r.Method == "POST":
		var v Client
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid Client")
			return
		}
		key := newKey()
		if v.Local {
			key = ""
			v.KeyHash = ""
		} else {
			v.KeyHash = keyHash(key)
		}
		c := cloneConfig(s.Store.Snapshot())
		c.Clients = append(c.Clients, v)
		if e := c.Validate(); e != nil {
			problem(w, 409, e.Error())
			return
		}
		if !v.Local {
			if e := s.Auth.SaveClientKey(r.Context(), v, key); e != nil {
				problem(w, 500, "cannot save Client credential")
				return
			}
		}
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"client": v, "api_key": key})
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/rotate") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "clients/"), "/rotate")
		c := cloneConfig(s.Store.Snapshot())
		key := newKey()
		found := false
		for i := range c.Clients {
			if c.Clients[i].ID == id && !c.Clients[i].Local {
				c.Clients[i].KeyHash = keyHash(key)
				found = true
			}
		}
		if !found {
			problem(w, 404, "Client not found")
			return
		}
		client, _ := c.Client(id)
		if e := s.Auth.SaveClientKey(r.Context(), *client, key); e != nil {
			problem(w, 500, "cannot save Client credential")
			return
		}
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"api_key": key})
	case strings.HasPrefix(path, "clients/") && strings.HasSuffix(path, "/key") && (r.Method == "GET" || r.Method == "PUT"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "clients/"), "/key")
		client, ok := s.Store.Snapshot().Client(id)
		if !ok {
			problem(w, 404, "Client not found")
			return
		}
		if r.Method == "PUT" {
			var v struct {
				Key string `json:"key"`
			}
			if decodeRequest(w, r, &v) != nil {
				problem(w, 400, "invalid Client credential")
				return
			}
			if e := s.Auth.SaveClientKey(r.Context(), *client, v.Key); e != nil {
				problem(w, 400, e.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		key, e := s.Auth.ClientKey(*client)
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"api_key": key, "available": key != ""})
	case strings.HasPrefix(path, "sources/") && strings.HasSuffix(path, "/pause") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "sources/"), "/pause")
		var v struct {
			Paused bool `json:"paused"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid pause value")
			return
		}
		c := cloneConfig(s.Store.Snapshot())
		paused := []string{}
		for _, p := range c.SourceState.Paused {
			if p != id {
				paused = append(paused, p)
			}
		}
		if v.Paused {
			paused = append(paused, id)
		}
		c.SourceState.Paused = paused
		if e := s.Store.Save(c, c.Revision); e != nil {
			problem(w, 409, e.Error())
			return
		}
		writeJSON(w, 200, s.Store.Snapshot())
	case strings.HasPrefix(path, "keys/") && r.Method == "PUT":
		id := strings.TrimPrefix(path, "keys/")
		if _, ok := s.Store.Snapshot().Source(id); !ok {
			problem(w, 404, "source not found")
			return
		}
		var v struct {
			Key string `json:"key"`
		}
		if decodeRequest(w, r, &v) != nil {
			problem(w, 400, "invalid credential")
			return
		}
		if e := s.Auth.SaveKey(r.Context(), id, v.Key); e != nil {
			problem(w, 400, e.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case strings.HasPrefix(path, "sources/") && strings.HasSuffix(path, "/discover-models") && r.Method == "GET":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "sources/"), "/discover-models")
		source, ok := s.Store.Snapshot().Source(id)
		if !ok {
			problem(w, 404, "source not found")
			return
		}
		result, e := s.Auth.DiscoverModels(r.Context(), s.Store.Snapshot(), source)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, result)
	case path == "model-discovery" && r.Method == "POST":
		var input struct {
			Source
			ProxyID string `json:"proxy_id"`
		}
		if decodeRequest(w, r, &input) != nil {
			problem(w, 400, "invalid source")
			return
		}
		source := input.Source
		draft := cloneConfig(s.Store.Snapshot())
		if !validID(source.ID) || source.ID == "copilot" {
			source.ID = "discovery-" + newID()[:8]
		}
		source.Name = "Model discovery"
		source.Deployments = []string{"discovery"}
		// Validate the draft independently of saved routes. Keep its source ID so
		// an existing resource-specific credential can override the account key.
		draft.Sources = []Source{source}
		draft.Pools = nil
		draft.Clients = nil
		draft.SourceState.Paused = nil
		if input.ProxyID != "" {
			draft.Proxies.Bindings[source.ID] = input.ProxyID
		}
		if e := draft.Validate(); e != nil {
			problem(w, 400, e.Error())
			return
		}
		source = draft.Sources[len(draft.Sources)-1]
		result, e := s.Auth.DiscoverModels(r.Context(), &draft, source)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, result)
	case path == "models" && r.Method == "GET":
		models, e := s.Auth.Models(r.Context(), r.URL.Query().Get("source"))
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"models": models})
	case path == "usage" && r.Method == "GET":
		to := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
		from := to.Add(-24 * time.Hour)
		for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
			if v := r.URL.Query().Get(key); v != "" {
				t, e := time.Parse(time.RFC3339, v)
				if e != nil {
					problem(w, 400, "timestamps must be RFC3339")
					return
				}
				*dst = t.UTC().Truncate(time.Hour)
			}
		}
		if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
			problem(w, 400, "choose a range up to one year")
			return
		}
		writeJSON(w, 200, s.Activity.Summary(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("client")))
	case (path == "errors" || path == "activity") && r.Method == "GET":
		writeJSON(w, 200, s.Activity.Recent(path == "errors"))
	case strings.HasPrefix(path, "accounts/"):
		s.accountAPI(w, r, strings.TrimPrefix(path, "accounts/"))
	case strings.HasPrefix(path, "logins/") && r.Method == "GET":
		s.loginJobs.mu.Lock()
		job := s.loginJobs.jobs[strings.TrimPrefix(path, "logins/")]
		var status map[string]any
		if job != nil {
			status = map[string]any{"done": job.done, "error": job.err}
		}
		s.loginJobs.mu.Unlock()
		if status == nil {
			problem(w, 404, "login not found")
			return
		}
		writeJSON(w, 200, status)
	default:
		problem(w, 404, "management endpoint not found")
	}
}

type loginJob struct {
	done   bool
	err    string
	cancel context.CancelFunc
}
type loginJobs struct {
	mu   sync.Mutex
	jobs map[string]*loginJob
}

func newLoginJobs() *loginJobs { return &loginJobs{jobs: map[string]*loginJob{}} }
func (s *Server) accountAPI(w http.ResponseWriter, r *http.Request, path string) {
	id, action, _ := strings.Cut(path, "/")
	account, ok := s.Store.Snapshot().Account(id)
	configured := false
	for _, a := range s.Store.Snapshot().Accounts {
		configured = configured || a.ID == id
	}
	if !ok || !configured {
		problem(w, 404, "account not found")
		return
	}
	if account.Kind == "azure" {
		switch {
		case action == "login" && r.Method == "POST":
			s.startAccountLogin(w, r, account)
		case action == "logout" && r.Method == "POST":
			if e := s.Auth.AzureLogin(r.Context(), id, true, func(any) {}); e != nil {
				problem(w, 400, e.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"logged_in": false})
		case r.Method == "GET":
			mapped := map[string]string{"status": "account-status", "info": "account", "subscriptions": "azure-subscriptions", "resources": "azure-resources", "deployments": "azure-deployments"}[action]
			if mapped == "" {
				problem(w, 404, "unknown Azure action")
				return
			}
			v, e := s.Auth.AzureAction(r.Context(), id, mapped, r.URL.Query().Get("subscription"), r.URL.Query().Get("resource"))
			if e != nil {
				problem(w, 502, e.Error())
				return
			}
			writeJSON(w, 200, v)
		default:
			problem(w, 404, "unknown Azure action")
		}
		return
	}
	if accountUsesAPIKey(account.Kind) {
		switch {
		case action == "key" && r.Method == "PUT":
			var v struct {
				Key string `json:"key"`
			}
			if decodeRequest(w, r, &v) != nil {
				problem(w, 400, "invalid API key")
				return
			}
			if e := s.Auth.SaveAccountKey(r.Context(), id, v.Key); e != nil {
				problem(w, 400, e.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
		case (action == "status" || action == "info") && r.Method == "GET":
			writeJSON(w, 200, map[string]any{"kind": account.Kind, "has_key": s.Auth.HasAccountKey(id), "message": "Sources inherit the account API key unless a resource-specific key is configured"})
		default:
			problem(w, 404, "API accounts use a key instead of device login")
		}
		return
	}

	switch {
	case action == "status" && r.Method == "GET":
		writeJSON(w, 200, map[string]bool{"logged_in": s.Auth.LoggedIn(id)})
	case action == "info" && r.Method == "GET":
		v, e := s.Auth.AccountInfo(r.Context(), id)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, v)
	case action == "logout" && r.Method == "POST":
		if e := s.Auth.Logout(r.Context(), id); e != nil {
			problem(w, 500, e.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"logged_in": false})
	case action == "login" && r.Method == "POST":
		s.startAccountLogin(w, r, account)
	default:
		problem(w, 404, "account action not found")
	}
}

type Model struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Claude bool   `json:"claude"`
	Codex  bool   `json:"codex"`
}

func (a *Auth) Models(ctx context.Context, id string) ([]Model, error) {
	if id == "" {
		id = "copilot"
	}
	c := a.store.Snapshot()
	if strings.HasPrefix(id, "pool:") {
		p, ok := c.Pool(id)
		if !ok {
			return nil, errors.New("pool not found")
		}
		models := []Model{}
		for _, name := range p.ModelNames() {
			models = append(models, Model{name, name, "Source pool", p.Agent == "claude-code", p.Agent == "codex"})
		}
		return models, nil
	}
	s, ok := c.Source(id)
	if !ok {
		return nil, errors.New("source not found")
	}
	models := []Model{}
	if s.Kind != "copilot" {
		for _, m := range s.Deployments {
			models = append(models, Model{m, m, s.Kind, s.SupportsAgentModel("claude-code", m), s.SupportsAgentModel("codex", m)})
		}
		return models, nil
	}
	l, e := a.Lease(ctx, c, s, false)
	if e != nil {
		return nil, e
	}
	r, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(l.Endpoint, "/")+"/models", nil)
	a.copilotHeaders(r, l, false)
	res, e := a.http.client(c.SourceProxy(id)).Do(r)
	if e != nil {
		return nil, errors.New("model catalog connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("model catalog rejected")
	}
	var data struct {
		Data []struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Vendor    string   `json:"vendor"`
			Endpoints []string `json:"supported_endpoints"`
			Policy    struct {
				State string `json:"state"`
			} `json:"policy"`
		} `json:"data"`
	}
	if e = readJSONBody(res.Body, &data); e != nil {
		return nil, e
	}
	for _, m := range data.Data {
		if m.Policy.State == "disabled" {
			continue
		}
		responses, messages := false, false
		for _, p := range m.Endpoints {
			responses = responses || p == "/responses"
			messages = messages || p == "/v1/messages"
		}
		if len(m.Endpoints) == 0 {
			responses = strings.HasPrefix(m.ID, "gpt-")
			messages = strings.HasPrefix(m.ID, "claude-")
		}
		_, known := copilotResponseEfforts[m.ID]
		codex := responses && known
		// Native Messages models are advertised by the upstream catalog.
		claude := codex || messages
		// Catalog visibility is separate from Agent compatibility. Keep every
		// enabled upstream model visible without promising unsupported protocols.
		name := m.Name
		if name == "" {
			name = m.ID
		}
		models = append(models, Model{m.ID, name, m.Vendor, claude, codex})
	}
	return models, nil
}

func (s *Server) startAccountLogin(w http.ResponseWriter, r *http.Request, account Account) {
	s.loginJobs.mu.Lock()
	active := 0
	for key, j := range s.loginJobs.jobs {
		if j.done {
			delete(s.loginJobs.jobs, key)
		} else {
			active++
		}
	}
	if active >= 8 {
		s.loginJobs.mu.Unlock()
		problem(w, 429, "too many active logins")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	jobID := newID()
	job := &loginJob{cancel: cancel}
	s.loginJobs.jobs[jobID] = job
	s.loginJobs.mu.Unlock()
	finish := func(e error) {
		cancel()
		s.loginJobs.mu.Lock()
		job.done = true
		if e != nil {
			job.err = e.Error()
		}
		s.loginJobs.mu.Unlock()
	}
	if account.Kind == "github" {
		d, e := s.Auth.BeginLogin(r.Context(), account.ID)
		if e != nil {
			finish(e)
			problem(w, 502, e.Error())
			return
		}
		go func() { finish(s.Auth.CompleteLogin(ctx, d)) }()
		writeJSON(w, 200, map[string]any{"job_id": jobID, "user_code": d.UserCode, "verification_uri": d.VerificationURI, "expires_in": d.ExpiresIn})
		return
	}
	challenges := make(chan map[string]any, 1)
	done := make(chan error, 1)
	go func() {
		e := s.Auth.AzureLogin(ctx, account.ID, false, func(v any) {
			if m, ok := v.(map[string]any); ok {
				select {
				case challenges <- m:
				default:
				}
			}
		})
		finish(e)
		done <- e
	}()
	select {
	case challenge := <-challenges:
		challenge["job_id"] = jobID
		writeJSON(w, 200, challenge)
	case e := <-done:
		if e != nil {
			problem(w, 502, e.Error())
		} else {
			writeJSON(w, 200, map[string]any{"job_id": jobID, "done": true})
		}
	case <-r.Context().Done():
		cancel()
	case <-time.After(40 * time.Second):
		cancel()
		problem(w, 504, "Azure CLI did not produce a device code")
	}
}

func (s *Server) requestOrigin(r *http.Request) string {
	if s.PublicURL != "" {
		return s.PublicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
