package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBestMovesScan_OK(t *testing.T) {
	var moves BestMoves
	err := moves.Scan([]byte("{1,2,3}"))
	require.NoError(t, err)
	require.Equal(t, BestMoves{1, 2, 3}, moves)
}

func TestBestMovesScan_InvalidType(t *testing.T) {
	var moves BestMoves
	err := moves.Scan(123) // passing an int instead of []byte
	require.Error(t, err)
	require.Equal(t, "cannot scan int into BestMoves", err.Error())
}

func TestBestMovesScan_NilBytes(t *testing.T) {
	var moves BestMoves
	err := moves.Scan([]byte(nil))
	require.Error(t, err)
	require.Equal(t, "cannot scan nil into BestMoves", err.Error())
}

func TestBestMovesScan_BrokenInt(t *testing.T) {
	var moves BestMoves
	err := moves.Scan([]byte("{1,abc,3}"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot convert abc to int")
}
