package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestDefaultValidationCommands(t *testing.T) {
	t.Run("contains expected commands", func(t *testing.T) {
		assert.Len(t, DefaultValidationCommands, 4)
		assert.Contains(t, DefaultValidationCommands, "magex format:fix")
		assert.Contains(t, DefaultValidationCommands, "magex lint")
		assert.Contains(t, DefaultValidationCommands, "magex test:race")
		assert.Contains(t, DefaultValidationCommands, "go-pre-commit run --all-files")
	})
}

func TestGitCommitStep(t *testing.T) {
	t.Run("creates step with correct properties", func(t *testing.T) {
		step := GitCommitStep("bugfix changes")

		assert.Equal(t, "git_commit", step.Name)
		assert.Equal(t, domain.StepTypeGit, step.Type)
		assert.Equal(t, "bugfix changes", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, 1*time.Minute, step.Timeout)
		assert.Equal(t, domain.GitOpCommit, step.Config["operation"])
	})

	t.Run("uses custom description", func(t *testing.T) {
		step := GitCommitStep("feature implementation")

		assert.Equal(t, "feature implementation", step.Description)
	})
}

func TestGitPushStep(t *testing.T) {
	t.Run("creates step with correct properties", func(t *testing.T) {
		step := GitPushStep()

		assert.Equal(t, "git_push", step.Name)
		assert.Equal(t, domain.StepTypeGit, step.Type)
		assert.Equal(t, "Push branch to remote", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, 2*time.Minute, step.Timeout)
		assert.Equal(t, 3, step.RetryCount)
		assert.Equal(t, domain.GitOpPush, step.Config["operation"])
	})
}

func TestGitPRStep(t *testing.T) {
	t.Run("creates step with correct properties", func(t *testing.T) {
		step := GitPRStep()

		assert.Equal(t, "git_pr", step.Name)
		assert.Equal(t, domain.StepTypeGit, step.Type)
		assert.Equal(t, "Create pull request", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, 2*time.Minute, step.Timeout)
		assert.Equal(t, 2, step.RetryCount)
		assert.Equal(t, domain.GitOpCreatePR, step.Config["operation"])
	})
}

func TestCIWaitStep(t *testing.T) {
	t.Run("creates step with correct properties", func(t *testing.T) {
		step := CIWaitStep()

		assert.Equal(t, "ci_wait", step.Name)
		assert.Equal(t, domain.StepTypeCI, step.Type)
		assert.Equal(t, "Wait for CI pipeline to complete", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, constants.DefaultCITimeout, step.Timeout)
		assert.NotNil(t, step.Config)
	})
}

func TestReviewStep(t *testing.T) {
	t.Run("creates step with correct properties", func(t *testing.T) {
		step := ReviewStep("Please review the changes")

		assert.Equal(t, "review", step.Name)
		assert.Equal(t, domain.StepTypeHuman, step.Type)
		assert.Equal(t, "Human review", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, "Please review the changes", step.Config["prompt"])
	})

	t.Run("uses custom prompt", func(t *testing.T) {
		step := ReviewStep("Custom review prompt for feature")

		assert.Equal(t, "Custom review prompt for feature", step.Config["prompt"])
	})
}

func TestStandardGitWorkflowSteps(t *testing.T) {
	t.Run("returns all five standard steps", func(t *testing.T) {
		steps := StandardGitWorkflowSteps("fix changes", "Review the fix")

		assert.Len(t, steps, 5)
		assert.Equal(t, "git_commit", steps[0].Name)
		assert.Equal(t, "git_push", steps[1].Name)
		assert.Equal(t, "git_pr", steps[2].Name)
		assert.Equal(t, "ci_wait", steps[3].Name)
		assert.Equal(t, "review", steps[4].Name)
	})

	t.Run("uses commit description in first step", func(t *testing.T) {
		steps := StandardGitWorkflowSteps("bugfix changes", "Review prompt")

		assert.Equal(t, "Create commit with bugfix changes", steps[0].Description)
	})

	t.Run("uses review prompt in last step", func(t *testing.T) {
		steps := StandardGitWorkflowSteps("feature changes", "Please review the feature")

		assert.Equal(t, "Please review the feature", steps[4].Config["prompt"])
	})

	t.Run("all steps have correct types", func(t *testing.T) {
		steps := StandardGitWorkflowSteps("changes", "prompt")

		assert.Equal(t, domain.StepTypeGit, steps[0].Type)
		assert.Equal(t, domain.StepTypeGit, steps[1].Type)
		assert.Equal(t, domain.StepTypeGit, steps[2].Type)
		assert.Equal(t, domain.StepTypeCI, steps[3].Type)
		assert.Equal(t, domain.StepTypeHuman, steps[4].Type)
	})
}

func TestValidationStep(t *testing.T) {
	t.Run("creates step with correct properties", func(t *testing.T) {
		step := ValidationStep()

		assert.Equal(t, "validate", step.Name)
		assert.Equal(t, domain.StepTypeValidation, step.Type)
		assert.Equal(t, "Run format, lint, and test commands", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, 10*time.Minute, step.Timeout)
		assert.Equal(t, 1, step.RetryCount)
	})
}

func TestVerifyStep(t *testing.T) {
	t.Run("creates required step with longer timeout", func(t *testing.T) {
		step := VerifyStep("gemini", []string{"security", "logic"}, true)

		assert.Equal(t, "verify", step.Name)
		assert.Equal(t, domain.StepTypeVerify, step.Type)
		assert.Equal(t, "AI verification of implementation", step.Description)
		assert.True(t, step.Required)
		assert.Equal(t, 10*time.Minute, step.Timeout)
		assert.Equal(t, "gemini", step.Config["agent"])
		assert.Equal(t, []string{"security", "logic"}, step.Config["checks"])
	})

	t.Run("creates optional step with shorter timeout", func(t *testing.T) {
		step := VerifyStep("claude", []string{"review"}, false)

		assert.False(t, step.Required)
		assert.Equal(t, 5*time.Minute, step.Timeout)
		assert.Equal(t, "claude", step.Config["agent"])
	})

	t.Run("sets empty model for agent default", func(t *testing.T) {
		step := VerifyStep("codex", []string{}, true)

		assert.Empty(t, step.Config["model"])
	})
}
