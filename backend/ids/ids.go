// Package ids generates random identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a random 128-bit hex identifier.
func New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
