package pacaroute

import "github.com/LandoWang/pacaroute/internal/pacaroute/oscred"

func platformSeal(b []byte) ([]byte, bool, error) { return oscred.Seal(b) }
func platformOpen(b []byte) ([]byte, bool, error) { return oscred.Open(b) }
func systemSourceKey(id string) (string, error)   { return oscred.SourceKey(id) }
func saveSystemSourceKey(id, key string) error    { return oscred.SaveSourceKey(id, key) }
