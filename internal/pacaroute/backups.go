package pacaroute

import "github.com/LandoWang/pacaroute/internal/pacaroute/clientconfig"

type configBackup struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Missing   bool   `json:"missing_file"`
	Connected bool   `json:"bridge_configured"`
	Valid     bool   `json:"valid"`
}

func listBackups(client string) (map[string]any, error) {
	result, e := clientconfig.ListBackups(client)
	if e != nil {
		return nil, e
	}
	rows := make([]configBackup, len(result.Backups))
	for i, b := range result.Backups {
		rows[i] = configBackup{b.ID, b.CreatedAt, b.Missing, b.Connected, b.Valid}
	}
	return map[string]any{"path": result.Path, "backups": rows}, nil
}

func restoreBackup(client, id, expected string, apply bool) (map[string]any, error) {
	result, e := clientconfig.RestoreBackup(client, id, expected, apply)
	if result == nil {
		return nil, e
	}
	out := map[string]any{"path": result.Path, "fingerprint": result.Fingerprint, "changes": result.Changes}
	if result.BackupPath != nil {
		out["backup_path"] = *result.BackupPath
	}
	return out, e
}
