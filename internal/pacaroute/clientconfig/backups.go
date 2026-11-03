package clientconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
	"github.com/pelletier/go-toml/v2"
)

// Backup describes an available Agent configuration backup.
type Backup struct {
	ID        string
	CreatedAt string
	Missing   bool
	Connected bool
	Valid     bool
}

// BackupList contains the configuration path and its available backups.
type BackupList struct {
	Path    string
	Backups []Backup
}

// RestoreResult describes a preview or completed backup restoration.
type RestoreResult struct {
	Path        string
	Fingerprint string
	Changes     []string
	// BackupPath is set once the current configuration has been backed up.
	BackupPath *string
}

func backupPath(target, id string) (string, bool, error) {
	if filepath.Base(id) != id {
		return "", false, errors.New("invalid backup ID")
	}
	modern := regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}Z\.(bak|missing)$`)
	legacy := regexp.MustCompile(`^` + regexp.QuoteMeta(filepath.Base(target)) + `\.bridge-\d{8}-\d{6}-[a-f0-9]{32}\.(bak|absent)$`)
	if modern.MatchString(id) {
		return filepath.Join(filepath.Dir(target), ".pacaroute-backups", filepath.Base(target), id), strings.HasSuffix(id, ".missing"), nil
	}
	if legacy.MatchString(id) {
		return filepath.Join(filepath.Dir(target), id), strings.HasSuffix(id, ".absent"), nil
	}
	return "", false, errors.New("backup does not belong to this Agent")
}
func readBackup(agent, target, id string) ([]byte, bool, error) {
	path, missing, e := backupPath(target, id)
	if e != nil {
		return nil, false, e
	}
	f, e := os.Lstat(path)
	if e != nil {
		return nil, false, e
	}
	if !f.Mode().IsRegular() || f.Size() > maxAgentConfigFile {
		return nil, false, errors.New("invalid backup file")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, false, e
	}
	if missing {
		if len(b) > 0 && string(b) != "copilot-bridge:missing-config:v1\n" {
			return nil, false, errors.New("invalid missing-file marker")
		}
		return nil, true, nil
	}
	if !validClientBytes(agent, b) {
		return nil, false, errors.New("invalid backup contents")
	}
	return b, false, nil
}
func validClientBytes(agent string, b []byte) bool {
	var m map[string]any
	if agent == "codex" {
		return toml.Unmarshal(b, &m) == nil && m != nil
	}
	return json.Unmarshal(b, &m) == nil && m != nil
}

// ListBackups lists current and legacy backups for the selected Agent.
func ListBackups(agent string) (BackupList, error) {
	target, e := AgentConfigPath(agent)
	if e != nil {
		return BackupList{}, e
	}
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".pacaroute-backups", filepath.Base(target), "*"))
	legacy, _ := filepath.Glob(target + ".bridge-*")
	paths = append(paths, legacy...)
	rows := []Backup{}
	for _, p := range paths {
		id := filepath.Base(p)
		if _, _, e = backupPath(target, id); e != nil {
			continue
		}
		f, e := os.Lstat(p)
		if e != nil || !f.Mode().IsRegular() {
			continue
		}
		b, missing, e := readBackup(agent, target, id)
		connected := strings.Contains(string(b), "copilot-bridge") || strings.Contains(string(b), "pacaroute")
		rows = append(rows, Backup{id, f.ModTime().UTC().Format(time.RFC3339), missing, connected, e == nil})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	return BackupList{Path: target, Backups: rows}, nil
}

// RestoreBackup previews or applies an exact configuration backup.
func RestoreBackup(agent, id, expected string, apply bool) (*RestoreResult, error) {
	target, e := AgentConfigPath(agent)
	if e != nil {
		return nil, e
	}
	unlock, e := atomicfile.Lock(context.Background(), target+".pacaroute.lock", 5*time.Second)
	if e != nil {
		return nil, e
	}
	defer unlock()
	if f, e := os.Lstat(target); e == nil && !f.Mode().IsRegular() {
		return nil, errors.New("config target is not a regular file")
	}
	old, e := os.ReadFile(target)
	oldMissing := os.IsNotExist(e)
	if e != nil && !oldMissing {
		return nil, e
	}
	next, missing, e := readBackup(agent, target, id)
	if e != nil {
		return nil, e
	}
	fingerprint := fingerprint(target + "\x00" + id + "\x00" + string(old) + "\x00" + string(next) + boolText(oldMissing) + boolText(missing))
	result := &RestoreResult{Path: target, Fingerprint: fingerprint, Changes: []string{"Restore the entire selected configuration backup", "Current configuration will first be backed up", "Open a new Agent session after restoring"}}
	if !apply {
		return result, nil
	}
	if fingerprint != expected {
		return nil, errors.New("config or backup changed; preview again")
	}
	backup := filepath.Join(filepath.Dir(target), ".pacaroute-backups", filepath.Base(target), time.Now().UTC().Format("20060102T150405.000000000Z")+".bak")
	if oldMissing {
		backup = strings.TrimSuffix(backup, ".bak") + ".missing"
	}
	if e = atomicfile.Write(backup, old, 0600); e != nil {
		return nil, e
	}
	if missing {
		e = os.Remove(target)
		if os.IsNotExist(e) {
			e = nil
		}
	} else {
		e = atomicfile.Write(target, next, 0600)
	}
	result.BackupPath = &backup
	return result, e
}
func boolText(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
