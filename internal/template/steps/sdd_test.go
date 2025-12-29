package steps

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewSDDExecutor(t *testing.T) {
	runner := &mockAIRunner{}
	executor := NewSDDExecutor(runner, "/tmp/artifacts")

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
	assert.Equal(t, "/tmp/artifacts", executor.artifactsDir)
}

func TestSDDExecutor_Type(t *testing.T) {
	executor := NewSDDExecutor(&mockAIRunner{}, "/tmp")

	assert.Equal(t, domain.StepTypeSDD, executor.Type())
}

func TestSDDExecutor_Execute_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{
			Output: "# Specification\nThis is the specification...",
		},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Build a user auth system",
		CurrentStep: 0,
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{
		Name: "sdd-specify",
		Type: domain.StepTypeSDD,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "sdd-specify", result.StepName)
	assert.Contains(t, result.Output, "Specification")
	assert.NotEmpty(t, result.ArtifactPath)

	// Verify artifact was saved
	content, err := os.ReadFile(result.ArtifactPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Specification")
}

func TestSDDExecutor_Execute_AllCommands(t *testing.T) {
	tests := []struct {
		name    string
		command string
		prompt  string
	}{
		{
			name:    "specify command",
			command: "specify",
			prompt:  "generate a specification for:",
		},
		{
			name:    "plan command",
			command: "plan",
			prompt:  "create an implementation plan for:",
		},
		{
			name:    "tasks command",
			command: "tasks",
			prompt:  "break down into tasks:",
		},
		{
			name:    "checklist command",
			command: "checklist",
			prompt:  "create a review checklist for:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tmpDir := t.TempDir()

			runner := &mockAIRunner{
				result: &domain.AIResult{Output: "done"},
			}
			executor := NewSDDExecutor(runner, tmpDir)

			task := &domain.Task{
				ID:          "task-123",
				Description: "Build feature X",
				Config:      domain.TaskConfig{Model: "sonnet"},
			}
			step := &domain.StepDefinition{
				Name: "sdd",
				Type: domain.StepTypeSDD,
				Config: map[string]any{
					"sdd_command": tt.command,
				},
			}

			_, err := executor.Execute(ctx, task, step)

			require.NoError(t, err)
			require.NotNil(t, runner.request)
			assert.Contains(t, runner.request.Prompt, tt.prompt)
			assert.Contains(t, runner.request.Prompt, "Build feature X")
		})
	}
}

func TestSDDExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewSDDExecutor(&mockAIRunner{}, "/tmp")
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestSDDExecutor_Execute_RunnerError(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		err: atlaserrors.ErrClaudeInvocation,
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "claude invocation failed")
}

func TestSDDExecutor_Execute_NoArtifactsDir(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Empty(t, result.ArtifactPath)
}

func TestSDDExecutor_Execute_DefaultCommand(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{ID: "task-123", Description: "Build it"}
	step := &domain.StepDefinition{
		Name:   "sdd",
		Type:   domain.StepTypeSDD,
		Config: nil, // No command specified
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should default to "specify"
	assert.Contains(t, runner.request.Prompt, "specification for:")
}

func TestSDDExecutor_saveArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewSDDExecutor(&mockAIRunner{}, tmpDir)

	path, err := executor.saveArtifact("task-123", SDDCmdSpecify, "Test content")

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.True(t, filepath.IsAbs(path))

	content, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	assert.Equal(t, "Test content", string(content))

	// Verify directory structure
	dir := filepath.Dir(path)
	assert.Equal(t, "task-123", filepath.Base(dir))
}

func TestSDDExecutor_saveArtifact_EmptyDir(t *testing.T) {
	executor := NewSDDExecutor(&mockAIRunner{}, "")

	path, err := executor.saveArtifact("task-123", SDDCmdSpecify, "content")

	require.NoError(t, err)
	assert.Empty(t, path)
}
