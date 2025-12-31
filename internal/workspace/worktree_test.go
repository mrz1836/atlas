package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// createTestRepo creates a temporary git repository for testing.
// Returns the repo path and a cleanup function.
func createTestRepo(t *testing.T) string {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo
	runGit(t, tmpDir, "init")

	// Configure git user for commits
	runGit(t, tmpDir, "config", "user.email", "test@test.com")
	runGit(t, tmpDir, "config", "user.name", "Test")

	// Create initial commit (required for worktrees)
	readme := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readme, []byte("# Test"), 0o600)
	require.NoError(t, err)

	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "Initial commit")

	return tmpDir
}

// runGit runs a git command in the specified directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, out)
	}
}

func TestNewGitWorktreeRunner(t *testing.T) {
	t.Run("with valid git repo", func(t *testing.T) {
		repoPath := createTestRepo(t)

		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)
		assert.NotNil(t, runner)

		// Compare resolved paths to handle symlinks (e.g., /var -> /private/var on macOS)
		expectedPath, _ := filepath.EvalSymlinks(repoPath)
		actualPath, _ := filepath.EvalSymlinks(runner.repoPath)
		assert.Equal(t, expectedPath, actualPath)
	})

	t.Run("with non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		runner, err := NewGitWorktreeRunner(tmpDir)
		require.Error(t, err)
		assert.Nil(t, runner)
		assert.ErrorIs(t, err, atlaserrors.ErrNotGitRepo)
	})

	t.Run("with subdirectory of git repo", func(t *testing.T) {
		repoPath := createTestRepo(t)

		// Create a subdirectory
		subdir := filepath.Join(repoPath, "subdir")
		err := os.MkdirAll(subdir, 0o750)
		require.NoError(t, err)

		runner, err := NewGitWorktreeRunner(subdir)
		require.NoError(t, err)
		assert.NotNil(t, runner)

		// Should resolve to repo root (compare with symlink resolution)
		expectedPath, _ := filepath.EvalSymlinks(repoPath)
		actualPath, _ := filepath.EvalSymlinks(runner.repoPath)
		assert.Equal(t, expectedPath, actualPath)
	})
}

func TestDetectRepoRoot(t *testing.T) {
	t.Run("finds repo root from subdirectory", func(t *testing.T) {
		repoPath := createTestRepo(t)
		subdir := filepath.Join(repoPath, "deep", "nested", "dir")
		err := os.MkdirAll(subdir, 0o750)
		require.NoError(t, err)

		root, err := DetectRepoRoot(context.Background(), subdir)
		require.NoError(t, err)

		// Compare resolved paths to handle symlinks
		expectedPath, _ := filepath.EvalSymlinks(repoPath)
		actualPath, _ := filepath.EvalSymlinks(root)
		assert.Equal(t, expectedPath, actualPath)
	})

	t.Run("returns error for non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := DetectRepoRoot(context.Background(), tmpDir)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrNotGitRepo)
	})
}

func TestSiblingPath(t *testing.T) {
	tests := []struct {
		name          string
		repoRoot      string
		workspaceName string
		expected      string
	}{
		{
			name:          "simple case",
			repoRoot:      "/Users/dev/projects/atlas",
			workspaceName: "auth",
			expected:      "/Users/dev/projects/atlas-auth",
		},
		{
			name:          "with dashes in repo name",
			repoRoot:      "/home/user/my-project",
			workspaceName: "feature",
			expected:      "/home/user/my-project-feature",
		},
		{
			name:          "with dashes in workspace name",
			repoRoot:      "/path/to/repo",
			workspaceName: "user-auth",
			expected:      "/path/to/repo-user-auth",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SiblingPath(tc.repoRoot, tc.workspaceName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name          string
		branchType    string
		workspaceName string
		expected      string
	}{
		{
			name:          "feat with simple name",
			branchType:    "feat",
			workspaceName: "auth",
			expected:      "feat/auth",
		},
		{
			name:          "fix with uppercase",
			branchType:    "fix",
			workspaceName: "NullPointer",
			expected:      "fix/nullpointer",
		},
		{
			name:          "chore with spaces",
			branchType:    "chore",
			workspaceName: "update deps",
			expected:      "chore/update-deps",
		},
		{
			name:          "feat with special chars",
			branchType:    "feat",
			workspaceName: "user@auth!system",
			expected:      "feat/user-auth-system",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := generateBranchName(tc.branchType, tc.workspaceName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEnsureUniquePath(t *testing.T) {
	t.Run("returns base path if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		basePath := filepath.Join(tmpDir, "new-path")

		result, err := ensureUniquePath(basePath)
		require.NoError(t, err)
		assert.Equal(t, basePath, result)
	})

	t.Run("appends -2 if base exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		basePath := filepath.Join(tmpDir, "existing")

		// Create the base path
		err := os.MkdirAll(basePath, 0o750)
		require.NoError(t, err)

		result, err := ensureUniquePath(basePath)
		require.NoError(t, err)
		assert.Equal(t, basePath+"-2", result)
	})

	t.Run("appends -3 if -2 also exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		basePath := filepath.Join(tmpDir, "existing")

		// Create base and -2
		err := os.MkdirAll(basePath, 0o750)
		require.NoError(t, err)
		err = os.MkdirAll(basePath+"-2", 0o750)
		require.NoError(t, err)

		result, err := ensureUniquePath(basePath)
		require.NoError(t, err)
		assert.Equal(t, basePath+"-3", result)
	})
}

func TestParseWorktreeList(t *testing.T) {
	t.Run("parses multiple worktrees", func(t *testing.T) {
		output := `worktree /path/to/main
HEAD abc123def456
branch refs/heads/main

worktree /path/to/feature
HEAD def456abc789
branch refs/heads/feat/auth

worktree /path/to/detached
HEAD 789abcdef123
detached
`
		result := parseWorktreeList(output)
		require.Len(t, result, 3)

		assert.Equal(t, "/path/to/main", result[0].Path)
		assert.Equal(t, "abc123def456", result[0].HeadCommit)
		assert.Equal(t, "main", result[0].Branch)

		assert.Equal(t, "/path/to/feature", result[1].Path)
		assert.Equal(t, "def456abc789", result[1].HeadCommit)
		assert.Equal(t, "feat/auth", result[1].Branch)

		assert.Equal(t, "/path/to/detached", result[2].Path)
		assert.Equal(t, "789abcdef123", result[2].HeadCommit)
		assert.Empty(t, result[2].Branch) // detached HEAD has no branch
	})

	t.Run("handles prunable and locked worktrees", func(t *testing.T) {
		output := `worktree /path/to/stale
HEAD abc123
branch refs/heads/stale
prunable

worktree /path/to/locked
HEAD def456
branch refs/heads/locked
locked
`
		result := parseWorktreeList(output)
		require.Len(t, result, 2)

		assert.True(t, result[0].IsPrunable)
		assert.False(t, result[0].IsLocked)

		assert.False(t, result[1].IsPrunable)
		assert.True(t, result[1].IsLocked)
	})

	t.Run("handles empty output", func(t *testing.T) {
		result := parseWorktreeList("")
		assert.Empty(t, result)
	})
}

func TestGitWorktreeRunner_Create(t *testing.T) {
	t.Run("creates new worktree successfully", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "auth",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		assert.NotEmpty(t, info.Path)
		assert.Equal(t, "feat/auth", info.Branch)
		assert.False(t, info.CreatedAt.IsZero())

		// Verify worktree was created
		_, err = os.Stat(info.Path)
		require.NoError(t, err)

		// Verify branch exists
		exists, err := runner.BranchExists(context.Background(), "feat/auth")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("creates worktree with sibling path", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "test",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Path should contain the workspace name
		assert.Contains(t, info.Path, "-test")

		// Worktree directory should exist
		_, err = os.Stat(info.Path)
		require.NoError(t, err)
	})

	t.Run("appends numeric suffix for existing path", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create first worktree
		info1, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "dup",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Create a blocking directory where -2 would go
		path2 := info1.Path + "-2"
		err = os.MkdirAll(path2, 0o750)
		require.NoError(t, err)

		// Create second worktree with same name - should get -3
		info3, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "dup",
			BranchType:    "fix",
		})
		require.NoError(t, err)

		assert.True(t, strings.HasSuffix(info3.Path, "-3"))
	})

	t.Run("appends timestamp for existing branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create first worktree
		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "same",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Create second worktree with same branch type/name
		// This will have existing branch "feat/same", so should get timestamp suffix
		info2, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "same",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Branch should have timestamp suffix
		assert.True(t, strings.HasPrefix(info2.Branch, "feat/same-"))
		assert.Greater(t, len(info2.Branch), len("feat/same"))
	})

	t.Run("returns error for empty workspace name", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "",
			BranchType:    "feat",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("returns error for empty branch type", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "valid-name",
			BranchType:    "",
		})
		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
		assert.Contains(t, err.Error(), "branch type")
	})

	t.Run("returns error for workspace name exceeding max length", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		longName := strings.Repeat("a", 256) // exceeds maxWorkspaceNameLength (255)
		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: longName,
			BranchType:    "feat",
		})
		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
		assert.Contains(t, err.Error(), "maximum length")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = runner.Create(ctx, WorktreeCreateOptions{
			WorkspaceName: "canceled",
			BranchType:    "feat",
		})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("creates worktree from base branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create a branch to use as base
		runGit(t, repoPath, "checkout", "-b", "develop")
		runGit(t, repoPath, "checkout", "-")

		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "feature",
			BranchType:    "feat",
			BaseBranch:    "develop",
		})
		require.NoError(t, err)
		assert.Equal(t, "feat/feature", info.Branch)
	})

	t.Run("cleans up on failure (atomic creation)", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Get the expected worktree path
		expectedPath := SiblingPath(repoPath, "atomic-test")

		// Verify path doesn't exist before
		_, err = os.Stat(expectedPath)
		require.True(t, os.IsNotExist(err), "worktree path should not exist before test")

		// Try to create with invalid base branch - this will fail
		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "atomic-test",
			BranchType:    "feat",
			BaseBranch:    "nonexistent-branch-xyz123",
		})
		require.Error(t, err, "Create should fail with invalid base branch")

		// Verify path is cleaned up after failure
		_, err = os.Stat(expectedPath)
		assert.True(t, os.IsNotExist(err), "worktree path should be cleaned up after failure")
	})
}

func TestGitWorktreeRunner_List(t *testing.T) {
	t.Run("lists main worktree only", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		worktrees, err := runner.List(context.Background())
		require.NoError(t, err)
		require.Len(t, worktrees, 1)

		// Compare using resolved paths to handle symlinks (e.g., /var -> /private/var on macOS)
		expectedPath, _ := filepath.EvalSymlinks(repoPath)
		actualPath, _ := filepath.EvalSymlinks(worktrees[0].Path)
		assert.Equal(t, expectedPath, actualPath)
	})

	t.Run("lists multiple worktrees", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create some worktrees
		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "wt1",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		_, err = runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "wt2",
			BranchType:    "fix",
		})
		require.NoError(t, err)

		worktrees, err := runner.List(context.Background())
		require.NoError(t, err)
		require.Len(t, worktrees, 3) // main + 2 created

		// Verify we have expected branches
		branches := make([]string, 0, 3)
		for _, wt := range worktrees {
			branches = append(branches, wt.Branch)
		}
		assert.Contains(t, branches, "feat/wt1")
		assert.Contains(t, branches, "fix/wt2")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.List(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestGitWorktreeRunner_Remove(t *testing.T) {
	t.Run("removes clean worktree", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create worktree
		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "toremove",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Remove it
		err = runner.Remove(context.Background(), info.Path, false)
		require.NoError(t, err)

		// Verify it's gone
		_, err = os.Stat(info.Path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("returns error for dirty worktree without force", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create worktree
		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "dirty",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Make it dirty by adding a new file
		newFile := filepath.Join(info.Path, "dirty.txt")
		err = os.WriteFile(newFile, []byte("dirty"), 0o600)
		require.NoError(t, err)

		// Try to remove without force
		err = runner.Remove(context.Background(), info.Path, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrWorktreeDirty)
	})

	t.Run("removes dirty worktree with force", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create worktree
		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "dirty2",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Make it dirty
		newFile := filepath.Join(info.Path, "dirty.txt")
		err = os.WriteFile(newFile, []byte("dirty"), 0o600)
		require.NoError(t, err)

		// Remove with force
		err = runner.Remove(context.Background(), info.Path, true)
		require.NoError(t, err)

		// Verify it's gone
		_, err = os.Stat(info.Path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("returns error for main repo", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		err = runner.Remove(context.Background(), repoPath, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrNotAWorktree)
	})

	t.Run("returns error for non-worktree path", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		tmpDir := t.TempDir()
		err = runner.Remove(context.Background(), tmpDir, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrNotAWorktree)
	})
}

func TestGitWorktreeRunner_Prune(t *testing.T) {
	t.Run("prunes stale worktrees", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create worktree
		info, err := runner.Create(context.Background(), WorktreeCreateOptions{
			WorkspaceName: "stale",
			BranchType:    "feat",
		})
		require.NoError(t, err)

		// Manually remove the directory (simulating stale worktree)
		err = os.RemoveAll(info.Path)
		require.NoError(t, err)

		// Prune should succeed
		err = runner.Prune(context.Background())
		require.NoError(t, err)

		// Verify worktree entry is pruned (list should only show main)
		worktrees, err := runner.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, worktrees, 1)
	})

	t.Run("succeeds when nothing to prune", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		err = runner.Prune(context.Background())
		assert.NoError(t, err)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.Prune(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestGitWorktreeRunner_BranchExists(t *testing.T) {
	t.Run("returns true for existing branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create a branch
		runGit(t, repoPath, "branch", "test-branch")

		exists, err := runner.BranchExists(context.Background(), "test-branch")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false for non-existing branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		exists, err := runner.BranchExists(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("handles branches with slashes", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create a branch with slash
		runGit(t, repoPath, "branch", "feat/my-feature")

		exists, err := runner.BranchExists(context.Background(), "feat/my-feature")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.BranchExists(ctx, "any")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestGitWorktreeRunner_DeleteBranch(t *testing.T) {
	t.Run("deletes merged branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create and checkout a branch
		runGit(t, repoPath, "branch", "to-delete")

		err = runner.DeleteBranch(context.Background(), "to-delete", false)
		require.NoError(t, err)

		// Verify branch is gone
		exists, err := runner.BranchExists(context.Background(), "to-delete")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("force deletes unmerged branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		// Create a branch with a commit
		runGit(t, repoPath, "checkout", "-b", "unmerged")
		newFile := filepath.Join(repoPath, "new.txt")
		err = os.WriteFile(newFile, []byte("new"), 0o600)
		require.NoError(t, err)
		runGit(t, repoPath, "add", ".")
		runGit(t, repoPath, "commit", "-m", "new commit")
		runGit(t, repoPath, "checkout", "-")

		// Force delete should work
		err = runner.DeleteBranch(context.Background(), "unmerged", true)
		require.NoError(t, err)

		exists, err := runner.BranchExists(context.Background(), "unmerged")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		repoPath := createTestRepo(t)
		runner, err := NewGitWorktreeRunner(repoPath)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = runner.DeleteBranch(ctx, "any", false)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestGitRunCommand(t *testing.T) {
	t.Run("returns stdout for successful command", func(t *testing.T) {
		repoPath := createTestRepo(t)

		output, err := git.RunCommand(context.Background(), repoPath, "rev-parse", "--show-toplevel")
		require.NoError(t, err)

		// Compare resolved paths to handle symlinks (e.g., /var -> /private/var on macOS)
		expectedPath, _ := filepath.EvalSymlinks(repoPath)
		actualPath, _ := filepath.EvalSymlinks(output)
		assert.Equal(t, expectedPath, actualPath)
	})

	t.Run("returns error with stderr for failed command", func(t *testing.T) {
		repoPath := createTestRepo(t)

		_, err := git.RunCommand(context.Background(), repoPath, "show-ref", "--verify", "refs/heads/nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git show-ref failed")
	})

	t.Run("respects context timeout", func(t *testing.T) {
		repoPath := createTestRepo(t)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // Ensure timeout triggers

		_, err := git.RunCommand(ctx, repoPath, "status")
		assert.Error(t, err)
	})
}
