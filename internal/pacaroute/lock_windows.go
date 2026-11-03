//go:build windows

package pacaroute

import (
	"context"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
)

func fileLock(ctx context.Context, path string, timeout time.Duration) (func(), error) {
	return atomicfile.Lock(ctx, path, timeout)
}
