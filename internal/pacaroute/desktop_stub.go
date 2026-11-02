//go:build !desktop || (!darwin && !windows)

package pacaroute

import (
	"context"
	"errors"
)

const desktopEnabled = false

func runDesktop(ctx context.Context, a arguments, s *ConfigStore, auth *Auth, port int, emit func(any), fail func(error) int) int {
	return fail(errors.New("use the PacaRoute desktop build for the native tray; this build supports serve and ui"))
}
