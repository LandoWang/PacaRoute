package pacaroute

import (
	"crypto/sha256"
	"encoding/hex"
)

func keyEqual(hash, key string) bool {
	expected, e := hex.DecodeString(hash)
	if e != nil {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	return len(expected) == 32 && constantEqual(expected, sum[:])
}
