package workflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// initTestGitRepo creates a minimal git repository in a temp dir for use by
// initializer tests that need real git operations.
func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // G204: test code
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o600))
	run("add", ".")
	run("commit", "-m", "Initial commit")
	return dir
}

func TestNewInitializer(t *testing.T) {
	logger := zerolog.Nop()
	init := NewInitializer(logger)
	assert.NotNil(t, init)
	assert.Equal(t, logger, init.logger)
}

func TestCreateWorkspaceSimple(t *testing.T) {
	t.Run("creates standalone function wrapper", func(_ *testing.T) {
		// This test verifies the standalone function exists and can be called
		// The actual workspace creation will fail without a real git repo, but we can verify the function signature
		ctx := context.Background()
		_, err := CreateWorkspaceSimple(ctx, "test", "/nonexistent", "feature", "master", "", false)
		// We expect an error since we're using a nonexistent path
		assert.Error(t, err)
	})
}

func TestFindGitRepository_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(_ *testing.T) {
		ctx := context.Background()
		// This will likely fail since we may not be in a git repo,
		// but we're testing that the function exists and can be called
		_, _ = FindGitRepository(ctx)
	})
}

func TestCleanupWorkspace_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(_ *testing.T) {
		ctx := context.Background()
		// This will fail but we're testing the function signature
		_ = CleanupWorkspace(ctx, "test-ws", "/nonexistent")
	})
}

func TestInitializer_FindGitRepository(t *testing.T) {
	t.Run("method exists", func(_ *testing.T) {
		logger := zerolog.Nop()
		init := NewInitializer(logger)
		ctx := context.Background()

		// This may succeed or fail depending on whether we're in a git repo
		// We're just testing that the method exists and can be called
		_, _ = init.FindGitRepository(ctx)
	})
}

func TestWorkspaceOptions(t *testing.T) {
	t.Run("struct fields exist", func(_ *testing.T) {
		opts := WorkspaceOptions{
			Name:          "test-workspace",
			RepoPath:      "/path/to/repo",
			BranchPrefix:  "feature",
			BaseBranch:    "master",
			TargetBranch:  "",
			UseLocal:      false,
			NoInteractive: false,
			OutputFormat:  "text",
			ErrorHandler:  func(_ string, err error) error { return err },
		}

		assert.Equal(t, "test-workspace", opts.Name)
		assert.Equal(t, "/path/to/repo", opts.RepoPath)
		assert.Equal(t, "feature", opts.BranchPrefix)
		assert.Equal(t, "master", opts.BaseBranch)
		assert.Empty(t, opts.TargetBranch)
		assert.False(t, opts.UseLocal)
		assert.False(t, opts.NoInteractive)
		assert.Equal(t, "text", opts.OutputFormat)
		assert.NotNil(t, opts.ErrorHandler)
	})
}

func TestCreateWorkspace_ErrorHandler(t *testing.T) {
	t.Run("error handler is called on workspace store creation failure", func(_ *testing.T) {
		logger := zerolog.Nop()
		init := NewInitializer(logger)
		ctx := context.Background()

		errorHandlerCalled := false
		var capturedWorkspaceName string
		var capturedError error

		opts := WorkspaceOptions{
			Name:         "test-workspace",
			RepoPath:     "/nonexistent/path/that/should/not/exist",
			BranchPrefix: "feature",
			BaseBranch:   "master",
			UseLocal:     false,
			ErrorHandler: func(wsName string, err error) error {
				errorHandlerCalled = true
				capturedWorkspaceName = wsName
				capturedError = err
				return err
			},
		}

		_, err := init.CreateWorkspace(ctx, opts)

		// Should have an error
		require.Error(t, err)

		// Error handler should have been called
		assert.True(t, errorHandlerCalled, "ErrorHandler should have been called")
		assert.Equal(t, "test-workspace", capturedWorkspaceName)
		assert.Error(t, capturedError)
	})
}

func TestCleanupWorkspace_Method(t *testing.T) {
	t.Run("cleanup workspace creates store and runner", func(_ *testing.T) {
		logger := zerolog.Nop()
		init := NewInitializer(logger)
		ctx := context.Background()

		// This will fail due to nonexistent path, but tests the method flow
		err := init.CleanupWorkspace(ctx, "test-workspace", "/nonexistent")
		assert.Error(t, err)
	})
}

func TestCreateWorkspace_BranchModes(t *testing.T) {
	t.Run("new branch mode vs existing branch mode options", func(_ *testing.T) {
		// Test that the two modes have different option structures

		// New branch mode
		newBranchOpts := WorkspaceOptions{
			Name:         "test-workspace",
			RepoPath:     "/path/to/repo",
			BranchPrefix: "feature", // Set for new branch mode
			BaseBranch:   "master",  // Set for new branch mode
			TargetBranch: "",        // Empty for new branch mode
			ErrorHandler: func(_ string, err error) error { return err },
		}

		assert.Equal(t, "feature", newBranchOpts.BranchPrefix)
		assert.Equal(t, "master", newBranchOpts.BaseBranch)
		assert.Empty(t, newBranchOpts.TargetBranch)

		// Existing branch mode
		existingBranchOpts := WorkspaceOptions{
			Name:         "test-workspace",
			RepoPath:     "/path/to/repo",
			BranchPrefix: "",             // Empty for existing branch mode
			BaseBranch:   "",             // Empty for existing branch mode
			TargetBranch: "patch/urgent", // Set for existing branch mode
			ErrorHandler: func(_ string, err error) error { return err },
		}

		assert.Empty(t, existingBranchOpts.BranchPrefix)
		assert.Empty(t, existingBranchOpts.BaseBranch)
		assert.Equal(t, "patch/urgent", existingBranchOpts.TargetBranch)
	})
}

// TestCreateWorkspace_ValidDir_NewGitWorktreeRunnerFails verifies that the error handler
// is invoked when the repo path is valid but not a git repository.
func TestCreateWorkspace_ValidDir_NewGitWorktreeRunnerFails(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	init := NewInitializer(logger)
	ctx := context.Background()

	errorHandlerCalled := false
	opts := WorkspaceOptions{
		Name:         "test-ws",
		RepoPath:     t.TempDir(), // valid dir but not a git repo
		BranchPrefix: "feature",
		BaseBranch:   "master",
		ErrorHandler: func(_ string, err error) error {
			errorHandlerCalled = true
			return err
		},
	}

	_, err := init.CreateWorkspace(ctx, opts)
	require.Error(t, err)
	assert.True(t, errorHandlerCalled, "ErrorHandler should be called when worktree runner fails")
}

// TestCreateWorkspace_WithGitRepo_NewBranch exercises the full CreateWorkspace path using
// a real git repository with new-branch mode. The workspace manager creates a worktree.
func TestCreateWorkspace_WithGitRepo_NewBranch(t *testing.T) {
	// Not parallel: creates sibling git worktree directories.
	repoPath := initTestGitRepo(t)

	logger := zerolog.Nop()
	init := NewInitializer(logger)
	ctx := context.Background()

	opts := WorkspaceOptions{
		Name:         "test-new-branch-ws",
		RepoPath:     repoPath,
		BranchPrefix: "feat",
		BaseBranch:   "",
		ErrorHandler: func(_ string, err error) error { return err },
	}

	ws, err := init.CreateWorkspace(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, ws)
	assert.Equal(t, "test-new-branch-ws", ws.Name)
}

// TestCreateWorkspace_WithGitRepo_ExistingBranch exercises the existing-branch mode of
// CreateWorkspace, which checks out a branch that already exists.
func TestCreateWorkspace_WithGitRepo_ExistingBranch(t *testing.T) {
	// Not parallel: creates sibling git worktree directories.
	repoPath := initTestGitRepo(t)

	logger := zerolog.Nop()
	init := NewInitializer(logger)
	ctx := context.Background()

	// Detect the default branch name used by git init (may be "master" or "main").
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	require.NoError(t, err)
	defaultBranch := strings.TrimSpace(string(out))

	// Create a new branch to check out via the existing-branch mode.
	cmd = exec.CommandContext(ctx, "git", "checkout", "-b", "feat/existing-target")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())
	// Switch back to the default branch so the target branch is not checked out.
	cmd = exec.CommandContext(ctx, "git", "checkout", defaultBranch) //nolint:gosec // G204: variable constructed from test-controlled values
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())

	opts := WorkspaceOptions{
		Name:         "test-existing-branch-ws",
		RepoPath:     repoPath,
		TargetBranch: "feat/existing-target",
		ErrorHandler: func(_ string, err error) error { return err },
	}

	ws, err := init.CreateWorkspace(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, ws)
	assert.Equal(t, "feat/existing-target", ws.Branch)
}

// TestInitializer_FindGitRepository_NonGitDir verifies that FindGitRepository returns
// ErrNotGitRepo when the working directory is not inside a git repository.
func TestInitializer_FindGitRepository_NonGitDir(t *testing.T) {
	t.Parallel()

	// Change to a temp dir that is NOT a git repo.
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	init := NewInitializer(zerolog.Nop())
	_, err = init.FindGitRepository(context.Background())
	assert.ErrorIs(t, err, atlaserrors.ErrNotGitRepo)
}

// TestCleanupWorkspace_ValidDir_RunnerFails verifies CleanupWorkspace returns an error
// when the repo path is not a git repository.
func TestCleanupWorkspace_ValidDir_RunnerFails(t *testing.T) {
	t.Parallel()
	init := NewInitializer(zerolog.Nop())
	ctx := context.Background()

	// Valid dir but not a git repo → NewGitWorktreeRunner fails.
	err := init.CleanupWorkspace(ctx, "some-ws", t.TempDir())
	assert.Error(t, err)
}
