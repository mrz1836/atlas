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
	"github.com/mrz1836/atlas/internal/validation"
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

// TestAIExecutor_Execute_AgentOverrideUsesAgentDefaultModel tests that when
// agent is overridden but model is not specified, the new agent's default model is used
func TestAIExecutor_Execute_AgentOverrideUsesAgentDefaultModel(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Test task",
		Config: domain.TaskConfig{
			Agent: domain.AgentClaude, // Task uses Claude
			Model: "opus",             // Task default model (Claude)
		},
	}
	step := &domain.StepDefinition{
		Name: "implement",
		Type: domain.StepTypeAI,
		Config: map[string]any{
			"agent": "gemini", // Override to Gemini
			// No model specified - should use Gemini's default
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, runner.request)
	assert.Equal(t, domain.AgentGemini, runner.request.Agent)
	// Should use Gemini's default model, not the task's "opus"
	assert.Equal(t, domain.AgentGemini.DefaultModel(), runner.request.Model)
}

// TestAIExecutor_Execute_AgentOverrideWithExplicitModel tests that explicit
// model is preserved when agent is overridden
func TestAIExecutor_Execute_AgentOverrideWithExplicitModel(t *testing.T) {
	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewAIExecutor(runner)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Test task",
		Config: domain.TaskConfig{
			Agent: domain.AgentClaude,
			Model: "opus",
		},
	}
	step := &domain.StepDefinition{
		Name: "implement",
		Type: domain.StepTypeAI,
		Config: map[string]any{
			"agent": "gemini",
			"model": "pro", // Explicit model for Gemini
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, runner.request)
	assert.Equal(t, domain.AgentGemini, runner.request.Agent)
	assert.Equal(t, "pro", runner.request.Model)
}

func TestAIExecutor_Execute_IncludePreviousErrors(t *testing.T) {
	t.Run("injects validation errors when flag is set", func(t *testing.T) {
		ctx := context.Background()
		runner := &mockAIRunner{
			result: &domain.AIResult{Output: "fixed"},
		}
		executor := NewAIExecutor(runner)

		// Create a task with previous validation step results containing errors
		task := &domain.Task{
			ID:          "task-123",
			Description: "Fix any issues",
			Config:      domain.TaskConfig{Model: "sonnet"},
			StepResults: []domain.StepResult{
				{
					StepName: "detect",
					Status:   "success",
					Metadata: map[string]any{
						"validation_failed": true,
						"pipeline_result": &validation.PipelineResult{
							Success: false,
							LintResults: []validation.Result{
								{
									Command: "golangci-lint",
									Success: false,
									Stderr:  "main.go:10:5: undefined: foo",
								},
							},
						},
					},
				},
			},
		}
		step := &domain.StepDefinition{
			Name: "fix",
			Type: domain.StepTypeAI,
			Config: map[string]any{
				"include_previous_errors": true,
			},
		}

		result, err := executor.Execute(ctx, task, step)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		require.NotNil(t, runner.request)
		// Verify validation errors were injected into the prompt
		assert.Contains(t, runner.request.Prompt, "Fix any issues")
		assert.Contains(t, runner.request.Prompt, "Validation Errors to Fix")
		assert.Contains(t, runner.request.Prompt, "main.go:10:5")
	})

	t.Run("no injection when no validation errors exist", func(t *testing.T) {
		ctx := context.Background()
		runner := &mockAIRunner{
			result: &domain.AIResult{Output: "done"},
		}
		executor := NewAIExecutor(runner)

		// Create a task with successful validation (no errors)
		task := &domain.Task{
			ID:          "task-123",
			Description: "Fix any issues",
			Config:      domain.TaskConfig{Model: "sonnet"},
			StepResults: []domain.StepResult{
				{
					StepName: "detect",
					Status:   "success",
					Metadata: map[string]any{
						"validation_failed": false, // No failures
						"pipeline_result": &validation.PipelineResult{
							Success: true,
						},
					},
				},
			},
		}
		step := &domain.StepDefinition{
			Name: "fix",
			Type: domain.StepTypeAI,
			Config: map[string]any{
				"include_previous_errors": true,
			},
		}

		result, err := executor.Execute(ctx, task, step)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		require.NotNil(t, runner.request)
		// Original prompt should be unchanged
		assert.Equal(t, "Fix any issues", runner.request.Prompt)
		assert.NotContains(t, runner.request.Prompt, "Validation Errors to Fix")
	})

	t.Run("no injection when flag is not set", func(t *testing.T) {
		ctx := context.Background()
		runner := &mockAIRunner{
			result: &domain.AIResult{Output: "done"},
		}
		executor := NewAIExecutor(runner)

		task := &domain.Task{
			ID:          "task-123",
			Description: "Fix any issues",
			Config:      domain.TaskConfig{Model: "sonnet"},
			StepResults: []domain.StepResult{
				{
					StepName: "detect",
					Status:   "success",
					Metadata: map[string]any{
						"validation_failed": true,
						"pipeline_result": &validation.PipelineResult{
							Success: false,
							LintResults: []validation.Result{
								{Command: "lint", Success: false, Stderr: "error"},
							},
						},
					},
				},
			},
		}
		step := &domain.StepDefinition{
			Name: "fix",
			Type: domain.StepTypeAI,
			// No include_previous_errors config
		}

		result, err := executor.Execute(ctx, task, step)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		// Prompt should NOT contain validation errors
		assert.Equal(t, "Fix any issues", runner.request.Prompt)
	})

	t.Run("no injection when no previous step results", func(t *testing.T) {
		ctx := context.Background()
		runner := &mockAIRunner{
			result: &domain.AIResult{Output: "done"},
		}
		executor := NewAIExecutor(runner)

		task := &domain.Task{
			ID:          "task-123",
			Description: "Fix any issues",
			Config:      domain.TaskConfig{Model: "sonnet"},
			StepResults: []domain.StepResult{}, // Empty
		}
		step := &domain.StepDefinition{
			Name: "fix",
			Type: domain.StepTypeAI,
			Config: map[string]any{
				"include_previous_errors": true,
			},
		}

		result, err := executor.Execute(ctx, task, step)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		assert.Equal(t, "Fix any issues", runner.request.Prompt)
	})
}
