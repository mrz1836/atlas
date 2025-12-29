package steps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewGitExecutor(t *testing.T) {
	executor := NewGitExecutor("/tmp/work")

	require.NotNil(t, executor)
	assert.Equal(t, "/tmp/work", executor.workDir)
}

func TestGitExecutor_Type(t *testing.T) {
	executor := NewGitExecutor("/tmp/work")

	assert.Equal(t, domain.StepTypeGit, executor.Type())
}

func TestGitExecutor_Execute_DefaultOperation(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work")

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{Name: "git", Type: domain.StepTypeGit}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "git", result.StepName)
	assert.Contains(t, result.Output, "commit")
	assert.Contains(t, result.Output, "/tmp/work")
}

func TestGitExecutor_Execute_Operations(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		expected  string
	}{
		{
			name:      "commit operation",
			operation: "commit",
			expected:  "Git commit placeholder",
		},
		{
			name:      "push operation",
			operation: "push",
			expected:  "Git push placeholder",
		},
		{
			name:      "create_pr operation",
			operation: "create_pr",
			expected:  "Git create_pr placeholder",
		},
		{
			name:      "smart_commit operation",
			operation: "smart_commit",
			expected:  "Git smart_commit placeholder",
		},
		{
			name:      "unknown operation",
			operation: "unknown",
			expected:  "Unknown git operation: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			executor := NewGitExecutor("/tmp/work")

			task := &domain.Task{ID: "task-123", CurrentStep: 0}
			step := &domain.StepDefinition{
				Name: "git",
				Type: domain.StepTypeGit,
				Config: map[string]any{
					"operation": tt.operation,
				},
			}

			result, err := executor.Execute(ctx, task, step)

			require.NoError(t, err)
			assert.Equal(t, "success", result.Status)
			assert.Contains(t, result.Output, tt.expected)
		})
	}
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

func TestGitExecutor_Execute_Timing(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work")

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "git", Type: domain.StepTypeGit}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestGitExecutor_Execute_NilConfig(t *testing.T) {
	ctx := context.Background()
	executor := NewGitExecutor("/tmp/work")

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name:   "git",
		Type:   domain.StepTypeGit,
		Config: nil,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	// Should default to commit
	assert.Contains(t, result.Output, "commit")
}
