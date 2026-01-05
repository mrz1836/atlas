// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/mrz1836/atlas/internal/domain"
)

// DryRunPlan represents what a step would do without executing.
type DryRunPlan struct {
	StepName    string         `json:"step_name"`
	StepType    string         `json:"step_type"`
	Description string         `json:"description"`
	WouldDo     []string       `json:"would_do"`
	Config      map[string]any `json:"config,omitempty"`
}

// String returns a human-readable representation of the plan.
func (p *DryRunPlan) String() string {
	var sb strings.Builder
	caser := cases.Title(language.English)
	sb.WriteString(fmt.Sprintf("[DRY-RUN] %s Step: '%s'\n", caser.String(p.StepType), p.StepName))
	if p.Description != "" {
		sb.WriteString(fmt.Sprintf("  Description: %s\n", p.Description))
	}
	if len(p.WouldDo) > 0 {
		sb.WriteString("  Would:\n")
		for _, action := range p.WouldDo {
			sb.WriteString(fmt.Sprintf("    - %s\n", action))
		}
	}
	return sb.String()
}

// DryRunPresenter generates plans for what steps would do.
type DryRunPresenter struct {
	deps ExecutorDeps
}

// NewDryRunPresenter creates a new presenter with dependencies.
func NewDryRunPresenter(deps ExecutorDeps) *DryRunPresenter {
	return &DryRunPresenter{deps: deps}
}

// Plan generates a dry-run plan for the given step.
func (p *DryRunPresenter) Plan(task *domain.Task, step *domain.StepDefinition) *DryRunPlan {
	plan := &DryRunPlan{
		StepName:    step.Name,
		StepType:    string(step.Type),
		Description: step.Description,
		Config:      make(map[string]any),
	}

	switch step.Type {
	case domain.StepTypeAI:
		p.planAI(plan, task, step)
	case domain.StepTypeValidation:
		p.planValidation(plan, task, step)
	case domain.StepTypeGit:
		p.planGit(plan, task, step)
	case domain.StepTypeHuman:
		p.planHuman(plan, task, step)
	case domain.StepTypeCI:
		p.planCI(plan, task, step)
	case domain.StepTypeVerify:
		p.planVerify(plan, task, step)
	case domain.StepTypeSDD:
		p.planSDD(plan, task, step)
	default:
		plan.WouldDo = append(plan.WouldDo, fmt.Sprintf("Execute unknown step type: %s", step.Type))
	}

	return plan
}

// planAI generates a plan for AI step execution.
func (p *DryRunPresenter) planAI(plan *DryRunPlan, task *domain.Task, step *domain.StepDefinition) {
	// Extract model from step config or template
	model := "claude-sonnet-4-20250514" // default
	if m, ok := step.Config["model"].(string); ok && m != "" {
		model = m
	}

	plan.Config["model"] = model
	plan.Config["prompt"] = task.Description
	plan.Config["working_directory"] = p.deps.WorkDir

	plan.WouldDo = append(plan.WouldDo,
		fmt.Sprintf("Execute AI with model: %s", model),
		fmt.Sprintf("Prompt: %q", task.Description),
		"AI output is non-deterministic and cannot be predicted",
	)
}

// planValidation generates a plan for validation step execution.
func (p *DryRunPresenter) planValidation(plan *DryRunPlan, _ *domain.Task, _ *domain.StepDefinition) {
	// Collect commands that would run
	var commands []string

	if len(p.deps.FormatCommands) > 0 {
		commands = append(commands, p.deps.FormatCommands...)
	} else {
		commands = append(commands, "(default format commands)")
	}

	if len(p.deps.LintCommands) > 0 {
		commands = append(commands, p.deps.LintCommands...)
	} else {
		commands = append(commands, "(default lint commands)")
	}

	if len(p.deps.TestCommands) > 0 {
		commands = append(commands, p.deps.TestCommands...)
	} else {
		commands = append(commands, "(default test commands)")
	}

	if len(p.deps.PreCommitCommands) > 0 {
		commands = append(commands, p.deps.PreCommitCommands...)
	}

	plan.Config["format_commands"] = p.deps.FormatCommands
	plan.Config["lint_commands"] = p.deps.LintCommands
	plan.Config["test_commands"] = p.deps.TestCommands
	plan.Config["pre_commit_commands"] = p.deps.PreCommitCommands

	plan.WouldDo = append(plan.WouldDo, "Run validation commands:")
	for _, cmd := range commands {
		plan.WouldDo = append(plan.WouldDo, fmt.Sprintf("  %s", cmd))
	}
	plan.WouldDo = append(plan.WouldDo, "Execution order: Format -> Lint|Test (parallel) -> Pre-commit")
}

// planGit generates a plan for git step execution.
func (p *DryRunPresenter) planGit(plan *DryRunPlan, task *domain.Task, step *domain.StepDefinition) {
	// Determine git operation from config
	operation := "commit" // default
	if op, ok := step.Config["operation"].(string); ok {
		operation = op
	}

	// Get branch from task metadata
	branch := "(unknown)"
	if task.Metadata != nil {
		if b, ok := task.Metadata["branch"].(string); ok && b != "" {
			branch = b
		}
	}

	plan.Config["operation"] = operation

	switch operation {
	case "commit":
		plan.WouldDo = append(plan.WouldDo,
			"Analyze staged and unstaged changes",
			"Group changes by semantic meaning",
			"Generate commit message(s) via AI",
			"Create git commit(s)",
		)
	case "push":
		plan.WouldDo = append(plan.WouldDo,
			fmt.Sprintf("Push branch '%s' to remote 'origin'", branch),
			"Set upstream tracking",
		)
		plan.Config["branch"] = branch
		plan.Config["remote"] = "origin"
	case "create_pr":
		baseBranch := p.deps.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		plan.WouldDo = append(plan.WouldDo,
			fmt.Sprintf("Create pull request: %s -> %s", branch, baseBranch),
			"Generate PR description via AI",
		)
		plan.Config["base_branch"] = baseBranch
		plan.Config["head_branch"] = branch
	default:
		plan.WouldDo = append(plan.WouldDo, fmt.Sprintf("Execute git operation: %s", operation))
	}
}

// planHuman generates a plan for human step execution.
func (p *DryRunPresenter) planHuman(plan *DryRunPlan, _ *domain.Task, step *domain.StepDefinition) {
	message := "Awaiting human approval"
	if msg, ok := step.Config["message"].(string); ok && msg != "" {
		message = msg
	}

	plan.Config["message"] = message
	plan.WouldDo = append(plan.WouldDo,
		"Pause task execution",
		fmt.Sprintf("Display: %q", message),
		"Wait for user to approve/reject",
	)
}

// planCI generates a plan for CI step execution.
func (p *DryRunPresenter) planCI(plan *DryRunPlan, _ *domain.Task, _ *domain.StepDefinition) {
	pollInterval := 2 * time.Minute
	timeout := 30 * time.Minute

	if p.deps.CIConfig != nil {
		if p.deps.CIConfig.PollInterval > 0 {
			pollInterval = p.deps.CIConfig.PollInterval
		}
		if p.deps.CIConfig.Timeout > 0 {
			timeout = p.deps.CIConfig.Timeout
		}
	}

	plan.Config["poll_interval"] = pollInterval.String()
	plan.Config["timeout"] = timeout.String()

	plan.WouldDo = append(plan.WouldDo,
		"Wait for CI pipeline to complete",
		fmt.Sprintf("Poll interval: %s", pollInterval),
		fmt.Sprintf("Timeout: %s", timeout),
		"Check GitHub Actions workflow status",
	)
}

// planVerify generates a plan for verification step execution.
func (p *DryRunPresenter) planVerify(plan *DryRunPlan, _ *domain.Task, step *domain.StepDefinition) {
	model := "claude-sonnet-4-20250514"
	if m, ok := step.Config["model"].(string); ok && m != "" {
		model = m
	}

	plan.Config["model"] = model
	plan.WouldDo = append(plan.WouldDo,
		fmt.Sprintf("Run AI verification with model: %s", model),
		"Check code correctness",
		"Review test coverage",
		"Detect garbage files",
		"Verification output is non-deterministic",
	)
}

// planSDD generates a plan for SDD step execution.
func (p *DryRunPresenter) planSDD(plan *DryRunPlan, task *domain.Task, _ *domain.StepDefinition) {
	plan.Config["prompt"] = task.Description
	plan.WouldDo = append(plan.WouldDo,
		"Generate SDD specification via AI",
		"Save SDD artifacts",
		"SDD output is non-deterministic",
	)
}

// DryRunExecutorWrapper wraps a StepExecutor to show what would happen.
type DryRunExecutorWrapper struct {
	stepType  domain.StepType
	presenter *DryRunPresenter
}

// NewDryRunExecutorWrapper creates a new wrapper for the given step type.
func NewDryRunExecutorWrapper(stepType domain.StepType, presenter *DryRunPresenter) *DryRunExecutorWrapper {
	return &DryRunExecutorWrapper{
		stepType:  stepType,
		presenter: presenter,
	}
}

// Execute generates a dry-run plan instead of executing the step.
func (w *DryRunExecutorWrapper) Execute(_ context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	plan := w.presenter.Plan(task, step)

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "would_execute",
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
		Output:      plan.String(),
		Metadata: map[string]any{
			"dry_run": true,
			"plan":    plan,
		},
	}, nil
}

// Type returns the step type this wrapper handles.
func (w *DryRunExecutorWrapper) Type() domain.StepType {
	return w.stepType
}

// NewDryRunRegistry creates a registry with dry-run executors for all step types.
func NewDryRunRegistry(deps ExecutorDeps) *ExecutorRegistry {
	registry := NewExecutorRegistry()
	presenter := NewDryRunPresenter(deps)

	// Register dry-run wrappers for all step types
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
		registry.Register(NewDryRunExecutorWrapper(st, presenter))
	}

	return registry
}
