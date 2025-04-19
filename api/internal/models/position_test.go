package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TODO add tests for position

// Dummy test
func TestNewPosition(t *testing.T) {
	_, err := NewPosition(0, 0)
	assert.NoError(t, err)
}
