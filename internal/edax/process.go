package edax

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
)

type Process struct {
	level      int
	cmd        *exec.Cmd
	cfg        *config.EdaxConfig
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	resultChan chan Result
	verbose    bool
}

// NewProcess initializes an edax process.
func NewProcess(resultChan chan Result) *Process {
	cfg := config.LoadEdaxConfig()

	// TODO enable verbosity via env var

	return &Process{
		level:      0,
		cfg:        cfg,
		resultChan: resultChan,
	}
}

// SetVerbose changes log verbosity.
func (p *Process) SetVerbose(verbose bool) {
	p.verbose = verbose
}

// DoJobSync does a job. It only returns the final evaluation.
func (p *Process) DoJobSync(job models.Job) (*models.JobResult, error) {
	go p.DoJob(job, true)

	result := <-p.resultChan
	if result.Err != nil {
		return nil, result.Err
	}

	return result.Result, nil
}

// DoJob should be used as a go-routine. It handles a job and returns one or more results in ResultChan.
func (p *Process) DoJob(job models.Job, finalEvalOnly bool) {
	if err := p.SetLevel(job.Level); err != nil {
		p.resultChan <- Result{Err: err}
		return
	}

	problem := job.Position.ToProblem()

	startTime := time.Now()

	if p.verbose {
		log.Printf("Edax stdin: %v", problem)
	}

	if _, err := p.stdin.Write([]byte(problem)); err != nil {
		err = fmt.Errorf("failed to write position: %w", err)
		p.resultChan <- Result{Err: err}
		return
	}

	reader := bufio.NewReader(p.stdout)
	parser := newParser(job, startTime, reader, p.resultChan, finalEvalOnly, p.verbose)
	parser.parseLines()
}

func (p *Process) SetLevel(level int) error {
	if level == 0 {
		return errors.New("attempt to set Edax level to 0")
	}

	if level == p.level {
		return nil
	}

	p.level = level
	if err := p.restart(); err != nil {
		return fmt.Errorf("failed to restart Edax: %w", err)
	}

	return nil
}

func (p *Process) restart() error {
	if p.cmd != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill Edax process: %w", err)
		}
	}

	edaxPath := p.cfg.EdaxPath
	cmd := exec.Command(edaxPath, "-solve", "/dev/stdin", "-level", strconv.Itoa(p.level), "-verbose", "3")
	cmd.Dir = filepath.Join(filepath.Dir(edaxPath), "..")

	if p.verbose {
		log.Printf("Starting edax edaxPath=%s args=%v dir=%s", edaxPath, cmd.Args, cmd.Dir)
	}

	if cmd.Err != nil {
		return fmt.Errorf("failed to set cmd.Dir: %w", cmd.Err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Edax process: %w", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = stdout

	return nil
}

func (p *Process) Kill() error {
	if p.cmd == nil {
		return nil
	}

	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill Edax process: %w", err)
	}

	return nil
}
