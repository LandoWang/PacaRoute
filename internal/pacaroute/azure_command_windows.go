//go:build windows

package pacaroute

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Official Azure CLI Windows distributions include Python beside wbin/az.cmd.
// Calling that interpreter directly avoids shell interpolation of resource names.
func azureCommand(ctx context.Context, args ...string) *exec.Cmd {
	candidates := []string{}
	if path, e := exec.LookPath("az"); e == nil {
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			return exec.CommandContext(ctx, path, args...)
		}
		candidates = append(candidates, filepath.Join(filepath.Dir(path), "..", "python.exe"))
	}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if base != "" {
			candidates = append(candidates, filepath.Join(base, "Microsoft SDKs", "Azure", "CLI2", "python.exe"))
		}
	}
	for _, python := range candidates {
		if f, e := os.Stat(python); e == nil && f.Mode().IsRegular() {
			return exec.CommandContext(ctx, python, append([]string{"-IBm", "azure.cli"}, args...)...)
		}
	}
	return exec.CommandContext(ctx, "pacaroute-azure-cli-not-installed.exe", args...)
}
