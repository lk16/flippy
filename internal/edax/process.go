package edax

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/lk16/flippy/internal/othello"
)

// pathEnvVar is the environment variable holding the path to the edax binary.
const pathEnvVar = "EDAX_PATH"

// PathFromEnv reads the edax binary path from the EDAX_PATH environment variable.
func PathFromEnv() (string, error) {
	path := os.Getenv(pathEnvVar)
	if path == "" {
		return "", fmt.Errorf("%s environment variable is not set", pathEnvVar)
	}
	return path, nil
}

// ErrStartFailed marks a failure to start the edax subprocess at all (missing binary, exec
// error), which retrying cannot fix.
var ErrStartFailed = errors.New("failed to start edax process")

// Process manages a long-running edax subprocess, restarted only when the requested level changes.
type Process struct {
	path          string
	tasks         int
	hashTableSize int

	mu     sync.Mutex
	level  int
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// NewProcess returns a Process for the edax binary at path; no subprocess starts until the first
// Evaluate. tasks caps the number of parallel search threads edax uses (its -n-tasks flag); tasks <= 0
// leaves it unset, so edax defaults to one thread per CPU on the machine. hashTableSize sets edax's
// hash table size in bits (its -hash-table-size flag, e.g. 20 = 2^20 entries); <= 0 leaves it at
// edax's default.
func NewProcess(path string, tasks, hashTableSize int) *Process {
	return &Process{path: path, tasks: tasks, hashTableSize: hashTableSize}
}

// Evaluate sends position to edax for a search at level; it must have a legal move (edax crashes otherwise).
func (p *Process) Evaluate(position othello.Position, level int) (Evaluation, error) {
	if level <= 0 {
		return Evaluation{}, fmt.Errorf("invalid level: %d", level)
	}
	if !position.HasMoves() {
		return Evaluation{}, errors.New("cannot evaluate a position with no legal moves")
	}

	stdin, stdout, err := p.ensureStarted(level)
	if err != nil {
		return Evaluation{}, fmt.Errorf("failed to start edax: %w", err)
	}

	if _, err := io.WriteString(stdin, problemLine(position)); err != nil {
		// The subprocess likely died (crash, OOM kill); tear it down so the next Evaluate starts a
		// fresh one instead of reusing dead pipes.
		_ = p.Close()
		return Evaluation{}, fmt.Errorf("failed to write problem to edax: %w", err)
	}

	eval, err := parseFinalEvaluation(stdout)
	if err != nil {
		_ = p.Close()
		return Evaluation{}, fmt.Errorf("failed to parse edax output: %w", err)
	}

	return eval, nil
}

// buildArgs returns the edax arguments for a -solve search at level; -n-tasks and -hash-table-size
// only when positive.
func buildArgs(level, tasks, hashTableSize int) []string {
	args := []string{"-solve", "/dev/stdin", "-level", strconv.Itoa(level), "-verbose", "3"}
	if tasks > 0 {
		args = append(args, "-n-tasks", strconv.Itoa(tasks))
	}
	if hashTableSize > 0 {
		args = append(args, "-hash-table-size", strconv.Itoa(hashTableSize))
	}
	return args
}

// ensureStarted returns a running edax process's stdin/stdout at level, restarting it if needed.
func (p *Process) ensureStarted(level int) (io.Writer, *bufio.Reader, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil && p.level == level {
		return p.stdin, p.stdout, nil
	}

	if p.cmd != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		// Clear the fields now so a failed restart below doesn't leave the killed process (and
		// its closed pipes) in place for the next Evaluate to reuse.
		p.cmd = nil
		p.level = 0
		p.stdin = nil
		p.stdout = nil
	}

	cmd := exec.Command(p.path, buildArgs(level, p.tasks, p.hashTableSize)...)
	cmd.Dir = filepath.Join(filepath.Dir(p.path), "..")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		// Close the pipes we opened so a repeatedly-failing Start doesn't leak file descriptors.
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, fmt.Errorf("%w: %w", ErrStartFailed, err)
	}

	p.cmd = cmd
	p.level = level
	p.stdin = stdin
	p.stdout = bufio.NewReader(stdout)

	return p.stdin, p.stdout, nil
}

// Close kills the edax subprocess, if running; a blocked Evaluate call returns with an error.
func (p *Process) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil {
		return nil
	}

	err := p.cmd.Process.Kill()
	_ = p.cmd.Wait()

	p.cmd = nil
	p.level = 0
	p.stdin = nil
	p.stdout = nil

	if err != nil {
		return fmt.Errorf("failed to kill edax process: %w", err)
	}
	return nil
}
