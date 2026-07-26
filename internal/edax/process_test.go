package edax

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// testProcess returns a Process backed by the real edax binary at
// EDAX_PATH, skipping the test if it's not set.
func testProcess(t *testing.T) *Process {
	t.Helper()

	path := os.Getenv(pathEnvVar)
	if path == "" {
		t.Skip("EDAX_PATH not set; skipping test requiring the real edax binary")
	}

	p := NewProcess(path)
	t.Cleanup(func() { _ = p.Close() })

	return p
}

func TestPathFromEnv_Unset(t *testing.T) {
	t.Setenv(pathEnvVar, "")
	_, err := PathFromEnv()
	require.Error(t, err)
}

func TestPathFromEnv_Set(t *testing.T) {
	t.Setenv(pathEnvVar, "/some/path/lEdax-x64")
	path, err := PathFromEnv()
	require.NoError(t, err)
	require.Equal(t, "/some/path/lEdax-x64", path)
}

func TestProcess_Evaluate(t *testing.T) {
	p := testProcess(t)

	eval, err := p.Evaluate(othello.NewBoardStart(), 10)
	require.NoError(t, err)
	require.Equal(t, 10, eval.Depth)
	require.Equal(t, 100, eval.Confidence)
	require.NotEmpty(t, eval.BestMoves)
}

func TestProcess_Evaluate_BinaryNotFound(t *testing.T) {
	p := NewProcess("/does/not/exist/lEdax-x64")

	_, err := p.Evaluate(othello.NewBoardStart(), 10)
	require.Error(t, err)
}

func TestProcess_Evaluate_InvalidLevel(t *testing.T) {
	p := testProcess(t)

	_, err := p.Evaluate(othello.NewBoardStart(), 0)
	require.Error(t, err)
}

func TestProcess_Evaluate_NoLegalMoves(t *testing.T) {
	p := testProcess(t)

	board, err := othello.NewBoard(^uint64(0), 0, othello.Black)
	require.NoError(t, err)

	_, err = p.Evaluate(board, 10)
	require.Error(t, err)
}

func TestProcess_Evaluate_ReusesProcessAtSameLevel(t *testing.T) {
	p := testProcess(t)

	_, err := p.Evaluate(othello.NewBoardStart(), 8)
	require.NoError(t, err)
	pid1 := p.cmd.Process.Pid

	board, err := othello.NewBoardStart().DoMove(19)
	require.NoError(t, err)

	_, err = p.Evaluate(board, 8)
	require.NoError(t, err)
	pid2 := p.cmd.Process.Pid

	require.Equal(t, pid1, pid2)
}

func TestProcess_Evaluate_RestartsOnLevelChange(t *testing.T) {
	p := testProcess(t)

	_, err := p.Evaluate(othello.NewBoardStart(), 6)
	require.NoError(t, err)
	pid1 := p.cmd.Process.Pid

	_, err = p.Evaluate(othello.NewBoardStart(), 8)
	require.NoError(t, err)
	pid2 := p.cmd.Process.Pid

	require.NotEqual(t, pid1, pid2)
}

func TestProcess_Close_KillsProcess(t *testing.T) {
	p := testProcess(t)

	_, err := p.Evaluate(othello.NewBoardStart(), 6)
	require.NoError(t, err)
	proc := p.cmd.Process

	require.NoError(t, p.Close())

	// The process should be gone: signal 0 (existence check only) should
	// report an error.
	err = proc.Signal(syscall.Signal(0))
	require.Error(t, err)
}

func TestProcess_Close_WithoutStart(t *testing.T) {
	p := testProcess(t)
	require.NoError(t, p.Close())
}

func TestProcess_Evaluate_AfterClose(t *testing.T) {
	p := testProcess(t)

	_, err := p.Evaluate(othello.NewBoardStart(), 6)
	require.NoError(t, err)
	pid1 := p.cmd.Process.Pid

	require.NoError(t, p.Close())

	_, err = p.Evaluate(othello.NewBoardStart(), 6)
	require.NoError(t, err)
	pid2 := p.cmd.Process.Pid

	require.NotEqual(t, pid1, pid2)
}
