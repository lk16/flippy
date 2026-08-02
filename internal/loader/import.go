package loader

import (
	"context"
	"fmt"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// ImportGames extracts NormalizedBoards from games and adds them to the DB, returning the number of
// rows actually inserted (boards already present are not counted).
func ImportGames(ctx context.Context, repo *db.Repository, games []*othello.Game) (int, error) {
	boards := ExtractBoards(games)

	added, err := repo.AddBoardsInserted(ctx, boards)
	if err != nil {
		return 0, fmt.Errorf("failed to add extracted boards: %w", err)
	}

	return added, nil
}

// ImportWTBFiles parses each file as a WTHOR (.wtb) archive and imports the
// games it contains.
func ImportWTBFiles(ctx context.Context, repo *db.Repository, filenames []string) (int, error) {
	games, err := parseWTBFiles(filenames, nil)
	if err != nil {
		return 0, err
	}

	return ImportGames(ctx, repo, games)
}

// ImportPGNFiles parses each file as one or more PGN games and imports them.
func ImportPGNFiles(ctx context.Context, repo *db.Repository, filenames []string) (int, error) {
	games, err := parsePGNFiles(filenames, nil)
	if err != nil {
		return 0, err
	}

	return ImportGames(ctx, repo, games)
}

// ImportPaths imports boards from files and/or folders, searching folders recursively for *.wtb/*.pgn
// files. progress, if non-nil, is called after each input file is parsed with (done, total).
func ImportPaths(ctx context.Context, repo *db.Repository, paths []string, progress func(done, total int)) (int, error) {
	wtbFiles, pgnFiles, err := ResolvePaths(paths)
	if err != nil {
		return 0, err
	}

	total := len(wtbFiles) + len(pgnFiles)
	done := 0

	onFile := func() {
		done++
		if progress != nil {
			progress(done, total)
		}
	}

	wtbGames, err := parseWTBFiles(wtbFiles, onFile)
	if err != nil {
		return 0, err
	}

	pgnGames, err := parsePGNFiles(pgnFiles, onFile)
	if err != nil {
		return 0, err
	}

	games := make([]*othello.Game, 0, len(wtbGames)+len(pgnGames))
	games = append(games, wtbGames...)
	games = append(games, pgnGames...)

	return ImportGames(ctx, repo, games)
}

// parseWTBFiles parses each file as a WTHOR (.wtb) archive, calling onFile once per file if non-nil.
func parseWTBFiles(filenames []string, onFile func()) ([]*othello.Game, error) {
	var games []*othello.Game

	for _, filename := range filenames {
		parsed, err := othello.ParseWTBFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
		}
		games = append(games, parsed...)
		if onFile != nil {
			onFile()
		}
	}

	return games, nil
}

// parsePGNFiles parses each file as one or more PGN games, calling onFile once per file if non-nil.
func parsePGNFiles(filenames []string, onFile func()) ([]*othello.Game, error) {
	var games []*othello.Game

	for _, filename := range filenames {
		parsed, err := othello.ParsePGNFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
		}
		games = append(games, parsed...)
		if onFile != nil {
			onFile()
		}
	}

	return games, nil
}

// ImportOthelloQuestMoves parses moveString as a single Othello Quest move
// sequence (e.g. "f5d6c5") and imports it.
func ImportOthelloQuestMoves(ctx context.Context, repo *db.Repository, moveString string) (int, error) {
	game, err := othello.ParseOthelloQuestMoves(moveString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse move string: %w", err)
	}

	return ImportGames(ctx, repo, []*othello.Game{game})
}
