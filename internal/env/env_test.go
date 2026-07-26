package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_NoEnvFile(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, Load())
}

func TestLoad_SetsUnsetVars(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FLIPPY_TEST_VAR=from-env-file\n"), 0o600))
	t.Chdir(dir)

	require.NoError(t, os.Unsetenv("FLIPPY_TEST_VAR"))
	t.Cleanup(func() { _ = os.Unsetenv("FLIPPY_TEST_VAR") })

	require.NoError(t, Load())
	require.Equal(t, "from-env-file", os.Getenv("FLIPPY_TEST_VAR"))
}

func TestLoad_DoesNotOverrideExistingVar(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FLIPPY_TEST_VAR=from-env-file\n"), 0o600))
	t.Chdir(dir)

	t.Setenv("FLIPPY_TEST_VAR", "from-shell")

	require.NoError(t, Load())
	require.Equal(t, "from-shell", os.Getenv("FLIPPY_TEST_VAR"))
}
