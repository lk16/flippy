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

// pathEnvVar is the environment variable holding the path to the edax
// binary.
const pathEnvVar = "EDAX_PATH"

// PathFromEnv reads the edax binary path from the EDAX_PATH environment
// variable.
func PathFromEnv() (string, error) {
	path := os.Getenv(pathEnvVar)
	if path == "" {
		return "", fmt.Errorf("%s environment variable is not set", pathEnvVar)
	}
	return path, nil
}

// Process manages a single long-running edax subprocess. The same
// subprocess handles every Evaluate call at a given level; it's only
// restarted when the requested level changes, since edax's level is a
// startup flag rather than something that can be changed mid-run.
type Process struct {
	path string

	mu     sync.Mutex
	level  int
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// NewProcess returns a Process that launches the edax binary at path. No
// subprocess is started until the first Evaluate call.
func NewProcess(path string) *Process {
	return &Process{path: path}
}

// Evaluate sends board to edax for a search at level and returns its final
// evaluation. board must have at least one legal move for the player to
// move — edax crashes on positions without one, so callers must resolve
// passes and finished games before calling Evaluate.
func (p *Process) Evaluate(board othello.Board, level int) (Evaluation, error) {
	if level <= 0 {
		return Evaluation{}, fmt.Errorf("invalid level: %d", level)
	}
	if !board.HasMoves() {
		return Evaluation{}, errors.New("cannot evaluate a board with no legal moves")
	}

	stdin, stdout, err := p.ensureStarted(level)
	if err != nil {
		return Evaluation{}, fmt.Errorf("failed to start edax: %w", err)
	}

	if _, err := io.WriteString(stdin, problemLine(board)); err != nil {
		return Evaluation{}, fmt.Errorf("failed to write problem to edax: %w", err)
	}

	eval, err := parseFinalEvaluation(stdout)
	if err != nil {
		return Evaluation{}, fmt.Errorf("failed to parse edax output: %w", err)
	}

	return eval, nil
}

// ensureStarted returns the current stdin/stdout of a running edax process
// at level, restarting it first if it's not running or is at a different
// level.
func (p *Process) ensureStarted(level int) (io.Writer, *bufio.Reader, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil && p.level == level {
		return p.stdin, p.stdout, nil
	}

	if p.cmd != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}

	cmd := exec.Command(p.path, "-solve", "/dev/stdin", "-level", strconv.Itoa(level), "-verbose", "3")
	cmd.Dir = filepath.Join(filepath.Dir(p.path), "..")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start edax process: %w", err)
	}

	p.cmd = cmd
	p.level = level
	p.stdin = stdin
	p.stdout = bufio.NewReader(stdout)

	return p.stdin, p.stdout, nil
}

// Close kills the edax subprocess, if one is running. Any Evaluate call
// blocked reading its output will return with an error.
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
