//go:build integration

package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_PushRunner_RealGitPush tests push operations with a real git repository.
// This test creates a local git repo with a bare remote and tests actual push behavior.
func TestIntegration_PushRunner_RealGitPush(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create temp directories for repo and bare remote
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	remoteDir := filepath.Join(tmpDir, "remote.git")

	// Initialize bare remote repository
	err := os.MkdirAll(remoteDir, 0o755)
	require.NoError(t, err)

	cmd := exec.CommandContext(ctx, "git", "init", "--bare")
	cmd.Dir = remoteDir
	err = cmd.Run()
	require.NoError(t, err, "failed to create bare remote")

	// Initialize local repository
	err = os.MkdirAll(repoDir, 0o755)
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "init")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err, "failed to init local repo")

	// Configure git user for commits
	cmd = exec.CommandContext(ctx, "git", "config", "user.email", "test@atlas.local")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "config", "user.name", "ATLAS Test")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	// Add remote
	cmd = exec.CommandContext(ctx, "git", "remote", "add", "origin", remoteDir)
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err, "failed to add remote")

	// Create a file and commit
	testFile := filepath.Join(repoDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "add", ".")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial commit")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err, "failed to commit")

	// Create a branch
	cmd = exec.CommandContext(ctx, "git", "checkout", "-b", "feat/test-push")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err, "failed to create branch")

	// Create CLIRunner
	runner, err := NewRunner(repoDir)
	require.NoError(t, err)

	// Create PushRunner with logger
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	pushRunner := NewPushRunner(runner, WithPushLogger(logger))

	t.Run("successful push with upstream", func(t *testing.T) {
		result, err := pushRunner.Push(ctx, PushOptions{
			Remote:      "origin",
			Branch:      "feat/test-push",
			SetUpstream: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "origin/feat/test-push", result.Upstream)
		assert.Equal(t, 1, result.Attempts)
	})

	t.Run("push again without new commits succeeds", func(t *testing.T) {
		// Push again - should succeed (nothing to push but not an error)
		result, err := pushRunner.Push(ctx, PushOptions{
			Remote:      "origin",
			Branch:      "feat/test-push",
			SetUpstream: false,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("push with new commit", func(t *testing.T) {
		// Add another commit
		testFile2 := filepath.Join(repoDir, "test2.txt")
		err := os.WriteFile(testFile2, []byte("more content"), 0o644)
		require.NoError(t, err)

		cmd := exec.CommandContext(ctx, "git", "add", ".")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		cmd = exec.CommandContext(ctx, "git", "commit", "-m", "second commit")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		// Push new commit
		result, err := pushRunner.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "feat/test-push",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("push with progress callback", func(t *testing.T) {
		// Add another commit
		testFile3 := filepath.Join(repoDir, "test3.txt")
		err := os.WriteFile(testFile3, []byte("even more content"), 0o644)
		require.NoError(t, err)

		cmd := exec.CommandContext(ctx, "git", "add", ".")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		cmd = exec.CommandContext(ctx, "git", "commit", "-m", "third commit")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		progressCalled := false
		result, err := pushRunner.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "feat/test-push",
			ProgressCallback: func(progress string) {
				progressCalled = true
				t.Logf("Progress: %s", progress)
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.True(t, progressCalled, "progress callback should have been called")
	})

	t.Run("push with confirmation callback approved", func(t *testing.T) {
		// Add another commit
		testFile4 := filepath.Join(repoDir, "test4.txt")
		err := os.WriteFile(testFile4, []byte("confirmation test"), 0o644)
		require.NoError(t, err)

		cmd := exec.CommandContext(ctx, "git", "add", ".")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		cmd = exec.CommandContext(ctx, "git", "commit", "-m", "fourth commit")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		confirmCalled := false
		result, err := pushRunner.Push(ctx, PushOptions{
			Remote:            "origin",
			Branch:            "feat/test-push",
			ConfirmBeforePush: true,
			ConfirmCallback: func(remote, branch string) (bool, error) {
				confirmCalled = true
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "feat/test-push", branch)
				return true, nil
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.True(t, confirmCalled, "confirmation callback should have been called")
	})
}

// TestIntegration_PushRunner_InvalidRemote tests push to a non-existent remote.
func TestIntegration_PushRunner_InvalidRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Initialize local repository
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = tmpDir
	err := cmd.Run()
	require.NoError(t, err)

	// Configure git user
	cmd = exec.CommandContext(ctx, "git", "config", "user.email", "test@atlas.local")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "config", "user.name", "ATLAS Test")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	// Create a commit (required before push)
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "add", ".")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	// Create CLIRunner and PushRunner
	runner, err := NewRunner(tmpDir)
	require.NoError(t, err)

	pushRunner := NewPushRunner(runner)

	// Try to push to non-existent remote
	result, err := pushRunner.Push(ctx, PushOptions{
		Remote: "nonexistent",
		Branch: "main",
	})

	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, PushErrorOther, result.ErrorType)
}
