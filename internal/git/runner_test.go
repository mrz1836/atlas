package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// setupTestRepo creates a temporary git repository for testing.
// Returns the path to the repo.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to init git repo")

	// Configure git user for commits
	cmd = exec.CommandContext(context.Background(), "git", "config", "user.email", "test@atlas.local")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to configure git email")

	cmd = exec.CommandContext(context.Background(), "git", "config", "user.name", "ATLAS Test")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to configure git name")

	return tmpDir
}

// createFile creates a file with content in the repo.
func createFile(t *testing.T, repoPath, filename, content string) {
	t.Helper()
	path := filepath.Join(repoPath, filename)
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err, "failed to create file")
}

// commitInitial stages and commits all changes in the repo with a standard initial commit message.
func commitInitial(t *testing.T, repoPath string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run(), "failed to add files")

	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "initial commit")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run(), "failed to commit")
}

// TestNewRunner tests the constructor.
func TestNewRunner(t *testing.T) {
	t.Run("success with valid git repo", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)
		assert.NotNil(t, runner)
	})

	t.Run("error with empty path", func(t *testing.T) {
		runner, err := NewRunner(context.Background(), "")
		assert.Nil(t, runner)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("error with non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		runner, err := NewRunner(context.Background(), tmpDir)
		assert.Nil(t, runner)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrNotGitRepo)
	})

	t.Run("error with non-existent path", func(t *testing.T) {
		runner, err := NewRunner(context.Background(), "/nonexistent/path/to/repo")
		assert.Nil(t, runner)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})
}

// TestCLIRunner_Status tests the Status method.
func TestCLIRunner_Status(t *testing.T) {
	t.Run("clean repository", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.True(t, status.IsClean())
		assert.Empty(t, status.Staged)
		assert.Empty(t, status.Unstaged)
		assert.Empty(t, status.Untracked)
	})

	t.Run("untracked file", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial content")
		commitInitial(t, repoPath)
		createFile(t, repoPath, "untracked.txt", "untracked content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.False(t, status.IsClean())
		assert.True(t, status.HasUntrackedFiles())
		assert.Contains(t, status.Untracked, "untracked.txt")
	})

	t.Run("staged file", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial content")
		commitInitial(t, repoPath)
		createFile(t, repoPath, "staged.txt", "staged content")

		// Stage the file
		cmd := exec.CommandContext(context.Background(), "git", "add", "staged.txt")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.True(t, status.HasStagedChanges())
		require.Len(t, status.Staged, 1)
		assert.Equal(t, "staged.txt", status.Staged[0].Path)
		assert.Equal(t, ChangeAdded, status.Staged[0].Status)
	})

	t.Run("modified unstaged file", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "initial content")
		commitInitial(t, repoPath)
		createFile(t, repoPath, "file.txt", "modified content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.True(t, status.HasUnstagedChanges())
		require.Len(t, status.Unstaged, 1)
		assert.Equal(t, "file.txt", status.Unstaged[0].Path)
		assert.Equal(t, ChangeModified, status.Unstaged[0].Status)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		status, err := runner.Status(ctx)
		assert.Nil(t, status)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("branch info with no commits", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		// New repos might not have a branch name until first commit
		assert.NotNil(t, status)
	})

	t.Run("branch info with commits", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		// Should have branch name (master or main depending on git config)
		assert.NotEmpty(t, status.Branch)
	})
}

// TestCLIRunner_Add tests the Add method.
func TestCLIRunner_Add(t *testing.T) {
	t.Run("add specific file", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file1.txt", "content1")
		createFile(t, repoPath, "file2.txt", "content2")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), []string{"file1.txt"})
		require.NoError(t, err)

		// Verify file1 is staged, file2 is not
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		require.Len(t, status.Staged, 1)
		assert.Equal(t, "file1.txt", status.Staged[0].Path)
		assert.Contains(t, status.Untracked, "file2.txt")
	})

	t.Run("add all files", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file1.txt", "content1")
		createFile(t, repoPath, "file2.txt", "content2")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		// Verify all files are staged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 2)
		assert.Empty(t, status.Untracked)
	})

	t.Run("add multiple specific files", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file1.txt", "content1")
		createFile(t, repoPath, "file2.txt", "content2")
		createFile(t, repoPath, "file3.txt", "content3")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), []string{"file1.txt", "file3.txt"})
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 2)
		assert.Contains(t, status.Untracked, "file2.txt")
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Add(ctx, nil)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_Commit tests the Commit method.
func TestCLIRunner_Commit(t *testing.T) {
	t.Run("simple commit", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		err = runner.Commit(context.Background(), "test commit")
		require.NoError(t, err)

		// Verify commit was made
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.True(t, status.IsClean())
	})

	t.Run("empty commit message error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		err = runner.Commit(context.Background(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("commit with nothing staged error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Commit(context.Background(), "empty commit")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Commit(ctx, "test")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_CurrentBranch tests the CurrentBranch method.
func TestCLIRunner_CurrentBranch(t *testing.T) {
	t.Run("get current branch", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		branch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)
		// Branch name is typically 'master' or 'main' depending on git config
		assert.NotEmpty(t, branch)
		assert.NotEqual(t, "HEAD", branch)
	})

	t.Run("detached HEAD state error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		// Checkout a specific commit (detached HEAD)
		cmd := exec.CommandContext(context.Background(), "git", "checkout", "HEAD^0")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		_, err = runner.CurrentBranch(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.CurrentBranch(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_CreateBranch tests the CreateBranch method.
func TestCLIRunner_CreateBranch(t *testing.T) {
	t.Run("create new branch from HEAD", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Empty baseBranch creates from current HEAD
		err = runner.CreateBranch(context.Background(), "feat/new-feature", "")
		require.NoError(t, err)

		// Verify we're on the new branch
		branch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "feat/new-feature", branch)
	})

	t.Run("create branch from specified base", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		// Get the default branch name (could be main or master)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)
		defaultBranch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)

		// Create a feature branch with a new commit
		cmd := exec.CommandContext(context.Background(), "git", "checkout", "-b", "develop")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		createFile(t, repoPath, "develop.txt", "develop content")
		cmd = exec.CommandContext(context.Background(), "git", "add", ".")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())
		cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "develop commit")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		// Go back to default branch
		//nolint:gosec // G204: defaultBranch is from trusted git output, not user input
		cmd = exec.CommandContext(context.Background(), "git", "checkout", defaultBranch)
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		// Create branch from develop
		err = runner.CreateBranch(context.Background(), "feat/from-develop", "develop")
		require.NoError(t, err)

		// Verify we're on the new branch
		branch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "feat/from-develop", branch)

		// Verify the branch has the develop.txt file (proving it was created from develop)
		_, err = os.Stat(filepath.Join(repoPath, "develop.txt"))
		assert.NoError(t, err, "branch should have been created from develop and contain develop.txt")
	})

	t.Run("branch already exists error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		// Create a branch first
		cmd := exec.CommandContext(context.Background(), "git", "branch", "existing-branch")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.CreateBranch(context.Background(), "existing-branch", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrBranchExists)
	})

	t.Run("empty branch name error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.CreateBranch(context.Background(), "", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.CreateBranch(ctx, "new-branch", "")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_Diff tests the DiffStaged and DiffUnstaged methods.
func TestCLIRunner_Diff(t *testing.T) {
	t.Run("no diff on clean repo", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		diff, err := runner.DiffUnstaged(context.Background())
		require.NoError(t, err)
		assert.Empty(t, diff)

		diff, err = runner.DiffStaged(context.Background())
		require.NoError(t, err)
		assert.Empty(t, diff)
	})

	t.Run("unstaged diff", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "initial content")
		commitInitial(t, repoPath)
		createFile(t, repoPath, "file.txt", "modified content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		diff, err := runner.DiffUnstaged(context.Background())
		require.NoError(t, err)
		assert.Contains(t, diff, "file.txt")
		assert.Contains(t, diff, "-initial content")
		assert.Contains(t, diff, "+modified content")
	})

	t.Run("staged diff", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "initial content")
		commitInitial(t, repoPath)
		createFile(t, repoPath, "file.txt", "modified content")

		// Stage the changes
		cmd := exec.CommandContext(context.Background(), "git", "add", "file.txt")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Unstaged diff should be empty
		diff, err := runner.DiffUnstaged(context.Background())
		require.NoError(t, err)
		assert.Empty(t, diff)

		// Staged diff should show changes
		diff, err = runner.DiffStaged(context.Background())
		require.NoError(t, err)
		assert.Contains(t, diff, "file.txt")
		assert.Contains(t, diff, "+modified content")
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.DiffUnstaged(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_Push tests the Push method.
func TestCLIRunner_Push(t *testing.T) {
	t.Run("push error without remote", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Should fail because there's no remote
		err = runner.Push(context.Background(), "origin", "master", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Push(ctx, "origin", "master", false)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestStatus_Methods tests the Status helper methods.
func TestStatus_Methods(t *testing.T) {
	t.Run("IsClean", func(t *testing.T) {
		status := &Status{}
		assert.True(t, status.IsClean())

		status.Staged = []FileChange{{Path: "file.txt"}}
		assert.False(t, status.IsClean())
	})

	t.Run("HasStagedChanges", func(t *testing.T) {
		status := &Status{}
		assert.False(t, status.HasStagedChanges())

		status.Staged = []FileChange{{Path: "file.txt"}}
		assert.True(t, status.HasStagedChanges())
	})

	t.Run("HasUnstagedChanges", func(t *testing.T) {
		status := &Status{}
		assert.False(t, status.HasUnstagedChanges())

		status.Unstaged = []FileChange{{Path: "file.txt"}}
		assert.True(t, status.HasUnstagedChanges())
	})

	t.Run("HasUntrackedFiles", func(t *testing.T) {
		status := &Status{}
		assert.False(t, status.HasUntrackedFiles())

		status.Untracked = []string{"file.txt"}
		assert.True(t, status.HasUntrackedFiles())
	})
}

// TestParseGitStatus tests the parseGitStatus function.
func TestParseGitStatus(t *testing.T) {
	t.Run("parse branch line", func(t *testing.T) {
		output := "## main...origin/main [ahead 2, behind 1]\n"
		status := parseGitStatus(output)
		assert.Equal(t, "main", status.Branch)
		assert.Equal(t, 2, status.Ahead)
		assert.Equal(t, 1, status.Behind)
	})

	t.Run("parse branch line ahead only", func(t *testing.T) {
		output := "## feat/test...origin/feat/test [ahead 3]\n"
		status := parseGitStatus(output)
		assert.Equal(t, "feat/test", status.Branch)
		assert.Equal(t, 3, status.Ahead)
		assert.Equal(t, 0, status.Behind)
	})

	t.Run("parse staged file", func(t *testing.T) {
		output := "## main\nA  newfile.txt\n"
		status := parseGitStatus(output)
		require.Len(t, status.Staged, 1)
		assert.Equal(t, "newfile.txt", status.Staged[0].Path)
		assert.Equal(t, ChangeAdded, status.Staged[0].Status)
	})

	t.Run("parse modified unstaged file", func(t *testing.T) {
		output := "## main\n M modified.txt\n"
		status := parseGitStatus(output)
		require.Len(t, status.Unstaged, 1)
		assert.Equal(t, "modified.txt", status.Unstaged[0].Path)
		assert.Equal(t, ChangeModified, status.Unstaged[0].Status)
	})

	t.Run("parse untracked file", func(t *testing.T) {
		output := "## main\n?? untracked.txt\n"
		status := parseGitStatus(output)
		require.Len(t, status.Untracked, 1)
		assert.Equal(t, "untracked.txt", status.Untracked[0])
	})

	t.Run("parse renamed file", func(t *testing.T) {
		output := "## main\nR  old.txt -> new.txt\n"
		status := parseGitStatus(output)
		require.Len(t, status.Staged, 1)
		assert.Equal(t, "new.txt", status.Staged[0].Path)
		assert.Equal(t, "old.txt", status.Staged[0].OldPath)
		assert.Equal(t, ChangeRenamed, status.Staged[0].Status)
	})

	t.Run("parse deleted file", func(t *testing.T) {
		output := "## main\nD  deleted.txt\n"
		status := parseGitStatus(output)
		require.Len(t, status.Staged, 1)
		assert.Equal(t, "deleted.txt", status.Staged[0].Path)
		assert.Equal(t, ChangeDeleted, status.Staged[0].Status)
	})

	t.Run("parse complex status", func(t *testing.T) {
		output := `## feat/test...origin/feat/test [ahead 1]
A  newfile.txt
 M modified.txt
?? untracked.txt
`
		status := parseGitStatus(output)
		assert.Equal(t, "feat/test", status.Branch)
		assert.Equal(t, 1, status.Ahead)
		assert.Len(t, status.Staged, 1)
		assert.Len(t, status.Unstaged, 1)
		assert.Len(t, status.Untracked, 1)
	})
}

// TestCLIRunner_ContextTimeout tests context timeout behavior.
func TestCLIRunner_ContextTimeout(t *testing.T) {
	t.Run("timeout during status", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Give the context time to expire
		time.Sleep(2 * time.Millisecond)

		_, err = runner.Status(ctx)
		require.Error(t, err)
		// Should be context deadline exceeded or canceled
		isTimeout := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
		assert.True(t, isTimeout)
	})
}

// TestCLIRunner_ErrorWrapping tests that errors are properly wrapped.
func TestCLIRunner_ErrorWrapping(t *testing.T) {
	t.Run("git operation error is wrapped", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Try to push without remote - should fail with wrapped error
		err = runner.Push(context.Background(), "origin", "main", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})
}

// setupRepoWithRemote creates a test repo with a remote repository.
// Returns the path to the local repo.
func setupRepoWithRemote(t *testing.T) string {
	t.Helper()

	// Create remote (bare) repo
	remotePath := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "init", "--bare")
	cmd.Dir = remotePath
	require.NoError(t, cmd.Run(), "failed to init bare repo")

	// Create local repo
	localPath := setupTestRepo(t)

	// Add remote
	//nolint:gosec // G204: remotePath is from t.TempDir(), not user input
	cmd = exec.CommandContext(context.Background(), "git", "remote", "add", "origin", remotePath)
	cmd.Dir = localPath
	require.NoError(t, cmd.Run(), "failed to add remote")

	return localPath
}

// setupRepoForRebase creates a test repo with commits on both base and feature branches.
// Returns the repo path and branch names.
func setupRepoForRebase(t *testing.T, baseBranch, featureBranch string) string {
	t.Helper()

	repoPath := setupTestRepo(t)

	// Create initial commit on default branch
	createFile(t, repoPath, "base.txt", "base content")
	commitInitial(t, repoPath)

	// Rename default branch to baseBranch
	cmd := exec.CommandContext(context.Background(), "git", "branch", "-m", baseBranch) //nolint:gosec // G204: git args are controlled test inputs
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run(), "failed to rename branch")

	// Create feature branch
	cmd = exec.CommandContext(context.Background(), "git", "checkout", "-b", featureBranch) //nolint:gosec // G204: git args are controlled test inputs
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run(), "failed to create feature branch")

	// Add commit on feature branch
	createFile(t, repoPath, "feature.txt", "feature content")
	cmd = exec.CommandContext(context.Background(), "git", "add", ".")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "feature commit")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())

	return repoPath
}

// createRebaseConflict creates a rebase conflict scenario.
func createRebaseConflict(t *testing.T, repoPath, baseBranch, featureBranch string) {
	t.Helper()

	// Go to base branch and modify the same file
	cmd := exec.CommandContext(context.Background(), "git", "checkout", baseBranch) //nolint:gosec // G204: git args are controlled test inputs
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())

	createFile(t, repoPath, "conflict.txt", "base version")
	cmd = exec.CommandContext(context.Background(), "git", "add", ".")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "base commit")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())

	// Go to feature branch and modify the same file differently
	cmd = exec.CommandContext(context.Background(), "git", "checkout", featureBranch) //nolint:gosec // G204: git args are controlled test inputs
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())

	createFile(t, repoPath, "conflict.txt", "feature version")
	cmd = exec.CommandContext(context.Background(), "git", "add", ".")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "feature conflict commit")
	cmd.Dir = repoPath
	require.NoError(t, cmd.Run())
}

// TestCLIRunner_Fetch tests the Fetch method.
func TestCLIRunner_Fetch(t *testing.T) {
	t.Run("fetch from origin", func(t *testing.T) {
		localPath := setupRepoWithRemote(t)

		// Create and push initial commit
		createFile(t, localPath, "file.txt", "content")
		commitInitial(t, localPath)
		cmd := exec.CommandContext(context.Background(), "git", "push", "-u", "origin", "master")
		cmd.Dir = localPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(context.Background(), localPath)
		require.NoError(t, err)

		// Fetch should succeed
		err = runner.Fetch(context.Background(), "origin")
		require.NoError(t, err)
	})

	t.Run("fetch with empty remote defaults to origin", func(t *testing.T) {
		localPath := setupRepoWithRemote(t)
		createFile(t, localPath, "file.txt", "content")
		commitInitial(t, localPath)

		runner, err := NewRunner(context.Background(), localPath)
		require.NoError(t, err)

		// Empty remote should default to "origin"
		err = runner.Fetch(context.Background(), "")
		require.NoError(t, err)
	})

	t.Run("fetch from non-existent remote", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Fetch(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Fetch(ctx, "origin")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_Rebase tests the Rebase method.
func TestCLIRunner_Rebase(t *testing.T) {
	t.Run("successful rebase", func(t *testing.T) {
		repoPath := setupRepoForRebase(t, "main", "feature")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Rebase feature onto main (should succeed)
		err = runner.Rebase(context.Background(), "main")
		require.NoError(t, err)
	})

	t.Run("rebase with conflicts", func(t *testing.T) {
		repoPath := setupRepoForRebase(t, "main", "feature")
		createRebaseConflict(t, repoPath, "main", "feature")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Rebase should fail with conflict error
		err = runner.Rebase(context.Background(), "main")
		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrRebaseConflict)

		// Clean up - abort the rebase
		_ = runner.RebaseAbort(context.Background())
	})

	t.Run("empty onto parameter error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Rebase(context.Background(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Rebase(ctx, "main")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_RebaseAbort tests the RebaseAbort method.
func TestCLIRunner_RebaseAbort(t *testing.T) {
	t.Run("abort rebase in progress", func(t *testing.T) {
		repoPath := setupRepoForRebase(t, "main", "feature")
		createRebaseConflict(t, repoPath, "main", "feature")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Start a rebase that will conflict
		_ = runner.Rebase(context.Background(), "main")

		// Abort should succeed
		err = runner.RebaseAbort(context.Background())
		require.NoError(t, err)
	})

	t.Run("no rebase in progress", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Should not error even if no rebase in progress
		err = runner.RebaseAbort(context.Background())
		require.NoError(t, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.RebaseAbort(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestParseGitStatus_EdgeCases tests edge cases in status parsing.
func TestParseGitStatus_EdgeCases(t *testing.T) {
	t.Run("empty output", func(t *testing.T) {
		status := parseGitStatus("")
		assert.NotNil(t, status)
		assert.Empty(t, status.Branch)
		assert.Empty(t, status.Staged)
		assert.Empty(t, status.Unstaged)
		assert.Empty(t, status.Untracked)
	})

	t.Run("branch only no tracking", func(t *testing.T) {
		output := "## main\n"
		status := parseGitStatus(output)
		assert.Equal(t, "main", status.Branch)
		assert.Equal(t, 0, status.Ahead)
		assert.Equal(t, 0, status.Behind)
	})

	t.Run("branch behind only", func(t *testing.T) {
		output := "## main...origin/main [behind 5]\n"
		status := parseGitStatus(output)
		assert.Equal(t, "main", status.Branch)
		assert.Equal(t, 0, status.Ahead)
		assert.Equal(t, 5, status.Behind)
	})

	t.Run("malformed branch line no bracket", func(t *testing.T) {
		output := "## main...origin/main\n"
		status := parseGitStatus(output)
		assert.Equal(t, "main", status.Branch)
		assert.Equal(t, 0, status.Ahead)
		assert.Equal(t, 0, status.Behind)
	})

	t.Run("malformed branch line incomplete bracket", func(t *testing.T) {
		output := "## main...origin/main [\n"
		status := parseGitStatus(output)
		assert.Equal(t, "main", status.Branch)
		assert.Equal(t, 0, status.Ahead)
		assert.Equal(t, 0, status.Behind)
	})

	t.Run("branch line too short after bracket", func(t *testing.T) {
		output := "## main...origin/main [a]\n"
		status := parseGitStatus(output)
		assert.Equal(t, "main", status.Branch)
	})

	t.Run("parse with both staged and unstaged changes", func(t *testing.T) {
		output := "## main\nMM file.txt\n"
		status := parseGitStatus(output)
		// MM means modified in index and in working tree
		require.Len(t, status.Staged, 1)
		require.Len(t, status.Unstaged, 1)
		assert.Equal(t, "file.txt", status.Staged[0].Path)
		assert.Equal(t, "file.txt", status.Unstaged[0].Path)
	})

	t.Run("short line ignored", func(t *testing.T) {
		output := "## main\nX\n"
		status := parseGitStatus(output)
		assert.Empty(t, status.Staged)
		assert.Empty(t, status.Unstaged)
		assert.Empty(t, status.Untracked)
	})
}

// TestParseAheadBehind_EdgeCases tests edge cases in ahead/behind parsing.
func TestParseAheadBehind_EdgeCases(t *testing.T) {
	t.Run("non-numeric ahead value", func(t *testing.T) {
		n := parseAheadBehind("ahead xyz", "ahead ")
		assert.Equal(t, 0, n)
	})

	t.Run("prefix not found", func(t *testing.T) {
		n := parseAheadBehind("behind 5", "ahead ")
		assert.Equal(t, 0, n)
	})

	t.Run("empty info string", func(t *testing.T) {
		n := parseAheadBehind("", "ahead ")
		assert.Equal(t, 0, n)
	})

	t.Run("ahead with trailing comma", func(t *testing.T) {
		n := parseAheadBehind("ahead 3, behind 2", "ahead ")
		assert.Equal(t, 3, n)
	})

	t.Run("behind with trailing text", func(t *testing.T) {
		n := parseAheadBehind("behind 7", "behind ")
		assert.Equal(t, 7, n)
	})
}

// TestCLIRunner_PushWithRemote tests push with actual remote.
func TestCLIRunner_PushWithRemote(t *testing.T) {
	t.Run("successful push", func(t *testing.T) {
		localPath := setupRepoWithRemote(t)
		createFile(t, localPath, "file.txt", "content")
		commitInitial(t, localPath)

		runner, err := NewRunner(context.Background(), localPath)
		require.NoError(t, err)

		// Get the actual branch name (could be master or main)
		branch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)

		// Push with set-upstream
		err = runner.Push(context.Background(), "origin", branch, true)
		require.NoError(t, err)
	})

	t.Run("push without set-upstream", func(t *testing.T) {
		localPath := setupRepoWithRemote(t)
		createFile(t, localPath, "file.txt", "content")
		commitInitial(t, localPath)

		runner, err := NewRunner(context.Background(), localPath)
		require.NoError(t, err)

		branch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)

		// First push needs upstream
		err = runner.Push(context.Background(), "origin", branch, true)
		require.NoError(t, err)

		// Second push doesn't need upstream flag
		createFile(t, localPath, "file2.txt", "content2")
		cmd := exec.CommandContext(context.Background(), "git", "add", ".")
		cmd.Dir = localPath
		require.NoError(t, cmd.Run())
		cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "second commit")
		cmd.Dir = localPath
		require.NoError(t, cmd.Run())

		err = runner.Push(context.Background(), "origin", branch, false)
		require.NoError(t, err)
	})
}

// TestCLIRunner_BranchExists_AllCases tests all BranchExists scenarios.
func TestCLIRunner_BranchExists_AllCases(t *testing.T) {
	t.Run("branch exists returns true", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		// Create a test branch
		cmd := exec.CommandContext(context.Background(), "git", "branch", "test-branch")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		exists, err := runner.BranchExists(context.Background(), "test-branch")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("branch does not exist returns false", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		exists, err := runner.BranchExists(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

// TestCLIRunner_Add_EmptyPaths tests Add with empty paths.
func TestCLIRunner_Add_EmptyPaths(t *testing.T) {
	t.Run("add with empty slice", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Empty slice should stage all (-A flag)
		err = runner.Add(context.Background(), []string{})
		require.NoError(t, err)

		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.True(t, status.HasStagedChanges())
	})
}

// TestCLIRunner_ResetFiles tests the ResetFiles method.
func TestCLIRunner_ResetFiles(t *testing.T) {
	t.Run("unstage single file", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file1.txt", "content1")
		createFile(t, repoPath, "file2.txt", "content2")
		commitInitial(t, repoPath)

		// Create and stage new files
		createFile(t, repoPath, "newfile1.txt", "new1")
		createFile(t, repoPath, "newfile2.txt", "new2")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Stage both files
		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		// Verify both are staged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 2)

		// Unstage only newfile1.txt
		err = runner.ResetFiles(context.Background(), []string{"newfile1.txt"})
		require.NoError(t, err)

		// Verify newfile1.txt is unstaged (untracked), newfile2.txt still staged
		status, err = runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 1)
		assert.Equal(t, "newfile2.txt", status.Staged[0].Path)
		assert.Contains(t, status.Untracked, "newfile1.txt")
	})

	t.Run("unstage multiple files", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial")
		commitInitial(t, repoPath)

		// Create and stage multiple new files
		createFile(t, repoPath, "garbage1.txt", "garbage1")
		createFile(t, repoPath, "garbage2.txt", "garbage2")
		createFile(t, repoPath, "keep.txt", "keep")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Stage all files
		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		// Unstage the garbage files
		err = runner.ResetFiles(context.Background(), []string{"garbage1.txt", "garbage2.txt"})
		require.NoError(t, err)

		// Verify only keep.txt is staged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 1)
		assert.Equal(t, "keep.txt", status.Staged[0].Path)
		assert.Contains(t, status.Untracked, "garbage1.txt")
		assert.Contains(t, status.Untracked, "garbage2.txt")
	})

	t.Run("empty paths does nothing", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		createFile(t, repoPath, "newfile.txt", "new")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		// Empty paths should be a no-op
		err = runner.ResetFiles(context.Background(), []string{})
		require.NoError(t, err)

		// File should still be staged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 1)
	})

	t.Run("nil paths does nothing", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		createFile(t, repoPath, "newfile.txt", "new")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		// Nil paths should be a no-op
		err = runner.ResetFiles(context.Background(), nil)
		require.NoError(t, err)

		// File should still be staged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 1)
	})

	t.Run("unstage modified file", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "initial content")
		commitInitial(t, repoPath)

		// Modify and stage the file
		createFile(t, repoPath, "file.txt", "modified content")

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), []string{"file.txt"})
		require.NoError(t, err)

		// Verify it's staged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Len(t, status.Staged, 1)

		// Unstage it
		err = runner.ResetFiles(context.Background(), []string{"file.txt"})
		require.NoError(t, err)

		// Should now be unstaged (modified in working tree)
		status, err = runner.Status(context.Background())
		require.NoError(t, err)
		assert.Empty(t, status.Staged)
		assert.Len(t, status.Unstaged, 1)
	})

	t.Run("unstage file in subdirectory", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial")
		commitInitial(t, repoPath)

		// Create subdirectory and file
		subdir := filepath.Join(repoPath, "internal", "garbage")
		require.NoError(t, os.MkdirAll(subdir, 0o750))
		garbageFile := filepath.Join(subdir, ".env")
		require.NoError(t, os.WriteFile(garbageFile, []byte("SECRET=value"), 0o600))

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// Stage all
		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		// Unstage the garbage file by path
		err = runner.ResetFiles(context.Background(), []string{"internal/garbage/.env"})
		require.NoError(t, err)

		// Verify it's unstaged
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.Empty(t, status.Staged)
		assert.Contains(t, status.Untracked, "internal/garbage/.env")
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.ResetFiles(ctx, []string{"file.txt"})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("non-existent file in staging is no-op", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "initial.txt", "initial")
		commitInitial(t, repoPath)

		runner, err := NewRunner(context.Background(), repoPath)
		require.NoError(t, err)

		// git reset -- nonexistent.txt doesn't error if file doesn't exist in staging
		// This is expected behavior - it's a no-op for files not in the index
		err = runner.ResetFiles(context.Background(), []string{"nonexistent.txt"})
		require.NoError(t, err)
	})
}
