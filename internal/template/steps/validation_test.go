package steps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockCommandRunner implements CommandRunner for testing.
type mockCommandRunner struct {
	results []mockCommandResult
	calls   []string
	index   int
}

type mockCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (m *mockCommandRunner) Run(_ context.Context, _, command string) (stdout, stderr string, exitCode int, err error) {
	m.calls = append(m.calls, command)
	if m.index >= len(m.results) {
		return "", "", 0, nil
	}
	r := m.results[m.index]
	m.index++
	return r.stdout, r.stderr, r.exitCode, r.err
}

func TestNewValidationExecutor(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	require.NotNil(t, executor)
	assert.Equal(t, "/tmp/work", executor.workDir)
	assert.NotNil(t, executor.runner)
}

func TestValidationExecutor_Type(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	assert.Equal(t, domain.StepTypeValidation, executor.Type())
}

func TestValidationExecutor_Execute_AllSuccess(t *testing.T) {
	ctx := context.Background()
	runner := &mockCommandRunner{
		results: []mockCommandResult{
			{stdout: "formatted", exitCode: 0},
			{stdout: "linted", exitCode: 0},
			{stdout: "tested", exitCode: 0},
		},
	}
	executor := NewValidationExecutorWithRunner("/tmp/work", runner)

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
		Config: domain.TaskConfig{
			ValidationCommands: []string{"format", "lint", "test"},
		},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "validate", result.StepName)
	assert.Contains(t, result.Output, "✓ format")
	assert.Contains(t, result.Output, "✓ lint")
	assert.Contains(t, result.Output, "✓ test")
	assert.Len(t, runner.calls, 3)
}

func TestValidationExecutor_Execute_FailsOnFirstError(t *testing.T) {
	ctx := context.Background()
	runner := &mockCommandRunner{
		results: []mockCommandResult{
			{stdout: "ok", exitCode: 0},
			{stdout: "lint output", stderr: "lint error", exitCode: 1, err: atlaserrors.ErrCommandFailed},
		},
	}
	executor := NewValidationExecutorWithRunner("/tmp/work", runner)

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
		Config: domain.TaskConfig{
			ValidationCommands: []string{"format", "lint", "test"},
		},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	assert.Contains(t, err.Error(), "lint")
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Output, "✗ Command failed: lint")
	assert.Contains(t, result.Output, "lint output")
	assert.Contains(t, result.Output, "lint error")
	assert.Len(t, runner.calls, 2) // Stopped after lint
}

func TestValidationExecutor_Execute_DefaultCommands(t *testing.T) {
	ctx := context.Background()
	runner := &mockCommandRunner{
		results: []mockCommandResult{
			{exitCode: 0},
			{exitCode: 0},
			{exitCode: 0},
		},
	}
	executor := NewValidationExecutorWithRunner("/tmp/work", runner)

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{}, // No validation commands
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, runner.calls, "magex format:fix")
	assert.Contains(t, runner.calls, "magex lint")
	assert.Contains(t, runner.calls, "magex test")
}

func TestValidationExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewValidationExecutor("/tmp/work")
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestValidationExecutor_Execute_CapturesOutput(t *testing.T) {
	ctx := context.Background()
	runner := &mockCommandRunner{
		results: []mockCommandResult{
			{stdout: "PASS\nAll tests passed", exitCode: 0},
		},
	}
	executor := NewValidationExecutorWithRunner("/tmp/work", runner)

	task := &domain.Task{
		ID:     "task-123",
		Config: domain.TaskConfig{ValidationCommands: []string{"test"}},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Contains(t, result.Output, "✓ test")
}

func TestValidationExecutor_Execute_EmptyCommands(t *testing.T) {
	ctx := context.Background()
	runner := &mockCommandRunner{
		results: []mockCommandResult{
			{exitCode: 0},
			{exitCode: 0},
			{exitCode: 0},
		},
	}
	executor := NewValidationExecutorWithRunner("/tmp/work", runner)

	task := &domain.Task{
		ID:     "task-123",
		Config: domain.TaskConfig{ValidationCommands: []string{}}, // Empty slice
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	// Should use defaults when empty
	assert.Len(t, runner.calls, 3)
}

func TestValidationExecutor_Execute_Timing(t *testing.T) {
	ctx := context.Background()
	runner := &mockCommandRunner{
		results: []mockCommandResult{{exitCode: 0}},
	}
	executor := NewValidationExecutorWithRunner("/tmp/work", runner)

	task := &domain.Task{
		ID:     "task-123",
		Config: domain.TaskConfig{ValidationCommands: []string{"test"}},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.True(t, result.CompletedAt.After(result.StartedAt) || result.CompletedAt.Equal(result.StartedAt))
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}
