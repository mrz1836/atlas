package task

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/validation"
)

var (
	errNotImplemented          = errors.New("not implemented")
	errRetryFailed             = errors.New("retry failed")
	errValidationContinuesFail = errors.New("validation continues to fail")
	errFailed                  = errors.New("failed")
)

// mockValidationRetryHandler implements the ValidationRetryHandler interface for testing.
type mockValidationRetryHandler struct {
	isEnabled       bool
	maxAttempts     int
	canRetryFunc    func(int) bool
	retryWithAIFunc func(context.Context, *validation.PipelineResult, string, int, *validation.RunnerConfig, domain.Agent, string, validation.AICompleteCallback) (*validation.RetryResult, error)
}

func (m *mockValidationRetryHandler) IsEnabled() bool {
	return m.isEnabled
}

func (m *mockValidationRetryHandler) MaxAttempts() int {
	if m.maxAttempts == 0 {
		return 3 // Default
	}
	return m.maxAttempts
}

func (m *mockValidationRetryHandler) CanRetry(attempt int) bool {
	if m.canRetryFunc != nil {
		return m.canRetryFunc(attempt)
	}
	return attempt <= m.MaxAttempts()
}

func (m *mockValidationRetryHandler) RetryWithAI(ctx context.Context, pr *validation.PipelineResult, workDir string, attempt int, cfg *validation.RunnerConfig, agent domain.Agent, model string, onAIComplete validation.AICompleteCallback) (*validation.RetryResult, error) {
	if m.retryWithAIFunc != nil {
		return m.retryWithAIFunc(ctx, pr, workDir, attempt, cfg, agent, model, onAIComplete)
	}
	return nil, errNotImplemented
}

// TestShouldAttemptValidationRetry_HandlerNil tests nil handler returns false
func TestShouldAttemptValidationRetry_HandlerNil(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	// Ensure handler is nil
	engine.validationRetryHandler = nil

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{},
		},
	}

	shouldRetry := engine.shouldAttemptValidationRetry(result)

	assert.False(t, shouldRetry)
}

// TestShouldAttemptValidationRetry_HandlerDisabled tests disabled handler returns false
func TestShouldAttemptValidationRetry_HandlerDisabled(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled: false,
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{},
		},
	}

	shouldRetry := engine.shouldAttemptValidationRetry(result)

	assert.False(t, shouldRetry)
}

// TestShouldAttemptValidationRetry_ResultNil tests nil result returns false
func TestShouldAttemptValidationRetry_ResultNil(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled: true,
	}

	shouldRetry := engine.shouldAttemptValidationRetry(nil)

	assert.False(t, shouldRetry)
}

// TestShouldAttemptValidationRetry_MetadataNil tests nil metadata returns false
func TestShouldAttemptValidationRetry_MetadataNil(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled: true,
	}

	result := &domain.StepResult{
		Metadata: nil,
	}

	shouldRetry := engine.shouldAttemptValidationRetry(result)

	assert.False(t, shouldRetry)
}

// TestShouldAttemptValidationRetry_NoPipelineResult tests missing pipeline_result key
func TestShouldAttemptValidationRetry_NoPipelineResult(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled: true,
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"other_key": "value",
		},
	}

	shouldRetry := engine.shouldAttemptValidationRetry(result)

	assert.False(t, shouldRetry)
}

// TestShouldAttemptValidationRetry_WrongType tests wrong type in pipeline_result
func TestShouldAttemptValidationRetry_WrongType(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled: true,
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": "wrong type string", // Should be *validation.PipelineResult
		},
	}

	shouldRetry := engine.shouldAttemptValidationRetry(result)

	assert.False(t, shouldRetry)
}

// TestShouldAttemptValidationRetry_ValidPipelineResult tests valid pipeline_result returns true
func TestShouldAttemptValidationRetry_ValidPipelineResult(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled: true,
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "build",
			},
		},
	}

	shouldRetry := engine.shouldAttemptValidationRetry(result)

	assert.True(t, shouldRetry)
}

// TestAttemptValidationRetry_PipelineResultNotFound tests missing pipeline_result error
func TestAttemptValidationRetry_PipelineResultNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata:    make(map[string]any),
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"other_key": "value", // Missing pipeline_result
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	assert.Nil(t, retryResult)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPipelineResultNotFound)
}

// TestAttemptValidationRetry_WorkDirNotFound tests missing worktree_dir error
func TestAttemptValidationRetry_WorkDirNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata:    make(map[string]any), // Missing worktree_dir
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "build",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	assert.Nil(t, retryResult)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkDirNotFound)
}

// TestAttemptValidationRetry_WorkDirMissing tests worktree directory doesn't exist
func TestAttemptValidationRetry_WorkDirMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata: map[string]any{
			"worktree_dir": "/nonexistent/directory/path",
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "build",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	assert.Nil(t, retryResult)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkDirNotFound)
	assert.Contains(t, err.Error(), "/nonexistent/directory/path")
}

// TestAttemptValidationRetry_SuccessFirstAttempt tests success on first attempt
func TestAttemptValidationRetry_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()

	// Create a real temporary directory
	workDir := t.TempDir()

	successResult := &validation.RetryResult{
		Success:       true,
		AttemptNumber: 1,
		PipelineResult: &validation.PipelineResult{
			DurationMs: 500,
		},
		AIResult: &domain.AIResult{
			FilesChanged: []string{"file1.go", "file2.go"},
		},
	}

	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, _ int, _ *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			return successResult, nil
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata: map[string]any{
			"worktree_dir": workDir,
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "build",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	require.NoError(t, err)
	require.NotNil(t, retryResult)
	assert.True(t, retryResult.Success)
	assert.Equal(t, 1, retryResult.AttemptNumber)
	// Verify validation_attempt was set in metadata
	assert.Equal(t, 1, task.Metadata["validation_attempt"])
}

// TestAttemptValidationRetry_SuccessNthAttempt tests success on Nth attempt
func TestAttemptValidationRetry_SuccessNthAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	workDir := t.TempDir()

	attemptCount := 0

	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, attempt int, _ *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			attemptCount++
			if attemptCount < 3 {
				// Fail first 2 attempts
				return &validation.RetryResult{
					Success:       false,
					AttemptNumber: attempt,
				}, errRetryFailed
			}
			// Succeed on 3rd attempt
			return &validation.RetryResult{
				Success:       true,
				AttemptNumber: 3,
				PipelineResult: &validation.PipelineResult{
					DurationMs: 1000,
				},
				AIResult: &domain.AIResult{
					FilesChanged: []string{"fixed.go"},
				},
			}, nil
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata: map[string]any{
			"worktree_dir": workDir,
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "test",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	require.NoError(t, err)
	require.NotNil(t, retryResult)
	assert.True(t, retryResult.Success)
	assert.Equal(t, 3, retryResult.AttemptNumber)
	assert.Equal(t, 3, attemptCount)
}

// TestAttemptValidationRetry_AttemptsExhausted tests all attempts exhausted
func TestAttemptValidationRetry_AttemptsExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	workDir := t.TempDir()

	attemptCount := 0
	expectedErr := errValidationContinuesFail

	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, attempt int, _ *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			attemptCount++
			return &validation.RetryResult{
				Success:       false,
				AttemptNumber: attempt,
			}, expectedErr
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata: map[string]any{
			"worktree_dir": workDir,
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "test",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	require.NotNil(t, retryResult)
	assert.False(t, retryResult.Success)
	assert.Equal(t, 3, attemptCount) // All 3 attempts were made
}

// TestAttemptValidationRetry_ContextCancellation tests context cancellation during retry
func TestAttemptValidationRetry_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	workDir := t.TempDir()

	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata: map[string]any{
			"worktree_dir": workDir,
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "test",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	assert.Nil(t, retryResult)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestGetValidationAttemptNumber_NilMetadata tests nil metadata returns 1
func TestGetValidationAttemptNumber_NilMetadata(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: nil,
	}

	attempt := engine.getValidationAttemptNumber(task)

	assert.Equal(t, 1, attempt)
}

// TestGetValidationAttemptNumber_NoValidationAttempt tests missing key returns 1
func TestGetValidationAttemptNumber_NoValidationAttempt(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: map[string]any{
			"other_key": "value",
		},
	}

	attempt := engine.getValidationAttemptNumber(task)

	assert.Equal(t, 1, attempt)
}

// TestGetValidationAttemptNumber_ExistingAttempt tests existing attempt increments
func TestGetValidationAttemptNumber_ExistingAttempt(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: map[string]any{
			"validation_attempt": 2,
		},
	}

	attempt := engine.getValidationAttemptNumber(task)

	assert.Equal(t, 3, attempt) // Should return next attempt (2 + 1)
}

// TestSetValidationAttemptNumber_CreatesMetadata tests metadata initialization
func TestSetValidationAttemptNumber_CreatesMetadata(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: nil,
	}

	engine.setValidationAttemptNumber(task, 3)

	require.NotNil(t, task.Metadata)
	assert.Equal(t, 3, task.Metadata["validation_attempt"])
}

// TestSetValidationAttemptNumber_UpdatesExisting tests updating existing value
func TestSetValidationAttemptNumber_UpdatesExisting(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: map[string]any{
			"validation_attempt": 1,
			"other_key":          "preserve",
		},
	}

	engine.setValidationAttemptNumber(task, 2)

	assert.Equal(t, 2, task.Metadata["validation_attempt"])
	assert.Equal(t, "preserve", task.Metadata["other_key"]) // Other keys preserved
}

// TestGetValidationWorkDir_NilMetadata tests nil metadata returns empty string
func TestGetValidationWorkDir_NilMetadata(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: nil,
	}

	workDir := engine.getValidationWorkDir(task)

	assert.Empty(t, workDir)
}

// TestGetValidationWorkDir_MissingKey tests missing key returns empty string
func TestGetValidationWorkDir_MissingKey(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		Metadata: map[string]any{
			"other_key": "value",
		},
	}

	workDir := engine.getValidationWorkDir(task)

	assert.Empty(t, workDir)
}

// TestGetValidationWorkDir_ValidPath tests valid path extraction
func TestGetValidationWorkDir_ValidPath(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	expectedPath := "/tmp/worktree/path"
	task := &domain.Task{
		Metadata: map[string]any{
			"worktree_dir": expectedPath,
		},
	}

	workDir := engine.getValidationWorkDir(task)

	assert.Equal(t, expectedPath, workDir)
}

// TestConvertRetryResultToStepResult_StandardConversion tests normal conversion
func TestConvertRetryResultToStepResult_StandardConversion(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task-1",
		CurrentStep: 2,
	}

	step := &domain.StepDefinition{
		Name: "validate",
		Type: domain.StepTypeValidation,
	}

	retryResult := &validation.RetryResult{
		Success:       true,
		AttemptNumber: 2,
		PipelineResult: &validation.PipelineResult{
			DurationMs: 1500,
		},
		AIResult: &domain.AIResult{
			FilesChanged: []string{"file1.go", "file2.go", "file3.go"},
		},
	}

	stepResult := engine.convertRetryResultToStepResult(task, step, retryResult)

	require.NotNil(t, stepResult)
	assert.Equal(t, 2, stepResult.StepIndex)
	assert.Equal(t, "validate", stepResult.StepName)
	assert.Equal(t, "success", stepResult.Status)
	assert.Equal(t, int64(1500), stepResult.DurationMs)
	assert.Contains(t, stepResult.Output, "attempt 2")
	assert.Contains(t, stepResult.Output, "3 files changed")

	// Verify metadata
	require.NotNil(t, stepResult.Metadata)
	assert.Equal(t, retryResult.PipelineResult, stepResult.Metadata["pipeline_result"])
	assert.Equal(t, 2, stepResult.Metadata["retry_attempt"])
	assert.Equal(t, 3, stepResult.Metadata["ai_files_changed"])
}

// TestConvertRetryResultToStepResult_NilAIResult tests nil AI result handling
func TestConvertRetryResultToStepResult_NilAIResult(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		CurrentStep: 1,
	}

	step := &domain.StepDefinition{
		Name: "validate",
	}

	retryResult := &validation.RetryResult{
		Success:       true,
		AttemptNumber: 1,
		PipelineResult: &validation.PipelineResult{
			DurationMs: 500,
		},
		AIResult: nil, // Nil AI result
	}

	stepResult := engine.convertRetryResultToStepResult(task, step, retryResult)

	require.NotNil(t, stepResult)
	assert.Equal(t, "success", stepResult.Status)
	assert.Contains(t, stepResult.Output, "0 files changed")
	assert.Equal(t, 0, stepResult.Metadata["ai_files_changed"])
}

// TestNotifyRetryAttempt_WithCallback tests progress callback invocation
func TestNotifyRetryAttempt_WithCallback(t *testing.T) {
	t.Parallel()

	store := newMockStore()

	var callbackInvoked bool
	var receivedEvent StepProgressEvent

	cfg := DefaultEngineConfig()
	cfg.ProgressCallback = func(event StepProgressEvent) {
		callbackInvoked = true
		receivedEvent = event
	}

	engine := NewEngine(store, nil, cfg, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task-123",
		WorkspaceID: "test-workspace",
		CurrentStep: 2,
	}

	engine.notifyRetryAttempt(task, 2, 3)

	assert.True(t, callbackInvoked)
	assert.Equal(t, "retry", receivedEvent.Type)
	assert.Equal(t, "test-task-123", receivedEvent.TaskID)
	assert.Equal(t, "test-workspace", receivedEvent.WorkspaceName)
	assert.Equal(t, 2, receivedEvent.StepIndex)
	assert.Equal(t, "Retry attempt 2/3", receivedEvent.Status)
}

// TestNotifyRetryAttempt_NoCallback tests no panic when callback is nil
func TestNotifyRetryAttempt_NoCallback(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	cfg := DefaultEngineConfig()
	cfg.ProgressCallback = nil // No callback

	engine := NewEngine(store, nil, cfg, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task-123",
		CurrentStep: 1,
	}

	// Should not panic
	engine.notifyRetryAttempt(task, 1, 3)
}

// TestAttemptValidationRetry_CanRetryFalse tests CanRetry returns false breaks loop
func TestAttemptValidationRetry_CanRetryFalse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	workDir := t.TempDir()

	attemptCount := 0

	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 5,
		canRetryFunc: func(attempt int) bool {
			// Stop after 2 attempts
			return attempt <= 2
		},
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, attempt int, _ *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			attemptCount++
			return &validation.RetryResult{
				Success:       false,
				AttemptNumber: attempt,
			}, errFailed
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Metadata: map[string]any{
			"worktree_dir": workDir,
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{},
		},
	}

	_, err := engine.attemptValidationRetry(ctx, task, result)

	require.Error(t, err)
	// CanRetry stopped at 2, so should only have 2 attempts
	assert.Equal(t, 2, attemptCount)
}

// TestBuildValidationRunnerConfig_UsesStoredCommands tests that runner config uses stored commands
func TestBuildValidationRunnerConfig_UsesStoredCommands(t *testing.T) {
	t.Parallel()

	customFormat := []string{"custom-format"}
	customLint := []string{"custom-lint"}
	customTest := []string{"custom-test:race"}
	customPreCommit := []string{"custom-pre-commit"}

	store := newMockStore()
	engine := NewEngine(
		store,
		nil,
		DefaultEngineConfig(),
		zerolog.Nop(),
		WithValidationCommands(customFormat, customLint, customTest, customPreCommit),
	)

	task := &domain.Task{}
	config := engine.buildValidationRunnerConfig(task)

	require.NotNil(t, config)
	assert.Equal(t, customFormat, config.FormatCommands)
	assert.Equal(t, customLint, config.LintCommands)
	assert.Equal(t, customTest, config.TestCommands)
	assert.Equal(t, customPreCommit, config.PreCommitCommands)
}

// TestBuildValidationRunnerConfig_EmptyCommands tests that empty commands are properly stored
func TestBuildValidationRunnerConfig_EmptyCommands(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(
		store,
		nil,
		DefaultEngineConfig(),
		zerolog.Nop(),
		WithValidationCommands([]string{}, []string{}, []string{}, []string{}),
	)

	task := &domain.Task{}
	config := engine.buildValidationRunnerConfig(task)

	require.NotNil(t, config)
	assert.Empty(t, config.FormatCommands)
	assert.Empty(t, config.LintCommands)
	assert.Empty(t, config.TestCommands)
	assert.Empty(t, config.PreCommitCommands)
}

// TestBuildValidationRunnerConfig_NilCommands tests that nil commands are properly stored
func TestBuildValidationRunnerConfig_NilCommands(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(
		store,
		nil,
		DefaultEngineConfig(),
		zerolog.Nop(),
		WithValidationCommands(nil, nil, nil, nil),
	)

	task := &domain.Task{}
	config := engine.buildValidationRunnerConfig(task)

	require.NotNil(t, config)
	assert.Nil(t, config.FormatCommands)
	assert.Nil(t, config.LintCommands)
	assert.Nil(t, config.TestCommands)
	assert.Nil(t, config.PreCommitCommands)
}

// TestBuildValidationRunnerConfig_NoOption tests that without option, commands are nil
func TestBuildValidationRunnerConfig_NoOption(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{}
	config := engine.buildValidationRunnerConfig(task)

	require.NotNil(t, config)
	assert.Nil(t, config.FormatCommands)
	assert.Nil(t, config.LintCommands)
	assert.Nil(t, config.TestCommands)
	assert.Nil(t, config.PreCommitCommands)
}

// TestWithValidationCommands_StoresCommands tests that WithValidationCommands option stores commands correctly
func TestWithValidationCommands_StoresCommands(t *testing.T) {
	t.Parallel()

	customFormat := []string{"gofmt", "-w", "."}
	customLint := []string{"golangci-lint", "run"}
	customTest := []string{"go", "test", "-race", "./..."}
	customPreCommit := []string{"pre-commit", "run", "--all"}

	store := newMockStore()
	engine := NewEngine(
		store,
		nil,
		DefaultEngineConfig(),
		zerolog.Nop(),
		WithValidationCommands(customFormat, customLint, customTest, customPreCommit),
	)

	assert.Equal(t, customFormat, engine.formatCommands)
	assert.Equal(t, customLint, engine.lintCommands)
	assert.Equal(t, customTest, engine.testCommands)
	assert.Equal(t, customPreCommit, engine.preCommitCommands)
}

// TestWithValidationCommands_MultipleOptions tests that multiple options can be combined
func TestWithValidationCommands_MultipleOptions(t *testing.T) {
	t.Parallel()

	customFormat := []string{"custom-format"}
	customLint := []string{"custom-lint"}
	customTest := []string{"custom-test"}
	customPreCommit := []string{"custom-pre-commit"}

	store := newMockStore()
	mockHandler := &mockValidationRetryHandler{isEnabled: true, maxAttempts: 3}

	engine := NewEngine(
		store,
		nil,
		DefaultEngineConfig(),
		zerolog.Nop(),
		WithValidationRetryHandler(mockHandler),
		WithValidationCommands(customFormat, customLint, customTest, customPreCommit),
	)

	// Verify validation commands are set
	assert.Equal(t, customFormat, engine.formatCommands)
	assert.Equal(t, customLint, engine.lintCommands)
	assert.Equal(t, customTest, engine.testCommands)
	assert.Equal(t, customPreCommit, engine.preCommitCommands)

	// Verify other options still work
	assert.NotNil(t, engine.validationRetryHandler)
	assert.Equal(t, mockHandler, engine.validationRetryHandler)
}

// TestConvertRetryResultToStepResult_IncludesValidationChecks tests that validation_checks is included in metadata
func TestConvertRetryResultToStepResult_IncludesValidationChecks(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task-1",
		CurrentStep: 2,
	}

	step := &domain.StepDefinition{
		Name: "validate",
		Type: domain.StepTypeValidation,
	}

	retryResult := &validation.RetryResult{
		Success:       true,
		AttemptNumber: 1,
		PipelineResult: &validation.PipelineResult{
			DurationMs: 1000,
			FormatResults: []validation.Result{
				{Success: true, Command: "format"},
			},
			LintResults: []validation.Result{
				{Success: true, Command: "lint"},
			},
			TestResults: []validation.Result{
				{Success: true, Command: "test"},
			},
			PreCommitResults: []validation.Result{
				{Success: true, Command: "pre-commit"},
			},
		},
		AIResult: &domain.AIResult{
			FilesChanged: []string{"file.go"},
		},
	}

	stepResult := engine.convertRetryResultToStepResult(task, step, retryResult)

	require.NotNil(t, stepResult)
	require.NotNil(t, stepResult.Metadata)

	// Verify validation_checks is present
	checksRaw, ok := stepResult.Metadata["validation_checks"]
	require.True(t, ok, "validation_checks should be present in metadata")

	checks, ok := checksRaw.([]map[string]any)
	require.True(t, ok, "validation_checks should be []map[string]any")
	require.Len(t, checks, 4, "should have 4 checks: Format, Lint, Test, Pre-commit")

	// Verify each check
	assert.Equal(t, "Format", checks[0]["name"])
	assert.True(t, checks[0]["passed"].(bool))

	assert.Equal(t, "Lint", checks[1]["name"])
	assert.True(t, checks[1]["passed"].(bool))

	assert.Equal(t, "Test", checks[2]["name"])
	assert.True(t, checks[2]["passed"].(bool))

	assert.Equal(t, "Pre-commit", checks[3]["name"])
	assert.True(t, checks[3]["passed"].(bool))
	// "skipped" key is omitted when false, so we verify it's not present
	_, hasSkipped := checks[3]["skipped"]
	assert.False(t, hasSkipped, "skipped key should not be present when pre-commit was not skipped")
}

// TestAttemptValidationRetry_UsesCustomCommands tests that custom commands are passed to retry handler
func TestAttemptValidationRetry_UsesCustomCommands(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	workDir := t.TempDir()

	customFormat := []string{"magex", "format:fix"}
	customLint := []string{"magex", "lint"}
	customTest := []string{"magex", "test:race"}
	customPreCommit := []string{"go-pre-commit", "run", "--all-files"}

	var receivedConfig *validation.RunnerConfig

	engine := NewEngine(
		store,
		nil,
		DefaultEngineConfig(),
		zerolog.Nop(),
		WithValidationCommands(customFormat, customLint, customTest, customPreCommit),
	)

	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, _ int, cfg *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			// Capture the config that was passed
			receivedConfig = cfg
			return &validation.RetryResult{
				Success:       true,
				AttemptNumber: 1,
				PipelineResult: &validation.PipelineResult{
					DurationMs: 500,
				},
				AIResult: &domain.AIResult{
					FilesChanged: []string{"fixed.go"},
				},
			}, nil
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata: map[string]any{
			"worktree_dir": workDir,
		},
	}

	result := &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{
				FailedStepName: "test",
			},
		},
	}

	retryResult, err := engine.attemptValidationRetry(ctx, task, result)

	require.NoError(t, err)
	require.NotNil(t, retryResult)
	assert.True(t, retryResult.Success)

	// Verify that the custom commands were passed to the retry handler
	require.NotNil(t, receivedConfig, "RunnerConfig should have been passed to retry handler")
	assert.Equal(t, customFormat, receivedConfig.FormatCommands)
	assert.Equal(t, customLint, receivedConfig.LintCommands)
	assert.Equal(t, customTest, receivedConfig.TestCommands)
	assert.Equal(t, customPreCommit, receivedConfig.PreCommitCommands)
}

// Tests for BuildChecksAsMap and hasFailedResult are in internal/validation/result_test.go
