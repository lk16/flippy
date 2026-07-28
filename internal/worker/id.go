package worker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID returns a random worker identifier; workers mint their own rather than registering with the server.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate worker id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
