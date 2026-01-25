package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewBugTemplate(t *testing.T) {
	tmpl := NewBugTemplate()

	require.NotNil(t, tmpl)
	assert.Equal(t, "bug", tmpl.Name)
	assert.Equal(t, "fix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.Equal(t, domain.AgentClaude, tmpl.DefaultAgent)
	assert.False(t, tmpl.Verify)
	assert.Equal(t, "opus", tmpl.VerifyModel)
	assert.NotEmpty(t, tmpl.Description)
}

func TestBugTemplate_StepOrder(t *testing.T) {
	tmpl := NewBugTemplate()

	expectedSteps := []string{
		"detect", "analyze", "implement", "verify", "validate", "git_commit",
		"git_push", "git_pr", "ci_wait", "review",
	}

	require.Len(t, tmpl.Steps, len(expectedSteps), "expected %d steps", len(expectedSteps))
	for i, name := range expectedSteps {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}
}

func TestBugTemplate_StepTypes(t *testing.T) {
	tmpl := NewBugTemplate()

	expectedTypes := map[string]domain.StepType{
		"detect":     domain.StepTypeValidation,
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

func TestBugTemplate_RequiredSteps(t *testing.T) {
	tmpl := NewBugTemplate()

	// In bug template, verify step is optional by default
	optionalSteps := map[string]bool{
		"verify": true, // Verification is OFF by default
	}

	for _, step := range tmpl.Steps {
		if optionalSteps[step.Name] {
			assert.False(t, step.Required, "step %s should be optional", step.Name)
		} else {
			assert.True(t, step.Required, "step %s should be required", step.Name)
		}
	}
}

func TestBugTemplate_Timeouts(t *testing.T) {
	tmpl := NewBugTemplate()

	stepTimeouts := map[string]time.Duration{
		"detect":     10 * time.Minute,
		"analyze":    15 * time.Minute,
		"implement":  constants.DefaultAITimeout,
		"verify":     5 * time.Minute,
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

func TestBugTemplate_RetryConfiguration(t *testing.T) {
	tmpl := NewBugTemplate()

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

func TestBugTemplate_ValidationCommands(t *testing.T) {
	tmpl := NewBugTemplate()

	expectedCommands := []string{
		"magex format:fix",
		"magex lint",
		"magex test:race",
		"go-pre-commit run --all-files --skip lint",
	}

	assert.Equal(t, expectedCommands, tmpl.ValidationCommands)
}

func TestBugTemplate_SmartSkipConditions(t *testing.T) {
	tmpl := NewBugTemplate()

	// Check detect step has skip_condition for has_description
	detectStep := findStep(tmpl, "detect")
	require.NotNil(t, detectStep)
	skipCond, ok := detectStep.Config["skip_condition"].(string)
	require.True(t, ok, "detect step should have skip_condition")
	assert.Equal(t, "has_description", skipCond,
		"detect step should be skipped when user provides a description")

	// Check analyze step has skip_condition for no_description
	analyzeStep := findStep(tmpl, "analyze")
	require.NotNil(t, analyzeStep)
	skipCond, ok = analyzeStep.Config["skip_condition"].(string)
	require.True(t, ok, "analyze step should have skip_condition")
	assert.Equal(t, "no_description", skipCond,
		"analyze step should be skipped in detect mode (no description)")
}

func TestBugTemplate_DetectStepConfig(t *testing.T) {
	tmpl := NewBugTemplate()

	detectStep := findStep(tmpl, "detect")
	require.NotNil(t, detectStep)

	// Detect step should use detect_only mode
	detectOnly, ok := detectStep.Config["detect_only"].(bool)
	require.True(t, ok, "detect_only should be a bool")
	assert.True(t, detectOnly, "detect step should use detect_only mode")
}

func TestBugTemplate_AnalyzeStepConfig(t *testing.T) {
	tmpl := NewBugTemplate()

	analyzeStep := findStep(tmpl, "analyze")
	require.NotNil(t, analyzeStep)

	assert.Equal(t, "plan", analyzeStep.Config["permission_mode"])
	assert.Equal(t, "analyze_bug", analyzeStep.Config["prompt_template"])
}

func TestBugTemplate_ImplementStepConfig(t *testing.T) {
	tmpl := NewBugTemplate()

	implementStep := findStep(tmpl, "implement")
	require.NotNil(t, implementStep)

	assert.Equal(t, "default", implementStep.Config["permission_mode"])
	assert.Equal(t, "implement_fix", implementStep.Config["prompt_template"])

	// Should include previous errors (from detect step if it ran)
	includePrevErrors, ok := implementStep.Config["include_previous_errors"].(bool)
	require.True(t, ok, "include_previous_errors should be a bool")
	assert.True(t, includePrevErrors, "implement step should include previous validation errors")
}

func TestBugTemplate_StepConfigurations(t *testing.T) {
	tmpl := NewBugTemplate()

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

func TestBugTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()

	tmpl, err := registry.Get("bug")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "bug", tmpl.Name)
}

func TestBugTemplate_AliasesWork(t *testing.T) {
	registry := NewDefaultRegistry()

	// "fix" alias should resolve to "bug" template
	fixTmpl, err := registry.Get("fix")
	require.NoError(t, err)
	assert.Equal(t, "bug", fixTmpl.Name)

	// "bugfix" alias should resolve to "bug" template
	bugfixTmpl, err := registry.Get("bugfix")
	require.NoError(t, err)
	assert.Equal(t, "bug", bugfixTmpl.Name)
}

func TestBugTemplate_VerifyStep(t *testing.T) {
	tmpl := NewBugTemplate()

	verifyStep := findStep(tmpl, "verify")
	require.NotNil(t, verifyStep, "bug template should have verify step")

	assert.Equal(t, domain.StepTypeVerify, verifyStep.Type)
	assert.False(t, verifyStep.Required, "verify step should be optional")
	assert.Equal(t, 5*time.Minute, verifyStep.Timeout)

	assert.Equal(t, "gemini", verifyStep.Config["agent"])
	assert.Empty(t, verifyStep.Config["model"])

	checks, ok := verifyStep.Config["checks"].([]string)
	require.True(t, ok, "checks should be a string slice")
	assert.Contains(t, checks, "code_correctness")
}

func TestBugTemplate_TwoModes(t *testing.T) {
	tmpl := NewBugTemplate()

	t.Run("analyze mode (with description)", func(t *testing.T) {
		// When user provides a description, analyze step runs, detect is skipped
		detectStep := findStep(tmpl, "detect")
		require.NotNil(t, detectStep)
		assert.Equal(t, "has_description", detectStep.Config["skip_condition"],
			"detect should be skipped when description provided")

		analyzeStep := findStep(tmpl, "analyze")
		require.NotNil(t, analyzeStep)
		assert.Equal(t, "no_description", analyzeStep.Config["skip_condition"],
			"analyze should NOT be skipped when description provided")
	})

	t.Run("detect mode (no description)", func(t *testing.T) {
		// When user provides no/short description, detect step runs, analyze is skipped
		detectStep := findStep(tmpl, "detect")
		require.NotNil(t, detectStep)
		// detect runs when no_description (inverse of has_description)

		analyzeStep := findStep(tmpl, "analyze")
		require.NotNil(t, analyzeStep)
		// analyze is skipped when no_description
	})
}

func TestBugTemplate_DescriptionThreshold(t *testing.T) {
	// The threshold for "substantive description" is 20 characters
	// This is tested in step_runner_test.go, but we document it here

	shortDescriptions := []string{
		"fix lint",
		"fix test",
		"fix format",
		"update deps",
	}

	longDescriptions := []string{
		"Fix the nil pointer exception in user service when fetching profile",
		"Update the authentication logic to handle expired tokens",
		"Resolve the race condition in concurrent map access",
	}

	for _, desc := range shortDescriptions {
		assert.LessOrEqual(t, len(desc), 20, "should be considered short: %s", desc)
	}

	for _, desc := range longDescriptions {
		assert.Greater(t, len(desc), 20, "should be considered long: %s", desc)
	}
}
