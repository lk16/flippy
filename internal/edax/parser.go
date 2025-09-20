package edax

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lk16/flippy/api/internal/api"
	"github.com/lk16/flippy/api/internal/othello"
)

const (
	tableBorder = "------+-----+--------------+-------------+----------+---------------------"
)

var (
	errEmptyLine         = errors.New("line is empty")
	errTableLine         = errors.New("line is table header or border line")
	errBoardASCIIArtLine = errors.New("line is board ascii art line")
	errProblemNumberLine = errors.New("line is problem number line")
	errScoreParseError   = errors.New("line has broken score")

	asciiArtLineRegex = regexp.MustCompile(`^\d [O*\-\.]`)
)

// parser reads edax output and send results over a channel. It should only be used for parsing the output of one job.
type parser struct {
	job              api.Job
	startTime        time.Time
	reader           *bufio.Reader
	resultChan       chan<- Result
	finalResultOnly  bool
	prevResult       *Result
	tableBorderCount int
	verbose          bool
}

func newParser(
	job api.Job,
	startTime time.Time,
	reader *bufio.Reader,
	resultChan chan<- Result,
	finalEvalOnly bool,
	verbose bool,
) *parser {
	return &parser{
		job:             job,
		startTime:       startTime,
		reader:          reader,
		resultChan:      resultChan,
		finalResultOnly: finalEvalOnly,
		verbose:         verbose,
	}
}

func (p *parser) parseLines() {
	for {
		line, err := p.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				// EOF likely means the edax process got killed.
				return
			}

			err = fmt.Errorf("error reading from stdout: %w", err)
			p.resultChan <- Result{Err: err}
			return
		}

		if p.verbose {
			log.Printf("Edax stdout: %v", line)
		}

		parsedLine, err := p.parseLine(line)
		if err != nil {
			if p.tableBorderCount == 2 {
				// Find final score and return
				p.sendFinalResult()
				return
			}

			if p.isExpectedError(err) {
				continue
			}

			err = fmt.Errorf("failed to parse output line: %w", err)
			p.resultChan <- Result{Err: err}
			return
		}

		p.sendIntermediateResult(parsedLine)
	}
}

func (p *parser) parseLine(line string) (*parsedLine, error) {
	if line == "\n" {
		return nil, errEmptyLine
	}

	if strings.Contains(line, tableBorder) {
		p.tableBorderCount++
		return nil, errTableLine
	}

	if strings.Contains(line, "depth") {
		return nil, errTableLine
	}

	if strings.Contains(line, "*** problem #") {
		return nil, errProblemNumberLine
	}

	if strings.Contains(line, "A B C D E F G H") {
		return nil, errBoardASCIIArtLine
	}

	if ok := asciiArtLineRegex.MatchString(line); ok {
		return nil, errBoardASCIIArtLine
	}

	// Normalize whitespace and split into columns
	columns := strings.Fields(line)
	if len(columns) < 2 {
		return nil, fmt.Errorf("not enough columns in output line: %s", line)
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
	scoreStr := columns[1]
	if scoreStr[0] == '>' || scoreStr[0] == '<' {
		return nil, errScoreParseError
	}

	score, err := strconv.Atoi(scoreStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse score: %w", err)
	}

	// Parse best moves
	bestFields := strings.Fields(line[53:])
	bestMoves := make([]int, len(bestFields))
	for i, field := range bestFields {
		bestMoves[i], err = othello.FieldToIndex(field)
		if err != nil {
			return nil, fmt.Errorf("failed to parse best move: %w", err)
		}
	}

	parsedLine := &parsedLine{
		depth:      depth,
		confidence: confidence,
		score:      score,
		bestMoves:  bestMoves,
	}

	return parsedLine, nil
}

func (*parser) isExpectedError(err error) bool {
	return errors.Is(err, errEmptyLine) ||
		errors.Is(err, errTableLine) ||
		errors.Is(err, errProblemNumberLine) ||
		errors.Is(err, errScoreParseError) ||
		errors.Is(err, errBoardASCIIArtLine)
}

func (p *parser) makeEvaluation(parsedLine *parsedLine) (*api.Evaluation, error) {
	evaluation := &api.Evaluation{
		Position:   p.job.Position,
		Level:      p.job.Level,
		Depth:      parsedLine.depth,
		Confidence: parsedLine.confidence,
		Score:      parsedLine.score,
		BestMoves:  parsedLine.bestMoves,
	}

	if err := evaluation.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate evaluation: %w", err)
	}

	return evaluation, nil
}

// sendIntermediateResult may send a previous intermediate result.
func (p *parser) sendIntermediateResult(parsedLine *parsedLine) {
	evaluation, err := p.makeEvaluation(parsedLine)
	if err != nil {
		err = fmt.Errorf("failed to validate evaluation: %w", err)
		p.resultChan <- Result{Err: err}
		return
	}

	result := Result{
		Result: &api.JobResult{
			Evaluation:      *evaluation,
			ComputationTime: time.Since(p.startTime).Seconds(),
		},
	}

	if !p.finalResultOnly && p.prevResult != nil {
		prevDepth := p.prevResult.Result.Evaluation.Depth
		if evaluation.Depth > prevDepth {
			p.resultChan <- *p.prevResult
		}
	}

	p.prevResult = &result
}

func (p *parser) sendFinalResult() {
	p.resultChan <- *p.prevResult
}
