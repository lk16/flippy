package models

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	metadataRegex = regexp.MustCompile(`\[(.*) "(.*)"\]`)
	dateRegex     = regexp.MustCompile(`(\d{4}_\d{2}_\d{2})\.pgn$`)
)

type Player struct {
	Name   string
	Rating int
}

// GameMetadata represents the metadata of a game.
type GameMetadata struct {
	// IsXot is true if this is an XOT variant game
	IsXot bool

	// Site is the site where the game was played
	Site string

	// Date is the date when the game was played
	Date time.Time

	// Players describes players indexed by color (WHITE/BLACK)
	Players [2]Player

	// Winner is the color of the winner (WHITE/BLACK), or EMPTY for a draw
	Winner int
}

func parseMetadata(lines []string, filename string) (*GameMetadata, error) {
	metadata := make(map[string]string)

	// Load metadata into a map
	for _, line := range lines {
		matches := metadataRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			return nil, fmt.Errorf("could not parse PGN metadata: %s", line)
		}

		key := matches[1]
		value := matches[2]
		metadata[key] = value
	}

	parser := metadataParser{
		metadata: metadata,
		filename: filename,
	}

	return parser.parse()
}

// metadataParser is a parser for PGN metadata.
type metadataParser struct {
	filename string
	metadata map[string]string
}

func (p metadataParser) parse() (*GameMetadata, error) {
	metadata := &GameMetadata{}

	var ok bool
	metadata.Site, ok = p.metadata["Site"]
	if !ok {
		return nil, errors.New("missing field Site in metadata")
	}

	metadata.Players[WHITE].Name, ok = p.metadata["White"]
	if !ok {
		return nil, errors.New("missing field White in metadata")
	}

	metadata.Players[BLACK].Name, ok = p.metadata["Black"]
	if !ok {
		return nil, errors.New("missing field Black in metadata")
	}

	var err error
	metadata.IsXot, err = p.parseVariant()
	if err != nil {
		return nil, fmt.Errorf("failed to parse variant: %w", err)
	}

	metadata.Players[WHITE].Rating, err = p.parseRating(WHITE)
	if err != nil {
		return nil, fmt.Errorf("failed to parse white rating: %w", err)
	}

	metadata.Players[BLACK].Rating, err = p.parseRating(BLACK)
	if err != nil {
		return nil, fmt.Errorf("failed to parse black rating: %w", err)
	}

	metadata.Date, err = p.parseDate()
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}

	metadata.Winner, err = p.getWinner()
	if err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return metadata, nil
}

func (p metadataParser) parseDate() (time.Time, error) {
	var ok bool
	var dateString string

	if dateString, ok = p.metadata["Date"]; !ok {
		return time.Time{}, errors.New("missing field Date in metadata")
	}

	var timeString string
	if timeString, ok = p.metadata["Time"]; !ok {
		if matches := dateRegex.FindStringSubmatch(p.filename); len(matches) > 0 {
			timeString = strings.ReplaceAll(matches[0], "_", ":")
		} else {
			timeString = "00:00:00"
		}
	}

	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load CET timezone: %w", err)
	}
	date, err := time.ParseInLocation("2006.01.02 15:04:05", dateString+" "+timeString, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse date: %w", err)
	}

	return date, nil
}

func (p metadataParser) getWinner() (int, error) {
	result, ok := p.metadata["Result"]
	if !ok {
		return 0, errors.New("missing field Result in metadata")
	}

	var black, white int
	_, err := fmt.Sscanf(result, "%d-%d", &black, &white)
	if err != nil {
		return 0, fmt.Errorf("failed to parse result: %w", err)
	}

	switch {
	case black > white:
		return BLACK, nil
	case white > black:
		return WHITE, nil
	default:
		return DRAW, nil
	}
}

func (p metadataParser) parseRating(color int) (int, error) {
	var prefix string
	switch color {
	case WHITE:
		prefix = "White"
	case BLACK:
		prefix = "Black"
	default:
		return 0, fmt.Errorf("invalid color: %d", color)
	}

	ratingField := prefix + "Rating"
	eloField := prefix + "Elo"

	ratingString, ok := p.metadata[ratingField]
	if !ok {
		ratingString, ok = p.metadata[eloField]
		if !ok {
			return 0, fmt.Errorf("missing field %s or %s in metadata", ratingField, eloField)
		}
	}
	rating, err := strconv.Atoi(ratingString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s rating: %w", prefix, err)
	}

	return rating, nil
}

func (p metadataParser) parseVariant() (bool, error) {
	variant, ok := p.metadata["Variant"]
	if !ok {
		// If variant is missing, assume normal game
		return false, nil
	}

	switch variant {
	case "xot":
		return true, nil
	default:
		return false, fmt.Errorf("unknown variant: %s", variant)
	}
}
