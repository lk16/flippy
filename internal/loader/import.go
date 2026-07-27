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

// ImportPaths imports boards from files and/or folders. Each folder is
// searched recursively for *.wtb and *.pgn files; each individual file must
// itself have a .wtb or .pgn extension (see ResolvePaths). All games found
// are extracted and imported together, so a board reached from both a WTB
// and a PGN file is only counted once.
//
// progress, if non-nil, is called after each input file has been parsed,
// with the number of files parsed so far and the total number of files to
// parse; parsing files is the slow part of a large import, so this lets
// callers report progress before the (fast, single-shot) DB insert at the
// end.
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

// parseWTBFiles parses each file as a WTHOR (.wtb) archive. onFile, if
// non-nil, is called once per file, after it has been parsed.
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

// parsePGNFiles parses each file as one or more PGN games. onFile, if
// non-nil, is called once per file, after it has been parsed.
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
// sequence (e.g. "A3B4C5") and imports it.
func ImportOthelloQuestMoves(ctx context.Context, repo *db.Repository, moveString string) (int, error) {
	game, err := othello.ParseOthelloQuestMoves(moveString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse move string: %w", err)
	}

	return ImportGames(ctx, repo, []*othello.Game{game})
}
