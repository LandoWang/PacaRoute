//go:build !windows

package pacaroute

func platformSeal(b []byte) ([]byte, bool, error) { return nil, false, nil }
func platformOpen(b []byte) ([]byte, bool, error) { return nil, false, nil }
