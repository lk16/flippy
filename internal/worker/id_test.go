package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewID_ReturnsDistinctIDs(t *testing.T) {
	a, err := NewID()
	require.NoError(t, err)
	require.NotEmpty(t, a)

	b, err := NewID()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}
