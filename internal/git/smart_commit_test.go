package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockAIRunner is a mock implementation of ai.Runner for testing.
type mockAIRunner struct {
	response *domain.AIResult
	err      error
}

func (m *mockAIRunner) Run(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// Compile-time interface check for mockAIRunner.
var _ ai.Runner = (*mockAIRunner)(nil)

func TestNewSmartCommitRunner(t *testing.T) {
	// Create a temp git repo
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)
	require.NotNil(t, runner)
	assert.Equal(t, tmpDir, runner.workDir)
	assert.NotNil(t, runner.garbageDetector)
}

func TestSmartCommitRunner_Options(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	artifactsDir := filepath.Join(tmpDir, "artifacts")
	customConfig := &GarbageConfig{DebugPatterns: []string{"*.debug"}}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil,
		WithTaskID("task-test-abc"),
		WithTemplateName("bugfix"),
		WithArtifactsDir(artifactsDir),
		WithGarbageConfig(customConfig),
		WithAgent("claude"),
		WithModel("haiku"),
	)

	assert.Equal(t, "task-test-abc", runner.taskID)
	assert.Equal(t, "bugfix", runner.templateName)
	assert.Equal(t, artifactsDir, runner.artifactsDir)
	assert.Equal(t, []string{"*.debug"}, runner.garbageDetector.config.DebugPatterns)
	assert.Equal(t, "claude", runner.agent)
	assert.Equal(t, "haiku", runner.model)
}

func TestSmartCommitRunner_Analyze_EmptyRepo(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	analysis, err := runner.Analyze(context.Background())
	require.NoError(t, err)
	assert.Empty(t, analysis.FileGroups)
	assert.Empty(t, analysis.GarbageFiles)
	assert.Equal(t, 0, analysis.TotalChanges)
	assert.False(t, analysis.HasGarbage)
}

func TestSmartCommitRunner_Analyze_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create some files
	internalDir := filepath.Join(tmpDir, "internal", "git")
	require.NoError(t, os.MkdirAll(internalDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(internalDir, "runner.go"), []byte("package git"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(internalDir, "runner_test.go"), []byte("package git"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	analysis, err := runner.Analyze(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, analysis.TotalChanges)
	assert.Len(t, analysis.FileGroups, 1)
	assert.Equal(t, "internal/git", analysis.FileGroups[0].Package)
	assert.False(t, analysis.HasGarbage)
}

func TestSmartCommitRunner_Analyze_WithGarbage(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create normal and garbage files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=test"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "coverage.out"), []byte("coverage"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	analysis, err := runner.Analyze(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, analysis.TotalChanges)
	assert.True(t, analysis.HasGarbage)
	assert.Len(t, analysis.GarbageFiles, 2)

	// Verify garbage is filtered from groups
	totalFilesInGroups := 0
	for _, g := range analysis.FileGroups {
		totalFilesInGroups += len(g.Files)
	}
	assert.Equal(t, 1, totalFilesInGroups) // Only main.go
}

func TestSmartCommitRunner_Analyze_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = runner.Analyze(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSmartCommitRunner_Commit_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create files
	internalDir := filepath.Join(tmpDir, "internal", "git")
	require.NoError(t, os.MkdirAll(internalDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(internalDir, "runner.go"), []byte("package git"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil,
		WithTaskID("task-xyz"),
		WithTemplateName("feature"),
	)

	result, err := runner.Commit(context.Background(), CommitOptions{
		DryRun: true,
	})

	require.NoError(t, err)
	assert.Len(t, result.Commits, 1)
	assert.Equal(t, "(dry-run)", result.Commits[0].Hash)
	assert.Contains(t, result.Commits[0].Message, "feat(git)")
	assert.Equal(t, 1, result.TotalFiles)
}

func TestSmartCommitRunner_Commit_SingleCommit(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create files in different packages
	gitDir := filepath.Join(tmpDir, "internal", "git")
	configDir := filepath.Join(tmpDir, "internal", "config")
	require.NoError(t, os.MkdirAll(gitDir, 0o750))
	require.NoError(t, os.MkdirAll(configDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "runner.go"), []byte("package git"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.go"), []byte("package config"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	result, err := runner.Commit(context.Background(), CommitOptions{
		SingleCommit: true,
		DryRun:       true,
	})

	require.NoError(t, err)
	assert.Len(t, result.Commits, 1)
	assert.Equal(t, "all", result.Commits[0].Package)
	assert.Equal(t, 2, result.TotalFiles)
}

func TestSmartCommitRunner_Commit_WithGarbage_Blocked(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create garbage file
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=test"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	_, err = runner.Commit(context.Background(), CommitOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "garbage files detected")
}

func TestSmartCommitRunner_Commit_WithGarbage_Skipped(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create normal file and garbage
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=test"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	// With SkipGarbageCheck, should succeed
	result, err := runner.Commit(context.Background(), CommitOptions{
		SkipGarbageCheck: true,
		DryRun:           true,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFiles) // Only non-garbage file
}

func TestSmartCommitRunner_Commit_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	_, err = runner.Commit(context.Background(), CommitOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no files to commit")
}

func TestSmartCommitRunner_Commit_RealCommit(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a file
	internalDir := filepath.Join(tmpDir, "internal", "git")
	require.NoError(t, os.MkdirAll(internalDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(internalDir, "runner.go"), []byte("package git"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	// Put artifacts outside the repo to avoid untracked files issue
	artifactsDir := filepath.Join(t.TempDir(), "artifacts")
	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil,
		WithTaskID("task-real-test"),
		WithTemplateName("feature"),
		WithArtifactsDir(artifactsDir),
	)

	result, err := runner.Commit(context.Background(), CommitOptions{})

	require.NoError(t, err)
	assert.Len(t, result.Commits, 1)
	assert.NotEqual(t, "(dry-run)", result.Commits[0].Hash)
	assert.NotEmpty(t, result.Commits[0].Hash)
	assert.GreaterOrEqual(t, len(result.Commits[0].Hash), 7) // Short hash is at least 7 chars
	assert.Equal(t, 1, result.TotalFiles)

	// Verify artifact was saved
	assert.NotEmpty(t, result.ArtifactPath)
	assert.FileExists(t, result.ArtifactPath)

	// Verify commit was actually created (repo should be clean now)
	status, err := gitRunner.Status(context.Background())
	require.NoError(t, err)
	assert.True(t, status.IsClean(), "working tree should be clean after commit")
}

func TestSmartCommitRunner_Commit_MultipleGroups(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create files in different packages
	gitDir := filepath.Join(tmpDir, "internal", "git")
	configDir := filepath.Join(tmpDir, "internal", "config")
	require.NoError(t, os.MkdirAll(gitDir, 0o750))
	require.NoError(t, os.MkdirAll(configDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "runner.go"), []byte("package git"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.go"), []byte("package config"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	result, err := runner.Commit(context.Background(), CommitOptions{})

	require.NoError(t, err)
	assert.Len(t, result.Commits, 2) // Two separate commits
	assert.Equal(t, 2, result.TotalFiles)

	// Verify both commits were created
	status, err := gitRunner.Status(context.Background())
	require.NoError(t, err)
	assert.True(t, status.IsClean())
}

func TestSmartCommitRunner_GenerateSimpleMessage(t *testing.T) {
	runner := &SmartCommitRunner{}

	tests := []struct {
		name            string
		group           FileGroup
		expectedSubject string
		expectedBody    string
	}{
		{
			name: "single file added",
			group: FileGroup{
				Package:    "internal/git",
				Files:      []FileChange{{Path: "internal/git/runner.go", Status: ChangeAdded}},
				CommitType: CommitTypeFeat,
			},
			expectedSubject: "feat(git): add runner.go",
			expectedBody:    "Updated runner.go in internal/git.",
		},
		{
			name: "single file modified",
			group: FileGroup{
				Package:    "internal/config",
				Files:      []FileChange{{Path: "internal/config/parser.go", Status: ChangeModified}},
				CommitType: CommitTypeFix,
			},
			expectedSubject: "fix(config): update parser.go",
			expectedBody:    "Updated parser.go in internal/config.",
		},
		{
			name: "single file deleted",
			group: FileGroup{
				Package:    "internal/task",
				Files:      []FileChange{{Path: "internal/task/old.go", Status: ChangeDeleted}},
				CommitType: CommitTypeChore,
			},
			expectedSubject: "chore(task): remove old.go",
			expectedBody:    "Updated old.go in internal/task.",
		},
		{
			name: "multiple files",
			group: FileGroup{
				Package: "internal/git",
				Files: []FileChange{
					{Path: "internal/git/runner.go", Status: ChangeModified},
					{Path: "internal/git/types.go", Status: ChangeModified},
				},
				CommitType: CommitTypeFeat,
			},
			expectedSubject: "feat(git): update 2 files",
			expectedBody:    "Updated 2 files in internal/git.",
		},
		{
			name: "docs group no scope",
			group: FileGroup{
				Package:    "docs",
				Files:      []FileChange{{Path: "README.md", Status: ChangeModified}},
				CommitType: CommitTypeDocs,
			},
			expectedSubject: "docs: update README.md",
			expectedBody:    "Updated README.md in docs.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := runner.generateSimpleMessage(tt.group)
			// Message should be: subject + blank line + synopsis body
			expected := fmt.Sprintf("%s\n\n%s", tt.expectedSubject, tt.expectedBody)
			assert.Equal(t, expected, message)
			// Also verify it contains both parts
			assert.Contains(t, message, tt.expectedSubject)
			assert.Contains(t, message, tt.expectedBody)
		})
	}
}

func TestIsValidConventionalCommit(t *testing.T) {
	tests := []struct {
		message string
		valid   bool
	}{
		{"feat(git): add runner", true},
		{"fix(config): handle nil", true},
		{"docs: update readme", true},
		{"chore(deps): bump version", true},
		{"test(task): add unit tests", true},
		{"refactor(ai): simplify logic", true},
		{"style(cli): format code", true},
		{"build(ci): update workflow", true},
		{"ci: add pipeline", true},
		{"invalid message", false},
		{"Feat(git): wrong case", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			result := isValidConventionalCommit(tt.message)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestWithModel(t *testing.T) {
	gitRunner := &MockRunner{}
	runner := NewSmartCommitRunner(gitRunner, "/tmp", nil,
		WithModel("haiku"),
	)

	assert.Equal(t, "haiku", runner.model)
}

func TestWithModel_Empty(t *testing.T) {
	gitRunner := &MockRunner{}
	runner := NewSmartCommitRunner(gitRunner, "/tmp", nil)

	// Default should be empty string (uses AIRunner's default)
	assert.Empty(t, runner.model)
}

func TestFormatArtifactMarkdown(t *testing.T) {
	artifact := &CommitArtifact{
		TaskID:    "task-test",
		Template:  "feature",
		Timestamp: "2025-12-30T10:00:00Z",
		Summary:   "Created 1 commit(s)",
		Commits: []CommitInfo{
			{
				Hash:         "abc1234",
				Message:      "feat(git): add smart commit\n\nAdded smart commit functionality for automatic message generation.",
				FileCount:    2,
				Package:      "internal/git",
				CommitType:   CommitTypeFeat,
				FilesChanged: []string{"internal/git/smart_commit.go", "internal/git/smart_commit_test.go"},
			},
		},
	}

	content := formatArtifactMarkdown(artifact)

	assert.Contains(t, content, "# Commit Summary")
	assert.Contains(t, content, "**Task:** task-test")
	assert.Contains(t, content, "**Template:** feature")
	assert.Contains(t, content, "abc1234")
	assert.Contains(t, content, "feat(git): add smart commit")
	assert.Contains(t, content, "smart commit functionality") // Synopsis body
	assert.Contains(t, content, "internal/git/smart_commit.go")
	// Trailers are no longer displayed in artifact markdown
	assert.NotContains(t, content, "**Trailers:**")
}

func TestSmartCommitRunner_BuildAIPrompt(t *testing.T) {
	runner := &SmartCommitRunner{}

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeModified},
			{Path: "internal/git/runner_test.go", Status: ChangeAdded},
		},
		CommitType: CommitTypeFeat,
	}

	prompt := runner.buildAIPrompt(group)

	assert.Contains(t, prompt, "Package: internal/git")
	assert.Contains(t, prompt, "internal/git/runner.go (M)")
	assert.Contains(t, prompt, "internal/git/runner_test.go (A)")
	assert.Contains(t, prompt, "Scope should be: git")
	assert.Contains(t, prompt, "conventional commits format")
	assert.Contains(t, prompt, "Do NOT include any AI attribution")
}

// initGitRepo initializes a git repo in the given directory
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	_, err := RunCommand(context.Background(), dir, "init")
	require.NoError(t, err)
	_, err = RunCommand(context.Background(), dir, "config", "user.email", "test@example.com")
	require.NoError(t, err)
	_, err = RunCommand(context.Background(), dir, "config", "user.name", "Test User")
	require.NoError(t, err)

	// Create initial commit so we have a valid HEAD
	initFile := filepath.Join(dir, ".gitkeep")
	require.NoError(t, os.WriteFile(initFile, []byte(""), 0o600))
	_, err = RunCommand(context.Background(), dir, "add", ".")
	require.NoError(t, err)
	_, err = RunCommand(context.Background(), dir, "commit", "-m", "initial commit")
	require.NoError(t, err)
}

// Tests for generateAIMessage with mock AI runner

func TestSmartCommitRunner_GenerateAIMessage_Success(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunner{
		response: &domain.AIResult{
			Success: true,
			Output:  "feat(git): add smart commit functionality",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	message, err := runner.generateAIMessage(context.Background(), group, 30*time.Second, "")
	require.NoError(t, err)
	assert.Equal(t, "feat(git): add smart commit functionality", message)
}

func TestSmartCommitRunner_GenerateAIMessage_AIError(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunner{
		err: atlaserrors.ErrAIError,
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	_, err = runner.generateAIMessage(context.Background(), group, 30*time.Second, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrAIError)
}

func TestSmartCommitRunner_GenerateAIMessage_AIReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunner{
		response: &domain.AIResult{
			Success: false,
			Error:   "rate limit exceeded",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files:   []FileChange{{Path: "a.go", Status: ChangeModified}},
	}

	_, err = runner.generateAIMessage(context.Background(), group, 30*time.Second, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrAIError)
}

func TestSmartCommitRunner_GenerateAIMessage_EmptyResponse(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunner{
		response: &domain.AIResult{
			Success: true,
			Output:  "   ", // whitespace only
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files:   []FileChange{{Path: "a.go", Status: ChangeModified}},
	}

	_, err = runner.generateAIMessage(context.Background(), group, 30*time.Second, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrAIEmptyResponse)
}

func TestSmartCommitRunner_GenerateAIMessage_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunner{
		response: &domain.AIResult{
			Success: true,
			Output:  "This is not a conventional commit message",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files:   []FileChange{{Path: "a.go", Status: ChangeModified}},
	}

	_, err = runner.generateAIMessage(context.Background(), group, 30*time.Second, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrAIInvalidFormat)
}

func TestSmartCommitRunner_GenerateAIMessage_ReturnsFullMessageWithBody(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	// AI returns message with subject and synopsis body
	mockAI := &mockAIRunner{
		response: &domain.AIResult{
			Success: true,
			Output:  "feat(git): add runner\n\nThis commit adds the runner implementation.",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files:   []FileChange{{Path: "runner.go", Status: ChangeAdded}},
	}

	message, err := runner.generateAIMessage(context.Background(), group, 30*time.Second, "")
	require.NoError(t, err)
	// Full message including body is now returned
	assert.Contains(t, message, "feat(git): add runner")
	assert.Contains(t, message, "This commit adds the runner implementation.")
}

// Test for IncludeGarbage option

func TestSmartCommitRunner_Commit_WithGarbage_Included(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create normal file and garbage
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=test"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	// With IncludeGarbage, should include garbage files
	result, err := runner.Commit(context.Background(), CommitOptions{
		IncludeGarbage: true,
		DryRun:         true,
	})

	require.NoError(t, err)
	// Should include both files (garbage is included)
	assert.Equal(t, 2, result.TotalFiles)
}

// Test for context cancellation during Commit

func TestSmartCommitRunner_Commit_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = runner.Commit(ctx, CommitOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// Test for getFileDescription edge cases

func TestGetFileDescription_AllStatuses(t *testing.T) {
	tests := []struct {
		status   ChangeType
		expected string
	}{
		{ChangeAdded, "add test.go"},
		{ChangeDeleted, "remove test.go"},
		{ChangeModified, "update test.go"},
		{ChangeRenamed, "rename test.go"},
		{ChangeCopied, "copy test.go"},
		{ChangeUnmerged, "resolve test.go"},
		{ChangeType("X"), "update test.go"}, // unknown/default case
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := getFileDescription(FileChange{Path: "test.go", Status: tt.status})
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test for getDiffSummary

func TestSmartCommitRunner_GetDiffSummary(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create and modify a file
	testFile := filepath.Join(tmpDir, "test.go")
	require.NoError(t, os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0o600))
	_, err := RunCommand(context.Background(), tmpDir, "add", "test.go")
	require.NoError(t, err)
	_, err = RunCommand(context.Background(), tmpDir, "commit", "-m", "add test.go")
	require.NoError(t, err)

	// Modify the file
	require.NoError(t, os.WriteFile(testFile, []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o600))

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	group := FileGroup{
		Package: "root",
		Files:   []FileChange{{Path: "test.go", Status: ChangeModified}},
	}

	summary := runner.getDiffSummary(context.Background(), group)
	assert.Contains(t, summary, "test.go")
	assert.Contains(t, summary, "+")
}

// Test for buildAIPromptWithDiff

func TestSmartCommitRunner_BuildAIPromptWithDiff(t *testing.T) {
	runner := &SmartCommitRunner{}

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	diffSummary := "  internal/git/runner.go: +10/-5 lines\n"

	prompt := runner.buildAIPromptWithDiff(group, diffSummary)

	assert.Contains(t, prompt, "Package: internal/git")
	assert.Contains(t, prompt, "internal/git/runner.go (M)")
	assert.Contains(t, prompt, "Change summary:")
	assert.Contains(t, prompt, "+10/-5 lines")
}

func TestSmartCommitRunner_BuildAIPromptWithDiff_NoDiff(t *testing.T) {
	runner := &SmartCommitRunner{}

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeModified},
		},
	}

	// Empty diff summary
	prompt := runner.buildAIPromptWithDiff(group, "")

	assert.Contains(t, prompt, "Package: internal/git")
	assert.NotContains(t, prompt, "Change summary:")
}

// Test for generateCommitMessage fallback behavior

func TestSmartCommitRunner_GenerateCommitMessage_FallbackOnAIError(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	// AI runner that always fails
	mockAI := &mockAIRunner{
		err: atlaserrors.ErrAIError,
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeAdded},
		},
		CommitType: CommitTypeFeat,
	}

	message := runner.generateCommitMessage(context.Background(), group)
	// Should fallback to simple message with synopsis body
	assert.Contains(t, message, "feat(git): add runner.go")
	assert.Contains(t, message, "Updated runner.go in internal/git.")
}

func TestSmartCommitRunner_GenerateCommitMessage_UsesSuggestedMessage(t *testing.T) {
	runner := &SmartCommitRunner{}

	group := FileGroup{
		Package:          "internal/git",
		Files:            []FileChange{{Path: "runner.go", Status: ChangeModified}},
		SuggestedMessage: "fix(git): handle nil pointer in runner",
	}

	message := runner.generateCommitMessage(context.Background(), group)
	assert.Equal(t, "fix(git): handle nil pointer in runner", message)
}
