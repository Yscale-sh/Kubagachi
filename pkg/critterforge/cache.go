package critterforge

import (
	"crypto/sha256"
	"encoding/hex"
)

// cacheKey produces a deterministic hash from an ordered list of inputs.
// Components are separated by a null byte so concatenation collisions can't
// happen (e.g. ["ab","c"] vs ["a","bc"] hash differently).
func cacheKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0x00})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// bytesSHA returns the hex sha256 of the given bytes.
func bytesSHA(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
