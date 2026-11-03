//go:build darwin

package pacaroute

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
)

func startupPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.landowang.pacaroute.tray.plist")
}
func startupEnabled() bool { _, e := os.Stat(startupPath()); return e == nil }
func setStartup(enabled bool, root string, port int, credentialRoot string) error {
	if enabled {
		if e := saveDesktopSettings(root, port); e != nil {
			return e
		}
	}
	if !enabled {
		e := os.Remove(startupPath())
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>com.landowang.pacaroute.tray</string><key>ProgramArguments</key><array><string>%s</string><string>desktop</string><string>--home</string><string>%s</string><string>--credential-root</string><string>%s</string><string>--no-open</string></array><key>RunAtLoad</key><true/><key>ProcessType</key><string>Interactive</string></dict></plist>`, escape.Replace(exe), escape.Replace(root), escape.Replace(credentialRoot))
	return atomicfile.Write(startupPath(), []byte(plist), 0600)
}
