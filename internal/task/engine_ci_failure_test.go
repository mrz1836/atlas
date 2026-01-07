package task

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// mockHubRunner implements git.HubRunner for testing CI failure handling.
type mockHubRunner struct {
	convertToDraftCalled bool
	convertToDraftErr    error
	convertToDraftPR     int
}

func (m *mockHubRunner) CreatePR(_ context.Context, _ git.PRCreateOptions) (*git.PRResult, error) {
	return &git.PRResult{}, nil
}

func (m *mockHubRunner) GetPRStatus(_ context.Context, _ int) (*git.PRStatus, error) {
	return &git.PRStatus{}, nil
}

func (m *mockHubRunner) WatchPRChecks(_ context.Context, _ git.CIWatchOptions) (*git.CIWatchResult, error) {
	return &git.CIWatchResult{}, nil
}

func (m *mockHubRunner) ConvertToDraft(_ context.Context, prNumber int) error {
	m.convertToDraftCalled = true
	m.convertToDraftPR = prNumber
	return m.convertToDraftErr
}

func (m *mockHubRunner) MergePR(_ context.Context, _ int, _ string, _, _ bool) error {
	return nil
}

func (m *mockHubRunner) AddPRReview(_ context.Context, _ int, _, _ string) error {
	return nil
}

func (m *mockHubRunner) AddPRComment(_ context.Context, _ int, _ string) error {
	return nil
}

// testLogger returns a no-op logger for testing.
func ciTestLogger() zerolog.Logger {
	return zerolog.Nop()
}

// TestEngine_WithCIFailureHandler tests that the CI failure handler option works.
func TestEngine_WithCIFailureHandler(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	cfg := DefaultEngineConfig()
	logger := ciTestLogger()

	hubRunner := &mockHubRunner{}
	ciFailureHandler := NewCIFailureHandler(hubRunner, WithCIFailureLogger(logger))

	engine := NewEngine(store, registry, cfg, logger, WithCIFailureHandler(ciFailureHandler))

	assert.NotNil(t, engine)
	assert.NotNil(t, engine.ciFailureHandler)
}

// TestEngine_ProcessCIFailureAction_ViewLogs tests the view logs action.
func TestEngine_ProcessCIFailureAction_ViewLogs(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	// Create handler with mock browser opener
	browserOpened := false
	openedURL := ""
	hubRunner := &mockHubRunner{}
	ciFailureHandler := NewCIFailureHandler(hubRunner,
		WithCIFailureLogger(logger),
		WithBrowserOpener(func(url string) error {
			browserOpened = true
			openedURL = url
			return nil
		}),
	)

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger, WithCIFailureHandler(ciFailureHandler))

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
		Metadata: map[string]any{
			"pr_number": 42,
			"ci_failure_result": &git.CIWatchResult{
				Status: git.CIStatusFailure,
				CheckResults: []git.CheckResult{
					{Name: "test", Bucket: "fail", URL: "https://github.com/actions/123"},
				},
			},
		},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCIFailureAction(ctx, task, CIFailureViewLogs)

	require.NoError(t, err)
	assert.True(t, browserOpened)
	assert.Equal(t, "https://github.com/actions/123", openedURL)
	// Task should remain in CIFailed status (view logs returns to options)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
}

// TestEngine_ProcessCIFailureAction_RetryImplement tests retrying from implement.
func TestEngine_ProcessCIFailureAction_RetryImplement(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	hubRunner := &mockHubRunner{}
	ciFailureHandler := NewCIFailureHandler(hubRunner, WithCIFailureLogger(logger))

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger, WithCIFailureHandler(ciFailureHandler))

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
		CurrentStep: 5, // At ci_wait step
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "commit", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "pr", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "ci_wait", Type: domain.StepTypeCI, Status: "failed"},
		},
		Metadata: map[string]any{
			"pr_number": 42,
			"ci_failure_result": &git.CIWatchResult{
				Status: git.CIStatusFailure,
				CheckResults: []git.CheckResult{
					{Name: "test", Bucket: "fail", URL: "https://github.com/actions/123"},
				},
			},
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCIFailureAction(ctx, task, CIFailureRetryImplement)

	require.NoError(t, err)
	// Task should transition back to running
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
	// Step should be reset to implement (index 1)
	assert.Equal(t, 1, task.CurrentStep)
	// Should have retry context
	assert.NotNil(t, task.Metadata["retry_context"])
}

// TestEngine_ProcessCIFailureAction_FixManually tests manual fix action.
func TestEngine_ProcessCIFailureAction_FixManually(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	hubRunner := &mockHubRunner{}
	ciFailureHandler := NewCIFailureHandler(hubRunner, WithCIFailureLogger(logger))

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger, WithCIFailureHandler(ciFailureHandler))

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
		Metadata: map[string]any{
			"pr_number":    42,
			"worktree_dir": "/tmp/worktree",
			"ci_failure_result": &git.CIWatchResult{
				Status: git.CIStatusFailure,
			},
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCIFailureAction(ctx, task, CIFailureFixManually)

	require.NoError(t, err)
	// Task should remain in CIFailed (waiting for manual fix)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
	// Should have manual fix instructions stored
	assert.NotNil(t, task.Metadata["manual_fix_instructions"])
}

// TestEngine_ProcessCIFailureAction_Abandon tests the abandon action.
func TestEngine_ProcessCIFailureAction_Abandon(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	hubRunner := &mockHubRunner{}
	ciFailureHandler := NewCIFailureHandler(hubRunner, WithCIFailureLogger(logger))

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger, WithCIFailureHandler(ciFailureHandler))

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
		Metadata: map[string]any{
			"pr_number": 42,
			"ci_failure_result": &git.CIWatchResult{
				Status: git.CIStatusFailure,
			},
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCIFailureAction(ctx, task, CIFailureAbandon)

	require.NoError(t, err)
	// Task should transition to abandoned
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
	// PR should have been converted to draft
	assert.True(t, hubRunner.convertToDraftCalled)
	assert.Equal(t, 42, hubRunner.convertToDraftPR)
}

// TestEngine_ProcessCIFailureAction_NoHandler tests error when handler is nil.
func TestEngine_ProcessCIFailureAction_NoHandler(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	// No CI failure handler configured
	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
	}

	err := engine.ProcessCIFailureAction(ctx, task, CIFailureViewLogs)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CI failure handler not configured")
}

// TestEngine_HandleCIFailure_TransitionsState tests CI failure handling transitions.
func TestEngine_HandleCIFailure(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	hubRunner := &mockHubRunner{}
	ciFailureHandler := NewCIFailureHandler(hubRunner, WithCIFailureLogger(logger))
	engine := NewEngine(store, registry, DefaultEngineConfig(), logger, WithCIFailureHandler(ciFailureHandler))

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 5,
		Metadata: map[string]any{
			"pr_number": 42,
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	ciResult := &git.CIWatchResult{
		Status: git.CIStatusFailure,
		CheckResults: []git.CheckResult{
			{Name: "test", Bucket: "fail", URL: "https://github.com/actions/123"},
		},
	}

	result := &domain.StepResult{
		StepName: "ci_wait",
		Status:   "failed",
		Error:    "ci checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)
	// Task should transition to CIFailed
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
	// CI result should be stored in metadata
	assert.NotNil(t, task.Metadata["ci_failure_result"])
}

// TestEngine_HandleGHFailure tests GitHub failure handling.
func TestEngine_HandleGHFailure(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	result := &domain.StepResult{
		StepName: "pr",
		Status:   "failed",
		Error:    "failed to create PR",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)
	// Task should transition to GHFailed
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
	// Error should be stored
	assert.Contains(t, task.Metadata["last_error"].(string), "failed to create PR")
}

// TestEngine_HandleCITimeout tests CI timeout handling.
func TestEngine_HandleCITimeout(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	result := &domain.StepResult{
		StepName: "ci_wait",
		Status:   "failed",
		Error:    "ci monitoring timed out",
	}

	ciResult := &git.CIWatchResult{
		Status:      git.CIStatusTimeout,
		ElapsedTime: 30 * time.Minute,
	}

	err := engine.handleCITimeout(ctx, task, result, ciResult)

	require.NoError(t, err)
	// Task should transition to CITimeout
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
}

// TestEngine_Resume_FromCIFailedState tests resuming from CI failed state.
func TestEngine_Resume_FromCIFailedState(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeCI,
		result:   &domain.StepResult{Status: "success"},
	})
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
		CurrentStep: 5, // At ci_wait step
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "commit", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "pr", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "ci_wait", Type: domain.StepTypeCI, Status: "failed"},
		},
		Metadata: map[string]any{
			"pr_number": 42,
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "analyze", Type: domain.StepTypeAI},
			{Name: "implement", Type: domain.StepTypeAI},
			{Name: "validate", Type: domain.StepTypeValidation},
			{Name: "commit", Type: domain.StepTypeGit},
			{Name: "pr", Type: domain.StepTypeGit},
			{Name: "ci_wait", Type: domain.StepTypeCI},
		},
	}

	err := engine.Resume(ctx, task, template)

	require.NoError(t, err)
	// Task should be in AwaitingApproval after successful CI
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_Resume_FromCITimeoutState tests resuming from CI timeout state.
func TestEngine_Resume_FromCITimeoutState(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeCI,
		result:   &domain.StepResult{Status: "success"},
	})
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCITimeout,
		CurrentStep: 5,
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "commit", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "pr", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "ci_wait", Type: domain.StepTypeCI, Status: "failed"},
		},
		Metadata: map[string]any{
			"pr_number": 42,
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "analyze", Type: domain.StepTypeAI},
			{Name: "implement", Type: domain.StepTypeAI},
			{Name: "validate", Type: domain.StepTypeValidation},
			{Name: "commit", Type: domain.StepTypeGit},
			{Name: "pr", Type: domain.StepTypeGit},
			{Name: "ci_wait", Type: domain.StepTypeCI},
		},
	}

	err := engine.Resume(ctx, task, template)

	require.NoError(t, err)
	// Task should be in AwaitingApproval after successful CI
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_ProcessGHFailureAction_Retry tests retrying GH operation.
func TestEngine_ProcessGHFailureAction_Retry(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 4, // At pr step
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessGHFailureAction(ctx, task, GHFailureRetry)

	require.NoError(t, err)
	// Task should transition back to running
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
	// Step index should remain the same (retry current step)
	assert.Equal(t, 4, task.CurrentStep)
}

// TestEngine_ProcessGHFailureAction_Abandon tests abandoning after GH failure.
func TestEngine_ProcessGHFailureAction_Abandon(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusGHFailed,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessGHFailureAction(ctx, task, GHFailureAbandon)

	require.NoError(t, err)
	// Task should transition to abandoned
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
}

// TestEngine_ProcessCITimeoutAction_ContinueWaiting tests continuing to wait.
func TestEngine_ProcessCITimeoutAction_ContinueWaiting(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCITimeout,
		CurrentStep: 5,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCITimeoutAction(ctx, task, CITimeoutContinueWaiting)

	require.NoError(t, err)
	// Task should transition back to running
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
	// Should flag extended timeout
	assert.True(t, task.Metadata["extended_ci_timeout"].(bool))
}

// TestEngine_HandleGHFailure_PushFailure tests GitHub push failure handling.
func TestEngine_HandleGHFailure_PushFailure(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 3,
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "push", Type: domain.StepTypeGit, Status: "failed"},
		},
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	result := &domain.StepResult{
		StepName: "push",
		Status:   "failed",
		Error:    "push authentication failed",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
	assert.Contains(t, task.Metadata["last_error"].(string), "authentication")
}

// TestEngine_HandleGHFailure_PRCreationFailure tests PR creation failure handling.
func TestEngine_HandleGHFailure_PRCreationFailure(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 4,
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "push", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "pr", Type: domain.StepTypeGit, Status: "failed"},
		},
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	result := &domain.StepResult{
		StepName: "pr",
		Status:   "failed",
		Error:    "PR creation failed: rate limit exceeded",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
}

// TestEngine_Resume_FromGHFailedState tests resuming from GitHub failed state.
func TestEngine_Resume_FromGHFailedState(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeGit,
		result:   &domain.StepResult{Status: "success"},
	})
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 3,
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "push", Type: domain.StepTypeGit, Status: "failed"},
		},
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "analyze", Type: domain.StepTypeAI},
			{Name: "implement", Type: domain.StepTypeAI},
			{Name: "validate", Type: domain.StepTypeValidation},
			{Name: "push", Type: domain.StepTypeGit},
		},
	}

	err := engine.Resume(ctx, task, template)

	require.NoError(t, err)
	// Task should be in AwaitingApproval after successful push
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_ProcessCITimeoutAction_Retry tests retry after CI timeout.
func TestEngine_ProcessCITimeoutAction_Retry(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCITimeout,
		CurrentStep: 5,
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "commit", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "pr", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "ci_wait", Type: domain.StepTypeCI, Status: "failed"},
		},
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCITimeoutAction(ctx, task, CITimeoutRetry)

	require.NoError(t, err)
	// Task should transition back to running
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
	// Step should be reset to implement (index 1)
	assert.Equal(t, 1, task.CurrentStep)
}

// TestEngine_ProcessCITimeoutAction_Abandon tests abandoning after CI timeout.
func TestEngine_ProcessCITimeoutAction_Abandon(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCITimeout,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCITimeoutAction(ctx, task, CITimeoutAbandon)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
}

// TestEngine_StateMachine_AllCITransitions tests all CI-related state transitions.
func TestEngine_StateMachine_AllCITransitions(t *testing.T) {
	testCases := []struct {
		name        string
		fromStatus  constants.TaskStatus
		toStatus    constants.TaskStatus
		expectValid bool
	}{
		// Running -> CI failure states
		{"Running to CIFailed", constants.TaskStatusRunning, constants.TaskStatusCIFailed, true},
		{"Running to CITimeout", constants.TaskStatusRunning, constants.TaskStatusCITimeout, true},
		{"Running to GHFailed", constants.TaskStatusRunning, constants.TaskStatusGHFailed, true},

		// CI failure recovery paths
		{"CIFailed to Running", constants.TaskStatusCIFailed, constants.TaskStatusRunning, true},
		{"CIFailed to Abandoned", constants.TaskStatusCIFailed, constants.TaskStatusAbandoned, true},
		{"CITimeout to Running", constants.TaskStatusCITimeout, constants.TaskStatusRunning, true},
		{"CITimeout to Abandoned", constants.TaskStatusCITimeout, constants.TaskStatusAbandoned, true},
		{"GHFailed to Running", constants.TaskStatusGHFailed, constants.TaskStatusRunning, true},
		{"GHFailed to Abandoned", constants.TaskStatusGHFailed, constants.TaskStatusAbandoned, true},

		// Invalid transitions
		{"CIFailed to Completed", constants.TaskStatusCIFailed, constants.TaskStatusCompleted, false},
		{"CITimeout to Validating", constants.TaskStatusCITimeout, constants.TaskStatusValidating, false},
		{"GHFailed to AwaitingApproval", constants.TaskStatusGHFailed, constants.TaskStatusAwaitingApproval, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid := IsValidTransition(tc.fromStatus, tc.toStatus)
			assert.Equal(t, tc.expectValid, valid)
		})
	}
}

// TestEngine_ProcessCITimeoutAction_FixManually tests manual fix after timeout.
func TestEngine_ProcessCITimeoutAction_FixManually(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCITimeout,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessCITimeoutAction(ctx, task, CITimeoutFixManually)

	require.NoError(t, err)
	// Task should remain in CITimeout (waiting for manual fix)
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
	// Should flag manual fix pending
	assert.True(t, task.Metadata["awaiting_manual_fix"].(bool))
}

// TestEngine_FindImplementStep tests finding the implement step index.
func TestEngine_FindImplementStep(t *testing.T) {
	testCases := []struct {
		name     string
		steps    []domain.Step
		expected int
	}{
		{
			name: "standard template",
			steps: []domain.Step{
				{Name: "analyze", Type: domain.StepTypeAI},
				{Name: "implement", Type: domain.StepTypeAI},
				{Name: "validate", Type: domain.StepTypeValidation},
			},
			expected: 1,
		},
		{
			name: "no implement step",
			steps: []domain.Step{
				{Name: "analyze", Type: domain.StepTypeAI},
				{Name: "validate", Type: domain.StepTypeValidation},
			},
			expected: 0, // Falls back to first AI step
		},
		{
			name: "implement is first",
			steps: []domain.Step{
				{Name: "implement", Type: domain.StepTypeAI},
				{Name: "validate", Type: domain.StepTypeValidation},
			},
			expected: 0,
		},
	}

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), ciTestLogger())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{Steps: tc.steps}
			idx := engine.findImplementStep(task)
			assert.Equal(t, tc.expected, idx)
		})
	}
}

// TestEngine_DispatchFailureByType tests the failure type dispatcher.
func TestEngine_DispatchFailureByType(t *testing.T) {
	testCases := []struct {
		name           string
		metadata       map[string]any
		expectHandled  bool
		expectErr      bool
		expectedStatus constants.TaskStatus
	}{
		{
			name:          "nil metadata returns not handled",
			metadata:      nil,
			expectHandled: false,
		},
		{
			name:          "empty metadata returns not handled",
			metadata:      map[string]any{},
			expectHandled: false,
		},
		{
			name:          "no failure_type returns not handled",
			metadata:      map[string]any{"other_key": "value"},
			expectHandled: false,
		},
		{
			name:          "empty failure_type returns not handled",
			metadata:      map[string]any{"failure_type": ""},
			expectHandled: false,
		},
		{
			name:          "unknown failure_type returns not handled",
			metadata:      map[string]any{"failure_type": "unknown_type"},
			expectHandled: false,
		},
		{
			name: "ci_failed dispatches to handler",
			metadata: map[string]any{
				"failure_type": "ci_failed",
				"ci_result": &git.CIWatchResult{
					Status: git.CIStatusFailure,
				},
			},
			expectHandled:  true,
			expectedStatus: constants.TaskStatusCIFailed,
		},
		{
			name: "ci_timeout dispatches to handler",
			metadata: map[string]any{
				"failure_type": "ci_timeout",
				"ci_result": &git.CIWatchResult{
					Status:      git.CIStatusTimeout,
					ElapsedTime: 30 * time.Minute,
				},
			},
			expectHandled:  true,
			expectedStatus: constants.TaskStatusCITimeout,
		},
		{
			name: "gh_failed dispatches to handler",
			metadata: map[string]any{
				"failure_type": "gh_failed",
			},
			expectHandled:  true,
			expectedStatus: constants.TaskStatusGHFailed,
		},
		{
			name: "ci_failed without ci_result still works",
			metadata: map[string]any{
				"failure_type": "ci_failed",
			},
			expectHandled:  true,
			expectedStatus: constants.TaskStatusCIFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := newMockStore()
			registry := steps.NewExecutorRegistry()
			engine := NewEngine(store, registry, DefaultEngineConfig(), ciTestLogger())

			task := &domain.Task{
				ID:          "task-123",
				WorkspaceID: "test-workspace",
				Status:      constants.TaskStatusRunning,
				Metadata:    map[string]any{},
				Transitions: []domain.Transition{},
			}
			store.tasks[task.ID] = task

			result := &domain.StepResult{
				StepName: "test_step",
				Status:   "failed",
				Error:    "test error",
				Metadata: tc.metadata,
			}

			handled, err := engine.DispatchFailureByType(ctx, task, result)

			assert.Equal(t, tc.expectHandled, handled)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tc.expectHandled && tc.expectedStatus != "" {
				assert.Equal(t, tc.expectedStatus, task.Status)
			}
		})
	}
}

// TestCIFailureHandler_HasHandler tests the HasHandler method.
func TestCIFailureHandler_HasHandler(t *testing.T) {
	testCases := []struct {
		name     string
		handler  *CIFailureHandler
		expected bool
	}{
		{
			name:     "nil handler returns false",
			handler:  nil,
			expected: false,
		},
		{
			name:     "handler with nil hubRunner returns false",
			handler:  &CIFailureHandler{hubRunner: nil},
			expected: false,
		},
		{
			name:     "handler with hubRunner returns true",
			handler:  NewCIFailureHandler(&mockHubRunner{}),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.handler.HasHandler()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEngine_ExtractPRNumber tests PR number extraction from various metadata types.
func TestEngine_ExtractPRNumber(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), ciTestLogger())

	testCases := []struct {
		name     string
		metadata map[string]any
		expected int
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: 0,
		},
		{
			name:     "no pr_number key",
			metadata: map[string]any{"other": "value"},
			expected: 0,
		},
		{
			name:     "pr_number as int",
			metadata: map[string]any{"pr_number": 42},
			expected: 42,
		},
		{
			name:     "pr_number as int64",
			metadata: map[string]any{"pr_number": int64(123)},
			expected: 123,
		},
		{
			name:     "pr_number as float64",
			metadata: map[string]any{"pr_number": float64(456)},
			expected: 456,
		},
		{
			name:     "pr_number as string (unsupported)",
			metadata: map[string]any{"pr_number": "789"},
			expected: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{Metadata: tc.metadata}
			result := engine.extractPRNumber(task)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEngine_ProcessGHFailureAction_FixAndRetry tests fix and retry action.
func TestEngine_ProcessGHFailureAction_FixAndRetry(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusGHFailed,
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.ProcessGHFailureAction(ctx, task, GHFailureFixAndRetry)

	require.NoError(t, err)
	// Task should remain in GHFailed (waiting for manual fix)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
	// Should flag awaiting manual fix
	assert.True(t, task.Metadata["awaiting_manual_fix"].(bool))
}

// TestGHFailureAction_String tests all GH failure action string values.
func TestGHFailureAction_String(t *testing.T) {
	testCases := []struct {
		action   GHFailureAction
		expected string
	}{
		{GHFailureRetry, "retry"},
		{GHFailureFixAndRetry, "fix_and_retry"},
		{GHFailureAbandon, "abandon"},
		{GHFailureAction(99), "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.action.String())
		})
	}
}

// TestCITimeoutAction_String tests all CI timeout action string values.
func TestCITimeoutAction_String(t *testing.T) {
	testCases := []struct {
		action   CITimeoutAction
		expected string
	}{
		{CITimeoutContinueWaiting, "continue_waiting"},
		{CITimeoutRetry, "retry"},
		{CITimeoutFixManually, "fix_manually"},
		{CITimeoutAbandon, "abandon"},
		{CITimeoutAction(99), "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.action.String())
		})
	}
}

// TestEngine_HandleStepResult_WithFailureType tests that HandleStepResult
// correctly dispatches to specialized failure handlers via DispatchFailureByType.
func TestEngine_HandleStepResult_WithFailureType(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	logger := ciTestLogger()

	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps: []domain.Step{
			{Name: "ci_wait", Type: domain.StepTypeCI, Status: "running"},
		},
		Metadata:    map[string]any{},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	// Create a step result with ci_failed failure_type
	result := &domain.StepResult{
		StepName:    "ci_wait",
		Status:      "failed",
		Error:       "CI checks failed",
		CompletedAt: time.Now(),
		Metadata: map[string]any{
			"failure_type": "ci_failed",
			"ci_result": &git.CIWatchResult{
				Status: git.CIStatusFailure,
				CheckResults: []git.CheckResult{
					{Name: "test", Bucket: "fail"},
				},
			},
		},
	}

	step := &domain.StepDefinition{
		Name: "ci_wait",
		Type: domain.StepTypeCI,
	}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Task should transition to CIFailed (via the specialized handler)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
	// CI failure result should be stored
	assert.NotNil(t, task.Metadata["ci_failure_result"])
}
