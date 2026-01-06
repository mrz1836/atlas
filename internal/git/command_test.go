package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// createTestGitRepo initializes a temporary git repository for testing.
// This helper function is used throughout git command tests.
func createTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repository
	cmd := exec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for commits
	_ = exec.CommandContext(context.Background(), "git", "-C", dir, "config", "user.email", "test@example.com").Run() // #nosec G204
	_ = exec.CommandContext(context.Background(), "git", "-C", dir, "config", "user.name", "Test User").Run()         // #nosec G204

	return dir
}

func TestRunCommand_Success(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	// Test successful command - rev-parse --git-dir should return ".git"
	output, err := RunCommand(ctx, dir, "rev-parse", "--git-dir")

	require.NoError(t, err)
	assert.Equal(t, ".git", output)
}

func TestRunCommand_WithStderr(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	// Attempt to show a non-existent commit
	_, err := RunCommand(ctx, dir, "show", "nonexistent-commit-hash")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	// Error message should include the git command and stderr
	assert.Contains(t, err.Error(), "git show failed")
}

func TestRunCommand_ContextCancellation(t *testing.T) {
	dir := createTestGitRepo(t)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := RunCommand(ctx, dir, "status")

	require.Error(t, err)
	// Should return context.Canceled
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunCommand_ContextTimeout(t *testing.T) {
	dir := createTestGitRepo(t)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Sleep briefly to ensure timeout
	time.Sleep(10 * time.Millisecond)

	_, err := RunCommand(ctx, dir, "status")

	require.Error(t, err)
	// Should return context deadline exceeded
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRunCommand_NonGitDirectory(t *testing.T) {
	// Use a non-git directory
	dir := t.TempDir()
	ctx := context.Background()

	_, err := RunCommand(ctx, dir, "status")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	assert.Contains(t, err.Error(), "git status failed")
	// Should contain "not a git repository" or similar in stderr
	errStr := err.Error()
	containsNotGit := assert.Contains(t, errStr, "not a git repository") ||
		assert.Contains(t, errStr, "not a git repo")
	assert.True(t, containsNotGit, "error should mention not a git repository")
}

func TestRunCommand_EmptyWorkDir(_ *testing.T) {
	ctx := context.Background()

	// RunCommand with empty workDir - should use current dir
	// This should fail unless current dir is a git repo
	_, err := RunCommand(ctx, "", "status")

	// Error behavior depends on current directory
	// Just verify it doesn't panic
	_ = err
}

func TestRunCommand_InvalidCommand(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	_, err := RunCommand(ctx, dir, "not-a-valid-git-command")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	assert.Contains(t, err.Error(), "git not-a-valid-git-command failed")
}

func TestRunCommand_StderrTrimming(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	// Command that produces stderr with whitespace
	_, err := RunCommand(ctx, dir, "show", "HEAD")

	// Should fail because there are no commits yet
	require.Error(t, err)
	// Error message should be trimmed (no leading/trailing whitespace)
	assert.NotContains(t, err.Error(), "\n\n")
}

func TestRunCommand_MultipleArgs(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	// Create a test file
	testFile := filepath.Join(dir, "test.txt")
	err := os.WriteFile(testFile, []byte("content"), 0o600)
	require.NoError(t, err)

	// Add the file
	_, err = RunCommand(ctx, dir, "add", "test.txt")
	require.NoError(t, err)

	// Verify file was added
	output, err := RunCommand(ctx, dir, "status", "--porcelain")
	require.NoError(t, err)
	assert.Contains(t, output, "test.txt")
}

func TestRunCommand_OutputTrimming(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	// Git commands often have trailing newlines
	output, err := RunCommand(ctx, dir, "rev-parse", "--git-dir")

	require.NoError(t, err)
	// Output should be trimmed
	assert.Equal(t, ".git", output)
	assert.NotContains(t, output, "\n")
	assert.NotContains(t, output, " ")
}

func TestRunCommand_EmptyOutput(t *testing.T) {
	dir := createTestGitRepo(t)
	ctx := context.Background()

	// Command with no output
	output, err := RunCommand(ctx, dir, "config", "user.name", "Test User")

	require.NoError(t, err)
	assert.Empty(t, output)
}
