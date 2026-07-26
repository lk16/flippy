package loader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePaths splits paths into WTHOR (.wtb) and PGN (.pgn) filenames.
// Directories are searched recursively for files with either extension;
// other files found this way are ignored. Each path that names a file
// directly, rather than a directory, must itself have a .wtb or .pgn
// extension — anything else is an error, and no filenames are returned in
// that case, so callers don't import a partial set.
func ResolvePaths(paths []string) (wtbFiles, pgnFiles []string, err error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to stat %s: %w", path, err)
		}

		if info.IsDir() {
			dirWTB, dirPGN, err := findGameFiles(path)
			if err != nil {
				return nil, nil, err
			}
			wtbFiles = append(wtbFiles, dirWTB...)
			pgnFiles = append(pgnFiles, dirPGN...)
			continue
		}

		switch strings.ToLower(filepath.Ext(path)) {
		case ".wtb":
			wtbFiles = append(wtbFiles, path)
		case ".pgn":
			pgnFiles = append(pgnFiles, path)
		default:
			return nil, nil, fmt.Errorf("%s is not a .wtb or .pgn file", path)
		}
	}

	return wtbFiles, pgnFiles, nil
}

// findGameFiles recursively collects .wtb and .pgn files under dir.
func findGameFiles(dir string) (wtbFiles, pgnFiles []string, err error) {
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		switch strings.ToLower(filepath.Ext(path)) {
		case ".wtb":
			wtbFiles = append(wtbFiles, path)
		case ".pgn":
			pgnFiles = append(pgnFiles, path)
		}

		return nil
	})
	if walkErr != nil {
		return nil, nil, fmt.Errorf("failed to search %s: %w", dir, walkErr)
	}

	return wtbFiles, pgnFiles, nil
}
