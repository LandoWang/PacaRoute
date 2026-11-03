package pacaroute

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
)

type desktopSettings struct {
	CredentialRoot string `json:"credential_root,omitempty"`
	Port           int    `json:"port"`
	AdminPort      int    `json:"admin_port,omitempty"`
	Serving        *bool  `json:"serving,omitempty"`
}

func (d *desktopControl) persist(port int, serving bool) error {
	authority, err := filepath.Abs(d.auth.vault.root)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(desktopSettings{Port: port, AdminPort: d.adminPort, Serving: &serving, CredentialRoot: authority})
	return atomicfile.Write(filepath.Join(d.store.Root, "desktop-settings.json"), b, 0600)
}
func readDesktopSettings(root string) desktopSettings {
	var v desktopSettings
	b, _ := os.ReadFile(filepath.Join(root, "desktop-settings.json"))
	_ = json.Unmarshal(b, &v)
	return v
}
func desktopPort(a arguments, root string, fallback int) int {
	if _, ok := a.flags["port"]; ok {
		return fallback
	}
	v := readDesktopSettings(root)
	if v.Port >= 1024 && v.Port <= 65535 {
		return v.Port
	}
	return fallback
}
func saveDesktopSettings(root string, port int) error {
	v := readDesktopSettings(root)
	v.Port = port
	b, _ := json.Marshal(v)
	return atomicfile.Write(filepath.Join(root, "desktop-settings.json"), b, 0600)
}
