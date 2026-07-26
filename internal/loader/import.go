package loader

import (
	"context"
	"fmt"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// ImportGames extracts NormalizedBoards from games (see ExtractBoards) and
// adds them to the DB, per AddBoards' own semantics: existing rows,
// including already-learned evaluations, are left untouched. It returns the
// number of distinct boards extracted.
func ImportGames(ctx context.Context, repo *db.Repository, games []*othello.Game) (int, error) {
	boards := ExtractBoards(games)

	if err := repo.AddBoards(ctx, boards); err != nil {
		return 0, fmt.Errorf("failed to add extracted boards: %w", err)
	}

	return len(boards), nil
}

// ImportWTBFiles parses each file as a WTHOR (.wtb) archive and imports the
// games it contains.
func ImportWTBFiles(ctx context.Context, repo *db.Repository, filenames []string) (int, error) {
	var games []*othello.Game

	for _, filename := range filenames {
		parsed, err := othello.ParseWTBFile(filename)
		if err != nil {
			return 0, fmt.Errorf("failed to parse %s: %w", filename, err)
		}
		games = append(games, parsed...)
	}

	return ImportGames(ctx, repo, games)
}

// ImportPGNFiles parses each file as one or more PGN games and imports them.
func ImportPGNFiles(ctx context.Context, repo *db.Repository, filenames []string) (int, error) {
	var games []*othello.Game

	for _, filename := range filenames {
		parsed, err := othello.ParsePGNFile(filename)
		if err != nil {
			return 0, fmt.Errorf("failed to parse %s: %w", filename, err)
		}
		games = append(games, parsed...)
	}

	return ImportGames(ctx, repo, games)
}

// ImportOthelloQuestMoves parses moveString as a single Othello Quest move
// sequence (e.g. "A3B4C5") and imports it.
func ImportOthelloQuestMoves(ctx context.Context, repo *db.Repository, moveString string) (int, error) {
	game, err := othello.ParseOthelloQuestMoves(moveString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse move string: %w", err)
	}

	return ImportGames(ctx, repo, []*othello.Game{game})
}
