package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const samplesDir = "pgn_samples"

func TestMetadataParsing(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	expectedMetadata := map[string]string{
		"Event":       "Online game",
		"Site":        "www.flyordie.com",
		"White":       "LK16",
		"Black":       "ozi28",
		"Result":      "0-1",
		"Date":        "2025.01.30",
		"Round":       "2",
		"WhiteRating": "591",
		"WhiteTime":   "R0:07:43",
		"BlackRating": "618",
		"BlackTime":   "R0:07:36",
		"Termination": "normal",
		"TimeControl": "600",
		"UTCDate":     "2025.01.30",
	}

	for key, expectedValue := range expectedMetadata {
		actualValue, exists := game.Metadata[key]
		require.True(t, exists, "Missing metadata key: %s", key)
		require.Equal(t, expectedValue, actualValue, "Metadata %s mismatch", key)
	}
}

func TestGetDate(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	expectedDate := time.Date(2025, 1, 30, 0, 0, 0, 0, time.UTC)
	actualDate, err := game.GetDate()
	require.NoError(t, err, "Failed to get date")

	require.Equal(t, expectedDate, actualDate)
}

func TestGetDateTime(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	expectedDateTime := time.Date(2021, 3, 11, 20, 13, 30, 0, time.UTC)
	actualDateTime, err := game.GetDateTime()
	require.NoError(t, err, "Failed to get datetime")

	require.Equal(t, expectedDateTime, actualDateTime)
}

func TestGetDateTimeMissing(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	// flyordie.pgn has no Time field, so GetDateTime should return just the date
	expectedDate := time.Date(2025, 1, 30, 0, 0, 0, 0, time.UTC)
	actualDateTime, err := game.GetDateTime()
	require.NoError(t, err, "Failed to get datetime")

	require.Equal(t, expectedDate, actualDateTime)
}

func TestGetPlayers(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	expectedWhite := "lk16"
	expectedBlack := "alcupone"

	require.Equal(t, expectedWhite, game.GetWhitePlayer())
	require.Equal(t, expectedBlack, game.GetBlackPlayer())
}

func TestGetWinner(t *testing.T) {
	playokGame, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	flyordieGame, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	// playok_normal.pgn: 47-17 result means black won
	winner, err := playokGame.GetWinner()
	require.NoError(t, err, "Failed to get winner")
	require.Equal(t, BLACK, winner, "Expected black winner")

	// flyordie.pgn: 0-1 result means white won
	winner, err = flyordieGame.GetWinner()
	require.NoError(t, err, "Failed to get winner")
	require.Equal(t, WHITE, winner, "Expected white winner")
}

func TestGetBlackScore(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	// 47-17 result means black won by 30 points
	expectedScore := 30
	actualScore, err := game.GetBlackScore()
	require.NoError(t, err, "Failed to get black score")

	require.Equal(t, expectedScore, actualScore)
}

func TestGetColor(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	// Test valid players
	color, err := game.GetColor("lk16")
	require.NoError(t, err, "Failed to get color for lk16")
	require.Equal(t, WHITE, color, "Expected lk16 to be white")

	color, err = game.GetColor("alcupone")
	require.NoError(t, err, "Failed to get color for alcupone")
	require.Equal(t, BLACK, color, "Expected alcupone to be black")

	// Test unknown player
	_, err = game.GetColor("unknown")
	require.Error(t, err, "Expected error for unknown player")
}

func TestGetColorAny(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	// Test with valid player in list
	color, err := game.GetColorAny([]string{"unknown", "lk16"})
	require.NoError(t, err, "Failed to get color for any of [unknown, lk16]")
	require.Equal(t, WHITE, color, "Expected lk16 to be white")

	// Test with no valid players
	_, err = game.GetColorAny([]string{"unknown"})
	require.Error(t, err, "Expected error for no valid players")
}

func TestIsXOT(t *testing.T) {
	xotGame, err := NewGameFromPGN(samplesDir + "/playok_xot.pgn")
	require.NoError(t, err, "Failed to load playok_xot.pgn")

	normalGame, err := NewGameFromPGN(samplesDir + "/playok_normal.pgn")
	require.NoError(t, err, "Failed to load playok_normal.pgn")

	require.True(t, xotGame.IsXOT(), "Expected playok_xot.pgn to be XOT variant")
	require.False(t, normalGame.IsXOT(), "Expected playok_normal.pgn to not be XOT variant")
}

func TestMovesParsing(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	// Test first few moves from flyordie.pgn
	expectedStart := []int{
		FieldToIndexMust("e6"),
		FieldToIndexMust("f4"),
		FieldToIndexMust("e3"),
		FieldToIndexMust("d6"),
	}

	require.GreaterOrEqual(t, len(game.Moves), 4, "Expected at least 4 moves")

	for i, expectedMove := range expectedStart {
		require.Equal(t, expectedMove, game.Moves[i], "Move %d mismatch", i)
	}
}

func TestBoardCount(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	// Number of boards should be number of moves + 1 (starting position)
	expectedBoardCount := len(game.Moves) + 1
	actualBoardCount := len(game.Boards)

	require.Equal(t, expectedBoardCount, actualBoardCount)
}

func TestZipBoardMoves(t *testing.T) {
	game, err := NewGameFromPGN(samplesDir + "/flyordie.pgn")
	require.NoError(t, err, "Failed to load flyordie.pgn")

	// Test that boards and moves can be zipped together
	pairs := game.ZipBoardMoves()
	movesCount := len(pairs)

	require.Equal(t, len(game.Moves), movesCount, "Expected %d board-move pairs", len(game.Moves))

	for i, pair := range pairs {
		require.True(t, pair.Board.IsValidMove(pair.Move), "Move %d (%d) is not valid for board %d", i, pair.Move, i)
	}
}

// Helper function to convert field to index without error handling for tests.
func FieldToIndexMust(field string) int {
	index, err := FieldToIndex(field)
	if err != nil {
		panic(err)
	}
	return index
}
