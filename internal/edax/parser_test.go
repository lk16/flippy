package edax

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParserParseLine(t *testing.T) {
	// Lines are generated using this command on Edax version 4.5.3:
	// echo 'OOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOO---OXOX----XXXXO----XOOO-------- X;' \
	// | /path/to/edax -solve /dev/stdin -level 60 -verbose 3

	tests := []struct {
		line    string
		want    *parsedLine
		wantErr error
	}{
		{
			line:    "*** problem # 1 ***\n",
			want:    nil,
			wantErr: errProblemNumberLine,
		},
		{
			line:    "\n",
			want:    nil,
			wantErr: errEmptyLine,
		},
		{
			line:    "  A B C D E F G H\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "1 O O O O O O O O 1\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "2 O O O O O O O O 2 * to move\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "3 O O O O O O O O 3\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "4 O O O O O O O O 4 *: discs =  7    moves =  5\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "5 - - . O * O * - 5 O: discs = 38    moves =  6\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "6 - - - * * * * O 6  empties = 19      ply = 42\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "7 - - - - * O O O 7\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "8 - - - - . . . . 8\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "  A B C D E F G H\n",
			want:    nil,
			wantErr: errBoardASCIIArtLine,
		},
		{
			line:    "\n",
			want:    nil,
			wantErr: errEmptyLine,
		},
		{
			line:    " depth|score|       time   |  nodes (N)  |   N/s    | principal variation\n",
			want:    nil,
			wantErr: errTableLine,
		},
		{
			line:    "------+-----+--------------+-------------+----------+---------------------\n",
			want:    nil,
			wantErr: errTableLine,
		},
		{
			line:    " 0@73%  -44        0:00.000             7            f8                  \n",
			want:    &parsedLine{depth: 0, confidence: 73, score: -44, bestMoves: []int{61}},
			wantErr: nil,
		},
		{
			line:    " 0@73%  -44        0:00.000             7            f8                  \n",
			want:    &parsedLine{depth: 0, confidence: 73, score: -44, bestMoves: []int{61}},
			wantErr: nil,
		},
		{
			line:    " 5@73%  -59        0:00.000           210            f8 H5 c5            ",
			want:    &parsedLine{depth: 5, confidence: 73, score: -59, bestMoves: []int{61, 39, 34}},
			wantErr: nil,
		},
		{
			line:    " 5@73%  -53        0:00.000           464            e8 H5 h8            \n",
			want:    &parsedLine{depth: 5, confidence: 73, score: -53, bestMoves: []int{60, 39, 63}},
			wantErr: nil,
		},
		{
			line:    " 5@73%  -53        0:00.000          1785            e8 H5 h8            \n",
			want:    &parsedLine{depth: 5, confidence: 73, score: -53, bestMoves: []int{60, 39, 63}},
			wantErr: nil,
		},
		{
			line:    " 7@73%  -50        0:00.000          2627            e8 H5 c5 C7 c6      \n",
			want:    &parsedLine{depth: 7, confidence: 73, score: -50, bestMoves: []int{60, 39, 34, 50, 42}},
			wantErr: nil,
		},
		{
			line:    " 7@73%  -50        0:00.000          4730            e8 H5 c5 C7 c6      \n",
			want:    &parsedLine{depth: 7, confidence: 73, score: -50, bestMoves: []int{60, 39, 34, 50, 42}},
			wantErr: nil,
		},
		{
			line:    " 9@73%  -51        0:00.001          6064    6064000 e8 H5 h8 C7 c6 C5 b8\n",
			want:    &parsedLine{depth: 9, confidence: 73, score: -51, bestMoves: []int{60, 39, 63, 50, 42, 34, 57}},
			wantErr: nil,
		},
		{
			line:    " 9@73%  -51        0:00.001          9439    9439000 e8 H5 h8 C7 c6 C5 b8\n",
			want:    &parsedLine{depth: 9, confidence: 73, score: -51, bestMoves: []int{60, 39, 63, 50, 42, 34, 57}},
			wantErr: nil,
		},
		{
			line:    "19@73% <-53        0:00.001         10657   10657000 e8 H5 h8 C7 c6 C5 b8\n",
			want:    nil,
			wantErr: errScoreParseError,
		},
		{
			line:    "19@73%  -60        0:00.003         24589    8196333 e8 H5 c5 B5 h8 C6 ps\n",
			want:    &parsedLine{depth: 19, confidence: 73, score: -60, bestMoves: []int{60, 39, 34, 33, 63, 42, -1}},
			wantErr: nil,
		},
		{
			line:    "   19  <-62        0:00.003         31405   10468333 e8 H5 c5 D8 h8 B5   \n",
			want:    nil,
			wantErr: errScoreParseError,
		},
		{
			line:    "   19   -60        0:00.003           352     117333 h8 H5 g8 C7 c6 B6 c5\n",
			want:    &parsedLine{depth: 19, confidence: 100, score: -60, bestMoves: []int{63, 39, 62, 50, 42, 41, 34}},
			wantErr: nil,
		},
		{
			line:    "   19   -60        0:00.003         52539   17513000 h8 H5 g8 C7 c6 B6 c5\n",
			want:    &parsedLine{depth: 19, confidence: 100, score: -60, bestMoves: []int{63, 39, 62, 50, 42, 41, 34}},
			wantErr: nil,
		},
		{
			line:    "\n",
			want:    nil,
			wantErr: errEmptyLine,
		},
		{
			line:    "------+-----+--------------+-------------+----------+---------------------\n",
			want:    nil,
			wantErr: errTableLine,
		},
	}

	parser := &parser{}

	for testIndex, test := range tests {
		testName := fmt.Sprintf("Line-%d", testIndex+1)
		t.Run(testName, func(t *testing.T) {
			got, err := parser.parseLine(test.line)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantErr, err)
		})
	}
}
