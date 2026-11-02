//go:build !darwin && !windows

package pacaroute

import "errors"

func startupEnabled() bool { return false }
func setStartup(bool, string, int, string) error {
	return errors.New("desktop startup is supported on macOS and Windows")
}
