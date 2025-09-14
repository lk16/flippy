package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	learnChunkSize = 100
	pgnJSONPath    = ".flippy/pgn.json"
)

type PgnState struct {
	LastReadPgn string `json:"last_read_pgn"`
}

func main() {
	// TODO use log instead of slog everywhere

	config.SetLogLevel()

	// Load configuration
	cfg := config.LoadLearnClientConfig()

	// Create API client
	client, err := book.NewAPIClient(cfg)
	if err != nil {
		slog.Error("Failed to create API client", "error", err)
		os.Exit(1)
	}

	// Create Edax manager
	resultChan := make(chan edax.Result)
	edaxManager := edax.NewProcess(resultChan)

	// Load PGN files
	if err = loadPgn(client, edaxManager); err != nil {
		slog.Error("Failed to load PGN", "error", err)
		os.Exit(1)
	}
}

func loadPgn(client *book.APIClient, edaxManager *edax.Process) error {
	// Get PGN target folder from environment
	targetFolder := os.Getenv("FLIPPY_PGN_TARGET_FOLDER")
	if targetFolder == "" {
		return errors.New("FLIPPY_PGN_TARGET_FOLDER environment variable not set")
	}

	targetFolder = strings.ReplaceAll(
		targetFolder,
		"PROJECT_ROOT",
		filepath.Dir(filepath.Dir(filepath.Dir(targetFolder))),
	)

	// Construct path to normal PGN files
	normalFolder := filepath.Join(targetFolder, "normal")

	// Get all PGN files
	pgnFiles, err := getPgnFiles(normalFolder)
	if err != nil {
		return fmt.Errorf("failed to get PGN files: %w", err)
	}

	// Load last read PGN state
	lastReadPgn, err := loadPgnState()
	if err != nil {
		slog.Warn("Failed to load PGN state", "error", err)
	}

	// Filter files to only process new ones
	if lastReadPgn != "" {
		pgnFiles = filterNewPgnFiles(pgnFiles, normalFolder, lastReadPgn)
	}

	if len(pgnFiles) == 0 {
		slog.Info("No new PGN files to load")
		return nil
	}

	// Extract positions from PGN files
	positions := make(map[models.NormalizedPosition]struct{})

	for i, file := range pgnFiles {
		var game *models.Game
		game, err = models.NewGameFromPGN(file)
		if err != nil {
			slog.Error("Failed to parse PGN file", "file", file, "error", err)
			continue
		}

		// Get all normalized positions from the game
		gamePositions := game.GetNormalizedPositionsWithChildren()
		for pos := range gamePositions {
			positions[pos] = struct{}{}
		}

		// Print progress
		percentage := 100.0 * float64(i+1) / float64(len(pgnFiles))
		slog.Info("Loading PGN files", "progress", fmt.Sprintf("%d/%d (%6.2f%%)", i+1, len(pgnFiles), percentage))
	}

	// Learn new positions
	if err = learnNewPositions(positions, client, edaxManager); err != nil {
		return fmt.Errorf("failed to learn new positions: %w", err)
	}

	// Save state
	if len(pgnFiles) > 0 {
		lastFile := pgnFiles[len(pgnFiles)-1]
		var relativePath string
		relativePath, err = filepath.Rel(normalFolder, lastFile)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		if err = savePgnState(relativePath); err != nil {
			return fmt.Errorf("failed to save PGN state: %w", err)
		}
	}

	return nil
}

func getPgnFiles(normalFolder string) ([]string, error) {
	var files []string

	err := filepath.Walk(normalFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".pgn") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort files
	sort.Strings(files)
	return files, nil
}

func filterNewPgnFiles(files []string, normalFolder, lastReadPgn string) []string {
	var newFiles []string

	for _, file := range files {
		relativePath, err := filepath.Rel(normalFolder, file)
		if err != nil {
			slog.Error("Failed to get relative path", "file", file, "error", err)
			continue
		}

		if relativePath > lastReadPgn {
			newFiles = append(newFiles, file)
		}
	}

	return newFiles
}

func loadPgnState() (string, error) {
	data, err := os.ReadFile(pgnJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var state PgnState
	if err = json.Unmarshal(data, &state); err != nil {
		return "", err
	}

	return state.LastReadPgn, nil
}

func savePgnState(lastReadPgn string) error {
	state := PgnState{
		LastReadPgn: lastReadPgn,
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(pgnJSONPath), 0744); err != nil { //nolint:gosec
		return err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return os.WriteFile(pgnJSONPath, data, 0600)
}

func learnNewPositions(
	positions map[models.NormalizedPosition]struct{},
	client *book.APIClient,
	edaxManager *edax.Process,
) error {
	// Filter positions that are DB savable
	pgnPositions := filterDBSavablePositions(positions)

	slog.Info("Looking up positions in DB", "count", len(pgnPositions))

	// Look up existing positions in batches
	foundPositions, err := lookupExistingPositions(client, pgnPositions)
	if err != nil {
		return fmt.Errorf("failed to lookup existing positions: %w", err)
	}

	// Find positions that need to be learned
	learnPositions := findPositionsToLearn(pgnPositions, foundPositions)

	if len(learnPositions) == 0 {
		slog.Info("No new positions to learn")
		return nil
	}

	slog.Info("Learning new positions", "count", len(learnPositions))

	// Process positions in chunks
	return processPositionsInChunks(learnPositions, client, edaxManager)
}

func filterDBSavablePositions(positions map[models.NormalizedPosition]struct{}) []models.NormalizedPosition {
	pgnPositions := make([]models.NormalizedPosition, 0)
	for pos := range positions {
		if pos.IsDBSavable() {
			pgnPositions = append(pgnPositions, pos)
		}
	}
	return pgnPositions
}

func lookupExistingPositions(
	client *book.APIClient,
	pgnPositions []models.NormalizedPosition,
) (map[models.NormalizedPosition]bool, error) {
	foundPositions := make(map[models.NormalizedPosition]bool)
	batchSize := 100

	for i := 0; i < len(pgnPositions); i += batchSize {
		end := i + batchSize
		if end > len(pgnPositions) {
			end = len(pgnPositions)
		}

		batch := pgnPositions[i:end]

		evaluations, err := client.LookupPositions(batch)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup positions: %w", err)
		}

		for _, eval := range evaluations {
			foundPositions[eval.Position] = true
		}
	}

	return foundPositions, nil
}

func findPositionsToLearn(
	pgnPositions []models.NormalizedPosition,
	foundPositions map[models.NormalizedPosition]bool,
) []models.NormalizedPosition {
	learnPositions := make([]models.NormalizedPosition, 0)
	for _, pos := range pgnPositions {
		if _, found := foundPositions[pos]; !found {
			learnPositions = append(learnPositions, pos)
		}
	}
	return learnPositions
}

func processPositionsInChunks(
	learnPositions []models.NormalizedPosition,
	client *book.APIClient,
	edaxManager *edax.Process,
) error {
	totalSeconds := 0.0

	chunkCount := int(math.Ceil(float64(len(learnPositions)) / learnChunkSize))
	for chunkID := range chunkCount {
		chunkStart := learnChunkSize * chunkID
		chunkEnd := learnChunkSize * (chunkID + 1)
		if chunkEnd > len(learnPositions) {
			chunkEnd = len(learnPositions)
		}

		chunk := learnPositions[chunkStart:chunkEnd]

		// Evaluate positions in chunk
		evaluations, seconds, err := evaluateChunk(chunk, edaxManager)
		if err != nil {
			return fmt.Errorf("failed to evaluate chunk %d: %w", chunkID, err)
		}

		totalSeconds += seconds

		// Save evaluations
		if err = client.SaveLearnedEvaluations(evaluations); err != nil {
			return fmt.Errorf("failed to save evaluations: %w", err)
		}

		// Calculate ETA
		average := totalSeconds / float64(chunkEnd)
		remainingPositions := len(learnPositions) - chunkEnd
		etaSeconds := average * float64(remainingPositions)
		eta := time.Now().Add(time.Duration(etaSeconds * float64(time.Second)))

		slog.Info("Processed chunk",
			"chunk_id", chunkID+1,
			"processed", chunkEnd,
			"total", len(learnPositions),
			"seconds", fmt.Sprintf("%7.3f", seconds),
			"eta", eta.Format("2006-01-02 15:04:05"),
		)
	}

	return nil
}

func evaluateChunk(chunk []models.NormalizedPosition, edaxManager *edax.Process) ([]models.Evaluation, float64, error) {
	evaluations := make([]models.Evaluation, 0, len(chunk))

	before := time.Now()

	for _, pos := range chunk {
		// If position has no moves, we need to pass
		if !pos.HasMoves() {
			pos = pos.Position().DoMove(models.PassMove).Normalized()
		}

		job := models.Job{
			Position: pos,
			Level:    config.MinBookLearnLevel,
		}

		result, err := edaxManager.DoJobSync(job)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to evaluate position: %w", err)
		}

		evaluations = append(evaluations, result.Evaluation)
	}

	after := time.Now()
	seconds := after.Sub(before).Seconds()

	return evaluations, seconds, nil
}
