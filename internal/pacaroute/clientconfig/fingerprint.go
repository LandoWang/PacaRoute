package clientconfig

import (
	"crypto/sha256"
	"encoding/hex"
)

func keyHash(key string) string { h := sha256.Sum256([]byte(key)); return hex.EncodeToString(h[:]) }
