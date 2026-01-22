package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewBugfixTemplate(t *testing.T) {
	tmpl := NewBugfixTemplate()

	require.NotNil(t, tmpl)
	assert.Equal(t, "bugfix", tmpl.Name)
	assert.Equal(t, "fix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.NotEmpty(t, tmpl.Description)
}

func TestBugfixTemplate_StepOrder(t *testing.T) {
	tmpl := NewBugfixTemplate()

	expectedSteps := []string{
		"analyze", "implement", "verify", "validate", "git_commit",
		"git_push", "git_pr", "ci_wait", "review",
	}

	require.Len(t, tmpl.Steps, len(expectedSteps), "expected %d steps", len(expectedSteps))
	for i, name := range expectedSteps {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}
}

func TestBugfixTemplate_StepTypes(t *testing.T) {
	tmpl := NewBugfixTemplate()

	expectedTypes := map[string]domain.StepType{
		"analyze":    domain.StepTypeAI,
		"implement":  domain.StepTypeAI,
		"verify":     domain.StepTypeVerify,
		"validate":   domain.StepTypeValidation,
		"git_commit": domain.StepTypeGit,
		"git_push":   domain.StepTypeGit,
		"git_pr":     domain.StepTypeGit,
		"ci_wait":    domain.StepTypeCI,
		"review":     domain.StepTypeHuman,
	}

	for _, step := range tmpl.Steps {
		expected, ok := expectedTypes[step.Name]
		require.True(t, ok, "unexpected step: %s", step.Name)
		assert.Equal(t, expected, step.Type, "step %s has wrong type", step.Name)
	}
}

func TestBugfixTemplate_RequiredSteps(t *testing.T) {
	tmpl := NewBugfixTemplate()

	// In bugfix template, verify step is optional by default
	optionalSteps := map[string]bool{
		"verify": true, // Verification is OFF by default for bugfix
	}

	for _, step := range tmpl.Steps {
		if optionalSteps[step.Name] {
			assert.False(t, step.Required, "step %s should be optional", step.Name)
		} else {
			assert.True(t, step.Required, "step %s should be required", step.Name)
		}
	}
}

func TestBugfixTemplate_Timeouts(t *testing.T) {
	tmpl := NewBugfixTemplate()

	stepTimeouts := map[string]time.Duration{
		"analyze":    15 * time.Minute,
		"implement":  constants.DefaultAITimeout,
		"validate":   10 * time.Minute,
		"git_commit": 1 * time.Minute,
		"git_push":   2 * time.Minute,
		"git_pr":     2 * time.Minute,
		"ci_wait":    constants.DefaultCITimeout,
	}

	for _, step := range tmpl.Steps {
		if expected, ok := stepTimeouts[step.Name]; ok {
			assert.Equal(t, expected, step.Timeout, "step %s has wrong timeout", step.Name)
		}
	}
}

func TestBugfixTemplate_RetryConfiguration(t *testing.T) {
	tmpl := NewBugfixTemplate()

	stepRetries := map[string]int{
		"analyze":   2,
		"implement": 3,
		"validate":  1,
		"git_push":  3,
		"git_pr":    2,
	}

	for _, step := range tmpl.Steps {
		if expected, ok := stepRetries[step.Name]; ok {
			assert.Equal(t, expected, step.RetryCount, "step %s has wrong retry count", step.Name)
		}
	}
}

func TestBugfixTemplate_ValidationCommands(t *testing.T) {
	tmpl := NewBugfixTemplate()

	expectedCommands := []string{
		"magex format:fix",
		"magex lint",
		"magex test:race",
		"go-pre-commit run --all-files --skip lint",
	}

	assert.Equal(t, expectedCommands, tmpl.ValidationCommands)
}

func TestBugfixTemplate_StepConfigurations(t *testing.T) {
	tmpl := NewBugfixTemplate()

	// Check analyze step config
	analyzeStep := findStep(tmpl, "analyze")
	require.NotNil(t, analyzeStep)
	assert.Equal(t, "plan", analyzeStep.Config["permission_mode"])
	assert.Equal(t, "analyze_bug", analyzeStep.Config["prompt_template"])

	// Check implement step config
	implementStep := findStep(tmpl, "implement")
	require.NotNil(t, implementStep)
	assert.Equal(t, "default", implementStep.Config["permission_mode"])
	assert.Equal(t, "implement_fix", implementStep.Config["prompt_template"])

	// Check git steps config
	gitCommitStep := findStep(tmpl, "git_commit")
	require.NotNil(t, gitCommitStep)
	assert.Equal(t, "commit", gitCommitStep.Config["operation"])

	gitPushStep := findStep(tmpl, "git_push")
	require.NotNil(t, gitPushStep)
	assert.Equal(t, "push", gitPushStep.Config["operation"])

	gitPRStep := findStep(tmpl, "git_pr")
	require.NotNil(t, gitPRStep)
	assert.Equal(t, "create_pr", gitPRStep.Config["operation"])

	// Check ci_wait step exists (config values come from runtime config or constants)
	ciWaitStep := findStep(tmpl, "ci_wait")
	require.NotNil(t, ciWaitStep)

	// Check review step config
	reviewStep := findStep(tmpl, "review")
	require.NotNil(t, reviewStep)
	assert.NotEmpty(t, reviewStep.Config["prompt"])
}
