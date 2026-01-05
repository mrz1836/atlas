package steps

// This test suite uses mockAIRunner to simulate AI execution without making real API calls.
// IMPORTANT: Tests NEVER make real API calls or use production API keys.
// All AI responses are pre-configured mock data to ensure test isolation.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockAIRunner implements ai.Runner for testing without making real API calls.
// It returns pre-configured results and captures requests for verification.
// This ensures tests are fast, deterministic, and never require real API keys.
type mockAIRunner struct {
	result  *domain.AIResult
	err     error
	request *domain.AIRequest
}

func (m *mockAIRunner) Run(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	m.request = req
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestNewAIExecutor(t *testing.T) {
	runner := &mockAIRunner{}
	executor := NewAIExecutor(runner)

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
}

func TestAIExecutor_Type(t *testing.T) {
	executor := NewAIExecutor(&mockAIRunner{})

	assert.Equal(t, domain.StepTypeAI, executor.Type())
}

func TestAIExecutor_Execute_Success(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{
			Output:       "Implementation complete",
			SessionID:    "test-session",
			NumTurns:     5,
			DurationMs:   5000,
			FilesChanged: []string{"file1.go", "file2.go"},
		},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Fix the bug",
		CurrentStep: 0,
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "implement", result.StepName)
	assert.Equal(t, 0, result.StepIndex)
	assert.Equal(t, "Implementation complete", result.Output)
	assert.Contains(t, result.FilesChanged, "file1.go")
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	// AC #2: Verify session_id and num_turns are captured
	assert.Equal(t, "test-session", result.SessionID)
	assert.Equal(t, 5, result.NumTurns)
}

func TestAIExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewAIExecutor(&mockAIRunner{})
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestAIExecutor_Execute_RunnerError(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		err: atlaserrors.ErrClaudeInvocation,
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "claude invocation failed")
}

func TestAIExecutor_Execute_PassesCorrectRequest(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Fix the null pointer",
		CurrentStep: 1,
		Config: domain.TaskConfig{
			Model:          "sonnet",
			MaxTurns:       10,
			Timeout:        5 * time.Minute,
			PermissionMode: "plan",
		},
	}
	step := &domain.StepDefinition{
		Name: "implement",
		Type: domain.StepTypeAI,
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, runner.request)
	assert.Equal(t, "Fix the null pointer", runner.request.Prompt)
	assert.Equal(t, "sonnet", runner.request.Model)
	assert.Equal(t, 10, runner.request.MaxTurns)
	assert.Equal(t, 5*time.Minute, runner.request.Timeout)
	assert.Equal(t, "plan", runner.request.PermissionMode)
}

func TestAIExecutor_Execute_StepConfigOverrides(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Base description",
		Config:      domain.TaskConfig{Model: "opus"},
	}
	step := &domain.StepDefinition{
		Name: "implement",
		Type: domain.StepTypeAI,
		Config: map[string]any{
			"permission_mode": "plan",
			"prompt_template": "Analyze this",
			"model":           "sonnet",
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, runner.request)
	assert.Equal(t, "Analyze this: Base description", runner.request.Prompt)
	assert.Equal(t, "plan", runner.request.PermissionMode)
	assert.Equal(t, "sonnet", runner.request.Model)
}

func TestAIExecutor_Execute_NilStepConfig(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Test task",
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{
		Name:   "implement",
		Type:   domain.StepTypeAI,
		Config: nil,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}

func TestAIExecutor_Execute_StepTimeout(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{
		Name:    "implement",
		Type:    domain.StepTypeAI,
		Timeout: 1 * time.Minute,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}
