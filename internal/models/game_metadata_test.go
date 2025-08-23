package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		filename string
		want     *GameMetadata
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid normal game metadata",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[Time "14:30:00"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "1500"]`,
				`[BlackRating "1600"]`,
				`[Result "0-1"]`,
			},
			filename: "test.pgn",
			want: &GameMetadata{
				IsXot:   false,
				Site:    "Test Site",
				Date:    time.Date(2024, 1, 15, 14, 30, 0, 0, mustLoadLocation(t)),
				Players: [2]Player{{Name: "Player2", Rating: 1600}, {Name: "Player1", Rating: 1500}},
				Winner:  WHITE,
			},
			wantErr: false,
		},
		{
			name: "valid XOT variant game metadata",
			lines: []string{
				`[Site "XOT Site"]`,
				`[Date "2024.02.20"]`,
				`[Time "16:45:00"]`,
				`[White "XOTPlayer1"]`,
				`[Black "XOTPlayer2"]`,
				`[WhiteElo "1800"]`,
				`[BlackElo "1700"]`,
				`[Result "1-0"]`,
				`[Variant "xot"]`,
			},
			filename: "xot_game.pgn",
			want: &GameMetadata{
				IsXot:   true,
				Site:    "XOT Site",
				Date:    time.Date(2024, 2, 20, 16, 45, 0, 0, mustLoadLocation(t)),
				Players: [2]Player{{Name: "XOTPlayer2", Rating: 1700}, {Name: "XOTPlayer1", Rating: 1800}},
				Winner:  BLACK,
			},
			wantErr: false,
		},
		{
			name: "draw game metadata",
			lines: []string{
				`[Site "Draw Site"]`,
				`[Date "2024.03.10"]`,
				`[White "DrawPlayer1"]`,
				`[Black "DrawPlayer2"]`,
				`[WhiteRating "1550"]`,
				`[BlackRating "1550"]`,
				`[Result "1-1"]`,
			},
			filename: "draw_game.pgn",
			want: &GameMetadata{
				IsXot:   false,
				Site:    "Draw Site",
				Date:    time.Date(2024, 3, 10, 0, 0, 0, 0, mustLoadLocation(t)),
				Players: [2]Player{{Name: "DrawPlayer2", Rating: 1550}, {Name: "DrawPlayer1", Rating: 1550}},
				Winner:  DRAW,
			},
			wantErr: false,
		},
		{
			name: "metadata with filename date extraction",
			lines: []string{
				`[Site "Filename Date Site"]`,
				`[Date "2024.04.25"]`,
				`[White "FilenamePlayer1"]`,
				`[Black "FilenamePlayer2"]`,
				`[WhiteRating "1650"]`,
				`[BlackRating "1750"]`,
				`[Result "0-1"]`,
			},
			filename: "2024_04_25.pgn",
			wantErr:  true,
			errMsg:   "failed to parse date",
		},
		{
			name: "invalid metadata line format",
			lines: []string{
				`[Site "Test Site"]`,
				`[Invalid Line]`,
				`[White "Player1"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "could not parse PGN metadata: [Invalid Line]",
		},
		{
			name: "missing Site field",
			lines: []string{
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field Site in metadata",
		},
		{
			name: "missing White field",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[Black "Player2"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field White in metadata",
		},
		{
			name: "missing Black field",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field Black in metadata",
		},
		{
			name: "missing Date field",
			lines: []string{
				`[Site "Test Site"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "1500"]`,
				`[BlackRating "1600"]`,
				`[Result "1-0"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field Date in metadata",
		},
		{
			name: "missing Result field",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "1500"]`,
				`[BlackRating "1600"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field Result in metadata",
		},
		{
			name: "missing rating fields",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[Result "1-0"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field WhiteRating or WhiteElo in metadata",
		},
		{
			name: "invalid date format",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "invalid-date"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "1500"]`,
				`[BlackRating "1600"]`,
				`[Result "1-0"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "failed to parse date",
		},
		{
			name: "invalid result format",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "1500"]`,
				`[BlackRating "1600"]`,
				`[Result "invalid"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "failed to parse result",
		},
		{
			name: "invalid rating format",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "invalid"]`,
				`[BlackRating "1600"]`,
				`[Result "1-0"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "failed to parse White rating",
		},
		{
			name: "unknown variant",
			lines: []string{
				`[Site "Test Site"]`,
				`[Date "2024.01.15"]`,
				`[White "Player1"]`,
				`[Black "Player2"]`,
				`[WhiteRating "1500"]`,
				`[BlackRating "1600"]`,
				`[Result "1-0"]`,
				`[Variant "unknown"]`,
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "unknown variant: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMetadata(tt.lines, tt.filename)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)

			// Compare fields individually to handle time comparison properly
			assert.Equal(t, tt.want.IsXot, got.IsXot)
			assert.Equal(t, tt.want.Site, got.Site)
			assert.Equal(t, tt.want.Players, got.Players)
			assert.Equal(t, tt.want.Winner, got.Winner)

			// Compare dates with tolerance for timezone differences
			if !tt.want.Date.IsZero() {
				assert.Less(t, got.Date.Sub(tt.want.Date), time.Second,
					"Date difference too large: got %v, want %v", got.Date, tt.want.Date)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		filename string
		want     time.Time
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid date with time",
			metadata: map[string]string{
				"Date": "2024.01.15",
				"Time": "14:30:00",
			},
			filename: "test.pgn",
			want:     time.Date(2024, 1, 15, 14, 30, 0, 0, mustLoadLocation(t)),
			wantErr:  false,
		},
		{
			name: "valid date without time, using filename",
			metadata: map[string]string{
				"Date": "2024.02.20",
			},
			filename: "2024_02_20.pgn",
			wantErr:  true,
			errMsg:   "failed to parse date",
		},
		{
			name: "valid date without time, no filename pattern",
			metadata: map[string]string{
				"Date": "2024.03.10",
			},
			filename: "test.pgn",
			want:     time.Date(2024, 3, 10, 0, 0, 0, 0, mustLoadLocation(t)),
			wantErr:  false,
		},
		{
			name: "missing date field",
			metadata: map[string]string{
				"Site": "Test Site",
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "missing field Date in metadata",
		},
		{
			name: "invalid date format",
			metadata: map[string]string{
				"Date": "invalid-date",
			},
			filename: "test.pgn",
			wantErr:  true,
			errMsg:   "failed to parse date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := metadataParser{
				metadata: tt.metadata,
				filename: tt.filename,
			}

			got, err := parser.parseDate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Less(t, got.Sub(tt.want), time.Second,
				"Date difference too large: got %v, want %v", got, tt.want)
		})
	}
}

func TestGetWinner(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     int
		wantErr  bool
		errMsg   string
	}{
		{
			name: "black wins",
			metadata: map[string]string{
				"Result": "1-0",
			},
			want:    BLACK,
			wantErr: false,
		},
		{
			name: "white wins",
			metadata: map[string]string{
				"Result": "0-1",
			},
			want:    WHITE,
			wantErr: false,
		},
		{
			name: "draw",
			metadata: map[string]string{
				"Result": "1-1",
			},
			want:    DRAW,
			wantErr: false,
		},
		{
			name: "draw with zero",
			metadata: map[string]string{
				"Result": "0-0",
			},
			want:    DRAW,
			wantErr: false,
		},
		{
			name: "missing result field",
			metadata: map[string]string{
				"Site": "Test Site",
			},
			wantErr: true,
			errMsg:  "missing field Result in metadata",
		},
		{
			name: "invalid result format",
			metadata: map[string]string{
				"Result": "invalid",
			},
			wantErr: true,
			errMsg:  "failed to parse result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := metadataParser{
				metadata: tt.metadata,
			}

			got, err := parser.getWinner()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRating(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		color    int
		want     int
		wantErr  bool
		errMsg   string
	}{
		{
			name: "white rating with Rating field",
			metadata: map[string]string{
				"WhiteRating": "1500",
			},
			color:   WHITE,
			want:    1500,
			wantErr: false,
		},
		{
			name: "white rating with Elo field",
			metadata: map[string]string{
				"WhiteElo": "1600",
			},
			color:   WHITE,
			want:    1600,
			wantErr: false,
		},
		{
			name: "black rating with Rating field",
			metadata: map[string]string{
				"BlackRating": "1700",
			},
			color:   BLACK,
			want:    1700,
			wantErr: false,
		},
		{
			name: "black rating with Elo field",
			metadata: map[string]string{
				"BlackElo": "1800",
			},
			color:   BLACK,
			want:    1800,
			wantErr: false,
		},
		{
			name: "missing rating fields",
			metadata: map[string]string{
				"Site": "Test Site",
			},
			color:   WHITE,
			wantErr: true,
			errMsg:  "missing field WhiteRating or WhiteElo in metadata",
		},
		{
			name: "invalid rating format",
			metadata: map[string]string{
				"WhiteRating": "invalid",
			},
			color:   WHITE,
			wantErr: true,
			errMsg:  "failed to parse White rating",
		},
		{
			name: "invalid color",
			metadata: map[string]string{
				"WhiteRating": "1500",
			},
			color:   999,
			wantErr: true,
			errMsg:  "invalid color: 999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := metadataParser{
				metadata: tt.metadata,
			}

			got, err := parser.parseRating(tt.color)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseVariant(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
		wantErr  bool
		errMsg   string
	}{
		{
			name: "XOT variant",
			metadata: map[string]string{
				"Variant": "xot",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "no variant field (normal game)",
			metadata: map[string]string{
				"Site": "Test Site",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "unknown variant",
			metadata: map[string]string{
				"Variant": "unknown",
			},
			wantErr: true,
			errMsg:  "unknown variant: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := metadataParser{
				metadata: tt.metadata,
			}

			got, err := parser.parseVariant()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Helper function to load the Stockholm timezone for testing.
func mustLoadLocation(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/Stockholm")
	require.NoError(t, err)
	return loc
}
