package models

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const samplesDir = "pgn_samples"

func sampleFiles() []string {
	files, err := os.ReadDir(samplesDir)
	if err != nil {
		panic(err)
	}

	filenames := make([]string, 0, len(files))
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".pgn") {
			filenames = append(filenames, file.Name())
		}
	}

	return filenames
}

func TestParsePGN(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		panic(err)
	}

	expectedValues := map[string]Game{
		samplesDir + "/flyordie_2025.pgn": {
			filename: samplesDir + "/flyordie_2025.pgn",
			metadata: &GameMetadata{
				IsXot: false,
				Date:  time.Date(2025, 1, 30, 0, 0, 0, 0, loc),
				Site:  "www.flyordie.com",
				Players: [2]Player{
					{Name: "ozi28", Rating: 618},
					{Name: "LK16", Rating: 591},
				},
				Winner: WHITE,
			},
			moves: []int{
				44, 29, 20, 43, 34, 21, 42, 18, 26, 19, 10, 17, 11, 33, 37, 25, 38, 12,
				13, 5, 41, 2, 45, 22, 31, 23, 16, 24, 32, 30, 8, 39, 14, 46, 4, 3, 47,
				7, 9, 52, 6, 15, 61, 59, 53, 51, 54, 50, 60, 58, 57, 63, 55, 62,
			},
		},
		samplesDir + "/playok_xot.pgn": {
			filename: samplesDir + "/playok_xot.pgn",
			metadata: &GameMetadata{
				IsXot: true,
				Date:  time.Date(2021, 3, 11, 21, 02, 25, 0, loc),
				Site:  "PlayOK",
				Players: [2]Player{
					{Name: "lk16", Rating: 1331},
					{Name: "oscareriksson", Rating: 1642},
				},
				Winner: BLACK,
			},
			moves: []int{
				37, 43, 18, 21, 34, 44, 19, 38, 51, 50, 45, 29, 53, 46, 52, 60, 22, 59, 20,
				58, 39, 31, 23, 30, 61, 62, 47, 42, 41, 25, 26, 32, 16, 33, 48, 40, 24, 17,
				10, 11, 4, 2, 12, 9, 0, 8, 3, 5, 1, 13, 49, 54, 6, 14, 63, 55, 15, 7, 57,
				56,
			},
		},
		samplesDir + "/playok_normal.pgn": {
			filename: samplesDir + "/playok_normal.pgn",
			metadata: &GameMetadata{
				IsXot: false,
				Date:  time.Date(2021, 3, 11, 20, 13, 30, 0, loc),
				Site:  "PlayOK",
				Players: [2]Player{
					{Name: "alcupone", Rating: 1224},
					{Name: "lk16", Rating: 1314},
				},
				Winner: BLACK,
			},
			moves: []int{
				37, 43, 26, 19, 18, 29, 34, 17, 10, 20, 11, 42, 25, 33, 13, 12, 21, 2, 4,
				3, 50, 5, 41, 58, 32, 24, 16, 51, 44, 6, 40, 38, 39, 46, 30, 47, 22, 31,
				23, 52, 45, 15, 60, 59, 57, 49, 61, 53, 62, 9, 8, 14, 7, 54, 48, -1, 56,
				-1, 55, 63, 1, 0,
			},
		},
	}

	for _, file := range sampleFiles() {
		filepath := samplesDir + "/" + file
		t.Run(file, func(t *testing.T) {
			var game *Game
			game, err = NewGameFromPGN(filepath)
			require.NoError(t, err, "Failed to load game from "+filepath)
			require.Equal(t, game.GetFilename(), filepath)

			expected, ok := expectedValues[filepath]
			if !ok {
				t.Fatalf("Unhandled test file: %s", filepath)
			}

			require.Equal(t, expected.filename, game.GetFilename())
			require.Equal(t, expected.metadata, game.metadata)
			require.Equal(t, expected.moves, game.moves)
		})
	}
}

func TestNewGame(t *testing.T) {
	game := NewGame()

	require.NotNil(t, game)
	require.NotNil(t, game.metadata)
	require.Empty(t, game.moves)
	require.Empty(t, game.filename)
}

func TestNewGameFromMoves(t *testing.T) {
	tests := []struct {
		name        string
		moves       []int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty moves",
			moves:       []int{},
			expectError: false,
		},
		{
			name:        "valid moves",
			moves:       []int{44, 29, 20, 43},
			expectError: false,
		},
		{
			name:        "invalid move",
			moves:       []int{44, 0}, // 0 is not a valid move
			expectError: true,
			errorMsg:    "invalid move: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game, err := NewGameFromMoves(tt.moves)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				require.Nil(t, game)
			} else {
				require.NoError(t, err)
				require.NotNil(t, game)
				require.Len(t, game.moves, len(tt.moves))
			}
		})
	}
}

func TestMetaData(t *testing.T) {
	tests := []struct {
		name     string
		game     *Game
		expected *GameMetadata
	}{
		{
			name:     "nil metadata",
			game:     &Game{metadata: nil},
			expected: nil,
		},
		{
			name: "with metadata",
			game: &Game{
				metadata: &GameMetadata{
					IsXot: false,
					Site:  "test",
				},
			},
			expected: &GameMetadata{
				IsXot: false,
				Site:  "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.game.MetaData()
			require.Equal(t, tt.expected, result)

			// Test that it returns a copy, not the original
			if result != nil && tt.game.metadata != nil {
				require.NotSame(t, result, tt.game.metadata)
			}
		})
	}
}

func TestGetNormalizedPositionsWithChildren(t *testing.T) {
	// Create a game with some moves

	tests := []struct {
		name     string
		moves    []int
		expected int // expected number of positions
	}{
		{
			name:     "start",
			moves:    []int{},
			expected: 2,
		},
		{
			name:     "after one move",
			moves:    []int{19},
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := NewGame()
			game.moves = tt.moves

			positions := game.GetNormalizedPositionsWithChildren()
			require.NotNil(t, positions)
			require.Len(t, positions, tt.expected)
		})
	}
}

func TestLastBoard(t *testing.T) {
	tests := []struct {
		name     string
		game     *Game
		expected int // expected number of moves in the board
	}{
		{
			name:     "empty game",
			game:     NewGame(),
			expected: 0,
		},
		{
			name: "game with moves",
			game: &Game{
				moves: []int{44, 29, 20, 43},
				start: NewBoardStart(),
			},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := tt.game.GetBoard()

			require.NotNil(t, board)

			// Count the moves that would lead to this board
			moveCount := 0
			for i := range len(tt.game.moves) {
				if tt.game.moves[i] != PassMove {
					moveCount++
				}
			}

			// The board should have the expected number of pieces
			// (starting with 4 pieces, each move adds 1)
			expectedPieces := 4 + moveCount
			actualPieces := board.Position().CountDiscs()
			require.Equal(t, expectedPieces, actualPieces)
		})
	}
}

func TestGetBoard(t *testing.T) {
	// Test board with moves
	game := &Game{
		moves: []int{44, 29},
		start: NewBoardStart(),
	}

	tests := []struct {
		name      string
		moveIndex int
		expected  int // expected number of pieces on the board
	}{
		{
			name:      "at game start",
			moveIndex: 0,
			expected:  4,
		},
		{
			name:      "after first move",
			moveIndex: 1,
			expected:  5,
		},
		{
			name:      "after second move",
			moveIndex: 2,
			expected:  6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := game.getBoard(tt.moveIndex)

			require.NotNil(t, board)
			require.Equal(t, tt.expected, board.Position().CountDiscs())
		})
	}
}

func TestPushMove(t *testing.T) {
	tests := []struct {
		name         string
		initialMoves []int
		move         int
		expectError  bool
		errorMsg     string
		expectedLen  int
	}{
		{
			name:         "valid first move",
			initialMoves: []int{},
			move:         44,
			expectError:  false,
			expectedLen:  1,
		},
		{
			name:         "valid subsequent move",
			initialMoves: []int{44},
			move:         29,
			expectError:  false,
			expectedLen:  2,
		},
		{
			name:         "invalid move",
			initialMoves: []int{44},
			move:         0,
			expectError:  true,
			errorMsg:     "invalid move: 0",
			expectedLen:  1,
		},
		{
			name:         "pass move after pass",
			initialMoves: []int{44, PassMove},
			move:         PassMove,
			expectError:  false,
			expectedLen:  2, // Should not add another pass
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := NewGame()
			game.moves = tt.initialMoves

			err := game.PushMove(tt.move)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				require.Len(t, game.moves, tt.expectedLen)
			} else {
				require.NoError(t, err)
				require.Len(t, game.moves, tt.expectedLen)
			}
		})
	}
}

func TestPopMove(t *testing.T) {
	tests := []struct {
		name         string
		initialMoves []int
		expectedLen  int
	}{
		{
			name:         "empty game",
			initialMoves: []int{},
			expectedLen:  0,
		},
		{
			name:         "single move",
			initialMoves: []int{44},
			expectedLen:  0,
		},
		{
			name:         "multiple moves",
			initialMoves: []int{44, 29, 20},
			expectedLen:  2,
		},
		{
			name:         "with earlier pass moves",
			initialMoves: []int{44, 29, PassMove, 20},
			expectedLen:  3,
		},
		{
			name:         "with pass move",
			initialMoves: []int{44, 29, PassMove},
			expectedLen:  1, // Should remove the pass move and the move before it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := NewGame()
			game.moves = tt.initialMoves

			game.PopMove()

			require.Len(t, game.moves, tt.expectedLen)
		})
	}
}

func TestGetFilename(t *testing.T) {
	tests := []struct {
		name     string
		game     *Game
		expected string
	}{
		{
			name:     "empty filename",
			game:     NewGame(),
			expected: "",
		},
		{
			name: "with filename",
			game: &Game{
				filename: "test.pgn",
			},
			expected: "test.pgn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.game.GetFilename()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGameMoveSequence(t *testing.T) {
	// Test a complete sequence of moves and undos
	game := NewGame()

	// Add some moves
	moves := []int{44, 29, 20, 43}
	for _, move := range moves {
		err := game.PushMove(move)
		require.NoError(t, err)
	}

	require.Len(t, game.moves, len(moves))

	// Verify board state
	board := game.GetBoard()
	require.Equal(t, 4+len(moves), board.Position().CountDiscs())

	// Undo moves one by one
	for i := len(moves) - 1; i >= 0; i-- {
		game.PopMove()
		require.Len(t, game.moves, i)

		if i > 0 {
			lastBoard := game.GetBoard()
			require.Equal(t, 4+i, lastBoard.Position().CountDiscs())
		}
	}

	// Should be back to empty
	require.Empty(t, game.moves)
	board = game.GetBoard()
	require.Equal(t, 4, board.Position().CountDiscs()) // Initial position
}
