package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetLevel(t *testing.T) {
	require.Equal(t, 24, TargetLevel(12))
	require.Equal(t, 24, TargetLevel(20))
	require.Equal(t, 24, TargetLevel(30))
}
