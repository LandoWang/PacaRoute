//go:build !darwin || !desktop

package pacaroute

import "os"

func systemSourceKey(string) (string, error)   { return "", os.ErrNotExist }
func saveSystemSourceKey(string, string) error { return os.ErrNotExist }
