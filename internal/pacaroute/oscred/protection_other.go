//go:build !windows

package oscred

func Seal(b []byte) ([]byte, bool, error) { return nil, false, nil }
func Open(b []byte) ([]byte, bool, error) { return nil, false, nil }
