package othello

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParsePGNFile_Samples(t *testing.T) {
	loc := pgnLocation

	tests := []struct {
		filename string
		metadata GameMetadata
		moves    []int
	}{
		{
			filename: "testdata/pgn/flyordie_2025.pgn",
			metadata: GameMetadata{
				IsXot:  false,
				Date:   time.Date(2025, 1, 30, 0, 0, 0, 0, loc),
				Site:   "www.flyordie.com",
				Winner: newColor(White),
				Players: [2]Player{
					{Name: "ozi28", Rating: 618},
					{Name: "LK16", Rating: 591},
				},
			},
			moves: []int{
				44, 29, 20, 43, 34, 21, 42, 18, 26, 19, 10, 17, 11, 33, 37, 25, 38, 12,
				13, 5, 41, 2, 45, 22, 31, 23, 16, 24, 32, 30, 8, 39, 14, 46, 4, 3, 47,
				7, 9, 52, 6, 15, 61, 59, 53, 51, 54, 50, 60, 58, 57, 63, 55, 62,
			},
		},
		{
			filename: "testdata/pgn/playok_normal.pgn",
			metadata: GameMetadata{
				IsXot:  false,
				Date:   time.Date(2021, 3, 11, 20, 13, 30, 0, loc),
				Site:   "PlayOK",
				Winner: newColor(Black),
				Players: [2]Player{
					{Name: "alcupone", Rating: 1224},
					{Name: "lk16", Rating: 1314},
				},
			},
			moves: []int{
				37, 43, 26, 19, 18, 29, 34, 17, 10, 20, 11, 42, 25, 33, 13, 12, 21, 2, 4,
				3, 50, 5, 41, 58, 32, 24, 16, 51, 44, 6, 40, 38, 39, 46, 30, 47, 22, 31,
				23, 52, 45, 15, 60, 59, 57, 49, 61, 53, 62, 9, 8, 14, 7, 54, 48, PassMove, 56,
				PassMove, 55, 63, 1, 0,
			},
		},
		{
			filename: "testdata/pgn/playok_xot.pgn",
			metadata: GameMetadata{
				IsXot:  true,
				Date:   time.Date(2021, 3, 11, 21, 2, 25, 0, loc),
				Site:   "PlayOK",
				Winner: newColor(Black),
				Players: [2]Player{
					{Name: "lk16", Rating: 1331},
					{Name: "oscareriksson", Rating: 1642},
				},
			},
			moves: []int{
				37, 43, 18, 21, 34, 44, 19, 38, 51, 50, 45, 29, 53, 46, 52, 60, 22, 59, 20,
				58, 39, 31, 23, 30, 61, 62, 47, 42, 41, 25, 26, 32, 16, 33, 48, 40, 24, 17,
				10, 11, 4, 2, 12, 9, 0, 8, 3, 5, 1, 13, 49, 54, 6, 14, 63, 55, 15, 7, 57,
				56,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			games, err := ParsePGNFile(tt.filename)
			require.NoError(t, err)
			require.Len(t, games, 1)

			game := games[0]
			require.Equal(t, tt.filename, game.Filename())
			require.Equal(t, &tt.metadata, game.Metadata())
			require.Equal(t, tt.moves, game.Moves())
		})
	}
}

func TestParsePGNFile_NotFound(t *testing.T) {
	_, err := ParsePGNFile("testdata/pgn/does-not-exist.pgn")
	require.Error(t, err)
}

func TestParsePGN_MultipleGames_ErrorInFirst(t *testing.T) {
	// The error must surface as soon as the second game's metadata block
	// starts, since that's what triggers parsing of the first (broken) one.
	broken := pgnBase(map[string]string{"Site": ""}, "1. e6 f4")
	valid := pgnBase(nil, "1. e6 f4")

	_, err := ParsePGN(broken+valid, "")
	require.Error(t, err)
}

func TestParsePGN_MultipleGames(t *testing.T) {
	a, err := os.ReadFile("testdata/pgn/playok_normal.pgn")
	require.NoError(t, err)
	b, err := os.ReadFile("testdata/pgn/playok_xot.pgn")
	require.NoError(t, err)

	games, err := ParsePGN(string(a)+string(b), "")
	require.NoError(t, err)
	require.Len(t, games, 2)

	require.False(t, games[0].Metadata().IsXot)
	require.True(t, games[1].Metadata().IsXot)
}

func TestParsePGN_FilenameTimeFallback(t *testing.T) {
	content := `[Site "test"]
[White "a"]
[Black "b"]
[Result "1-0"]
[WhiteElo "1"]
[BlackElo "1"]
[Date "2024.05.06"]

1. e6 f4
`

	games, err := ParsePGN(content, filepath.Join("archive", "18_00_55.pgn"))
	require.NoError(t, err)
	require.Len(t, games, 1)

	require.Equal(t, time.Date(2024, 5, 6, 18, 0, 55, 0, pgnLocation), games[0].Metadata().Date)
}

func pgnBase(overrides map[string]string, moves string) string {
	fields := map[string]string{
		"Site":     "test",
		"White":    "a",
		"Black":    "b",
		"Result":   "1-0",
		"WhiteElo": "1",
		"BlackElo": "1",
		"Date":     "2024.05.06",
		"Time":     "00:00:00",
	}
	for k, v := range overrides {
		if v == "" {
			delete(fields, k)
			continue
		}
		fields[k] = v
	}

	content := ""
	for _, key := range []string{"Site", "White", "Black", "Result", "WhiteElo", "BlackElo", "Date", "Time", "Variant"} {
		if v, ok := fields[key]; ok {
			content += "[" + key + " \"" + v + "\"]\n"
		}
	}
	content += "\n" + moves + "\n"
	return content
}

func TestParsePGN_Errors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "missing site",
			content: pgnBase(map[string]string{"Site": ""}, "1. e6 f4"),
		},
		{
			name:    "missing white",
			content: pgnBase(map[string]string{"White": ""}, "1. e6 f4"),
		},
		{
			name:    "missing black",
			content: pgnBase(map[string]string{"Black": ""}, "1. e6 f4"),
		},
		{
			name:    "missing white rating",
			content: pgnBase(map[string]string{"WhiteElo": ""}, "1. e6 f4"),
		},
		{
			name:    "missing black rating",
			content: pgnBase(map[string]string{"BlackElo": ""}, "1. e6 f4"),
		},
		{
			name:    "non-numeric rating",
			content: pgnBase(map[string]string{"WhiteElo": "not-a-number"}, "1. e6 f4"),
		},
		{
			name:    "missing date",
			content: pgnBase(map[string]string{"Date": ""}, "1. e6 f4"),
		},
		{
			name:    "malformed date",
			content: pgnBase(map[string]string{"Date": "not-a-date"}, "1. e6 f4"),
		},
		{
			name:    "unknown variant",
			content: pgnBase(map[string]string{"Variant": "bongcloud"}, "1. e6 f4"),
		},
		{
			name:    "invalid move token",
			content: pgnBase(nil, "1. z9 f4"),
		},
		{
			name:    "illegal move",
			content: pgnBase(nil, "1. e6 e6"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePGN(tt.content, "")
			require.Error(t, err)
		})
	}
}

func TestParsePGN_DrawResult(t *testing.T) {
	games, err := ParsePGN(pgnBase(map[string]string{"Result": "1-1"}, "1. e6 f4"), "")
	require.NoError(t, err)
	require.Len(t, games, 1)
	require.Nil(t, games[0].Metadata().Winner)
}

// TestParsePGN_UnknownWinner covers Result values seen in the wild that
// don't fit the "<black>-<white>" disc count format: absent entirely (some
// older exports), PGN's own draw notation, and a malformed/placeholder
// value questgames.net writes for at least one unfinished game. None of
// these should fail parsing, since nothing downstream relies on Winner.
func TestParsePGN_UnknownWinner(t *testing.T) {
	tests := []struct {
		name   string
		result string
	}{
		{name: "missing", result: ""},
		{name: "pgn draw notation", result: "1/2-1/2"},
		{name: "malformed", result: "-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			games, err := ParsePGN(pgnBase(map[string]string{"Result": tt.result}, "1. e6 f4"), "")
			require.NoError(t, err)
			require.Len(t, games, 1)
			require.Nil(t, games[0].Metadata().Winner)
		})
	}
}

// TestParsePGN_ProvisionalRating covers flyordie.com's convention of
// prefixing a provisional rating with "?" (e.g. "?144").
func TestParsePGN_ProvisionalRating(t *testing.T) {
	games, err := ParsePGN(pgnBase(map[string]string{"WhiteElo": "?144"}, "1. e6 f4"), "")
	require.NoError(t, err)
	require.Len(t, games, 1)
	require.Equal(t, 144, games[0].Metadata().Players[White].Rating)
}

// TestParsePGN_StarTerminator covers PGN's "*" result token, written for an
// unfinished or ongoing game: it must be skipped like the disc-count result
// tokens rather than parsed as a move (which would fail).
func TestParsePGN_StarTerminator(t *testing.T) {
	games, err := ParsePGN(pgnBase(nil, "1. e6 f4 *"), "")
	require.NoError(t, err)
	require.Len(t, games, 1)

	moves, err := ParseField("e6")
	require.NoError(t, err)
	f4, err := ParseField("f4")
	require.NoError(t, err)
	require.Equal(t, []int{moves, f4}, games[0].Moves())
}

// TestParsePGN_VariantCaseInsensitive covers XOT variant tags written in any
// case (e.g. "XOT", "Xot"): they must be recognized rather than rejected as an
// unknown variant.
func TestParsePGN_VariantCaseInsensitive(t *testing.T) {
	for _, variant := range []string{"xot", "XOT", "Xot"} {
		t.Run(variant, func(t *testing.T) {
			games, err := ParsePGN(pgnBase(map[string]string{"Variant": variant}, "1. e6 f4"), "")
			require.NoError(t, err)
			require.Len(t, games, 1)
			require.True(t, games[0].Metadata().IsXot)
		})
	}
}

func TestParsePGN_MalformedMetadataLine(t *testing.T) {
	_, err := ParsePGN("[not a valid metadata line]\n\n1. e6 f4\n", "")
	require.Error(t, err)
}

func TestParsePGN_Empty(t *testing.T) {
	games, err := ParsePGN("", "")
	require.NoError(t, err)
	require.Empty(t, games)
}
