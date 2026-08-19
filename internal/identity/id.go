package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns a compact, sortable-enough identifier with a domain prefix.
func New(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Errorf("generate identifier: %w", err))
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}
