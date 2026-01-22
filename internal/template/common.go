// Package template provides task template management for ATLAS.
package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// DefaultValidationCommands are the standard validation commands
// used across all templates. These can be overridden per-template.
//
//nolint:gochecknoglobals // Package-level default configuration
var DefaultValidationCommands = []string{
	"magex format:fix",
	"magex lint",
	"magex test:race",
	"go-pre-commit run --all-files --skip lint",
}

// GitCommitStep creates a standard git commit step with the given description.
func GitCommitStep(description string) domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "git_commit",
		Type:        domain.StepTypeGit,
		Description: description,
		Required:    true,
		Timeout:     1 * time.Minute,
		Config: map[string]any{
			"operation": domain.GitOpCommit,
		},
	}
}

// GitPushStep creates a standard git push step.
func GitPushStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "git_push",
		Type:        domain.StepTypeGit,
		Description: "Push branch to remote",
		Required:    true,
		Timeout:     2 * time.Minute,
		RetryCount:  3,
		Config: map[string]any{
			"operation": domain.GitOpPush,
		},
	}
}

// GitPRStep creates a standard git pull request step.
func GitPRStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "git_pr",
		Type:        domain.StepTypeGit,
		Description: "Create pull request",
		Required:    true,
		Timeout:     2 * time.Minute,
		RetryCount:  2,
		Config: map[string]any{
			"operation": domain.GitOpCreatePR,
		},
	}
}

// CIWaitStep creates a standard CI wait step.
func CIWaitStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "ci_wait",
		Type:        domain.StepTypeCI,
		Description: "Wait for CI pipeline to complete",
		Required:    true,
		Timeout:     constants.DefaultCITimeout,
		Config:      map[string]any{},
	}
}

// ReviewStep creates a standard human review step with the given prompt.
func ReviewStep(prompt string) domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "review",
		Type:        domain.StepTypeHuman,
		Description: "Human review",
		Required:    true,
		Config: map[string]any{
			"prompt": prompt,
		},
	}
}

// StandardGitWorkflowSteps returns the common git workflow steps:
// git_commit -> git_push -> git_pr -> ci_wait -> review
//
// commitDescription describes what is being committed (e.g., "fix changes", "feature changes").
// reviewPrompt is the prompt shown during human review.
func StandardGitWorkflowSteps(commitDescription, reviewPrompt string) []domain.StepDefinition {
	return []domain.StepDefinition{
		GitCommitStep("Create commit with " + commitDescription),
		GitPushStep(),
		GitPRStep(),
		CIWaitStep(),
		ReviewStep(reviewPrompt),
	}
}

// ValidationStep creates a standard validation step.
func ValidationStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "validate",
		Type:        domain.StepTypeValidation,
		Description: "Run format, lint, and test commands",
		Required:    true,
		Timeout:     10 * time.Minute,
		RetryCount:  1,
	}
}

// VerifyStep creates an optional AI verification step.
// agent specifies which AI agent to use for verification (e.g., "gemini").
// checks specifies which verification checks to run.
func VerifyStep(agent string, checks []string, required bool) domain.StepDefinition {
	timeout := 5 * time.Minute
	if required {
		timeout = 10 * time.Minute
	}

	return domain.StepDefinition{
		Name:        "verify",
		Type:        domain.StepTypeVerify,
		Description: "AI verification of implementation",
		Required:    required,
		Timeout:     timeout,
		Config: map[string]any{
			"agent":  agent,
			"model":  "", // Will use agent's default model
			"checks": checks,
		},
	}
}
