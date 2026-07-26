package loader

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

func TestImportGames_AddsExtractedBoards(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	game, err := othello.NewGameFromMoves([]int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2})
	require.NoError(t, err)

	want := ExtractBoards([]*othello.Game{game})
	require.NotEmpty(t, want)

	count, err := ImportGames(ctx, repo, []*othello.Game{game})
	require.NoError(t, err)
	require.Equal(t, len(want), count)

	for _, board := range want {
		eval, err := repo.GetBoard(ctx, board.Board())
		require.NoError(t, err)
		require.Equal(t, db.Evaluation{}, eval)
	}
}

func TestImportGames_DoesNotOverwriteLearnedEvaluation(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board := testBoard(t, book.LeafDiscs)
	game := othello.NewGameWithStart(board.Board())

	_, err := ImportGames(ctx, repo, []*othello.Game{game})
	require.NoError(t, err)

	learned := db.Evaluation{Level: 24, Depth: 24, Confidence: 100, Score: 3}
	require.NoError(t, repo.SaveEvaluation(ctx, board, learned))

	_, err = ImportGames(ctx, repo, []*othello.Game{game})
	require.NoError(t, err)

	got, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, learned, got)
}

func TestImportOthelloQuestMoves_AddsBoards(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	count, err := ImportOthelloQuestMoves(ctx, repo, "f5d6c4d3c3f4c5b3c2e3d2c6b4b5f2e2")
	require.NoError(t, err)
	require.Positive(t, count)
}

func TestImportOthelloQuestMoves_InvalidMoveString(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	_, err := ImportOthelloQuestMoves(ctx, repo, "not-a-move-string")
	require.Error(t, err)
}

func TestImportPGNFiles_AddsBoards(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	pgn := `[Site "Test"]
[Date "2024.01.01"]
[Black "A"]
[White "B"]
[Result "34-30"]
[BlackElo "1000"]
[WhiteElo "1000"]

1. f5 d6 2. c4 d3 3. c3 f4 4. c5 b3 5. c2 e3 6. d2 c6 7. b4 b5 8. f2 e2
`
	path := filepath.Join(t.TempDir(), "game.pgn")
	require.NoError(t, os.WriteFile(path, []byte(pgn), 0o644))

	count, err := ImportPGNFiles(ctx, repo, []string{path})
	require.NoError(t, err)
	require.Positive(t, count)
}

func TestImportPGNFiles_MissingFile(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	_, err := ImportPGNFiles(ctx, repo, []string{"testdata/does-not-exist.pgn"})
	require.Error(t, err)
}

// encodeWTBRecord builds a single 68-byte WTHOR game record for moves,
// encoding each move as row*10+col with row/col 1-based.
func encodeWTBRecord(moves []int) []byte {
	const (
		recordSize = 68
		movesStart = 8
	)

	record := make([]byte, recordSize)
	for i, move := range moves {
		x, y := move%8, move/8
		record[movesStart+i] = byte((y+1)*10 + (x + 1))
	}

	return record
}

// encodeWTB builds a minimal WTHOR archive with one game per entry of
// gamesMoves, matching the format ParseWTB decodes.
func encodeWTB(gamesMoves [][]int) []byte {
	const headerSize = 16

	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(gamesMoves)))

	for _, moves := range gamesMoves {
		data = append(data, encodeWTBRecord(moves)...)
	}

	return data
}

func TestImportWTBFiles_AddsBoards(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	data := encodeWTB([][]int{{19, 18, 17, 9, 1, 0, 37, 43, 51, 2}})
	path := filepath.Join(t.TempDir(), "games.wtb")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	count, err := ImportWTBFiles(ctx, repo, []string{path})
	require.NoError(t, err)
	require.Positive(t, count)
}

func TestImportWTBFiles_MissingFile(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	_, err := ImportWTBFiles(ctx, repo, []string{"testdata/does-not-exist.wtb"})
	require.Error(t, err)
}
