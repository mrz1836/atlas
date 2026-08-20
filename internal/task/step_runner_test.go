package task

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// TestApplyAgentModelOverride exercises the pure single-layer override helper
// directly, covering every branch of the agent-changed/model-defaulting logic.
func TestApplyAgentModelOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		agent         domain.Agent
		model         string
		overrideAgent string
		overrideModel string
		expectedAgent domain.Agent
		expectedModel string
	}{
		{
			name:          "empty_overrides_leave_values_unchanged",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "",
			overrideModel: "",
			expectedAgent: domain.AgentClaude,
			expectedModel: "sonnet",
		},
		{
			name:          "model_only_override_keeps_agent",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "",
			overrideModel: "opus",
			expectedAgent: domain.AgentClaude,
			expectedModel: "opus",
		},
		{
			name:          "agent_switch_without_model_uses_new_agent_default",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "gemini",
			overrideModel: "",
			expectedAgent: domain.AgentGemini,
			expectedModel: domain.AgentGemini.DefaultModel(),
		},
		{
			name:          "agent_switch_with_model_uses_override_model",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "gemini",
			overrideModel: "pro",
			expectedAgent: domain.AgentGemini,
			expectedModel: "pro",
		},
		{
			name:          "same_agent_override_keeps_existing_model",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "claude",
			overrideModel: "",
			expectedAgent: domain.AgentClaude,
			expectedModel: "sonnet",
		},
		{
			name:          "unknown_agent_switch_without_model_yields_empty_default",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "mystery",
			overrideModel: "",
			expectedAgent: domain.Agent("mystery"),
			expectedModel: "", // unknown agents have no default model
		},
		{
			name:          "empty_agent_override_still_applies_model",
			agent:         domain.AgentClaude,
			model:         "sonnet",
			overrideAgent: "",
			overrideModel: "haiku",
			expectedAgent: domain.AgentClaude,
			expectedModel: "haiku",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agent, model := applyAgentModelOverride(tt.agent, tt.model, tt.overrideAgent, tt.overrideModel)
			assert.Equal(t, tt.expectedAgent, agent, "agent mismatch")
			assert.Equal(t, tt.expectedModel, model, "model mismatch")
		})
	}
}

// TestResolveStepAgentModel_OpsConfigLayer covers the operations-config tier of
// ResolveStepAgentModel, including agent switching and step-type routing that
// the existing agent/model precedence tests do not exercise.
func TestResolveStepAgentModel_OpsConfigLayer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		taskAgent     domain.Agent
		taskModel     string
		stepName      string
		stepType      domain.StepType
		stepConfig    map[string]any
		opsConfig     *config.OperationsConfig
		expectedAgent domain.Agent
		expectedModel string
	}{
		{
			name:      "ops_config_agent_switch_applies_new_agent_default_model",
			taskAgent: domain.AgentClaude,
			taskModel: "sonnet",
			stepName:  "implement",
			stepType:  domain.StepTypeAI,
			opsConfig: &config.OperationsConfig{
				Implement: config.OperationAIConfig{Agent: "gemini"},
			},
			expectedAgent: domain.AgentGemini,
			expectedModel: domain.AgentGemini.DefaultModel(),
		},
		{
			name:      "step_config_model_layers_on_top_of_ops_config_agent",
			taskAgent: domain.AgentClaude,
			taskModel: "sonnet",
			stepName:  "implement",
			stepType:  domain.StepTypeAI,
			stepConfig: map[string]any{
				"model": "opus",
			},
			opsConfig: &config.OperationsConfig{
				Implement: config.OperationAIConfig{Agent: "gemini", Model: "pro"},
			},
			expectedAgent: domain.AgentGemini, // from ops config
			expectedModel: "opus",             // step config wins for model
		},
		{
			name:      "verify_step_type_routes_to_verify_ops_config",
			taskAgent: domain.AgentClaude,
			taskModel: "sonnet",
			stepName:  "cross-check",
			stepType:  domain.StepTypeVerify,
			opsConfig: &config.OperationsConfig{
				Verify: config.OperationAIConfig{Model: "opus"},
			},
			expectedAgent: domain.AgentClaude,
			expectedModel: "opus",
		},
		{
			name:      "sdd_step_type_routes_to_sdd_ops_config",
			taskAgent: domain.AgentClaude,
			taskModel: "sonnet",
			stepName:  "sdd-plan",
			stepType:  domain.StepTypeSDD,
			opsConfig: &config.OperationsConfig{
				SDD: config.OperationAIConfig{Agent: "codex"},
			},
			expectedAgent: domain.AgentCodex,
			expectedModel: domain.AgentCodex.DefaultModel(),
		},
		{
			name:      "empty_ops_config_for_step_falls_back_to_task_defaults",
			taskAgent: domain.AgentClaude,
			taskModel: "sonnet",
			stepName:  "unmatched-step",
			stepType:  domain.StepTypeAI,
			opsConfig: &config.OperationsConfig{
				Implement: config.OperationAIConfig{Agent: "gemini"},
			},
			expectedAgent: domain.AgentClaude,
			expectedModel: "sonnet",
		},
		{
			name:          "nil_ops_config_and_nil_step_config_uses_task_defaults",
			taskAgent:     domain.AgentClaude,
			taskModel:     "opus",
			stepName:      "implement",
			stepType:      domain.StepTypeAI,
			opsConfig:     nil,
			expectedAgent: domain.AgentClaude,
			expectedModel: "opus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			task := &domain.Task{
				Config: domain.TaskConfig{Agent: tt.taskAgent, Model: tt.taskModel},
			}
			step := &domain.StepDefinition{
				Name:   tt.stepName,
				Type:   tt.stepType,
				Config: tt.stepConfig,
			}

			agent, model := ResolveStepAgentModel(task, step, tt.opsConfig)
			assert.Equal(t, tt.expectedAgent, agent, "agent mismatch")
			assert.Equal(t, tt.expectedModel, model, "model mismatch")
		})
	}
}

// TestEngine_BuildStepLogEvent verifies that buildStepLogEvent enriches log
// events with the correct fields: agent/model only for AI/verify steps, and
// duration_ms only when a positive duration is supplied.
func TestEngine_BuildStepLogEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		stepType       domain.StepType
		durationMs     int64
		expectAgent    bool
		expectDuration bool
	}{
		{
			name:           "ai_step_includes_agent_model_and_duration",
			stepType:       domain.StepTypeAI,
			durationMs:     150,
			expectAgent:    true,
			expectDuration: true,
		},
		{
			name:           "verify_step_includes_agent_model",
			stepType:       domain.StepTypeVerify,
			durationMs:     0,
			expectAgent:    true,
			expectDuration: false,
		},
		{
			name:           "git_step_omits_agent_model",
			stepType:       domain.StepTypeGit,
			durationMs:     0,
			expectAgent:    false,
			expectDuration: false,
		},
		{
			name:           "validation_step_with_duration_no_agent",
			stepType:       domain.StepTypeValidation,
			durationMs:     42,
			expectAgent:    false,
			expectDuration: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := zerolog.New(&buf)
			store := newMockStore()
			registry := steps.NewExecutorRegistry()
			engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

			task := &domain.Task{
				ID: "task-log",
				Config: domain.TaskConfig{
					Agent: domain.AgentClaude,
					Model: "opus",
				},
			}
			step := &domain.StepDefinition{Name: "implement", Type: tt.stepType}

			engine.buildStepLogEvent(task, step, zerolog.InfoLevel, tt.durationMs).Msg("executing step")

			var fields map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &fields))

			assert.Equal(t, "task-log", fields["task_id"])
			assert.Equal(t, "implement", fields["step_name"])
			assert.Equal(t, string(tt.stepType), fields["step_type"])

			if tt.expectAgent {
				assert.Equal(t, "claude", fields["agent"])
				assert.Equal(t, "opus", fields["model"])
			} else {
				assert.NotContains(t, fields, "agent")
				assert.NotContains(t, fields, "model")
			}

			if tt.expectDuration {
				require.Contains(t, fields, "duration_ms")
				assert.InDelta(t, float64(tt.durationMs), fields["duration_ms"], 0.001)
			} else {
				assert.NotContains(t, fields, "duration_ms")
			}
		})
	}
}

// TestEngine_ShouldSkipForNoIssues covers the direct branches of
// shouldSkipForNoIssues for every step type and the missing/false flag paths.
func TestEngine_ShouldSkipForNoIssues(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	tests := []struct {
		name     string
		metadata map[string]any
		step     *domain.StepDefinition
		expected bool
	}{
		{
			name:     "ai_step_with_no_issues_is_skipped",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeAI},
			expected: true,
		},
		{
			name:     "validation_non_detect_only_is_skipped",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeValidation, Config: map[string]any{"detect_only": false}},
			expected: true,
		},
		{
			name:     "validation_detect_only_is_not_skipped",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeValidation, Config: map[string]any{"detect_only": true}},
			expected: false,
		},
		{
			name:     "git_step_is_never_skipped_for_no_issues",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeGit},
			expected: false,
		},
		{
			name:     "ci_step_is_never_skipped_for_no_issues",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeCI},
			expected: false,
		},
		{
			name:     "verify_step_is_never_skipped_for_no_issues",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeVerify},
			expected: false,
		},
		{
			name:     "sdd_step_is_never_skipped_for_no_issues",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeSDD},
			expected: false,
		},
		{
			name:     "loop_step_is_never_skipped_for_no_issues",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeLoop},
			expected: false,
		},
		{
			name:     "human_step_is_never_skipped_for_no_issues",
			metadata: map[string]any{"no_issues_detected": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeHuman},
			expected: false,
		},
		{
			name:     "missing_flag_is_not_skipped",
			metadata: map[string]any{"other": true},
			step:     &domain.StepDefinition{Type: domain.StepTypeAI},
			expected: false,
		},
		{
			name:     "flag_false_is_not_skipped",
			metadata: map[string]any{"no_issues_detected": false},
			step:     &domain.StepDefinition{Type: domain.StepTypeAI},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			task := &domain.Task{ID: "task-noissues", Metadata: tt.metadata}
			assert.Equal(t, tt.expected, engine.shouldSkipForNoIssues(task, tt.step))
		})
	}
}

// TestEngine_ShouldSkipStep_SkipCondition covers the skip_condition branch of
// shouldSkipStep, which routes through evaluateSkipCondition and
// hasSubstantiveDescription.
func TestEngine_ShouldSkipStep_SkipCondition(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	const longDesc = "this is a substantive bug description that exceeds twenty chars"
	const shortDesc = "fix lint"

	tests := []struct {
		name        string
		description string
		condition   string
		expected    bool
	}{
		{
			name:        "has_description_true_when_substantive",
			description: longDesc,
			condition:   "has_description",
			expected:    true,
		},
		{
			name:        "has_description_false_when_short",
			description: shortDesc,
			condition:   "has_description",
			expected:    false,
		},
		{
			name:        "no_description_true_when_short",
			description: shortDesc,
			condition:   "no_description",
			expected:    true,
		},
		{
			name:        "no_description_false_when_substantive",
			description: longDesc,
			condition:   "no_description",
			expected:    false,
		},
		{
			name:        "unknown_condition_does_not_skip",
			description: longDesc,
			condition:   "totally_unknown",
			expected:    false,
		},
		{
			name:        "empty_condition_ignored",
			description: longDesc,
			condition:   "",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			task := &domain.Task{
				ID:          "task-skipcond",
				Description: tt.description,
			}
			// Required=true and nil metadata isolate the skip_condition branch:
			// without a matching condition the step should NOT be skipped.
			step := &domain.StepDefinition{
				Name:     "conditional",
				Type:     domain.StepTypeAI,
				Required: true,
				Config:   map[string]any{"skip_condition": tt.condition},
			}
			assert.Equal(t, tt.expected, engine.shouldSkipStep(task, step))
		})
	}
}

// TestEngine_HandleSkippedStep verifies that a skipped step is marked, recorded
// in step results, and the task advances to the next step with a checkpoint.
func TestEngine_HandleSkippedStep(t *testing.T) {
	t.Parallel()

	t.Run("marks_skipped_records_result_and_advances", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-skip",
			WorkspaceID: "ws",
			CurrentStep: 0,
			Steps: []domain.Step{
				{Name: "push", Type: domain.StepTypeGit, Status: constants.StepStatusPending},
			},
			StepResults: []domain.StepResult{},
			// no_issues_detected drives an informative skip reason for an AI step.
			Metadata: map[string]any{"no_issues_detected": true},
		}
		step := &domain.StepDefinition{Name: "push", Type: domain.StepTypeAI, Required: true}

		err := engine.handleSkippedStep(ctx, task, step)
		require.NoError(t, err)

		// Step marked skipped.
		assert.Equal(t, constants.StepStatusSkipped, task.Steps[0].Status)
		// Advanced past the skipped step.
		assert.Equal(t, 1, task.CurrentStep)
		// Result recorded with skip reason.
		require.Len(t, task.StepResults, 1)
		assert.Equal(t, constants.StepStatusSkipped, task.StepResults[0].Status)
		assert.Equal(t, "push", task.StepResults[0].StepName)
		assert.Contains(t, task.StepResults[0].Output, "no issues to fix")
		// Checkpoint saved once by advanceToNextStep.
		assert.Equal(t, 1, store.updateCalls)
	})

	t.Run("returns_error_when_checkpoint_save_fails", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		store := newMockStore()
		store.updateErr = errTest
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-skip-fail",
			WorkspaceID: "ws",
			CurrentStep: 0,
			Steps: []domain.Step{
				{Name: "optional", Type: domain.StepTypeHuman, Status: constants.StepStatusPending},
			},
			StepResults: []domain.StepResult{},
		}
		step := &domain.StepDefinition{Name: "optional", Type: domain.StepTypeHuman, Required: false}

		err := engine.handleSkippedStep(ctx, task, step)
		require.Error(t, err)
		require.ErrorIs(t, err, errTest)
		// Result is still recorded even though the checkpoint save failed.
		require.Len(t, task.StepResults, 1)
		assert.Contains(t, task.StepResults[0].Output, "optional step not enabled")
	})
}
