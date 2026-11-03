//go:build !darwin || !desktop

package oscred

import "os"

func SourceKey(string) (string, error)   { return "", os.ErrNotExist }
func SaveSourceKey(string, string) error { return os.ErrNotExist }
