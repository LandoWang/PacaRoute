package pacaroute

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

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
	if account.Kind == domain.AccountAzure {
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
	if domain.AccountUsesAPIKey(account.Kind) {
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
func (s *Server) startAccountLogin(w http.ResponseWriter, r *http.Request, account domain.Account) {
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
	jobID := domain.NewID()
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
	if account.Kind == domain.AccountGitHub {
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

func (s *Server) loginStatus(w http.ResponseWriter, r *http.Request, path string) {
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
}
