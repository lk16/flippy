package edax

import (
	"bufio"
	"fmt"
	"io"
	"log"
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

var edaxManager *EdaxManager

type EdaxManager struct {
	level   int
	cmd     *exec.Cmd
	cfg     *config.EdaxConfig
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	verbose bool
}

func GetEdaxManager(verbose bool) *EdaxManager {
	if edaxManager == nil {
		cfg := config.LoadEdaxConfig()

		edaxManager = &EdaxManager{
			level:   0,
			cfg:     cfg,
			verbose: verbose,
		}
	}
	return edaxManager
}

func (m *EdaxManager) logVerbose(format string, args ...any) {
	if m.verbose {
		log.Printf(format, args...)
	}
}

func (m *EdaxManager) Restart() error {
	m.logVerbose("Restarting Edax")

	if m.cmd != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill Edax process: %w", err)
		}
	}

	edaxPath := m.cfg.EdaxPath
	m.logVerbose("edaxPath: %s", edaxPath)

	cmd := exec.Command(edaxPath, "-solve", "/dev/stdin", "-level", fmt.Sprintf("%d", m.level), "-verbose", "3")
	m.logVerbose("cmd.Args: %v", cmd.Args)

	cmd.Dir = filepath.Join(filepath.Dir(edaxPath), "..")
	m.logVerbose("cmd.Dir: %s", cmd.Dir)

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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Edax process: %w", err)
	}

	m.cmd = cmd
	m.stdin = stdin
	m.stdout = stdout

	m.logVerbose("Edax process restarted")
	return nil
}

func (m *EdaxManager) ParseOutput(reader *bufio.Reader) (string, error) {
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
		m.logVerbose("Edax stdout: %s", line)
		lines = append(lines, line)

		if strings.Contains(line, tableBorder) {
			tableBorderLineCount++
		}

		if tableBorderLineCount == 2 {
			evalLine = lines[len(lines)-3]
			m.logVerbose("Evaluation line: %s", evalLine)
			break
		}
	}

	return evalLine, nil
}

func (m *EdaxManager) DoJob(job models.Job) (*models.JobResult, error) {
	if err := m.SetLevel(job.Level); err != nil {
		return nil, fmt.Errorf("failed to set Edax level: %w", err)
	}

	problem := job.Position.ToProblem()

	startTime := time.Now()

	m.logVerbose("Edax stdin: %s", problem)
	if _, err := m.stdin.Write([]byte(problem)); err != nil {
		log.Printf("Failed to write position: %v", err)
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

func (m *EdaxManager) SetLevel(level int) error {
	if level == 0 {
		panic("Attempt to set Edax level to 0")
	}

	if level == m.level {
		m.logVerbose("Edax level already set to %d", level)
		return nil
	}

	m.logVerbose("Setting Edax level to %d", level)
	m.level = level
	if err := m.Restart(); err != nil {
		return fmt.Errorf("failed to restart Edax: %w", err)
	}

	return nil
}

func (m *EdaxManager) parseEvaluationLine(line string, job models.Job, startTime time.Time) (*models.JobResult, error) {
	if line == "" {
		return nil, fmt.Errorf("empty evaluation line")
	}

	// Skip table borders and other non-evaluation lines
	if strings.Contains(line, "-----") || strings.Contains(line, "positions;") || strings.Contains(line, "/dev/stdin") {
		return nil, fmt.Errorf("invalid evaluation line: %s", line)
	}

	// Normalize whitespace and split into columns
	columns := strings.Fields(line)
	if len(columns) < 2 {
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
		bestMoves[i] = models.FieldToIndex(field)
	}

	evaluation := models.Evaluation{
		Position:   job.Position,
		Level:      job.Level,
		Depth:      depth,
		Confidence: confidence,
		Score:      score,
		BestMoves:  bestMoves,
	}

	if err := evaluation.Validate(); err != nil {
		return nil, fmt.Errorf("invalid evaluation: %w", err)
	}

	result := &models.JobResult{
		Evaluation:      evaluation,
		ComputationTime: time.Since(startTime).Seconds(),
	}

	m.logVerbose("Edax result: %+v", result)

	return result, nil
}
