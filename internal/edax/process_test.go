package edax

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

func TestBuildArgs_OmitsNTasksWhenUnset(t *testing.T) {
	require.NotContains(t, buildArgs(16, 0), "-n-tasks")
	require.NotContains(t, buildArgs(16, -1), "-n-tasks")
}

func TestBuildArgs_IncludesNTasksWhenPositive(t *testing.T) {
	args := buildArgs(16, 4)
	idx := slices.Index(args, "-n-tasks")
	require.GreaterOrEqual(t, idx, 0)
	require.Equal(t, "4", args[idx+1])
}

// testProcess returns a Process backed by the real edax binary at
// EDAX_PATH, skipping the test if it's not set.
func testProcess(t *testing.T) *Process {
	t.Helper()

	path := os.Getenv(pathEnvVar)
	if path == "" {
		t.Skip("EDAX_PATH not set; skipping test requiring the real edax binary")
	}

	p := NewProcess(path, 0)
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
	p := NewProcess("/does/not/exist/lEdax-x64", 0)

	_, err := p.Evaluate(othello.NewBoardStart(), 10)
	require.Error(t, err)
}

// TestProcess_Evaluate_FailedStartDoesNotLeakFDs guards that repeated failing Evaluate calls
// don't accumulate open file descriptors.
func TestProcess_Evaluate_FailedStartDoesNotLeakFDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("open-fd count is read from /proc, which is Linux-only")
	}

	p := NewProcess("/does/not/exist/lEdax-x64", 0)

	// Warm up once so any one-time allocations aren't counted as a leak.
	_, err := p.Evaluate(othello.NewBoardStart(), 10)
	require.Error(t, err)

	before := openFDCount(t)
	for range 50 {
		_, err := p.Evaluate(othello.NewBoardStart(), 10)
		require.Error(t, err)
	}
	after := openFDCount(t)

	// Slack for unrelated runtime activity; 50 leaked calls (2 fds each) would blow well past it.
	require.LessOrEqual(t, after-before, 5, "open file descriptors grew from %d to %d", before, after)
}

// openFDCount returns the number of file descriptors currently open by this
// process, read from /proc/self/fd (Linux only).
func openFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	return len(entries)
}

// TestProcess_EnsureStarted_FailedRestartClearsStaleState covers a restart whose replacement fails
// to start: stale state must be cleared or the next same-level Evaluate would reuse dead pipes.
func TestProcess_EnsureStarted_FailedRestartClearsStaleState(t *testing.T) {
	prev := exec.Command("sleep", "60")
	require.NoError(t, prev.Start())
	t.Cleanup(func() {
		_ = prev.Process.Kill()
		_ = prev.Wait()
	})

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	// Simulate a running edax at level 6.
	p := NewProcess("/does/not/exist/lEdax-x64", 0)
	p.cmd = prev
	p.level = 6
	p.stdin = w
	p.stdout = bufio.NewReader(r)

	// A different level forces a restart: prev is killed, then the bad path
	// fails to start.
	_, _, err = p.ensureStarted(8)
	require.Error(t, err)

	require.Nil(t, p.cmd)
	require.Zero(t, p.level)
	require.Nil(t, p.stdin)
	require.Nil(t, p.stdout)
}

func TestProcess_Evaluate_InvalidLevel(t *testing.T) {
	p := testProcess(t)

	_, err := p.Evaluate(othello.NewBoardStart(), 0)
	require.Error(t, err)
}

func TestProcess_Evaluate_NoLegalMoves(t *testing.T) {
	p := testProcess(t)

	board, err := othello.NewBoard(^uint64(0), 0)
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

	// Signal 0 (existence check only) should now report an error.
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
