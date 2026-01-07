package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// mockSmartCommitter is a mock implementation of git.SmartCommitService.
type mockSmartCommitter struct {
	analyzeFunc func(ctx context.Context) (*git.CommitAnalysis, error)
	commitFunc  func(ctx context.Context, opts git.CommitOptions) (*git.CommitResult, error)
}

func (m *mockSmartCommitter) Analyze(ctx context.Context) (*git.CommitAnalysis, error) {
	if m.analyzeFunc != nil {
		return m.analyzeFunc(ctx)
	}
	return &git.CommitAnalysis{}, nil
}

func (m *mockSmartCommitter) Commit(ctx context.Context, opts git.CommitOptions) (*git.CommitResult, error) {
	if m.commitFunc != nil {
		return m.commitFunc(ctx, opts)
	}
	return &git.CommitResult{}, nil
}

// mockPusher is a mock implementation of git.PushService.
type mockPusher struct {
	pushFunc func(ctx context.Context, opts git.PushOptions) (*git.PushResult, error)
}

func (m *mockPusher) Push(ctx context.Context, opts git.PushOptions) (*git.PushResult, error) {
	if m.pushFunc != nil {
		return m.pushFunc(ctx, opts)
	}
	return &git.PushResult{Success: true}, nil
}

// mockHubRunner is a mock implementation of git.HubRunner.
type mockHubRunner struct {
	createPRFunc     func(ctx context.Context, opts git.PRCreateOptions) (*git.PRResult, error)
	getPRStatusFunc  func(ctx context.Context, prNumber int) (*git.PRStatus, error)
	watchPRFunc      func(ctx context.Context, opts git.CIWatchOptions) (*git.CIWatchResult, error)
	convertDraftFunc func(ctx context.Context, prNumber int) error
	mergePRFunc      func(ctx context.Context, prNumber int, mergeMethod string, adminBypass bool) error
	addPRReviewFunc  func(ctx context.Context, prNumber int, body, event string) error
	addPRCommentFunc func(ctx context.Context, prNumber int, body string) error
}

func (m *mockHubRunner) CreatePR(ctx context.Context, opts git.PRCreateOptions) (*git.PRResult, error) {
	if m.createPRFunc != nil {
		return m.createPRFunc(ctx, opts)
	}
	return &git.PRResult{Number: 1, URL: "https://github.com/test/repo/pull/1"}, nil
}

func (m *mockHubRunner) GetPRStatus(ctx context.Context, prNumber int) (*git.PRStatus, error) {
	if m.getPRStatusFunc != nil {
		return m.getPRStatusFunc(ctx, prNumber)
	}
	return &git.PRStatus{}, nil
}

func (m *mockHubRunner) WatchPRChecks(ctx context.Context, opts git.CIWatchOptions) (*git.CIWatchResult, error) {
	if m.watchPRFunc != nil {
		return m.watchPRFunc(ctx, opts)
	}
	return &git.CIWatchResult{}, nil
}

func (m *mockHubRunner) ConvertToDraft(ctx context.Context, prNumber int) error {
	if m.convertDraftFunc != nil {
		return m.convertDraftFunc(ctx, prNumber)
	}
	return nil
}

func (m *mockHubRunner) MergePR(ctx context.Context, prNumber int, mergeMethod string, adminBypass bool) error {
	if m.mergePRFunc != nil {
		return m.mergePRFunc(ctx, prNumber, mergeMethod, adminBypass)
	}
	return nil
}

func (m *mockHubRunner) AddPRReview(ctx context.Context, prNumber int, body, event string) error {
	if m.addPRReviewFunc != nil {
		return m.addPRReviewFunc(ctx, prNumber, body, event)
	}
	return nil
}

func (m *mockHubRunner) AddPRComment(ctx context.Context, prNumber int, body string) error {
	if m.addPRCommentFunc != nil {
		return m.addPRCommentFunc(ctx, prNumber, body)
	}
	return nil
}

// mockPRDescriptionGenerator is a mock implementation of git.PRDescriptionGenerator.
type mockPRDescriptionGenerator struct {
	generateFunc func(ctx context.Context, opts git.PRDescOptions) (*git.PRDescription, error)
}

func (m *mockPRDescriptionGenerator) Generate(ctx context.Context, opts git.PRDescOptions) (*git.PRDescription, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, opts)
	}
	return &git.PRDescription{
		Title: "feat(test): test PR",
		Body:  "## Summary\nTest\n\n## Changes\nTest\n\n## Test Plan\nTest",
	}, nil
}

func TestNewGitExecutor(t *testing.T) {
	executor := NewGitExecutor("/tmp/work")

	require.NotNil(t, executor)
	assert.Equal(t, "/tmp/work", executor.workDir)
}

// testArtifactSaver is a mock for tracking artifact saves in tests.
type testArtifactSaver struct {
	savedArtifacts  map[string][]byte
	versionCounters map[string]int
}

func newTestArtifactSaver() *testArtifactSaver {
	return &testArtifactSaver{
		savedArtifacts:  make(map[string][]byte),
		versionCounters: make(map[string]int),
	}
}

func (s *testArtifactSaver) SaveArtifact(_ context.Context, _, _, filename string, data []byte) error {
	s.savedArtifacts[filename] = data
	return nil
}

func (s *testArtifactSaver) SaveVersionedArtifact(_ context.Context, _, _, baseName string, data []byte) (string, error) {
	s.versionCounters[baseName]++
	filename := fmt.Sprintf("%s.%d", baseName, s.versionCounters[baseName])
	s.savedArtifacts[filename] = data
	return filename, nil
}

func TestNewGitExecutor_WithOptions(t *testing.T) {
	committer := &mockSmartCommitter{}
	pusher := &mockPusher{}
	hubRunner := &mockHubRunner{}
	prDescGen := &mockPRDescriptionGenerator{}
	saver := newTestArtifactSaver()

	executor := NewGitExecutor("/tmp/work",
		WithSmartCommitter(committer),
		WithPusher(pusher),
		WithHubRunner(hubRunner),
		WithPRDescriptionGenerator(prDescGen),
		WithGitArtifactSaver(saver),
	)

	require.NotNil(t, executor)
	assert.Equal(t, "/tmp/work", executor.workDir)
	assert.NotNil(t, executor.artifactSaver)
	assert.NotNil(t, executor.smartCommitter)
	assert.NotNil(t, executor.pusher)
	assert.NotNil(t, executor.hubRunner)
	assert.NotNil(t, executor.prDescGen)
}

func TestGitExecutor_Type(t *testing.T) {
	executor := NewGitExecutor("/tmp/work")

	assert.Equal(t, domain.StepTypeGit, executor.Type())
}

func TestGitExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewGitExecutor("/tmp/work")
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "git", Type: domain.StepTypeGit}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestGitExecutor_Execute_UnknownOperation(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work",
		WithSmartCommitter(&mockSmartCommitter{}),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "invalid_op",
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown git operation")
}

func TestGitExecutor_ExecuteCommit_NoCommitter(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work") // No committer configured

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "commit",
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "smart committer not configured")
}

func TestGitExecutor_ExecuteCommit_NoChanges(t *testing.T) {
	ctx := context.Background()
	committer := &mockSmartCommitter{
		analyzeFunc: func(_ context.Context) (*git.CommitAnalysis, error) {
			return &git.CommitAnalysis{
				FileGroups:   []git.FileGroup{},
				TotalChanges: 0,
			}, nil
		},
	}

	executor := NewGitExecutor("/tmp/work", WithSmartCommitter(committer))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "commit",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusNoChanges, result.Status)
	assert.Equal(t, "No changes to commit - AI made no modifications", result.Output)
}

func TestGitExecutor_ExecuteCommit_WithGarbage(t *testing.T) {
	ctx := context.Background()
	committer := &mockSmartCommitter{
		analyzeFunc: func(_ context.Context) (*git.CommitAnalysis, error) {
			return &git.CommitAnalysis{
				FileGroups: []git.FileGroup{
					{Package: "internal/git", Files: []git.FileChange{{Path: "file.go"}}},
				},
				GarbageFiles: []git.GarbageFile{
					{Path: ".env", Category: git.GarbageSecrets, Reason: "matches pattern: .env"},
				},
				TotalChanges: 2,
				HasGarbage:   true,
			}, nil
		},
	}

	executor := NewGitExecutor("/tmp/work", WithSmartCommitter(committer))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "commit",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Contains(t, result.Output, ".env")
	assert.Contains(t, result.Output, "garbage")
}

func TestGitExecutor_ExecuteCommit_Success(t *testing.T) {
	ctx := context.Background()
	committer := &mockSmartCommitter{
		analyzeFunc: func(_ context.Context) (*git.CommitAnalysis, error) {
			return &git.CommitAnalysis{
				FileGroups: []git.FileGroup{
					{Package: "internal/git", Files: []git.FileChange{{Path: "file.go"}}},
				},
				TotalChanges: 1,
				HasGarbage:   false,
			}, nil
		},
		commitFunc: func(_ context.Context, opts git.CommitOptions) (*git.CommitResult, error) {
			// Trailers are deprecated - opts.Trailers should be empty now
			assert.Empty(t, opts.Trailers)
			return &git.CommitResult{
				Commits: []git.CommitInfo{
					{Hash: "abc123", Message: "feat: test", FileCount: 1, FilesChanged: []string{"file.go"}},
				},
				TotalFiles: 1,
			}, nil
		},
	}

	executor := NewGitExecutor("/tmp/work", WithSmartCommitter(committer))

	task := &domain.Task{ID: "task-123", TemplateID: "bugfix", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "commit",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, result.Output, "1 commit(s)")
	assert.Contains(t, result.FilesChanged, "file.go")
}

func TestGitExecutor_ExecuteCommit_WithArtifacts(t *testing.T) {
	ctx := context.Background()
	saver := newTestArtifactSaver()

	committer := &mockSmartCommitter{
		analyzeFunc: func(_ context.Context) (*git.CommitAnalysis, error) {
			return &git.CommitAnalysis{
				FileGroups: []git.FileGroup{
					{Package: "internal/git", Files: []git.FileChange{{Path: "file.go"}}},
				},
				TotalChanges: 1,
			}, nil
		},
		commitFunc: func(_ context.Context, _ git.CommitOptions) (*git.CommitResult, error) {
			return &git.CommitResult{
				Commits: []git.CommitInfo{
					{Hash: "abc123", Message: "feat: test", FileCount: 1, FilesChanged: []string{"file.go"}},
				},
				TotalFiles: 1,
			}, nil
		},
	}

	executor := NewGitExecutor("/tmp/work",
		WithSmartCommitter(committer),
		WithGitArtifactSaver(saver),
	)

	task := &domain.Task{ID: "task-123", WorkspaceID: "test-ws", TemplateID: "bugfix", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "commit_step",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "commit",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)

	// Verify artifact was saved via the saver
	expectedFilename := filepath.Join("commit_step", "commit-result.json")
	data, ok := saver.savedArtifacts[expectedFilename]
	require.True(t, ok, "artifact should have been saved")

	// Verify JSON content
	var savedResult git.CommitResult
	err = json.Unmarshal(data, &savedResult) //nolint:musttag // external type from git package
	require.NoError(t, err)
	assert.Len(t, savedResult.Commits, 1)
}

func TestGitExecutor_ExecutePush_NoPusher(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work") // No pusher configured

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "push",
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pusher not configured")
}

func TestGitExecutor_ExecutePush_NoBranch(t *testing.T) {
	ctx := context.Background()
	pusher := &mockPusher{}

	executor := NewGitExecutor("/tmp/work", WithPusher(pusher))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "push",
			// No branch configured
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch name not configured")
}

func TestGitExecutor_ExecutePush_Success(t *testing.T) {
	ctx := context.Background()
	pusher := &mockPusher{
		pushFunc: func(_ context.Context, opts git.PushOptions) (*git.PushResult, error) {
			assert.Equal(t, "origin", opts.Remote)
			assert.Equal(t, "feat/test-branch", opts.Branch)
			assert.True(t, opts.SetUpstream)
			return &git.PushResult{
				Success:  true,
				Upstream: "origin/feat/test-branch",
			}, nil
		},
	}

	executor := NewGitExecutor("/tmp/work", WithPusher(pusher))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "push",
			"branch":    "feat/test-branch",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, result.Output, "origin")
}

func TestGitExecutor_ExecutePush_AuthFailure(t *testing.T) {
	ctx := context.Background()
	pusher := &mockPusher{
		pushFunc: func(_ context.Context, _ git.PushOptions) (*git.PushResult, error) {
			return &git.PushResult{
				Success:   false,
				ErrorType: git.PushErrorAuth,
			}, atlaserrors.ErrPushAuthFailed
		},
	}

	executor := NewGitExecutor("/tmp/work", WithPusher(pusher))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "push",
			"branch":    "feat/test-branch",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err) // Returns result, not error, for auth failures
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "gh_failed")
}

func TestGitExecutor_ExecuteCreatePR_NoHubRunner(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work") // No hub runner configured

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "create_pr",
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hub runner not configured")
}

func TestGitExecutor_ExecuteCreatePR_NoPRDescGen(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work",
		WithHubRunner(&mockHubRunner{}),
	) // No PR description generator configured

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "create_pr",
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "PR description generator not configured")
}

func TestGitExecutor_ExecuteCreatePR_NoBranch(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work",
		WithHubRunner(&mockHubRunner{}),
		WithPRDescriptionGenerator(&mockPRDescriptionGenerator{}),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "create_pr",
			// No branch configured
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "head branch not configured")
}

func TestGitExecutor_ExecuteCreatePR_Success(t *testing.T) {
	ctx := context.Background()
	prDescGen := &mockPRDescriptionGenerator{
		generateFunc: func(_ context.Context, opts git.PRDescOptions) (*git.PRDescription, error) {
			assert.Equal(t, "task-123", opts.TaskID)
			assert.Equal(t, "bugfix", opts.TemplateName)
			return &git.PRDescription{
				Title: "fix(test): fix bug",
				Body:  "## Summary\nFix\n\n## Changes\nFix\n\n## Test Plan\nTest",
			}, nil
		},
	}

	hubRunner := &mockHubRunner{
		createPRFunc: func(_ context.Context, opts git.PRCreateOptions) (*git.PRResult, error) {
			assert.Equal(t, "fix(test): fix bug", opts.Title)
			assert.Equal(t, "main", opts.BaseBranch)
			assert.Equal(t, "fix/test-branch", opts.HeadBranch)
			return &git.PRResult{
				Number: 42,
				URL:    "https://github.com/test/repo/pull/42",
				State:  "open",
			}, nil
		},
	}

	executor := NewGitExecutor("/tmp/work",
		WithHubRunner(hubRunner),
		WithPRDescriptionGenerator(prDescGen),
	)

	task := &domain.Task{
		ID:          "task-123",
		TemplateID:  "bugfix",
		Description: "Fix a bug",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "create_pr",
			"branch":    "fix/test-branch",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, result.Output, "PR #42")
	assert.Contains(t, result.Output, "https://github.com/test/repo/pull/42")
}

func TestGitExecutor_ExecuteCreatePR_RateLimited(t *testing.T) {
	ctx := context.Background()
	hubRunner := &mockHubRunner{
		createPRFunc: func(_ context.Context, _ git.PRCreateOptions) (*git.PRResult, error) {
			return &git.PRResult{
				ErrorType: git.PRErrorRateLimit,
			}, atlaserrors.ErrGHRateLimited
		},
	}

	executor := NewGitExecutor("/tmp/work",
		WithHubRunner(hubRunner),
		WithPRDescriptionGenerator(&mockPRDescriptionGenerator{}),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "git",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "create_pr",
			"branch":    "feat/test",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err) // Returns result, not error, for rate limit
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "gh_failed")
}

func TestGitExecutor_HandleGarbageDetected(t *testing.T) {
	tests := []struct {
		name       string
		action     GarbageHandlingAction
		wantErr    bool
		errContain string
	}{
		{
			name:    "remove and continue",
			action:  GarbageRemoveAndContinue,
			wantErr: false,
		},
		{
			name:    "include anyway",
			action:  GarbageIncludeAnyway,
			wantErr: false,
		},
		{
			name:       "abort manual",
			action:     GarbageAbortManual,
			wantErr:    true,
			errContain: "commit aborted",
		},
		{
			name:       "unknown action",
			action:     GarbageHandlingAction(99),
			wantErr:    true,
			errContain: "unknown garbage handling action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			executor := NewGitExecutor("/tmp/work",
				WithGitRunner(&mockRunner{}),
			)

			garbageFiles := []git.GarbageFile{
				{Path: ".env", Category: git.GarbageSecrets},
			}

			err := executor.HandleGarbageDetected(ctx, garbageFiles, tt.action)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGitExecutor_HandleGarbageDetected_NoRunner(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work") // No git runner

	err := executor.HandleGarbageDetected(ctx, []git.GarbageFile{}, GarbageRemoveAndContinue)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "git runner not configured")
}

func TestFormatGarbageWarning(t *testing.T) {
	files := []git.GarbageFile{
		{Path: ".env", Category: git.GarbageSecrets, Reason: "matches pattern: .env"},
		{Path: "coverage.out", Category: git.GarbageBuildArtifact, Reason: "matches pattern: coverage.out"},
	}

	warning := formatGarbageWarning(files)

	assert.Contains(t, warning, ".env")
	assert.Contains(t, warning, "secrets")
	assert.Contains(t, warning, "coverage.out")
	assert.Contains(t, warning, "build_artifact")
	assert.Contains(t, warning, "[r] Remove")
	assert.Contains(t, warning, "[i] Include")
	assert.Contains(t, warning, "[a] Abort")
}

func TestFormatGarbageWarning_Empty(t *testing.T) {
	warning := formatGarbageWarning(nil)
	assert.Empty(t, warning)
}

func TestGetBranchFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		task     *domain.Task
		expected string
	}{
		{
			name: "from step config",
			config: map[string]any{
				"branch": "feat/from-config",
			},
			task:     &domain.Task{},
			expected: "feat/from-config",
		},
		{
			name:   "from task metadata",
			config: map[string]any{},
			task: &domain.Task{
				Metadata: map[string]any{
					"branch": "feat/from-metadata",
				},
			},
			expected: "feat/from-metadata",
		},
		{
			name:   "from task config variables",
			config: map[string]any{},
			task: &domain.Task{
				Config: domain.TaskConfig{
					Variables: map[string]string{
						"branch": "feat/from-variables",
					},
				},
			},
			expected: "feat/from-variables",
		},
		{
			name:     "not found",
			config:   map[string]any{},
			task:     &domain.Task{},
			expected: "",
		},
		{
			name: "step config takes precedence",
			config: map[string]any{
				"branch": "feat/from-config",
			},
			task: &domain.Task{
				Metadata: map[string]any{
					"branch": "feat/from-metadata",
				},
			},
			expected: "feat/from-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBranchFromConfig(tt.config, tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCommitMessages(t *testing.T) {
	results := []domain.StepResult{
		{Status: "success", Output: "Message 1"},
		{Status: "failed", Output: "Message 2"},
		{Status: "success", Output: "Message 3"},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 2)
	assert.Contains(t, messages, "Message 1")
	assert.Contains(t, messages, "Message 3")
}

func TestExtractFilesChanged(t *testing.T) {
	results := []domain.StepResult{
		{FilesChanged: []string{"file1.go", "file2.go"}},
		{FilesChanged: []string{"file3.go"}},
		{FilesChanged: nil},
	}

	files := extractFilesChanged(results)

	assert.Len(t, files, 3)
	assert.Contains(t, files, "file1.go")
	assert.Contains(t, files, "file2.go")
	assert.Contains(t, files, "file3.go")
}

func TestConvertToFileChanges(t *testing.T) {
	paths := []string{"file1.go", "file2.go"}

	changes := convertToFileChanges(paths)

	assert.Len(t, changes, 2)
	assert.Equal(t, "file1.go", changes[0].Path)
	assert.Equal(t, "file2.go", changes[1].Path)
}

func TestJoinArtifactPaths(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{"empty", nil, ""},
		{"single", []string{"path1"}, "path1"},
		{"multiple", []string{"path1", "path2", "path3"}, "path1;path2;path3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinArtifactPaths(tt.paths)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockRunner is a minimal mock for git.Runner interface.
type mockRunner struct{}

func (m *mockRunner) Status(_ context.Context) (*git.Status, error) {
	return &git.Status{}, nil
}

func (m *mockRunner) Add(_ context.Context, _ []string) error {
	return nil
}

func (m *mockRunner) Commit(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (m *mockRunner) Push(_ context.Context, _, _ string, _ bool) error {
	return nil
}

func (m *mockRunner) CurrentBranch(_ context.Context) (string, error) {
	return "main", nil
}

func (m *mockRunner) CreateBranch(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockRunner) Diff(_ context.Context, _ bool) (string, error) {
	return "", nil
}

func (m *mockRunner) BranchExists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockRunner) Fetch(_ context.Context, _ string) error {
	return nil
}

func (m *mockRunner) Rebase(_ context.Context, _ string) error {
	return nil
}

func (m *mockRunner) RebaseAbort(_ context.Context) error {
	return nil
}

func (m *mockRunner) Reset(_ context.Context) error {
	return nil
}
