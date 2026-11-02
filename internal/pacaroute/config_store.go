package pacaroute

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// A revisioned document keeps cross-entity validation and publication atomic.
// Request routing uses immutable memory snapshots; external writers are observed
// at most once per second, or immediately through Reload.
type ConfigStore struct {
	Root      string
	mu        sync.Mutex
	db        *sql.DB
	current   *Config
	nextRead  time.Time
	lastError string
}

var configFiles = []string{"accounts.json", "sources.json", "source-pools.json", "proxies.json", "clients.json", "source-state.json"}

func NewConfigStore(root string) (*ConfigStore, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	unlock, e := fileLock(context.Background(), filepath.Join(root, "config.lock"), 5*time.Second)
	if e != nil {
		return nil, e
	}
	defer unlock()
	path := filepath.Join(root, "pacaroute.sqlite")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	f.Close()
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &ConfigStore{Root: root, db: db}
	fail := func(e error) (*ConfigStore, error) { db.Close(); return nil, e }
	if _, e = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;
 CREATE TABLE IF NOT EXISTS configuration (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL, revision INTEGER NOT NULL, document TEXT NOT NULL);`); e != nil {
		return fail(e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM configuration`).Scan(&count); e != nil {
		return fail(e)
	}
	if count == 0 {
		c, e := s.importLegacy()
		if e != nil {
			return fail(e)
		}
		b, e := json.Marshal(c)
		if e != nil {
			return fail(e)
		}
		if _, e = db.Exec(`INSERT INTO configuration(id,version,revision,document) VALUES(1,1,1,?)`, string(b)); e != nil {
			return fail(e)
		}
	}
	if _, e = s.Reload(); e != nil {
		return fail(e)
	}
	return s, nil
}
func (s *ConfigStore) Close() error { return s.db.Close() }
func (s *ConfigStore) importLegacy() (Config, error) {
	c := Config{Proxies: Proxies{AccountProxyID: "system", Bindings: map[string]string{}}}
	legacy := false
	parts := []any{&c.Accounts, &c.Sources, &c.Pools, &c.Proxies, &c.Clients, &c.SourceState}
	// The old journal denotes a committed transaction; use it in preference to
	// partially replaced split files. Preserve every legacy file for rollback.
	if b, e := os.ReadFile(filepath.Join(s.Root, "config.transaction.json")); e == nil {
		if len(b) > 24<<20 {
			return c, errors.New("legacy transaction too large")
		}
		if e = json.Unmarshal(b, &c); e != nil {
			return c, errors.New("invalid legacy transaction")
		}
		legacy = true
	} else if !os.IsNotExist(e) {
		return c, e
	} else {
		for i, name := range configFiles {
			b, e := os.ReadFile(filepath.Join(s.Root, name))
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return c, e
			}
			legacy = true
			if len(b) > 4<<20 {
				return c, fmt.Errorf("legacy %s too large", name)
			}
			if e = json.Unmarshal(b, parts[i]); e != nil {
				return c, fmt.Errorf("invalid legacy %s", name)
			}
		}
	}
	if _, exists := c.Client("default"); !exists {
		c.Clients = append([]Client{{ID: "default", Name: "本机", Local: true, LegacyDirect: legacy, EnabledAgents: []string{"codex", "claude-code"}, Routes: []Route{}}}, c.Clients...)
	}
	c.Revision = ""
	return c, c.Validate()
}
func (s *ConfigStore) Snapshot() *Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().After(s.nextRead) {
		if e := s.readLocked(); e != nil {
			s.lastError = e.Error()
		}
		s.nextRead = time.Now().Add(time.Second)
	}
	return s.current
}
func (s *ConfigStore) Reload() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.readLocked()
	if e != nil {
		s.lastError = e.Error()
	}
	s.nextRead = time.Now().Add(time.Second)
	return s.current, e
}
func (s *ConfigStore) readLocked() error {
	var revision int64
	var version int
	var document string
	if e := s.db.QueryRow(`SELECT version,revision,document FROM configuration WHERE id=1`).Scan(&version, &revision, &document); e != nil {
		return e
	}
	if version != 1 {
		return errors.New("unsupported configuration database version")
	}
	if s.current != nil && s.current.Revision == strconv.FormatInt(revision, 10) {
		s.lastError = ""
		return nil
	}
	if len(document) > 24<<20 {
		return errors.New("configuration too large")
	}
	var c Config
	if e := json.Unmarshal([]byte(document), &c); e != nil {
		return errors.New("invalid configuration database document")
	}
	if e := c.Validate(); e != nil {
		return e
	}
	c.Revision = strconv.FormatInt(revision, 10)
	s.current = &c
	s.lastError = ""
	return nil
}
func (s *ConfigStore) Save(c Config, expected string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c = cloneConfig(&c)
	if e := c.Validate(); e != nil {
		return e
	}
	if e := s.readLocked(); e != nil {
		return e
	}
	if expected != "" && expected != s.current.Revision {
		return errors.New("configuration changed; reload before saving")
	}
	// Legacy compatibility may only be imported, never re-enabled by an edit.
	for _, client := range c.Clients {
		if client.LegacyDirect {
			old, ok := s.current.Client(client.ID)
			if !ok || !old.LegacyDirect {
				return errors.New("legacy routing cannot be enabled")
			}
		}
	}
	c.Revision = ""
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	if len(b) > 24<<20 {
		return errors.New("configuration too large")
	}
	result, e := s.db.Exec(`UPDATE configuration SET document=?,revision=revision+1 WHERE id=1 AND revision=?`, string(b), s.current.Revision)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("configuration changed; reload before saving")
	}
	return s.readLocked()
}
func (s *ConfigStore) Error() string { s.mu.Lock(); defer s.mu.Unlock(); return s.lastError }
