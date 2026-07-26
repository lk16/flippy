package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvePaths_IndividualFiles(t *testing.T) {
	dir := t.TempDir()

	wtbPath := filepath.Join(dir, "games.wtb")
	require.NoError(t, os.WriteFile(wtbPath, []byte("wtb"), 0o644))

	pgnPath := filepath.Join(dir, "games.pgn")
	require.NoError(t, os.WriteFile(pgnPath, []byte("pgn"), 0o644))

	wtbFiles, pgnFiles, err := ResolvePaths([]string{wtbPath, pgnPath})
	require.NoError(t, err)
	require.Equal(t, []string{wtbPath}, wtbFiles)
	require.Equal(t, []string{pgnPath}, pgnFiles)
}

func TestResolvePaths_RejectsUnknownExtension(t *testing.T) {
	dir := t.TempDir()

	badPath := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(badPath, []byte("notes"), 0o644))

	wtbFiles, pgnFiles, err := ResolvePaths([]string{badPath})
	require.Error(t, err)
	require.Empty(t, wtbFiles)
	require.Empty(t, pgnFiles)
}

func TestResolvePaths_RejectsUnknownExtensionAmongValidPaths(t *testing.T) {
	dir := t.TempDir()

	wtbPath := filepath.Join(dir, "games.wtb")
	require.NoError(t, os.WriteFile(wtbPath, []byte("wtb"), 0o644))

	badPath := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(badPath, []byte("notes"), 0o644))

	_, _, err := ResolvePaths([]string{wtbPath, badPath})
	require.Error(t, err)
}

func TestResolvePaths_MissingPath(t *testing.T) {
	_, _, err := ResolvePaths([]string{filepath.Join(t.TempDir(), "does-not-exist.wtb")})
	require.Error(t, err)
}

func TestResolvePaths_FolderRecursesAndIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()

	nested := filepath.Join(dir, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	wtbPath := filepath.Join(dir, "top.wtb")
	require.NoError(t, os.WriteFile(wtbPath, []byte("wtb"), 0o644))

	nestedPGNPath := filepath.Join(nested, "sub.pgn")
	require.NoError(t, os.WriteFile(nestedPGNPath, []byte("pgn"), 0o644))

	ignoredPath := filepath.Join(nested, "readme.txt")
	require.NoError(t, os.WriteFile(ignoredPath, []byte("ignored"), 0o644))

	wtbFiles, pgnFiles, err := ResolvePaths([]string{dir})
	require.NoError(t, err)
	require.Equal(t, []string{wtbPath}, wtbFiles)
	require.Equal(t, []string{nestedPGNPath}, pgnFiles)
}
