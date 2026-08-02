package edax

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// singleProblemOutput is real `lEdax-x64 -solve ... -level 10 -verbose 3`
// stdout for the start position after black plays d3, captured directly
// from the binary (see internal/othello/gen for how boards are produced;
// this fixture predates that and was captured manually against the real
// edax-reversi binary during development).
const singleProblemOutput = `
*** problem # 1 ***

  A B C D E F G H
1 - - - - - - - - 1
2 - - - - - - - - 2 * to move
3 - - - . - - - - 3
4 - - . O * - - - 4 *: discs =  2    moves =  4
5 - - - * O . - - 5 O: discs =  2    moves =  4
6 - - - - . - - - 6  empties = 60      ply =  1
7 - - - - - - - - 7
8 - - - - - - - - 8
  A B C D E F G H

 depth|score|       time   |  nodes (N)  |   N/s    | principal variation
------+-----+--------------+-------------+----------+---------------------
    0   -04        0:00.000             6            d3
    0   -04        0:00.000             6            d3
    6   -04        0:00.000           489            d3 C5 f6 E3
    6   -04        0:00.000          2280            d3 C5 f6 E3
    8   -03        0:00.000          5574            d3 E3 f3 E2 f4 G3
    8   -03        0:00.002         16839    8419500 d3 E3 f3 E2 f4 G3
   10   +00        0:00.003         53244   17748000 d3 C5 e6 D2 c3 E3 f3
   10   +00        0:00.007        155022   22146000 d3 C5 e6 D2 c3 E3 f3

------+-----+--------------+-------------+----------+---------------------
/tmp/foo: 155022 nodes in  0:00.007 (22146000 nodes/s).
1 positions; 0 erroneous move; 0 erroneous score; mean absolute score error = 0.000; mean absolute move error = 0.000
`

// selectiveOutput is real output from the same position at -level 20,
// which (unlike level 10) uses probabilistic (non-100%-confidence) search
// for most of the iterative deepening, showing both the "@NN%" confidence
// suffix and '<'/'>' non-exact-bound rows that must be skipped.
const selectiveOutput = `
*** problem # 1 ***

  A B C D E F G H
1 - - - - - - - - 1
2 - - - - - - - - 2 * to move
3 - - - . - - - - 3
4 - - . O * - - - 4 *: discs =  2    moves =  4
5 - - - * O . - - 5 O: discs =  2    moves =  4
6 - - - - . - - - 6  empties = 60      ply =  1
7 - - - - - - - - 7
8 - - - - - - - - 8
  A B C D E F G H

 depth|score|       time   |  nodes (N)  |   N/s    | principal variation
------+-----+--------------+-------------+----------+---------------------
 0@73%  -04        0:00.000             6            d3
 0@73%  -04        0:00.000             6            d3
16@73% >+00        0:00.008        204111   25513875 d3 E3 f4 C3 c4 D6 e6
16@73%  +00        0:00.009        207460   23051111 d3 E3 f4 C3 c4 D6 e6
18@73% <-01        0:00.013        405552   31196308 d3 C5
18@73%  +00        0:00.025       1175140   47005600 d3 E3 f4 C3 c4 D6 e6
20@73% <-01        0:00.054       4019509   74435352 d3 C5 f6 F5 e6
20@73%  -01        0:00.072       7044923   97846153 d3 C5 f6 F5 e6 E3 c3

------+-----+--------------+-------------+----------+---------------------
/tmp/foo: 7044923 nodes in  0:00.072 (97846153 nodes/s).
1 positions; 0 erroneous move; 0 erroneous score; mean absolute score error = 0.000; mean absolute move error = 0.000
`

// twoProblemOutput is real output from sending the same problem twice to
// one long-running edax process at -level 6, confirming the process stays
// alive and keeps printing "*** problem #" blocks back to back rather than
// exiting after the first.
const twoProblemOutput = `
*** problem # 1 ***

  A B C D E F G H
1 - - - - - - - - 1
2 - - - - - - - - 2 * to move
3 - - - . - - - - 3
4 - - . O * - - - 4 *: discs =  2    moves =  4
5 - - - * O . - - 5 O: discs =  2    moves =  4
6 - - - - . - - - 6  empties = 60      ply =  1
7 - - - - - - - - 7
8 - - - - - - - - 8
  A B C D E F G H

 depth|score|       time   |  nodes (N)  |   N/s    | principal variation
------+-----+--------------+-------------+----------+---------------------
    0   -04        0:00.000             6            d3
    0   -04        0:00.000             6            d3
    4   -04        0:00.000            61            d3 E3
    4   -04        0:00.000           316            d3 E3
    6   -04        0:00.000           784            d3 C5 f6 E3
    6   -04        0:00.001          2525    2525000 d3 C5 f6 E3

------+-----+--------------+-------------+----------+---------------------

*** problem # 2 ***

  A B C D E F G H
1 - - - - - - - - 1
2 - - - - - - - - 2 * to move
3 - - - . - - - - 3
4 - - . O * - - - 4 *: discs =  2    moves =  4
5 - - - * O . - - 5 O: discs =  2    moves =  4
6 - - - - . - - - 6  empties = 60      ply =  1
7 - - - - - - - - 7
8 - - - - - - - - 8
  A B C D E F G H

 depth|score|       time   |  nodes (N)  |   N/s    | principal variation
------+-----+--------------+-------------+----------+---------------------
    0   -04        0:00.000             6            d3
    0   -04        0:00.000             6            d3
    4   -04        0:00.000            61            d3 E3
    4   -04        0:00.000           316            d3 E3
    6   -04        0:00.000           784            d3 C5 f6 E3
    6   -04        0:00.000          2525            d3 C5 f6 E3

------+-----+--------------+-------------+----------+---------------------
/tmp/foo: 5050 nodes in  0:00.001 ( 5050000 nodes/s).
2 positions; 0 erroneous move; 0 erroneous score; mean absolute score error = 0.000; mean absolute move error = 0.000
`

func mustParseFields(t *testing.T, fields ...string) []int {
	t.Helper()
	moves := make([]int, len(fields))
	for i, f := range fields {
		move, err := othello.ParseField(f)
		require.NoError(t, err)
		moves[i] = move
	}
	return moves
}

func TestParseFinalEvaluation_ExactSearch(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(singleProblemOutput))

	got, err := parseFinalEvaluation(r)
	require.NoError(t, err)

	want := Evaluation{
		Depth:      10,
		Confidence: 100,
		Score:      0,
		BestMoves:  mustParseFields(t, "d3", "C5", "e6", "D2", "c3", "E3", "f3"),
	}
	require.Equal(t, want, got)
}

func TestParseFinalEvaluation_SelectiveSearch(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(selectiveOutput))

	got, err := parseFinalEvaluation(r)
	require.NoError(t, err)

	want := Evaluation{
		Depth:      20,
		Confidence: 73,
		Score:      -1,
		BestMoves:  mustParseFields(t, "d3", "C5", "f6", "F5", "e6", "E3", "c3"),
	}
	require.Equal(t, want, got)
}

func TestParseFinalEvaluation_MultipleProblemsOnSameReader(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(twoProblemOutput))

	want := Evaluation{
		Depth:      6,
		Confidence: 100,
		Score:      -4,
		BestMoves:  mustParseFields(t, "d3", "C5", "f6", "E3"),
	}

	first, err := parseFinalEvaluation(r)
	require.NoError(t, err)
	require.Equal(t, want, first)

	second, err := parseFinalEvaluation(r)
	require.NoError(t, err)
	require.Equal(t, want, second)
}

func TestParseFinalEvaluation_EmptyInput(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))

	_, err := parseFinalEvaluation(r)
	require.Error(t, err)
}

func TestParseFinalEvaluation_TruncatedBeforeSecondBorder(t *testing.T) {
	truncated := strings.SplitAfter(singleProblemOutput, tableBorder)[0] +
		"\n    0   -04        0:00.000             6            d3                  \n"
	r := bufio.NewReader(strings.NewReader(truncated))

	_, err := parseFinalEvaluation(r)
	require.Error(t, err)
}

func TestParseFinalEvaluation_NoDataRowsBeforeSecondBorder(t *testing.T) {
	input := tableBorder + "\n" + tableBorder + "\n"
	r := bufio.NewReader(strings.NewReader(input))

	_, err := parseFinalEvaluation(r)
	require.Error(t, err)
}

func TestParseResultLine_SkipsNonDataLines(t *testing.T) {
	nonDataLines := []string{
		"\n",
		"   \n",
		"*** problem # 1 ***\n",
		"  A B C D E F G H\n",
		"1 - - - - - - - - 1\n",
		"4 - - . O * - - - 4 *: discs =  2    moves =  4\n",
		" depth|score|       time   |  nodes (N)  |   N/s    | principal variation\n",
	}

	for _, line := range nonDataLines {
		_, ok := parseResultLine(line)
		require.False(t, ok, "expected line to be skipped: %q", line)
	}
}

func TestParseResultLine_ExactRow(t *testing.T) {
	eval, ok := parseResultLine("   10   +00        0:00.003         53244   17748000 d3 C5 e6 D2 c3 E3 f3\n")
	require.True(t, ok)
	require.Equal(t, Evaluation{
		Depth:      10,
		Confidence: 100,
		Score:      0,
		BestMoves:  mustParseFields(t, "d3", "C5", "e6", "D2", "c3", "E3", "f3"),
	}, eval)
}

func TestParseResultLine_ConfidenceSuffix(t *testing.T) {
	eval, ok := parseResultLine(" 0@73%  -04        0:00.000             6            d3                  \n")
	require.True(t, ok)
	require.Equal(t, 73, eval.Confidence)
	require.Equal(t, 0, eval.Depth)
	require.Equal(t, -4, eval.Score)
}

func TestParseResultLine_SkipsNonExactBound(t *testing.T) {
	_, ok := parseResultLine("16@73% >+00        0:00.008        204111   25513875 d3 E3 f4 C3 c4 D6 e6\n")
	require.False(t, ok)

	_, ok = parseResultLine("18@73% <-01        0:00.013        405552   31196308 d3 C5               \n")
	require.False(t, ok)
}

func TestParseResultLine_TooFewColumns(t *testing.T) {
	_, ok := parseResultLine("garbage\n")
	require.False(t, ok)
}

func TestParseResultLine_InvalidDepth(t *testing.T) {
	_, ok := parseResultLine("xx   +00        0:00.000             6            d3\n")
	require.False(t, ok)
}

func TestParseResultLine_InvalidConfidence(t *testing.T) {
	_, ok := parseResultLine("0@xx%  -04        0:00.000             6            d3\n")
	require.False(t, ok)
}

func TestParseResultLine_InvalidScore(t *testing.T) {
	_, ok := parseResultLine("10   xx        0:00.000             6            d3\n")
	require.False(t, ok)
}

func TestParseResultLine_InvalidBestMoveField(t *testing.T) {
	_, ok := parseResultLine("   10   +00        0:00.003         53244   17748000 zz\n")
	require.False(t, ok)
}

func TestParseResultLine_NoBestMoves(t *testing.T) {
	// Shorter than bestMovesByteOffset: no move list present.
	eval, ok := parseResultLine("0 +00\n")
	require.True(t, ok)
	require.Empty(t, eval.BestMoves)
}
