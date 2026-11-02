//go:build windows

package pacaroute

import (
	"golang.org/x/sys/windows/registry"
	"os"
	"syscall"
)

const startupRegistry = `Software\Microsoft\Windows\CurrentVersion\Run`

func startupEnabled() bool {
	k, e := registry.OpenKey(registry.CURRENT_USER, startupRegistry, registry.QUERY_VALUE)
	if e != nil {
		return false
	}
	defer k.Close()
	_, _, e = k.GetStringValue("PacaRoute")
	return e == nil
}
func setStartup(enabled bool, root string, port int, credentialRoot string) error {
	if enabled {
		if e := saveDesktopSettings(root, port); e != nil {
			return e
		}
	}
	k, _, e := registry.CreateKey(registry.CURRENT_USER, startupRegistry, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer k.Close()
	if !enabled {
		e = k.DeleteValue("PacaRoute")
		if e == registry.ErrNotExist {
			return nil
		}
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	return k.SetStringValue("PacaRoute", syscall.EscapeArg(exe)+" desktop --home "+syscall.EscapeArg(root)+" --credential-root "+syscall.EscapeArg(credentialRoot)+" --no-open")
}
