package othello

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	pgnMetadataLineRegex = regexp.MustCompile(`\[(.*) "(.*)"\]`)
	pgnFilenameTimeRegex = regexp.MustCompile(`(\d{2}_\d{2}_\d{2})\.pgn$`)
)

// pgnLocation is the timezone PGN dates/times are assumed to be in.
var pgnLocation *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		panic(fmt.Sprintf("failed to load Europe/Stockholm timezone: %v", err))
	}
	pgnLocation = loc
}

// Player is one side of a PGN game.
type Player struct {
	Name   string
	Rating int
}

// GameMetadata is the metadata of a game loaded from a PGN file.
type GameMetadata struct {
	// IsXot is true if this is an XOT variant game.
	IsXot bool

	// Site is the site where the game was played.
	Site string

	// Date is the date and time the game was played.
	Date time.Time

	// Players describes the players, indexed by Color.
	Players [2]Player

	// Winner is the color of the winner, or nil for a draw.
	Winner *Color
}

func parsePGNMetadata(lines []string, filename string) (*GameMetadata, error) {
	fields := make(map[string]string, len(lines))

	for _, line := range lines {
		matches := pgnMetadataLineRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			return nil, fmt.Errorf("could not parse PGN metadata line: %s", line)
		}

		fields[matches[1]] = matches[2]
	}

	parser := pgnMetadataParser{fields: fields, filename: filename}
	return parser.parse()
}

// pgnMetadataParser parses the fields of a single PGN game's metadata block.
type pgnMetadataParser struct {
	filename string
	fields   map[string]string
}

func (p pgnMetadataParser) parse() (*GameMetadata, error) {
	metadata := &GameMetadata{}

	var ok bool

	metadata.Site, ok = p.fields["Site"]
	if !ok {
		return nil, errors.New("missing field Site in metadata")
	}

	metadata.Players[White].Name, ok = p.fields["White"]
	if !ok {
		return nil, errors.New("missing field White in metadata")
	}

	metadata.Players[Black].Name, ok = p.fields["Black"]
	if !ok {
		return nil, errors.New("missing field Black in metadata")
	}

	var err error

	metadata.IsXot, err = p.parseVariant()
	if err != nil {
		return nil, fmt.Errorf("failed to parse variant: %w", err)
	}

	metadata.Players[White].Rating, err = p.parseRating(White)
	if err != nil {
		return nil, fmt.Errorf("failed to parse white rating: %w", err)
	}

	metadata.Players[Black].Rating, err = p.parseRating(Black)
	if err != nil {
		return nil, fmt.Errorf("failed to parse black rating: %w", err)
	}

	metadata.Date, err = p.parseDate()
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}

	metadata.Winner = p.parseWinner()

	return metadata, nil
}

func (p pgnMetadataParser) parseDate() (time.Time, error) {
	dateString, ok := p.fields["Date"]
	if !ok {
		return time.Time{}, errors.New("missing field Date in metadata")
	}

	timeString, ok := p.fields["Time"]
	if !ok {
		if matches := pgnFilenameTimeRegex.FindStringSubmatch(p.filename); len(matches) > 0 {
			timeString = strings.ReplaceAll(matches[1], "_", ":")
		} else {
			timeString = "00:00:00"
		}
	}

	date, err := time.ParseInLocation("2006.01.02 15:04:05", dateString+" "+timeString, pgnLocation)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse date: %w", err)
	}

	return date, nil
}

// parseWinner reads the Result field, if present, as a "<black>-<white>"
// disc count and returns the color with more discs, or nil for a draw.
// Result is missing from some older exports, and some sources (e.g.
// questgames.net) write non-numeric or otherwise malformed values for
// unfinished games (e.g. "1/2-1/2", "-2"); since nothing downstream relies
// on Winner, any of that is treated as "unknown" rather than a parse error.
func (p pgnMetadataParser) parseWinner() *Color {
	result, ok := p.fields["Result"]
	if !ok {
		return nil
	}

	var black, white int
	if _, err := fmt.Sscanf(result, "%d-%d", &black, &white); err != nil {
		return nil
	}

	switch {
	case black > white:
		return newColor(Black)
	case white > black:
		return newColor(White)
	default:
		return nil
	}
}

func (p pgnMetadataParser) parseRating(color Color) (int, error) {
	prefix := "Black"
	if color == White {
		prefix = "White"
	}

	ratingField := prefix + "Rating"
	eloField := prefix + "Elo"

	ratingString, ok := p.fields[ratingField]
	if !ok {
		ratingString, ok = p.fields[eloField]
		if !ok {
			return 0, fmt.Errorf("missing field %s or %s in metadata", ratingField, eloField)
		}
	}

	// Some sources (e.g. flyordie.com) prefix a provisional rating with "?".
	ratingString = strings.TrimPrefix(ratingString, "?")

	rating, err := strconv.Atoi(ratingString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s rating: %w", prefix, err)
	}

	return rating, nil
}

func (p pgnMetadataParser) parseVariant() (bool, error) {
	variant, ok := p.fields["Variant"]
	if !ok {
		return false, nil
	}

	if variant == "xot" {
		return true, nil
	}

	return false, fmt.Errorf("unknown variant: %s", variant)
}
