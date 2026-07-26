package worker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID returns a random identifier a worker can use to identify itself to
// the API server across its lifetime (job claims, heartbeats). Workers mint
// their own ID rather than registering with the server for one.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate worker id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
