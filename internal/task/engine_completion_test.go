package task

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/validation"
)

// completeEventCounter returns a ProgressCallback that counts "complete" events
// and records their status, so tests can assert exactly-one-completion semantics.
func completeEventCounter(count *int, statuses *[]string) StepProgressCallback {
	return func(event StepProgressEvent) {
		if event.Type == "complete" {
			*count++
			*statuses = append(*statuses, event.Status)
		}
	}
}

func newValidationStep() *domain.StepDefinition {
	return &domain.StepDefinition{Type: domain.StepTypeValidation, Name: "validate"}
}

func newValidationFailureResult() *domain.StepResult {
	return &domain.StepResult{
		Metadata: map[string]any{
			"pipeline_result": &validation.PipelineResult{FailedStepName: "build"},
		},
	}
}

// TestTryValidationRetry_SuccessEmitsNoCompleteEvent guards the fix for the
// duplicate "✓ ... completed" line: on retry success, tryValidationRetry must NOT
// emit a complete event (the main run loop emits the single completion). Before
// the fix it emitted one here AND the run loop emitted another, producing two
// identical completion lines.
func TestTryValidationRetry_SuccessEmitsNoCompleteEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	workDir := t.TempDir()

	var completeCount int
	var statuses []string

	cfg := DefaultEngineConfig()
	cfg.ProgressCallback = completeEventCounter(&completeCount, &statuses)

	engine := NewEngine(store, nil, cfg, zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, _ int, _ *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			return &validation.RetryResult{
				Success:        true,
				AttemptNumber:  1,
				PipelineResult: &validation.PipelineResult{DurationMs: 500},
				AIResult:       &domain.AIResult{FilesChanged: []string{"fixed.go"}},
			}, nil
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata:    map[string]any{"worktree_dir": workDir},
	}

	newResult, err := engine.tryValidationRetry(ctx, task, newValidationStep(), newValidationFailureResult(), errFailed)

	require.NoError(t, err)
	require.NotNil(t, newResult)
	assert.Equal(t, constants.StepStatusSuccess, newResult.Status)
	assert.Equal(t, 1, newResult.Metadata["retry_attempt"], "converted result carries retry metadata for the run loop's completion event")
	assert.Equal(t, 0, completeCount, "tryValidationRetry must not emit a complete event on success")
}

// TestTryValidationRetry_ExhaustedReturnsOriginalNoCompleteEvent confirms that
// when all retry attempts fail, tryValidationRetry returns the original result and
// error unchanged and emits no complete event (the deferred failure completion is
// emitted by handleStepExecutionResult, not here).
func TestTryValidationRetry_ExhaustedReturnsOriginalNoCompleteEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	workDir := t.TempDir()

	var completeCount int
	var statuses []string

	cfg := DefaultEngineConfig()
	cfg.ProgressCallback = completeEventCounter(&completeCount, &statuses)

	engine := NewEngine(store, nil, cfg, zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{
		isEnabled:   true,
		maxAttempts: 3,
		retryWithAIFunc: func(_ context.Context, _ *validation.PipelineResult, _ string, attempt int, _ *validation.RunnerConfig, _ domain.Agent, _ string, _ validation.AICompleteCallback) (*validation.RetryResult, error) {
			return &validation.RetryResult{Success: false, AttemptNumber: attempt}, errRetryFailed
		},
	}

	task := &domain.Task{
		ID:          "test-task-1",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusRunning,
		Metadata:    map[string]any{"worktree_dir": workDir},
	}

	originalResult := newValidationFailureResult()
	gotResult, err := engine.tryValidationRetry(ctx, task, newValidationStep(), originalResult, errFailed)

	require.ErrorIs(t, err, errFailed, "original error is returned unchanged when retries are exhausted")
	assert.Same(t, originalResult, gotResult, "original result is returned unchanged when retries are exhausted")
	assert.Equal(t, 0, completeCount, "tryValidationRetry emits no complete event on the exhausted path")
}

// TestTryValidationRetry_NonValidationStepIsNoop confirms non-validation steps
// short-circuit without emitting completion events or invoking the retry handler.
func TestTryValidationRetry_NonValidationStepIsNoop(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()

	var completeCount int
	var statuses []string

	cfg := DefaultEngineConfig()
	cfg.ProgressCallback = completeEventCounter(&completeCount, &statuses)

	engine := NewEngine(store, nil, cfg, zerolog.Nop())
	engine.validationRetryHandler = &mockValidationRetryHandler{isEnabled: true, maxAttempts: 3}

	step := &domain.StepDefinition{Type: domain.StepTypeAI, Name: "implement"}
	originalResult := &domain.StepResult{Status: constants.StepStatusFailed}

	gotResult, err := engine.tryValidationRetry(ctx, &domain.Task{ID: "t"}, step, originalResult, errFailed)

	require.ErrorIs(t, err, errFailed)
	assert.Same(t, originalResult, gotResult)
	assert.Equal(t, 0, completeCount)
}
