package edax

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/lk16/flippy/internal/othello"
)

// tableBorder is the line edax prints after its results-table header and again after the last result row.
const tableBorder = "------+-----+--------------+-------------+----------+---------------------"

// bestMovesByteOffset is the fixed column edax's principal-variation move list starts at (edax's search.c).
const bestMovesByteOffset = 53

// asciiArtLineRegex matches a row of edax's board dump, e.g. "1 - - - - - - - - 1".
var asciiArtLineRegex = regexp.MustCompile(`^\d [O*\-.]`)

// parseFinalEvaluation reads one problem's final evaluation from r, stopping right after its second
// table-border line so the rest of r is left unread for the next problem on the same edax process.
func parseFinalEvaluation(r *bufio.Reader) (Evaluation, error) {
	var (
		last       *Evaluation
		borderSeen int
	)

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Evaluation{}, fmt.Errorf("edax output ended before a final result: %w", io.ErrUnexpectedEOF)
			}
			return Evaluation{}, fmt.Errorf("failed to read edax output: %w", err)
		}

		if strings.Contains(line, tableBorder) {
			borderSeen++
			if borderSeen == 2 {
				if last == nil {
					return Evaluation{}, errors.New("no evaluation found before final table border")
				}
				return *last, nil
			}
			continue
		}

		if eval, ok := parseResultLine(line); ok {
			last = &eval
		}
	}
}

// parseResultLine parses one edax search-progress row; ok=false for non-data lines (headers, art, etc).
func parseResultLine(line string) (Evaluation, bool) {
	if strings.TrimSpace(line) == "" {
		return Evaluation{}, false
	}
	if strings.Contains(line, "depth") {
		return Evaluation{}, false // header line
	}
	if strings.Contains(line, "*** problem #") {
		return Evaluation{}, false
	}
	if strings.Contains(line, "A B C D E F G H") {
		return Evaluation{}, false
	}
	if asciiArtLineRegex.MatchString(line) {
		return Evaluation{}, false
	}

	columns := strings.Fields(line)
	if len(columns) < 2 {
		return Evaluation{}, false
	}

	depth, confidence, ok := parseDepthConfidence(columns[0])
	if !ok {
		return Evaluation{}, false
	}

	scoreField := columns[1]
	if scoreField[0] == '<' || scoreField[0] == '>' {
		// Non-exact bound: the search isn't done at this depth yet.
		return Evaluation{}, false
	}
	score, err := strconv.Atoi(scoreField)
	if err != nil {
		return Evaluation{}, false
	}

	bestMoves, ok := parseBestMoves(line)
	if !ok {
		return Evaluation{}, false
	}

	return Evaluation{
		Depth:      depth,
		Confidence: confidence,
		Score:      score,
		BestMoves:  bestMoves,
	}, true
}

// parseDepthConfidence parses a field like "10" or "6@73%"; confidence defaults to 100 without "@".
func parseDepthConfidence(field string) (depth, confidence int, ok bool) {
	depthStr, confidenceStr, hasConfidence := strings.Cut(field, "@")

	depth, err := strconv.Atoi(depthStr)
	if err != nil {
		return 0, 0, false
	}

	if !hasConfidence {
		return depth, 100, true
	}

	confidence, err = strconv.Atoi(strings.TrimSuffix(confidenceStr, "%"))
	if err != nil {
		return 0, 0, false
	}

	return depth, confidence, true
}

// parseBestMoves parses the principal-variation move list starting at bestMovesByteOffset in line.
func parseBestMoves(line string) ([]int, bool) {
	if len(line) <= bestMovesByteOffset {
		return nil, true
	}

	var bestMoves []int
	for _, field := range strings.Fields(line[bestMovesByteOffset:]) {
		move, err := othello.ParseField(field)
		if err != nil {
			return nil, false
		}
		bestMoves = append(bestMoves, move)
	}

	return bestMoves, true
}
