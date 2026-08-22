package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	t.Run("ldflags value wins", func(t *testing.T) {
		old := Commit
		defer func() { Commit = old }()

		Commit = "abc123"
		assert.Equal(t, "abc123", Get())
	})

	t.Run("without ldflags", func(t *testing.T) {
		old := Commit
		defer func() { Commit = old }()

		Commit = ""
		// Test binaries carry no VCS revision, so this exercises the fallback chain.
		assert.NotEmpty(t, Get())
	})
}
