package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetLevel(t *testing.T) {
	require.Equal(t, 32, TargetLevel(12))
	require.Equal(t, 32, TargetLevel(16))
	require.Equal(t, 30, TargetLevel(17))
	require.Equal(t, 30, TargetLevel(20))
	require.Equal(t, 28, TargetLevel(21))
	require.Equal(t, 28, TargetLevel(30))
}
