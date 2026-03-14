package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestDetectRepo(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) string
		wantErr    bool
		errType    error
		isWorktree bool
	}{
		{
			name: "main repository",
			setup: func(t *testing.T) string {
				t.Helper()
				return setupTestRepo(t)
			},
			wantErr:    false,
			isWorktree: false,
		},
		{
			name: "linked worktree",
			setup: func(t *testing.T) string {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)

				// Create a worktree
				wtPath := filepath.Join(t.TempDir(), "worktree")
				cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", wtPath, "-b", "feature") //#nosec G204 -- test code with safe inputs
				cmd.Dir = repoPath
				require.NoError(t, cmd.Run(), "failed to create worktree")

				return wtPath
			},
			wantErr:    false,
			isWorktree: true,
		},
		{
			name: "not a git repo",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: true,
			errType: atlaserrors.ErrNotGitRepo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			info, err := DetectRepo(context.Background(), path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					require.ErrorIs(t, err, tt.errType)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.NotEmpty(t, info.Root)
			assert.NotEmpty(t, info.WorktreePath)
			assert.NotEmpty(t, info.CommonDir)
			assert.Equal(t, tt.isWorktree, info.IsWorktree)

			if tt.isWorktree {
				// In a worktree, Root should be the main repo path
				// and WorktreePath should be the worktree path
				assert.NotEqual(t, info.Root, info.WorktreePath)
			} else {
				// In a main repo, Root and WorktreePath should be the same
				assert.Equal(t, info.Root, info.WorktreePath)
			}
		})
	}
}

func TestDetectRepo_ContextCancellation(t *testing.T) {
	repoPath := setupTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := DetectRepo(ctx, repoPath)
	require.Error(t, err)
	// Git command should fail due to context cancellation
}

func TestListWorktrees(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantLen int
		wantErr bool
	}{
		{
			name: "no worktrees",
			setup: func(t *testing.T) string {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)
				return repoPath
			},
			wantLen: 1, // Main worktree always listed
			wantErr: false,
		},
		{
			name: "single worktree",
			setup: func(t *testing.T) string {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)

				wtPath := filepath.Join(t.TempDir(), "worktree1")
				cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", wtPath, "-b", "feature1") //#nosec G204 -- test code with safe inputs
				cmd.Dir = repoPath
				require.NoError(t, cmd.Run())

				return repoPath
			},
			wantLen: 2, // Main + 1 worktree
			wantErr: false,
		},
		{
			name: "multiple worktrees",
			setup: func(t *testing.T) string {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)

				// Create multiple worktrees
				for i := 1; i <= 3; i++ {
					wtPath := filepath.Join(t.TempDir(), "worktree"+string(rune('0'+i)))
					cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", wtPath, "-b", "feature"+string(rune('0'+i))) //#nosec G204 -- test code with safe inputs
					cmd.Dir = repoPath
					require.NoError(t, cmd.Run())
				}

				return repoPath
			},
			wantLen: 4, // Main + 3 worktrees
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			worktrees, err := ListWorktrees(context.Background(), path)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, worktrees, tt.wantLen)

			// Verify structure of returned worktrees
			for _, wt := range worktrees {
				assert.NotEmpty(t, wt.Path, "worktree should have a path")
				assert.NotEmpty(t, wt.Head, "worktree should have a HEAD commit")
				// Branch may be empty for detached HEAD, so don't assert it
			}
		})
	}
}

func TestFindWorktreeByName(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (repoPath, searchName string)
		wantErr   bool
		errType   error
		checkFunc func(t *testing.T, wt *WorktreeEntry)
	}{
		{
			name: "exact name match",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)

				wtPath := filepath.Join(t.TempDir(), "feature-auth")
				cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", wtPath, "-b", "feature-auth") //#nosec G204 -- test code with safe inputs
				cmd.Dir = repoPath
				require.NoError(t, cmd.Run())

				return repoPath, "feature-auth"
			},
			wantErr: false,
			checkFunc: func(t *testing.T, wt *WorktreeEntry) {
				t.Helper()
				assert.Contains(t, wt.Path, "feature-auth")
			},
		},
		{
			name: "suffix match",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)

				wtPath := filepath.Join(t.TempDir(), "atlas-bugfix")
				cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", wtPath, "-b", "bugfix") //#nosec G204 -- test code with safe inputs
				cmd.Dir = repoPath
				require.NoError(t, cmd.Run())

				return repoPath, "bugfix"
			},
			wantErr: false,
			checkFunc: func(t *testing.T, wt *WorktreeEntry) {
				t.Helper()
				assert.Contains(t, wt.Path, "bugfix")
			},
		},
		{
			name: "empty name error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return setupTestRepo(t), ""
			},
			wantErr: true,
			errType: atlaserrors.ErrEmptyValue,
		},
		{
			name: "not found error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				repoPath := setupTestRepo(t)
				createFile(t, repoPath, "README.md", "# Test")
				commitInitial(t, repoPath)

				return repoPath, "nonexistent-worktree"
			},
			wantErr: true,
			errType: atlaserrors.ErrWorktreeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath, searchName := tt.setup(t)

			wt, err := FindWorktreeByName(context.Background(), repoPath, searchName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					require.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, wt)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, wt)

			if tt.checkFunc != nil {
				tt.checkFunc(t, wt)
			}
		})
	}
}

func TestParseWorktreeListOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		validate func(t *testing.T, worktrees []WorktreeEntry)
	}{
		{
			name:    "empty output",
			input:   "",
			wantLen: 0,
		},
		{
			name: "single worktree",
			input: `worktree /path/to/repo
HEAD abc123def
branch refs/heads/main
`,
			wantLen: 1,
			validate: func(t *testing.T, worktrees []WorktreeEntry) {
				t.Helper()
				assert.Equal(t, "/path/to/repo", worktrees[0].Path)
				assert.Equal(t, "abc123def", worktrees[0].Head)
				assert.Equal(t, "main", worktrees[0].Branch)
				assert.False(t, worktrees[0].IsPrunable)
				assert.False(t, worktrees[0].IsLocked)
			},
		},
		{
			name: "multiple worktrees",
			input: `worktree /path/to/main
HEAD abc123
branch refs/heads/main

worktree /path/to/feature
HEAD def456
branch refs/heads/feat/auth

worktree /path/to/bugfix
HEAD ghi789
branch refs/heads/bugfix
`,
			wantLen: 3,
			validate: func(t *testing.T, worktrees []WorktreeEntry) {
				t.Helper()
				assert.Len(t, worktrees, 3)
				assert.Equal(t, "/path/to/main", worktrees[0].Path)
				assert.Equal(t, "main", worktrees[0].Branch)

				assert.Equal(t, "/path/to/feature", worktrees[1].Path)
				assert.Equal(t, "feat/auth", worktrees[1].Branch)

				assert.Equal(t, "/path/to/bugfix", worktrees[2].Path)
				assert.Equal(t, "bugfix", worktrees[2].Branch)
			},
		},
		{
			name: "prunable worktree",
			input: `worktree /path/to/missing
HEAD abc123
branch refs/heads/feature
prunable
`,
			wantLen: 1,
			validate: func(t *testing.T, worktrees []WorktreeEntry) {
				t.Helper()
				assert.True(t, worktrees[0].IsPrunable)
				assert.False(t, worktrees[0].IsLocked)
			},
		},
		{
			name: "locked worktree",
			input: `worktree /path/to/locked
HEAD abc123
branch refs/heads/feature
locked reason: in use
`,
			wantLen: 1,
			validate: func(t *testing.T, worktrees []WorktreeEntry) {
				t.Helper()
				assert.False(t, worktrees[0].IsPrunable)
				assert.True(t, worktrees[0].IsLocked)
			},
		},
		{
			name: "detached HEAD (no branch)",
			input: `worktree /path/to/detached
HEAD abc123
`,
			wantLen: 1,
			validate: func(t *testing.T, worktrees []WorktreeEntry) {
				t.Helper()
				assert.Equal(t, "/path/to/detached", worktrees[0].Path)
				assert.Equal(t, "abc123", worktrees[0].Head)
				assert.Empty(t, worktrees[0].Branch)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktrees := parseWorktreeListOutput(tt.input)

			assert.Len(t, worktrees, tt.wantLen)

			if tt.validate != nil {
				tt.validate(t, worktrees)
			}
		})
	}
}
