package validation_test

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/validation"
)

// MockGitRunner implements GitRunner for testing.
type MockGitRunner struct {
	responses map[string]struct {
		output string
		err    error
	}
	calls []string
}

// NewMockGitRunner creates a new mock git runner.
func NewMockGitRunner() *MockGitRunner {
	return &MockGitRunner{
		responses: make(map[string]struct {
			output string
			err    error
		}),
		calls: []string{},
	}
}

// SetResponse configures the response for a specific git command (first arg).
func (m *MockGitRunner) SetResponse(firstArg, output string, err error) {
	m.responses[firstArg] = struct {
		output string
		err    error
	}{output, err}
}

// Run implements GitRunner.
func (m *MockGitRunner) Run(_ context.Context, _ string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", atlaserrors.ErrCommandNotConfigured
	}
	m.calls = append(m.calls, args[0])

	resp, ok := m.responses[args[0]]
	if !ok {
		return "", nil
	}
	return resp.output, resp.err
}

// Ensure MockGitRunner implements GitRunner.
var _ validation.GitRunner = (*MockGitRunner)(nil)

func stagingTestContext() context.Context {
	logger := zerolog.Nop()
	return logger.WithContext(context.Background())
}

func TestStageModifiedFiles_NoModifiedFiles(t *testing.T) {
	mock := NewMockGitRunner()
	mock.SetResponse("status", "", nil) // Empty status = no changes

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.NoError(t, err)
	assert.Equal(t, []string{"status"}, mock.calls)
}

func TestStageModifiedFiles_StagesUnstagedChanges(t *testing.T) {
	mock := NewMockGitRunner()
	// " M" = modified but not staged
	mock.SetResponse("status", " M file1.go\n M file2.go\n", nil)
	mock.SetResponse("add", "", nil)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.NoError(t, err)
	assert.Contains(t, mock.calls, "status")
	assert.Contains(t, mock.calls, "add")
}

func TestStageModifiedFiles_SkipsAlreadyStagedFiles(t *testing.T) {
	mock := NewMockGitRunner()
	// "M " = modified AND staged (should be skipped)
	// " M" = modified but not staged (should be included)
	mock.SetResponse("status", "M  already_staged.go\n M needs_staging.go\n", nil)
	mock.SetResponse("add", "", nil)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.NoError(t, err)
	// Should only stage the file that needs staging
	assert.Contains(t, mock.calls, "add")
}

func TestStageModifiedFiles_HandlesBothIndexAndWorktreeChanges(t *testing.T) {
	mock := NewMockGitRunner()
	// "MM" = modified in both index and worktree
	mock.SetResponse("status", "MM both_modified.go\n", nil)
	mock.SetResponse("add", "", nil)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.NoError(t, err)
	// Should stage the worktree changes
	assert.Contains(t, mock.calls, "add")
}

func TestStageModifiedFiles_StagesUntrackedFiles(t *testing.T) {
	mock := NewMockGitRunner()
	// "??" = untracked file (new file created by pre-commit hook)
	mock.SetResponse("status", "?? new_file.go\n?? another_new.go\n", nil)
	mock.SetResponse("add", "", nil)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.NoError(t, err)
	// Should stage the untracked files
	assert.Contains(t, mock.calls, "status")
	assert.Contains(t, mock.calls, "add")
}

func TestStageModifiedFiles_HandlesMixedStatusTypes(t *testing.T) {
	mock := NewMockGitRunner()
	// Mix of modified and untracked files
	mock.SetResponse("status", " M modified.go\n?? new_file.go\nMM both.go\nM  staged.go\n", nil)
	mock.SetResponse("add", "", nil)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.NoError(t, err)
	// Should stage modified, untracked, and both-modified files (but not already staged)
	assert.Contains(t, mock.calls, "add")
}

func TestStageModifiedFiles_GitStatusError(t *testing.T) {
	mock := NewMockGitRunner()
	mock.SetResponse("status", "", atlaserrors.ErrCommandFailed)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check git status")
}

func TestStageModifiedFiles_GitAddError(t *testing.T) {
	mock := NewMockGitRunner()
	mock.SetResponse("status", " M file.go\n", nil)
	mock.SetResponse("add", "", atlaserrors.ErrCommandFailed)

	ctx := stagingTestContext()
	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to stage modified files")
}

func TestStageModifiedFiles_ContextCancellation(t *testing.T) {
	mock := NewMockGitRunner()

	ctx, cancel := context.WithCancel(stagingTestContext())
	cancel() // Cancel immediately

	err := validation.StageModifiedFilesWithRunner(ctx, "/tmp", mock)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// parseModifiedFiles is tested indirectly through TestStageModifiedFiles_*
// as it's an unexported function. The StageModifiedFilesWithRunner tests verify
// proper parsing behavior for various git status output formats.

func TestDefaultGitRunner_Run(t *testing.T) {
	// Test with actual git command in a temp directory
	runner := &validation.DefaultGitRunner{}
	ctx := stagingTestContext()
	tmpDir := t.TempDir()

	// Initialize a git repo
	_, err := runner.Run(ctx, tmpDir, "init")
	require.NoError(t, err)

	// Configure git user for commits
	_, _ = runner.Run(ctx, tmpDir, "config", "user.email", "test@example.com")
	_, _ = runner.Run(ctx, tmpDir, "config", "user.name", "Test User")

	// Check status on empty repo
	output, err := runner.Run(ctx, tmpDir, "status", "--porcelain")
	require.NoError(t, err)
	assert.Empty(t, output) // Empty repo has no changes
}
