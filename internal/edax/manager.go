package edax

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	tableBorder = "------+-----+--------------+-------------+----------+---------------------"
)

type Manager struct {
	level  int
	cmd    *exec.Cmd
	cfg    *config.EdaxConfig
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func NewManager() *Manager {
	cfg := config.LoadEdaxConfig()

	return &Manager{
		level: 0,
		cfg:   cfg,
	}
}

func (m *Manager) Restart() error {
	slog.Debug("Restarting Edax")

	if m.cmd != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill Edax process: %w", err)
		}
	}

	edaxPath := m.cfg.EdaxPath
	cmd := exec.Command(edaxPath, "-solve", "/dev/stdin", "-level", strconv.Itoa(m.level), "-verbose", "3")
	cmd.Dir = filepath.Join(filepath.Dir(edaxPath), "..")

	slog.Debug("Starting Edax", "edaxPath", edaxPath, "cmd.Args", cmd.Args, "cmd.Dir", cmd.Dir)

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

	m.cmd = cmd
	m.stdin = stdin
	m.stdout = stdout

	slog.Debug("Edax process restarted successfully", "level", m.level)
	return nil
}

func (m *Manager) ParseOutput(reader *bufio.Reader) (string, error) {
	lines := []string{}
	tableBorderLineCount := 0
	evalLine := ""

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error reading from stdout: %w", err)
		}
		slog.Debug("Edax stdout", "line", line)
		lines = append(lines, line)

		if strings.Contains(line, tableBorder) {
			tableBorderLineCount++
		}

		if tableBorderLineCount == 2 { //nolint:mnd
			evalLine = lines[len(lines)-3]
			slog.Debug("Evaluation line", "line", evalLine)
			break
		}
	}

	return evalLine, nil
}

func (m *Manager) DoJob(job models.Job) (*models.JobResult, error) {
	if err := m.SetLevel(job.Level); err != nil {
		return nil, fmt.Errorf("failed to set Edax level: %w", err)
	}

	problem := job.Position.ToProblem()

	startTime := time.Now()

	slog.Debug("Edax stdin", "stdin", problem)
	if _, err := m.stdin.Write([]byte(problem)); err != nil {
		return nil, fmt.Errorf("failed to write position: %w", err)
	}

	reader := bufio.NewReader(m.stdout)
	evalLine, err := m.ParseOutput(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse output: %w", err)
	}

	result, err := m.parseEvaluationLine(evalLine, job, startTime)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (m *Manager) SetLevel(level int) error {
	if level == 0 {
		panic("Attempt to set Edax level to 0")
	}

	if level == m.level {
		slog.Debug("Edax already at requested level", "level", level)
		return nil
	}

	slog.Debug("Setting Edax level", "level", level)
	m.level = level
	if err := m.Restart(); err != nil {
		return fmt.Errorf("failed to restart Edax: %w", err)
	}

	return nil
}

func (m *Manager) parseEvaluationLine(line string, job models.Job, startTime time.Time) (*models.JobResult, error) {
	if line == "" {
		return nil, errors.New("empty evaluation line")
	}

	// Skip table borders and other non-evaluation lines
	if strings.Contains(line, "-----") || strings.Contains(line, "positions;") || strings.Contains(line, "/dev/stdin") {
		return nil, fmt.Errorf("invalid evaluation line: %s", line)
	}

	// Normalize whitespace and split into columns
	columns := strings.Fields(line)
	if len(columns) < 2 { //nolint:mnd  // TODO this constant is wrong
		return nil, fmt.Errorf("not enough columns in evaluation line: %s", line)
	}

	// Parse depth and confidence
	depthConfidence := columns[0]
	depthStr := strings.Split(depthConfidence, "@")[0]
	depth, err := strconv.Atoi(depthStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse depth: %w", err)
	}

	confidence := 100
	if strings.Contains(depthConfidence, "@") {
		confidenceStr := strings.Split(strings.Split(depthConfidence, "@")[1], "%")[0]
		confidence, err = strconv.Atoi(confidenceStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse confidence: %w", err)
		}
	}

	// Parse score
	scoreStr := strings.Trim(columns[1], "<>")
	score, err := strconv.Atoi(scoreStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse score: %w", err)
	}

	// Parse best moves
	bestFields := strings.Fields(line[53:])
	bestMoves := make([]int, len(bestFields))
	for i, field := range bestFields {
		bestMoves[i], err = models.FieldToIndex(field)
		if err != nil {
			return nil, fmt.Errorf("failed to parse best move: %w", err)
		}
	}

	evaluation := models.Evaluation{
		Position:   job.Position,
		Level:      job.Level,
		Depth:      depth,
		Confidence: confidence,
		Score:      score,
		BestMoves:  bestMoves,
	}

	if err = evaluation.Validate(); err != nil {
		return nil, fmt.Errorf("invalid evaluation: %w", err)
	}

	result := &models.JobResult{
		Evaluation:      evaluation,
		ComputationTime: time.Since(startTime).Seconds(),
	}

	slog.Debug("Edax result", "result", result)

	return result, nil
}
