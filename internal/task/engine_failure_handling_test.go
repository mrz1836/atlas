package task

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

var errStoreUpdateFailed = errors.New("store update failed")

// Helper to create a test task with basic configuration
func newTestTask(status constants.TaskStatus) *domain.Task {
	return &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      status,
		Metadata:    make(map[string]any),
		Steps: []domain.Step{
			{Name: "prepare", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "pending"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "pending"},
		},
		CurrentStep: 1,
	}
}

// Helper to create a test CI result
func newTestCIResult(status git.CIStatus) *git.CIWatchResult {
	return &git.CIWatchResult{
		Status:       status,
		CheckResults: []git.CheckResult{},
		ElapsedTime:  5 * time.Minute,
		Error:        nil,
	}
}

// TestDispatchFailureByType_NilMetadata tests that nil metadata returns (false, nil)
func TestDispatchFailureByType_NilMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Metadata: nil,
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.False(t, handled)
	require.NoError(t, err)
}

// TestDispatchFailureByType_EmptyMetadata tests that empty metadata returns (false, nil)
func TestDispatchFailureByType_EmptyMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Metadata: make(map[string]any),
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.False(t, handled)
	require.NoError(t, err)
}

// TestDispatchFailureByType_NoFailureType tests missing failure_type key
func TestDispatchFailureByType_NoFailureType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Metadata: map[string]any{
			"other_key": "value",
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.False(t, handled)
	require.NoError(t, err)
}

// TestDispatchFailureByType_EmptyFailureType tests empty failure_type string
func TestDispatchFailureByType_EmptyFailureType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Metadata: map[string]any{
			"failure_type": "",
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.False(t, handled)
	require.NoError(t, err)
}

// TestDispatchFailureByType_UnknownFailureType tests unknown failure type
func TestDispatchFailureByType_UnknownFailureType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Metadata: map[string]any{
			"failure_type": "unknown_failure",
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.False(t, handled)
	require.NoError(t, err)
}

// TestDispatchFailureByType_CIFailed tests ci_failed dispatches correctly
func TestDispatchFailureByType_CIFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)

	result := &domain.StepResult{
		Error: "CI checks failed",
		Metadata: map[string]any{
			"failure_type": "ci_failed",
			"ci_result":    ciResult,
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
}

// TestDispatchFailureByType_CITimeout tests ci_timeout dispatches correctly
func TestDispatchFailureByType_CITimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusTimeout)

	result := &domain.StepResult{
		Error: "CI monitoring timed out",
		Metadata: map[string]any{
			"failure_type": "ci_timeout",
			"ci_result":    ciResult,
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
}

// TestDispatchFailureByType_GHFailed tests gh_failed dispatches correctly
func TestDispatchFailureByType_GHFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)

	result := &domain.StepResult{
		Error: "gh_failed: non_fast_forward",
		Metadata: map[string]any{
			"failure_type": "gh_failed",
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
}

// TestDispatchFailureByType_CIFetchError tests ci_fetch_error dispatches correctly
func TestDispatchFailureByType_CIFetchError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)

	result := &domain.StepResult{
		Error: "Failed to fetch CI status",
		Metadata: map[string]any{
			"failure_type":   "ci_fetch_error",
			"original_error": "rate limit exceeded",
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestDispatchFailureByType_CIResultExtraction tests CI result is extracted
func TestDispatchFailureByType_CIResultExtraction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)

	result := &domain.StepResult{
		Error: "CI checks failed",
		Metadata: map[string]any{
			"failure_type": "ci_failed",
			"ci_result":    ciResult,
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.NoError(t, err)

	// Verify CI result was stored in task metadata
	storedResult, ok := task.Metadata["ci_failure_result"].(*git.CIWatchResult)
	assert.True(t, ok)
	assert.Equal(t, ciResult.Status, storedResult.Status)
}

// TestDispatchFailureByType_NilCIResult tests handler works without CI result
func TestDispatchFailureByType_NilCIResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)

	result := &domain.StepResult{
		Error: "CI checks failed",
		Metadata: map[string]any{
			"failure_type": "ci_failed",
			// No ci_result provided
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
}

// TestDispatchFailureByType_ContextCancellation tests context cancellation
func TestDispatchFailureByType_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "CI checks failed",
		Metadata: map[string]any{
			"failure_type": "ci_failed",
		},
	}

	handled, err := engine.DispatchFailureByType(ctx, task, result)

	assert.True(t, handled)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestHandleCIFailure_TransitionsToCIFailed tests state transition
func TestHandleCIFailure_TransitionsToCIFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
}

// TestHandleCIFailure_StoresCIResult tests CI result storage
func TestHandleCIFailure_StoresCIResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)

	// Verify metadata storage
	assert.NotNil(t, task.Metadata)
	storedResult, ok := task.Metadata["ci_failure_result"].(*git.CIWatchResult)
	assert.True(t, ok)
	assert.Equal(t, git.CIStatusFailure, storedResult.Status)

	storedError, ok := task.Metadata["last_error"].(string)
	assert.True(t, ok)
	assert.Equal(t, "CI checks failed", storedError)
}

// TestHandleCIFailure_SavesTaskState tests store update call
func TestHandleCIFailure_SavesTaskState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, 1, store.updateCalls)
}

// TestHandleCIFailure_StoreUpdateError tests error handling when store fails
func TestHandleCIFailure_StoreUpdateError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	store.updateErr = errStoreUpdateFailed
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save task state")
}

// TestHandleCIFailure_NilMetadata tests metadata initialization
func TestHandleCIFailure_NilMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	task.Metadata = nil // Explicitly set to nil
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.NotNil(t, task.Metadata)
	assert.Contains(t, task.Metadata, "ci_failure_result")
}

// TestHandleCIFailure_NilCIResult tests handling nil CI result
func TestHandleCIFailure_NilCIResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, nil)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)

	// Verify nil CI result stored
	storedResult, ok := task.Metadata["ci_failure_result"]
	assert.True(t, ok)
	assert.Nil(t, storedResult)
}

// TestHandleCIFailure_ContextCancellation tests context cancellation
func TestHandleCIFailure_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestHandleCITimeout_TransitionsToCITimeout tests state transition
func TestHandleCITimeout_TransitionsToCITimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusTimeout)
	result := &domain.StepResult{
		Error: "CI monitoring timed out",
	}

	err := engine.handleCITimeout(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
}

// TestHandleCITimeout_StoresTimeoutResult tests timeout result storage
func TestHandleCITimeout_StoresTimeoutResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusTimeout)
	ciResult.ElapsedTime = 30 * time.Minute
	result := &domain.StepResult{
		Error: "CI monitoring timed out",
	}

	err := engine.handleCITimeout(ctx, task, result, ciResult)

	require.NoError(t, err)

	storedResult, ok := task.Metadata["ci_timeout_result"].(*git.CIWatchResult)
	assert.True(t, ok)
	assert.Equal(t, 30*time.Minute, storedResult.ElapsedTime)

	storedError, ok := task.Metadata["last_error"].(string)
	assert.True(t, ok)
	assert.Equal(t, "CI monitoring timed out", storedError)
}

// TestHandleCITimeout_NilCIResult tests nil CI result handling
func TestHandleCITimeout_NilCIResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "CI monitoring timed out",
	}

	err := engine.handleCITimeout(ctx, task, result, nil)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
}

// TestHandleCITimeout_ContextCancellation tests context cancellation
func TestHandleCITimeout_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusTimeout)
	result := &domain.StepResult{
		Error: "CI monitoring timed out",
	}

	err := engine.handleCITimeout(ctx, task, result, ciResult)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestHandleGHFailure_TransitionsToGHFailed tests state transition
func TestHandleGHFailure_TransitionsToGHFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "push failed",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
}

// TestHandleGHFailure_ExtractsPushErrorType tests error type extraction
func TestHandleGHFailure_ExtractsPushErrorType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		errorString   string
		expectedType  string
		shouldExtract bool
	}{
		{
			name:          "non_fast_forward",
			errorString:   "gh_failed: non_fast_forward",
			expectedType:  "non_fast_forward",
			shouldExtract: true,
		},
		{
			name:          "authentication_failed",
			errorString:   "gh_failed: authentication_failed",
			expectedType:  "authentication_failed",
			shouldExtract: true,
		},
		{
			name:          "rate_limit",
			errorString:   "gh_failed: rate_limit",
			expectedType:  "rate_limit",
			shouldExtract: true,
		},
		{
			name:          "no_error_type",
			errorString:   "push failed",
			expectedType:  "",
			shouldExtract: false,
		},
		{
			name:          "empty_error",
			errorString:   "",
			expectedType:  "",
			shouldExtract: false,
		},
		{
			name:          "gh_failed_with_empty_type",
			errorString:   "gh_failed: ",
			expectedType:  "",
			shouldExtract: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			store := newMockStore()
			engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

			task := newTestTask(constants.TaskStatusRunning)
			result := &domain.StepResult{
				Error: tt.errorString,
			}

			err := engine.handleGHFailure(ctx, task, result)

			require.NoError(t, err)

			if tt.shouldExtract {
				pushErrorType, ok := task.Metadata["push_error_type"].(string)
				assert.True(t, ok)
				assert.Equal(t, tt.expectedType, pushErrorType)
			} else {
				_, exists := task.Metadata["push_error_type"]
				assert.False(t, exists)
			}
		})
	}
}

// TestHandleGHFailure_StoresErrorContext tests error context storage
func TestHandleGHFailure_StoresErrorContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "push failed",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)

	storedError, ok := task.Metadata["last_error"].(string)
	assert.True(t, ok)
	assert.Equal(t, "push failed", storedError)
}

// TestHandleGHFailure_ContextCancellation tests context cancellation
func TestHandleGHFailure_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "push failed",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestHandleCIFetchErrorFailure_TransitionsToAwaitingApproval tests multi-step transition
func TestHandleCIFetchErrorFailure_TransitionsToAwaitingApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "Failed to fetch CI status",
		Metadata: map[string]any{
			"original_error": "rate limit exceeded",
		},
	}

	err := engine.handleCIFetchErrorFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestHandleCIFetchErrorFailure_SetsCIFetchErrorFlag tests flag storage
func TestHandleCIFetchErrorFailure_SetsCIFetchErrorFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "Failed to fetch CI status",
		Metadata: map[string]any{
			"original_error": "rate limit exceeded",
		},
	}

	err := engine.handleCIFetchErrorFailure(ctx, task, result)

	require.NoError(t, err)

	flag, ok := task.Metadata["ci_fetch_error"].(bool)
	assert.True(t, ok)
	assert.True(t, flag)

	originalErr, ok := task.Metadata["last_error"].(string)
	assert.True(t, ok)
	assert.Equal(t, "rate limit exceeded", originalErr)
}

// TestHandleCIFetchErrorFailure_FromValidatingStatus tests transition from Validating
func TestHandleCIFetchErrorFailure_FromValidatingStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusValidating)
	result := &domain.StepResult{
		Error: "Failed to fetch CI status",
	}

	err := engine.handleCIFetchErrorFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestHandleCIFetchErrorFailure_NilMetadata tests nil metadata handling
func TestHandleCIFetchErrorFailure_NilMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error:    "Failed to fetch CI status",
		Metadata: nil,
	}

	err := engine.handleCIFetchErrorFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)

	flag, ok := task.Metadata["ci_fetch_error"].(bool)
	assert.True(t, ok)
	assert.True(t, flag)
}

// TestHandleCIFetchErrorFailure_ContextCancellation tests context cancellation
func TestHandleCIFetchErrorFailure_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "Failed to fetch CI status",
	}

	err := engine.handleCIFetchErrorFailure(ctx, task, result)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestExtractPRNumber tests PR number extraction from metadata
func TestExtractPRNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		expected int
	}{
		{
			name:     "nil_metadata",
			metadata: nil,
			expected: 0,
		},
		{
			name:     "no_pr_number_key",
			metadata: map[string]any{},
			expected: 0,
		},
		{
			name: "pr_number_as_int",
			metadata: map[string]any{
				"pr_number": 42,
			},
			expected: 42,
		},
		{
			name: "pr_number_as_int64",
			metadata: map[string]any{
				"pr_number": int64(123),
			},
			expected: 123,
		},
		{
			name: "pr_number_as_float64",
			metadata: map[string]any{
				"pr_number": 456.0,
			},
			expected: 456,
		},
		{
			name: "pr_number_as_string",
			metadata: map[string]any{
				"pr_number": "789",
			},
			expected: 0,
		},
		{
			name: "pr_number_zero",
			metadata: map[string]any{
				"pr_number": 0,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newMockStore()
			engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

			task := &domain.Task{
				ID:          "test-task",
				WorkspaceID: "test-workspace",
				Metadata:    tt.metadata,
			}

			result := engine.extractPRNumber(task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractPushErrorType tests error type extraction
func TestExtractPushErrorType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "non_fast_forward",
			input:    "gh_failed: non_fast_forward",
			expected: "non_fast_forward",
		},
		{
			name:     "authentication_failed",
			input:    "gh_failed: authentication_failed",
			expected: "authentication_failed",
		},
		{
			name:     "rate_limit",
			input:    "gh_failed: rate_limit",
			expected: "rate_limit",
		},
		{
			name:     "other_error",
			input:    "some other error",
			expected: "",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "gh_failed_with_empty_type",
			input:    "gh_failed: ",
			expected: "",
		},
		{
			name:     "gh_failed_colon_only",
			input:    "gh_failed:",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractPushErrorType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindImplementStep tests finding the implement step
func TestFindImplementStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		steps    []domain.Step
		expected int
	}{
		{
			name: "standard_template",
			steps: []domain.Step{
				{Name: "prepare", Type: domain.StepTypeGit, Status: "completed"},
				{Name: "implement", Type: domain.StepTypeAI, Status: "pending"},
				{Name: "validate", Type: domain.StepTypeValidation, Status: "pending"},
			},
			expected: 1,
		},
		{
			name: "no_implement_step",
			steps: []domain.Step{
				{Name: "prepare", Type: domain.StepTypeGit, Status: "completed"},
				{Name: "build", Type: domain.StepTypeAI, Status: "pending"},
				{Name: "validate", Type: domain.StepTypeValidation, Status: "pending"},
			},
			expected: 1, // Falls back to first AI step
		},
		{
			name: "implement_is_first",
			steps: []domain.Step{
				{Name: "implement", Type: domain.StepTypeAI, Status: "pending"},
				{Name: "validate", Type: domain.StepTypeValidation, Status: "pending"},
			},
			expected: 0,
		},
		{
			name: "multiple_ai_steps",
			steps: []domain.Step{
				{Name: "prepare", Type: domain.StepTypeGit, Status: "completed"},
				{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
				{Name: "implement", Type: domain.StepTypeAI, Status: "pending"},
				{Name: "review", Type: domain.StepTypeAI, Status: "pending"},
			},
			expected: 2, // Named "implement"
		},
		{
			name: "no_ai_steps",
			steps: []domain.Step{
				{Name: "prepare", Type: domain.StepTypeGit, Status: "completed"},
				{Name: "validate", Type: domain.StepTypeValidation, Status: "pending"},
			},
			expected: 0, // Default to 0
		},
		{
			name:     "empty_steps",
			steps:    []domain.Step{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newMockStore()
			engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

			task := &domain.Task{
				ID:          "test-task",
				WorkspaceID: "test-workspace",
				Steps:       tt.steps,
			}

			result := engine.findImplementStep(task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessCIFailureAction_NoHandler tests missing handler error
func TestProcessCIFailureAction_NoHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())
	// Don't set ciFailureHandler - leave it nil

	task := newTestTask(constants.TaskStatusCIFailed)

	err := engine.ProcessCIFailureAction(ctx, task, CIFailureViewLogs)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrExecutorNotFound)
}
