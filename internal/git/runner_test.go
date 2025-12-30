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

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)
		assert.NotNil(t, runner)
	})

	t.Run("error with empty path", func(t *testing.T) {
		runner, err := NewRunner("")
		assert.Nil(t, runner)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("error with non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		runner, err := NewRunner(tmpDir)
		assert.Nil(t, runner)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrNotGitRepo)
	})

	t.Run("error with non-existent path", func(t *testing.T) {
		runner, err := NewRunner("/nonexistent/path/to/repo")
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

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		status, err := runner.Status(ctx)
		assert.Nil(t, status)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("branch info with no commits", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
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
		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		err = runner.Commit(context.Background(), "test commit", nil)
		require.NoError(t, err)

		// Verify commit was made
		status, err := runner.Status(context.Background())
		require.NoError(t, err)
		assert.True(t, status.IsClean())
	})

	t.Run("commit with trailers", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		trailers := map[string]string{
			"ATLAS-Task":     "task-abc-xyz",
			"ATLAS-Template": "bugfix",
		}
		err = runner.Commit(context.Background(), "fix: resolve issue", trailers)
		require.NoError(t, err)

		// Verify trailers in commit message
		cmd := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%B")
		cmd.Dir = repoPath
		output, cmdErr := cmd.Output()
		require.NoError(t, cmdErr)
		commitMsg := string(output)
		assert.Contains(t, commitMsg, "ATLAS-Task: task-abc-xyz")
		assert.Contains(t, commitMsg, "ATLAS-Template: bugfix")
	})

	t.Run("empty commit message error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.Add(context.Background(), nil)
		require.NoError(t, err)

		err = runner.Commit(context.Background(), "", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("commit with nothing staged error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.Commit(context.Background(), "empty commit", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Commit(ctx, "test", nil)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_CurrentBranch tests the CurrentBranch method.
func TestCLIRunner_CurrentBranch(t *testing.T) {
	t.Run("get current branch", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(repoPath)
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

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		_, err = runner.CurrentBranch(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.CurrentBranch(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_CreateBranch tests the CreateBranch method.
func TestCLIRunner_CreateBranch(t *testing.T) {
	t.Run("create new branch", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.CreateBranch(context.Background(), "feat/new-feature")
		require.NoError(t, err)

		// Verify we're on the new branch
		branch, err := runner.CurrentBranch(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "feat/new-feature", branch)
	})

	t.Run("branch already exists error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		// Create a branch first
		cmd := exec.CommandContext(context.Background(), "git", "branch", "existing-branch")
		cmd.Dir = repoPath
		require.NoError(t, cmd.Run())

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.CreateBranch(context.Background(), "existing-branch")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrBranchExists)
	})

	t.Run("empty branch name error", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		err = runner.CreateBranch(context.Background(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.CreateBranch(ctx, "new-branch")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_Diff tests the Diff method.
func TestCLIRunner_Diff(t *testing.T) {
	t.Run("no diff on clean repo", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		diff, err := runner.Diff(context.Background(), false)
		require.NoError(t, err)
		assert.Empty(t, diff)

		diff, err = runner.Diff(context.Background(), true)
		require.NoError(t, err)
		assert.Empty(t, diff)
	})

	t.Run("unstaged diff", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "initial content")
		commitInitial(t, repoPath)
		createFile(t, repoPath, "file.txt", "modified content")

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		diff, err := runner.Diff(context.Background(), false)
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

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		// Unstaged diff should be empty
		diff, err := runner.Diff(context.Background(), false)
		require.NoError(t, err)
		assert.Empty(t, diff)

		// Staged diff should show changes
		diff, err = runner.Diff(context.Background(), true)
		require.NoError(t, err)
		assert.Contains(t, diff, "file.txt")
		assert.Contains(t, diff, "+modified content")
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.Diff(ctx, false)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestCLIRunner_Push tests the Push method.
func TestCLIRunner_Push(t *testing.T) {
	t.Run("push error without remote", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		createFile(t, repoPath, "file.txt", "content")
		commitInitial(t, repoPath)

		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		// Should fail because there's no remote
		err = runner.Push(context.Background(), "origin", "master", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)
		runner, err := NewRunner(repoPath)
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
		runner, err := NewRunner(repoPath)
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
		runner, err := NewRunner(repoPath)
		require.NoError(t, err)

		// Try to push without remote - should fail with wrapped error
		err = runner.Push(context.Background(), "origin", "main", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrGitOperation)
	})
}
