package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotFiles(t *testing.T) {
	// Create temp dir
	tmpDir := t.TempDir()

	// Create a test file
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")
	err := os.WriteFile(filePath, content, 0o600)
	require.NoError(t, err)

	// Create a sub directory and file
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o700))
	subFilePath := filepath.Join(subDir, "sub.txt")
	subContent := []byte("hello subdir")
	require.NoError(t, os.WriteFile(subFilePath, subContent, 0o600))

	// Files to snapshot
	paths := []string{filePath, subFilePath, filepath.Join(tmpDir, "nonexistent.txt")}

	// Run snapshot
	snapshots := snapshotFiles(paths)

	require.Len(t, snapshots, 3)

	// Check first file
	assert.Equal(t, filePath, snapshots[0].Path)
	assert.True(t, snapshots[0].Exists)
	assert.Equal(t, int64(len(content)), snapshots[0].Size)
	assert.NotEmpty(t, snapshots[0].ModTime)

	hash := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(hash[:])[:16]
	assert.Equal(t, expectedHash, snapshots[0].SHA256)

	// Check second file
	assert.Equal(t, subFilePath, snapshots[1].Path)
	assert.True(t, snapshots[1].Exists)
	assert.Equal(t, int64(len(subContent)), snapshots[1].Size)

	// Check nonexistent file
	assert.Equal(t, paths[2], snapshots[2].Path)
	assert.False(t, snapshots[2].Exists)
	assert.Equal(t, int64(0), snapshots[2].Size)
	assert.Empty(t, snapshots[2].SHA256)
}

func TestHashFile_Limit(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.bin")

	// Create a file larger than the 10MB limit (e.g., 11MB)
	//nolint:gosec // G304: Test file path is from t.TempDir()
	f, err := os.Create(filePath)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	// Write 11MB of zeros
	chunk := make([]byte, 1024*1024)
	for i := 0; i < 11; i++ {
		_, writeErr := f.Write(chunk)
		require.NoError(t, writeErr)
	}

	// Hash it
	hash, err := hashFile(filePath)
	require.NoError(t, err)

	// Identify expected hash of 10MB of zeros
	sha := sha256.New()
	for i := 0; i < 10; i++ {
		sha.Write(chunk)
	}
	expectedFullHash := hex.EncodeToString(sha.Sum(nil))
	expectedHash := expectedFullHash[:16]

	assert.Equal(t, expectedHash, hash)
}
