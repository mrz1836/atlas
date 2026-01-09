package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// defaultValidationCommands contains the standard validation command sequence.
// These commands run during the validate step in all templates.
//
//nolint:gochecknoglobals // Constant-like slice used by all templates
var defaultValidationCommands = []string{
	"magex format:fix",
	"magex lint",
	"magex test:race",
	"go-pre-commit run --all-files",
}

// newGitCommitStep creates a git commit step with the given description.
// All templates use this step to commit changes before pushing.
func newGitCommitStep(description string) domain.StepDefinition {
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

// newGitPushStep creates a git push step.
// Includes retry logic for transient network failures.
func newGitPushStep() domain.StepDefinition {
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

// newGitPRStep creates a git pull request step.
// Includes retry logic for transient API failures.
func newGitPRStep() domain.StepDefinition {
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

// newCIWaitStep creates a CI pipeline wait step.
// Uses the default CI timeout from constants.
func newCIWaitStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "ci_wait",
		Type:        domain.StepTypeCI,
		Description: "Wait for CI pipeline to complete",
		Required:    true,
		Timeout:     constants.DefaultCITimeout,
		Config:      map[string]any{},
	}
}

// newReviewStep creates a human review step with the given prompt.
func newReviewStep(description, prompt string) domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "review",
		Type:        domain.StepTypeHuman,
		Description: description,
		Required:    true,
		Config: map[string]any{
			"prompt": prompt,
		},
	}
}
