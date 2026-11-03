package pacaroute

import "github.com/LandoWang/pacaroute/internal/pacaroute/clientconfig"

func listBackups(client string) (map[string]any, error) {
	return clientconfig.ListBackups(client)
}

func restoreBackup(client, id, expected string, apply bool) (map[string]any, error) {
	return clientconfig.RestoreBackup(client, id, expected, apply)
}
