package testfakes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TempAtlasEnv creates a temporary Atlas home directory that mirrors the
// structure of ~/.atlas/. It creates tasks/ and workspaces/ subdirectories.
// The directory is removed automatically when the test ends.
// Returns the absolute path to the root (the ~/.atlas/ equivalent).
func TempAtlasEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tasks"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "workspaces"), 0o700))
	return dir
}
