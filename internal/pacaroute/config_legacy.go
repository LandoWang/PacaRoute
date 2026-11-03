package pacaroute

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

// maxLegacyConfigFile bounds each split legacy configuration file.
const maxLegacyConfigFile = 4 << 20

var configFiles = []string{"accounts.json", "sources.json", "source-pools.json", "proxies.json", "clients.json", "source-state.json"}

// Fresh installations have no pre-created accounts or sources. Existing routes
// that relied on implicit identities are made explicit once, preserving IDs so
// Auth keeps ownership of the original credentials and locks.
func (s *ConfigStore) materializeLegacyIdentities() error {
	var c domain.Config
	var revision int64
	var version int
	var document string
	if err := s.db.QueryRow(`SELECT version,revision,document FROM configuration WHERE id=1`).Scan(&version, &revision, &document); err != nil {
		return err
	}
	if version != 1 || len(document) > maxConfigDocument {
		return errors.New("unsupported or oversized configuration document")
	}
	if err := json.Unmarshal([]byte(document), &c); err != nil {
		return err
	}
	if !materializeLegacyIdentities(&c) {
		return nil
	}
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE configuration SET document=?,revision=revision+1 WHERE id=1 AND revision=?`, string(b), revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("configuration changed during identity migration; retry")
	}
	return nil
}
func materializeLegacyIdentities(c *domain.Config) bool {
	changed, needsCopilot := false, false
	for _, client := range c.Clients {
		needsCopilot = needsCopilot || client.LegacyDirect
		for _, route := range client.Routes {
			needsCopilot = needsCopilot || route.SourceID == "copilot"
		}
	}
	for _, pool := range c.Pools {
		for _, member := range pool.Members {
			needsCopilot = needsCopilot || member.SourceID == "copilot"
		}
	}
	_, bound := c.Proxies.Bindings["copilot"]
	needsCopilot = needsCopilot || bound || c.Paused("copilot")
	for _, source := range c.Sources {
		if source.ID == "copilot" {
			needsCopilot = false
			break
		}
	}
	if needsCopilot {
		c.Sources = append(c.Sources, domain.Source{ID: "copilot", Name: "GitHub Copilot", Kind: domain.SourceCopilot, AccountID: "copilot", Deployments: []string{}})
		changed = true
	}
	for i := range c.Sources {
		source := &c.Sources[i]
		if source.AccountID == "" {
			source.AccountID = "azure-local"
			if source.Kind == domain.SourceCopilot {
				source.AccountID = "copilot"
			}
			changed = true
		}
		if source.AccountID != "copilot" && source.AccountID != "azure-local" {
			continue
		}
		found := false
		for _, account := range c.Accounts {
			found = found || account.ID == source.AccountID
		}
		if !found {
			account := domain.Account{ID: "copilot", Name: "GitHub", Kind: domain.AccountGitHub, LoginMode: "device"}
			if source.AccountID == "azure-local" {
				account = domain.Account{ID: "azure-local", Name: "Azure CLI", Kind: domain.AccountAzure, LoginMode: "azure-cli"}
			}
			c.Accounts = append(c.Accounts, account)
			changed = true
		}
	}
	return changed
}
func (s *ConfigStore) importLegacy() (domain.Config, error) {
	c := domain.Config{Proxies: domain.Proxies{AccountProxyID: "system", Bindings: map[string]string{}}}
	legacy := false
	parts := []any{&c.Accounts, &c.Sources, &c.Pools, &c.Proxies, &c.Clients, &c.SourceState}
	// The old journal denotes a committed transaction; use it in preference to
	// partially replaced split files. Preserve every legacy file for rollback.
	if b, e := os.ReadFile(filepath.Join(s.Root, "config.transaction.json")); e == nil {
		if len(b) > maxConfigDocument {
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
			if len(b) > maxLegacyConfigFile {
				return c, fmt.Errorf("legacy %s too large", name)
			}
			if e = json.Unmarshal(b, parts[i]); e != nil {
				return c, fmt.Errorf("invalid legacy %s", name)
			}
		}
	}
	if _, exists := c.Client("default"); !exists {
		c.Clients = append([]domain.Client{{ID: "default", Name: "本机", Local: true, LegacyDirect: legacy, EnabledAgents: []domain.GrantKind{domain.GrantCodex, domain.GrantClaudeCode}, Routes: []domain.Route{}}}, c.Clients...)
	}
	c.Revision = ""
	return c, c.Validate()
}
