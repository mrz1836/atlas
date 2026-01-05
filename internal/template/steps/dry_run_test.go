// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestDryRunPlan_String(t *testing.T) {
	tests := []struct {
		name     string
		plan     *DryRunPlan
		contains []string
	}{
		{
			name: "basic plan",
			plan: &DryRunPlan{
				StepName: "implement",
				StepType: "ai",
				WouldDo:  []string{"Execute AI", "Modify files"},
			},
			contains: []string{"[DRY-RUN]", "Ai Step", "implement", "Would:", "Execute AI", "Modify files"},
		},
		{
			name: "with description",
			plan: &DryRunPlan{
				StepName:    "validate",
				StepType:    "validation",
				Description: "Run validation commands",
				WouldDo:     []string{"Run lint", "Run test"},
			},
			contains: []string{"Description:", "Run validation commands"},
		},
		{
			name: "empty would do",
			plan: &DryRunPlan{
				StepName: "empty",
				StepType: "unknown",
			},
			contains: []string{"[DRY-RUN]", "Unknown Step"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.plan.String()
			for _, s := range tt.contains {
				assert.Contains(t, result, s)
			}
		})
	}
}

func TestDryRunPresenter_Plan_AI(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{
		WorkDir: "/test/workdir",
	})

	task := &domain.Task{
		Description: "fix null pointer bug",
	}

	step := &domain.StepDefinition{
		Name: "implement",
		Type: domain.StepTypeAI,
		Config: map[string]any{
			"model": "claude-opus-4-20250514",
		},
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "implement", plan.StepName)
	assert.Equal(t, "ai", plan.StepType)
	assert.Equal(t, "claude-opus-4-20250514", plan.Config["model"])
	assert.Equal(t, "fix null pointer bug", plan.Config["prompt"])
	assert.Equal(t, "/test/workdir", plan.Config["working_directory"])
	assert.Contains(t, plan.WouldDo, "Execute AI with model: claude-opus-4-20250514")
	assert.Contains(t, plan.WouldDo, "AI output is non-deterministic and cannot be predicted")
}

func TestDryRunPresenter_Plan_Validation(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{
		FormatCommands:    []string{"go fmt ./..."},
		LintCommands:      []string{"golangci-lint run"},
		TestCommands:      []string{"go test ./..."},
		PreCommitCommands: []string{"pre-commit run"},
	})

	task := &domain.Task{}
	step := &domain.StepDefinition{
		Name: "validate",
		Type: domain.StepTypeValidation,
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "validate", plan.StepName)
	assert.Equal(t, "validation", plan.StepType)
	assert.Equal(t, []string{"go fmt ./..."}, plan.Config["format_commands"])
	assert.Equal(t, []string{"golangci-lint run"}, plan.Config["lint_commands"])
	assert.Equal(t, []string{"go test ./..."}, plan.Config["test_commands"])
	assert.Contains(t, plan.WouldDo, "Run validation commands:")
	assert.Contains(t, plan.WouldDo, "Execution order: Format -> Lint|Test (parallel) -> Pre-commit")
}

func TestDryRunPresenter_Plan_Git_Commit(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})

	task := &domain.Task{
		Metadata: map[string]any{"branch": "fix/test-branch"},
	}
	step := &domain.StepDefinition{
		Name: "commit",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "commit",
		},
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "commit", plan.Config["operation"])
	assert.Contains(t, plan.WouldDo, "Analyze staged and unstaged changes")
	assert.Contains(t, plan.WouldDo, "Create git commit(s)")
}

func TestDryRunPresenter_Plan_Git_Push(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})

	task := &domain.Task{
		Metadata: map[string]any{"branch": "fix/test-branch"},
	}
	step := &domain.StepDefinition{
		Name: "push",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "push",
		},
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "push", plan.Config["operation"])
	assert.Equal(t, "fix/test-branch", plan.Config["branch"])
	assert.Equal(t, "origin", plan.Config["remote"])
	assert.Contains(t, plan.WouldDo, "Push branch 'fix/test-branch' to remote 'origin'")
}

func TestDryRunPresenter_Plan_Git_CreatePR(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{
		BaseBranch: "develop",
	})

	task := &domain.Task{
		Metadata: map[string]any{"branch": "feature/new-feature"},
	}
	step := &domain.StepDefinition{
		Name: "create_pr",
		Type: domain.StepTypeGit,
		Config: map[string]any{
			"operation": "create_pr",
		},
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "create_pr", plan.Config["operation"])
	assert.Equal(t, "develop", plan.Config["base_branch"])
	assert.Equal(t, "feature/new-feature", plan.Config["head_branch"])
	assert.Contains(t, plan.WouldDo, "Create pull request: feature/new-feature -> develop")
}

func TestDryRunPresenter_Plan_Human(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})

	task := &domain.Task{}
	step := &domain.StepDefinition{
		Name: "approve",
		Type: domain.StepTypeHuman,
		Config: map[string]any{
			"message": "Please review the changes",
		},
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "approve", plan.StepName)
	assert.Equal(t, "human", plan.StepType)
	assert.Equal(t, "Please review the changes", plan.Config["message"])
	assert.Contains(t, plan.WouldDo, "Pause task execution")
	assert.Contains(t, plan.WouldDo, "Wait for user to approve/reject")
}

func TestDryRunPresenter_Plan_CI(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{
		CIConfig: &config.CIConfig{
			PollInterval: 5 * time.Minute,
			Timeout:      1 * time.Hour,
		},
	})

	task := &domain.Task{}
	step := &domain.StepDefinition{
		Name: "wait_ci",
		Type: domain.StepTypeCI,
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "wait_ci", plan.StepName)
	assert.Equal(t, "ci", plan.StepType)
	assert.Equal(t, "5m0s", plan.Config["poll_interval"])
	assert.Equal(t, "1h0m0s", plan.Config["timeout"])
	assert.Contains(t, plan.WouldDo, "Wait for CI pipeline to complete")
}

func TestDryRunPresenter_Plan_Verify(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})

	task := &domain.Task{}
	step := &domain.StepDefinition{
		Name: "verify",
		Type: domain.StepTypeVerify,
		Config: map[string]any{
			"model": "claude-opus-4-20250514",
		},
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "verify", plan.StepName)
	assert.Equal(t, "verify", plan.StepType)
	assert.Equal(t, "claude-opus-4-20250514", plan.Config["model"])
	assert.Contains(t, plan.WouldDo, "Run AI verification with model: claude-opus-4-20250514")
	assert.Contains(t, plan.WouldDo, "Detect garbage files")
}

func TestDryRunPresenter_Plan_SDD(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})

	task := &domain.Task{
		Description: "implement user auth",
	}
	step := &domain.StepDefinition{
		Name: "sdd",
		Type: domain.StepTypeSDD,
	}

	plan := presenter.Plan(task, step)

	assert.Equal(t, "sdd", plan.StepName)
	assert.Equal(t, "sdd", plan.StepType)
	assert.Equal(t, "implement user auth", plan.Config["prompt"])
	assert.Contains(t, plan.WouldDo, "Generate SDD specification via AI")
}

func TestDryRunExecutorWrapper_Execute(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{
		WorkDir: "/test/dir",
	})
	wrapper := NewDryRunExecutorWrapper(domain.StepTypeAI, presenter)

	task := &domain.Task{
		CurrentStep: 2,
		Description: "test task",
	}
	step := &domain.StepDefinition{
		Name: "test_step",
		Type: domain.StepTypeAI,
	}

	result, err := wrapper.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, 2, result.StepIndex)
	assert.Equal(t, "test_step", result.StepName)
	assert.Equal(t, "would_execute", result.Status)
	assert.True(t, result.Metadata["dry_run"].(bool))
	assert.NotNil(t, result.Metadata["plan"])
	assert.Contains(t, result.Output, "[DRY-RUN]")
}

func TestDryRunExecutorWrapper_Type(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})

	tests := []struct {
		stepType domain.StepType
	}{
		{domain.StepTypeAI},
		{domain.StepTypeValidation},
		{domain.StepTypeGit},
		{domain.StepTypeHuman},
		{domain.StepTypeCI},
		{domain.StepTypeVerify},
		{domain.StepTypeSDD},
	}

	for _, tt := range tests {
		t.Run(string(tt.stepType), func(t *testing.T) {
			wrapper := NewDryRunExecutorWrapper(tt.stepType, presenter)
			assert.Equal(t, tt.stepType, wrapper.Type())
		})
	}
}

func TestNewDryRunRegistry(t *testing.T) {
	registry := NewDryRunRegistry(ExecutorDeps{
		WorkDir:    "/test",
		BaseBranch: "main",
	})

	// Verify all step types are registered
	stepTypes := []domain.StepType{
		domain.StepTypeAI,
		domain.StepTypeValidation,
		domain.StepTypeGit,
		domain.StepTypeHuman,
		domain.StepTypeSDD,
		domain.StepTypeCI,
		domain.StepTypeVerify,
	}

	for _, st := range stepTypes {
		t.Run(string(st), func(t *testing.T) {
			assert.True(t, registry.Has(st), "registry should have %s executor", st)

			executor, err := registry.Get(st)
			require.NoError(t, err)
			assert.Equal(t, st, executor.Type())
		})
	}
}

func TestDryRunExecutorWrapper_NoSideEffects(t *testing.T) {
	// This test verifies that dry-run executors don't actually execute anything
	registry := NewDryRunRegistry(ExecutorDeps{
		WorkDir: "/nonexistent/path",
	})

	task := &domain.Task{
		Description: "test",
		Metadata:    map[string]any{"branch": "test-branch"},
	}

	// Test each step type - none should fail even with invalid paths
	steps := []domain.StepDefinition{
		{Name: "ai", Type: domain.StepTypeAI},
		{Name: "validation", Type: domain.StepTypeValidation},
		{Name: "git", Type: domain.StepTypeGit, Config: map[string]any{"operation": "commit"}},
		{Name: "human", Type: domain.StepTypeHuman},
		{Name: "ci", Type: domain.StepTypeCI},
		{Name: "verify", Type: domain.StepTypeVerify},
		{Name: "sdd", Type: domain.StepTypeSDD},
	}

	for _, step := range steps {
		t.Run(step.Name, func(t *testing.T) {
			executor, err := registry.Get(step.Type)
			require.NoError(t, err)

			result, err := executor.Execute(context.Background(), task, &step)
			require.NoError(t, err, "dry-run executor should not fail")
			assert.Equal(t, "would_execute", result.Status)
			assert.True(t, result.Metadata["dry_run"].(bool))

			// Verify plan was generated
			plan, ok := result.Metadata["plan"].(*DryRunPlan)
			require.True(t, ok, "metadata should contain plan")
			assert.NotEmpty(t, plan.WouldDo, "plan should have actions")
		})
	}
}

func TestDryRunPresenter_Plan_DefaultValues(t *testing.T) {
	// Test with minimal deps to ensure defaults are applied
	presenter := NewDryRunPresenter(ExecutorDeps{})

	t.Run("AI defaults to sonnet model", func(t *testing.T) {
		plan := presenter.Plan(&domain.Task{}, &domain.StepDefinition{
			Name: "ai",
			Type: domain.StepTypeAI,
		})
		// Model should default if not in config
		assert.Contains(t, plan.WouldDo[0], "claude-sonnet-4-20250514")
	})

	t.Run("Git PR defaults to main base branch", func(t *testing.T) {
		plan := presenter.Plan(&domain.Task{Metadata: map[string]any{"branch": "feat"}}, &domain.StepDefinition{
			Name:   "pr",
			Type:   domain.StepTypeGit,
			Config: map[string]any{"operation": "create_pr"},
		})
		assert.Equal(t, "main", plan.Config["base_branch"])
	})

	t.Run("CI defaults to standard intervals", func(t *testing.T) {
		plan := presenter.Plan(&domain.Task{}, &domain.StepDefinition{
			Name: "ci",
			Type: domain.StepTypeCI,
		})
		assert.Equal(t, "2m0s", plan.Config["poll_interval"])
		assert.Equal(t, "30m0s", plan.Config["timeout"])
	})
}

func TestDryRunExecutorWrapper_ContextCancellation(t *testing.T) {
	presenter := NewDryRunPresenter(ExecutorDeps{})
	wrapper := NewDryRunExecutorWrapper(domain.StepTypeAI, presenter)

	// Even with canceled context, dry-run should work (it doesn't do real I/O)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := wrapper.Execute(ctx, &domain.Task{}, &domain.StepDefinition{
		Name: "test",
		Type: domain.StepTypeAI,
	})

	// Dry-run doesn't check context since it's instant
	require.NoError(t, err)
	assert.NotNil(t, result)
}
