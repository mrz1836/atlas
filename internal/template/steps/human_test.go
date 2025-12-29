package steps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewHumanExecutor(t *testing.T) {
	executor := NewHumanExecutor()

	require.NotNil(t, executor)
}

func TestHumanExecutor_Type(t *testing.T) {
	executor := NewHumanExecutor()

	assert.Equal(t, domain.StepTypeHuman, executor.Type())
}

func TestHumanExecutor_Execute_DefaultPrompt(t *testing.T) {
	ctx := context.Background()
	executor := NewHumanExecutor()

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{Name: "approve", Type: domain.StepTypeHuman}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Equal(t, "approve", result.StepName)
	assert.Equal(t, 0, result.StepIndex)
	assert.Equal(t, "Review required", result.Output)
	assert.Empty(t, result.Error)
}

func TestHumanExecutor_Execute_CustomPrompt(t *testing.T) {
	ctx := context.Background()
	executor := NewHumanExecutor()

	task := &domain.Task{ID: "task-123", CurrentStep: 2}
	step := &domain.StepDefinition{
		Name: "approve",
		Type: domain.StepTypeHuman,
		Config: map[string]any{
			"prompt": "Please review the implementation before merging",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Equal(t, "approve", result.StepName)
	assert.Equal(t, 2, result.StepIndex)
	assert.Equal(t, "Please review the implementation before merging", result.Output)
}

func TestHumanExecutor_Execute_EmptyPrompt(t *testing.T) {
	ctx := context.Background()
	executor := NewHumanExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name: "approve",
		Type: domain.StepTypeHuman,
		Config: map[string]any{
			"prompt": "",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	// Should use default when empty
	assert.Equal(t, "Review required", result.Output)
}

func TestHumanExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewHumanExecutor()
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "approve", Type: domain.StepTypeHuman}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestHumanExecutor_Execute_NilConfig(t *testing.T) {
	ctx := context.Background()
	executor := NewHumanExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name:   "approve",
		Type:   domain.StepTypeHuman,
		Config: nil,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Equal(t, "Review required", result.Output)
}

func TestHumanExecutor_Execute_Timing(t *testing.T) {
	ctx := context.Background()
	executor := NewHumanExecutor()

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "approve", Type: domain.StepTypeHuman}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.Equal(t, int64(0), result.DurationMs) // Should be instant
}
