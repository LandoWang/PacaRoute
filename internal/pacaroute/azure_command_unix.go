//go:build !windows

package pacaroute

import (
	"context"
	"os/exec"
)

func azureCommand(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, azureCLI(), args...)
}
