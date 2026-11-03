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

type configBackup struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Missing   bool   `json:"missing_file"`
	Connected bool   `json:"bridge_configured"`
	Valid     bool   `json:"valid"`
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
	if !f.Mode().IsRegular() || f.Size() > 4<<20 {
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
func ListBackups(agent string) (map[string]any, error) {
	target, e := AgentConfigPath(agent)
	if e != nil {
		return nil, e
	}
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".pacaroute-backups", filepath.Base(target), "*"))
	legacy, _ := filepath.Glob(target + ".bridge-*")
	paths = append(paths, legacy...)
	rows := []configBackup{}
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
		rows = append(rows, configBackup{id, f.ModTime().UTC().Format(time.RFC3339), missing, connected, e == nil})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	return map[string]any{"path": target, "backups": rows}, nil
}

// RestoreBackup previews or applies an exact configuration backup.
func RestoreBackup(agent, id, expected string, apply bool) (map[string]any, error) {
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
	result := map[string]any{"path": target, "fingerprint": fingerprint, "changes": []string{"Restore the entire selected configuration backup", "Current configuration will first be backed up", "Open a new Agent session after restoring"}}
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
	result["backup_path"] = backup
	return result, e
}
func boolText(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
