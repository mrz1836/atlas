package steps

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewCIExecutor(t *testing.T) {
	executor := NewCIExecutor()

	require.NotNil(t, executor)
}

func TestCIExecutor_Type(t *testing.T) {
	executor := NewCIExecutor()

	assert.Equal(t, domain.StepTypeCI, executor.Type())
}

func TestCIExecutor_Execute_Success(t *testing.T) {
	ctx := context.Background()
	executor := NewCIExecutor()

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "ci-wait",
		Type: domain.StepTypeCI,
		Config: map[string]any{
			"poll_interval": 10 * time.Millisecond,
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "ci-wait", result.StepName)
	assert.Contains(t, result.Output, "CI completed successfully")
	assert.Contains(t, result.Output, "3 polls") // placeholder polls 3 times
}

func TestCIExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewCIExecutor()
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCIExecutor_Execute_Timeout(t *testing.T) {
	ctx := context.Background()
	executor := NewCIExecutor()

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name:    "ci-wait",
		Type:    domain.StepTypeCI,
		Timeout: 1 * time.Millisecond, // Very short timeout
		Config: map[string]any{
			"poll_interval": 10 * time.Second, // Long interval
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrCITimeout)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Output, "timed out")
}

func TestCIExecutor_Execute_DefaultConfig(t *testing.T) {
	// This test runs with short timeout to test defaults are applied
	// but we can't easily verify the exact values
	ctx := context.Background()
	executor := NewCIExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name:   "ci",
		Type:   domain.StepTypeCI,
		Config: nil, // Use defaults
	}

	// Create a context with short timeout to prevent long test
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	// This will timeout because defaults are 30s poll interval
	result, err := executor.Execute(shortCtx, task, step)
	// Either succeeds or times out - both are valid outcomes
	if err != nil {
		// Could be ErrCITimeout or context.DeadlineExceeded depending on race
		require.Error(t, err)
		assert.Equal(t, "failed", result.Status)
	} else {
		// Polling completed before timeout (unlikely but possible)
		assert.Equal(t, "success", result.Status)
	}
}

func TestCIExecutor_Execute_Timing(t *testing.T) {
	ctx := context.Background()
	executor := NewCIExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name: "ci",
		Type: domain.StepTypeCI,
		Config: map[string]any{
			"poll_interval": 1 * time.Millisecond,
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.True(t, result.CompletedAt.After(result.StartedAt))
	assert.Positive(t, result.DurationMs)
}

func TestCIExecutor_Execute_StepTimeout(t *testing.T) {
	ctx := context.Background()
	executor := NewCIExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name:    "ci",
		Type:    domain.StepTypeCI,
		Timeout: 50 * time.Millisecond,
		Config: map[string]any{
			"poll_interval": 100 * time.Millisecond,
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrCITimeout)
	assert.Equal(t, "failed", result.Status)
}

func TestCIExecutor_Execute_PollingCount(t *testing.T) {
	ctx := context.Background()
	executor := NewCIExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name: "ci",
		Type: domain.StepTypeCI,
		Config: map[string]any{
			"poll_interval": 1 * time.Millisecond,
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Placeholder implementation polls 3 times
	assert.Contains(t, result.Output, "3 polls")
}
