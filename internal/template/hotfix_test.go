package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewHotfixTemplate(t *testing.T) {
	tmpl := NewHotfixTemplate()

	require.NotNil(t, tmpl)
	assert.Equal(t, "hotfix", tmpl.Name)
	assert.Equal(t, "hotfix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.Equal(t, domain.AgentClaude, tmpl.DefaultAgent)
	assert.False(t, tmpl.Verify)
	assert.Equal(t, "opus", tmpl.VerifyModel, "should have VerifyModel set for --verify flag support")
	assert.NotEmpty(t, tmpl.Description)
	assert.Contains(t, tmpl.Description, "existing branch")
}

func TestHotfixTemplate_StepOrder(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Hotfix template should NOT have git_pr, ci_wait, or review steps
	// But SHOULD have verify step (optional) like other templates
	expectedSteps := []string{
		"detect", "fix", "verify", "validate", "git_commit", "git_push",
	}

	require.Len(t, tmpl.Steps, len(expectedSteps), "expected %d steps (no git_pr, ci_wait, review)", len(expectedSteps))
	for i, name := range expectedSteps {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}
}

func TestHotfixTemplate_NoPRStep(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Verify that git_pr step is NOT present
	prStep := findStep(tmpl, "git_pr")
	assert.Nil(t, prStep, "hotfix template should not have git_pr step")

	// Verify that ci_wait step is NOT present
	ciWaitStep := findStep(tmpl, "ci_wait")
	assert.Nil(t, ciWaitStep, "hotfix template should not have ci_wait step")

	// Verify that review step is NOT present
	reviewStep := findStep(tmpl, "review")
	assert.Nil(t, reviewStep, "hotfix template should not have review step")
}

func TestHotfixTemplate_StepTypes(t *testing.T) {
	tmpl := NewHotfixTemplate()

	expectedTypes := map[string]domain.StepType{
		"detect":     domain.StepTypeValidation,
		"fix":        domain.StepTypeAI,
		"verify":     domain.StepTypeVerify,
		"validate":   domain.StepTypeValidation,
		"git_commit": domain.StepTypeGit,
		"git_push":   domain.StepTypeGit,
	}

	for _, step := range tmpl.Steps {
		expected, ok := expectedTypes[step.Name]
		require.True(t, ok, "unexpected step: %s", step.Name)
		assert.Equal(t, expected, step.Type, "step %s has wrong type", step.Name)
	}
}

func TestHotfixTemplate_RequiredSteps(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// In hotfix template, detect and verify are optional, others are required
	expectedRequired := map[string]bool{
		"detect":     false, // Optional - user may already know what to fix
		"fix":        true,
		"verify":     false, // Optional - enable with --verify flag
		"validate":   true,
		"git_commit": true,
		"git_push":   true,
	}

	for _, step := range tmpl.Steps {
		expected, ok := expectedRequired[step.Name]
		require.True(t, ok, "unexpected step: %s", step.Name)
		assert.Equal(t, expected, step.Required, "step %s has wrong required setting", step.Name)
	}
}

func TestHotfixTemplate_Timeouts(t *testing.T) {
	tmpl := NewHotfixTemplate()

	stepTimeouts := map[string]time.Duration{
		"detect":     10 * time.Minute,
		"fix":        constants.DefaultAITimeout,
		"verify":     5 * time.Minute,
		"validate":   10 * time.Minute,
		"git_commit": constants.GitCommitTimeout,
		"git_push":   constants.GitPushTimeout,
	}

	for _, step := range tmpl.Steps {
		if expected, ok := stepTimeouts[step.Name]; ok {
			assert.Equal(t, expected, step.Timeout, "step %s has wrong timeout", step.Name)
		}
	}
}

func TestHotfixTemplate_RetryConfiguration(t *testing.T) {
	tmpl := NewHotfixTemplate()

	stepRetries := map[string]int{
		"fix":      3,
		"validate": 1,
		"git_push": constants.GitPushRetryCount,
	}

	for _, step := range tmpl.Steps {
		if expected, ok := stepRetries[step.Name]; ok {
			assert.Equal(t, expected, step.RetryCount, "step %s has wrong retry count", step.Name)
		}
	}
}

func TestHotfixTemplate_ValidationCommands(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Should use the same default validation commands
	expectedCommands := []string{
		"magex format:fix",
		"magex lint",
		"magex test:race",
		"go-pre-commit run --all-files",
	}

	assert.Equal(t, expectedCommands, tmpl.ValidationCommands)
}

func TestHotfixTemplate_StepConfigurations(t *testing.T) {
	tmpl := NewHotfixTemplate()

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
}

func TestHotfixTemplate_DesignedForExistingBranch(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// The hotfix template is designed to work with --target flag
	// but can also work with BranchPrefix as fallback
	assert.Equal(t, "hotfix", tmpl.BranchPrefix,
		"hotfix template should have a fallback branch prefix")

	// Description should mention existing branch
	assert.Contains(t, tmpl.Description, "existing branch",
		"description should mention that this template is for existing branches")
}

func TestHotfixTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()

	tmpl, err := registry.Get("hotfix")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "hotfix", tmpl.Name)
}

func TestHotfixTemplate_VerifyStep(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Verify step should exist and match other templates' pattern
	verifyStep := findStep(tmpl, "verify")
	require.NotNil(t, verifyStep, "hotfix template should have verify step for --verify flag support")

	// Check verify step configuration matches other templates
	assert.Equal(t, domain.StepTypeVerify, verifyStep.Type)
	assert.False(t, verifyStep.Required, "verify step should be optional")
	assert.Equal(t, 5*time.Minute, verifyStep.Timeout)

	// Check verify step config
	assert.Equal(t, "gemini", verifyStep.Config["agent"], "verify should use gemini agent")
	assert.Empty(t, verifyStep.Config["model"], "verify should use default model")

	checks, ok := verifyStep.Config["checks"].([]string)
	require.True(t, ok, "checks should be a string slice")
	assert.Contains(t, checks, "code_correctness", "verify should check code correctness")
}

func TestHotfixTemplate_DetectStepConfig(t *testing.T) {
	tmpl := NewHotfixTemplate()

	detectStep := findStep(tmpl, "detect")
	require.NotNil(t, detectStep)

	// Detect step should be in detect_only mode
	detectOnly, ok := detectStep.Config["detect_only"].(bool)
	require.True(t, ok, "detect_only should be a bool")
	assert.True(t, detectOnly, "detect step should use detect_only mode")

	// Detect step should be optional
	assert.False(t, detectStep.Required, "detect step should be optional for user-described issues")
}

func TestHotfixTemplate_FixStepConfig(t *testing.T) {
	tmpl := NewHotfixTemplate()

	fixStep := findStep(tmpl, "fix")
	require.NotNil(t, fixStep)

	// Fix step should include previous errors
	includePrevErrors, ok := fixStep.Config["include_previous_errors"].(bool)
	require.True(t, ok, "include_previous_errors should be a bool")
	assert.True(t, includePrevErrors, "fix step should include previous validation errors")

	// Fix step should use default permission mode
	permMode, ok := fixStep.Config["permission_mode"].(string)
	require.True(t, ok, "permission_mode should be a string")
	assert.Equal(t, "default", permMode, "fix step should use default permission mode")

	// Fix step should be required
	assert.True(t, fixStep.Required)

	// Fix step should have retries
	assert.Equal(t, 3, fixStep.RetryCount)
}

func TestHotfixTemplate_ComparedToOtherTemplates(t *testing.T) {
	registry := NewDefaultRegistry()

	hotfix, err := registry.Get("hotfix")
	require.NoError(t, err)

	bugfix, err := registry.Get("bugfix")
	require.NoError(t, err)

	fix, err := registry.Get("fix")
	require.NoError(t, err)

	task, err := registry.Get("task")
	require.NoError(t, err)

	// Hotfix should have fewer steps than templates that create PRs
	assert.Less(t, len(hotfix.Steps), len(bugfix.Steps), "hotfix should have fewer steps than bugfix (no PR workflow)")
	assert.Less(t, len(hotfix.Steps), len(task.Steps), "hotfix should have fewer steps than task (no PR workflow)")
	assert.Less(t, len(hotfix.Steps), len(fix.Steps), "hotfix should have fewer steps than fix (no PR workflow)")

	// Hotfix should use same model as other fix-type templates
	assert.Equal(t, fix.DefaultModel, hotfix.DefaultModel, "hotfix should use same model as fix")

	// Hotfix should have VerifyModel like other templates
	assert.Equal(t, bugfix.VerifyModel, hotfix.VerifyModel, "hotfix should have same VerifyModel as bugfix")
	assert.Equal(t, task.VerifyModel, hotfix.VerifyModel, "hotfix should have same VerifyModel as task")
}

func TestHotfixTemplate_StepOrderIsLogical(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Find indices of key steps
	detectIdx, fixIdx, verifyIdx, validateIdx, commitIdx, pushIdx := -1, -1, -1, -1, -1, -1

	for i, step := range tmpl.Steps {
		switch step.Name {
		case "detect":
			detectIdx = i
		case "fix":
			fixIdx = i
		case "verify":
			verifyIdx = i
		case "validate":
			validateIdx = i
		case "git_commit":
			commitIdx = i
		case "git_push":
			pushIdx = i
		}
	}

	// Verify logical ordering
	assert.Less(t, detectIdx, fixIdx, "detect should come before fix")
	assert.Less(t, fixIdx, verifyIdx, "fix should come before verify")
	assert.Less(t, verifyIdx, validateIdx, "verify should come before validate")
	assert.Less(t, validateIdx, commitIdx, "validate should come before commit")
	assert.Less(t, commitIdx, pushIdx, "commit should come before push")
}

func TestHotfixTemplate_TwoIssueDetectionPaths(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Path 1: User describes the issue (detect is optional)
	detectStep := findStep(tmpl, "detect")
	require.NotNil(t, detectStep)
	assert.False(t, detectStep.Required, "detect should be optional for path 1 (user-described issues)")

	// Path 2: Auto-detect issues (detect runs and captures errors)
	detectOnly, ok := detectStep.Config["detect_only"].(bool)
	require.True(t, ok)
	assert.True(t, detectOnly, "detect should capture errors without failing for path 2")

	// Both paths: Fix step receives errors
	fixStep := findStep(tmpl, "fix")
	require.NotNil(t, fixStep)
	includeErrors, ok := fixStep.Config["include_previous_errors"].(bool)
	require.True(t, ok)
	assert.True(t, includeErrors, "fix should receive errors from detect step (path 2)")
}

func TestHotfixTemplate_ValidForUseCases(t *testing.T) {
	tmpl := NewHotfixTemplate()

	testCases := []struct {
		name        string
		description string
		usesDetect  bool
	}{
		{
			name:        "lint error fix",
			description: "fix lint errors in authentication module",
			usesDetect:  false, // User knows what to fix
		},
		{
			name:        "test failure fix",
			description: "fix failing tests in user service",
			usesDetect:  false, // User knows what to fix
		},
		{
			name:        "auto-detect and fix",
			description: "run validations and fix any issues",
			usesDetect:  true, // Let detect step find issues
		},
		{
			name:        "format fix",
			description: "fix formatting issues",
			usesDetect:  false, // User knows what to fix
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify template has required steps for this use case
			fixStep := findStep(tmpl, "fix")
			assert.NotNil(t, fixStep, "template should have fix step for: %s", tc.description)

			validateStep := findStep(tmpl, "validate")
			assert.NotNil(t, validateStep, "template should have validate step for: %s", tc.description)

			pushStep := findStep(tmpl, "git_push")
			assert.NotNil(t, pushStep, "template should have push step for: %s", tc.description)

			// Verify no PR step (that's the key difference)
			prStep := findStep(tmpl, "git_pr")
			assert.Nil(t, prStep, "template should NOT have PR step for: %s", tc.description)
		})
	}
}

func TestHotfixTemplate_VerifyFlagCompatibility(t *testing.T) {
	tmpl := NewHotfixTemplate()

	// Verify flag should work with hotfix template
	assert.Equal(t, "opus", tmpl.VerifyModel, "VerifyModel should be set for --verify flag")
	assert.False(t, tmpl.Verify, "Verify should be false by default (opt-in)")

	// Verify step should exist and be optional
	verifyStep := findStep(tmpl, "verify")
	require.NotNil(t, verifyStep, "verify step required for --verify flag")
	assert.False(t, verifyStep.Required, "verify step should be optional")
	assert.Equal(t, domain.StepTypeVerify, verifyStep.Type)
}
