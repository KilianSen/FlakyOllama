package hash

import (
	"crypto/sha256"
	"encoding/hex"
)

func ComputeHash(input string) string {
	h := sha256.New()
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}
