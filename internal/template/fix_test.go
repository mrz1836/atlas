package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewFixTemplate(t *testing.T) {
	tmpl := NewFixTemplate()

	require.NotNil(t, tmpl)
	assert.Equal(t, "fix", tmpl.Name)
	assert.Equal(t, "fix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.Equal(t, domain.AgentClaude, tmpl.DefaultAgent)
	assert.False(t, tmpl.Verify)
	assert.NotEmpty(t, tmpl.Description)
}

func TestFixTemplate_StepOrder(t *testing.T) {
	tmpl := NewFixTemplate()

	expectedSteps := []string{
		"detect", "fix", "validate", "git_commit",
		"git_push", "git_pr", "ci_wait", "review",
	}

	require.Len(t, tmpl.Steps, len(expectedSteps), "expected %d steps", len(expectedSteps))
	for i, name := range expectedSteps {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}
}

func TestFixTemplate_StepTypes(t *testing.T) {
	tmpl := NewFixTemplate()

	expectedTypes := map[string]domain.StepType{
		"detect":     domain.StepTypeValidation, // detect runs validation commands
		"fix":        domain.StepTypeAI,
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

func TestFixTemplate_RequiredSteps(t *testing.T) {
	tmpl := NewFixTemplate()

	// In fix template, all steps are required
	for _, step := range tmpl.Steps {
		assert.True(t, step.Required, "step %s should be required", step.Name)
	}
}

func TestFixTemplate_Timeouts(t *testing.T) {
	tmpl := NewFixTemplate()

	stepTimeouts := map[string]time.Duration{
		"detect":     10 * time.Minute,
		"fix":        20 * time.Minute,
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

func TestFixTemplate_RetryConfiguration(t *testing.T) {
	tmpl := NewFixTemplate()

	stepRetries := map[string]int{
		// detect step has no retry (runs validation to detect issues)
		"fix":      3,
		"validate": 1,
		"git_push": 3,
		"git_pr":   2,
	}

	for _, step := range tmpl.Steps {
		if expected, ok := stepRetries[step.Name]; ok {
			assert.Equal(t, expected, step.RetryCount, "step %s has wrong retry count", step.Name)
		}
	}
}

func TestFixTemplate_ValidationCommands(t *testing.T) {
	tmpl := NewFixTemplate()

	expectedCommands := []string{
		"magex format:fix",
		"magex lint",
		"magex test:race",
		"go-pre-commit run --all-files --skip lint",
	}

	assert.Equal(t, expectedCommands, tmpl.ValidationCommands)
}

func TestFixTemplate_StepConfigurations(t *testing.T) {
	tmpl := NewFixTemplate()

	// Check detect step config (validation in detect_only mode)
	detectStep := findStep(tmpl, "detect")
	require.NotNil(t, detectStep)
	assert.Equal(t, true, detectStep.Config["detect_only"],
		"detect step should use detect_only mode to capture errors without failing")

	// Check fix step config (AI with previous errors injection)
	fixStep := findStep(tmpl, "fix")
	require.NotNil(t, fixStep)
	assert.Equal(t, "default", fixStep.Config["permission_mode"])
	assert.Equal(t, true, fixStep.Config["include_previous_errors"],
		"fix step should include previous validation errors in AI prompt")

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

	// Check ci_wait step exists
	ciWaitStep := findStep(tmpl, "ci_wait")
	require.NotNil(t, ciWaitStep)

	// Check review step config
	reviewStep := findStep(tmpl, "review")
	require.NotNil(t, reviewStep)
	assert.NotEmpty(t, reviewStep.Config["prompt"])
}

func TestFixTemplate_DetectStepHasDetectOnlyMode(t *testing.T) {
	tmpl := NewFixTemplate()

	detectStep := findStep(tmpl, "detect")
	require.NotNil(t, detectStep)

	// Detect step runs validation in detect_only mode to find issues without failing
	assert.Equal(t, true, detectStep.Config["detect_only"],
		"detect step should use detect_only mode to capture errors without failing")
}

func TestFixTemplate_FixStepInjectsPreviousErrors(t *testing.T) {
	tmpl := NewFixTemplate()

	fixStep := findStep(tmpl, "fix")
	require.NotNil(t, fixStep)

	// Fix step should include previous validation errors in the AI prompt
	assert.Equal(t, true, fixStep.Config["include_previous_errors"],
		"fix step should include previous validation errors in AI prompt")

	// Fix step should use "default" mode to allow making changes
	assert.Equal(t, "default", fixStep.Config["permission_mode"],
		"fix step should use default mode to allow making changes")
}
