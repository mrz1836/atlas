package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func expectedQualitySteps() []string {
	return []string{
		"analyze_and_fix", "verify", "validate", "git_commit", "git_push", "git_pr", "ci_wait", "review",
	}
}

// assertQualityTemplateBase validates fields common to all quality templates.
func assertQualityTemplateBase(t *testing.T, tmpl *domain.Template, name string) {
	t.Helper()
	require.NotNil(t, tmpl)
	assert.Equal(t, name, tmpl.Name)
	assert.Equal(t, "quality", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.Equal(t, domain.AgentClaude, tmpl.DefaultAgent)
	assert.False(t, tmpl.Verify)
	assert.Equal(t, "opus", tmpl.VerifyModel)
	assert.NotEmpty(t, tmpl.Description)
}

// assertQualityTemplateSteps validates the step order, types, and required flags.
func assertQualityTemplateSteps(t *testing.T, tmpl *domain.Template, promptTemplate string) {
	t.Helper()

	require.Len(t, tmpl.Steps, len(expectedQualitySteps()))
	for i, name := range expectedQualitySteps() {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}

	expectedTypes := map[string]domain.StepType{
		"analyze_and_fix": domain.StepTypeAI,
		"verify":          domain.StepTypeVerify,
		"validate":        domain.StepTypeValidation,
		"git_commit":      domain.StepTypeGit,
		"git_push":        domain.StepTypeGit,
		"git_pr":          domain.StepTypeGit,
		"ci_wait":         domain.StepTypeCI,
		"review":          domain.StepTypeHuman,
	}
	for _, step := range tmpl.Steps {
		expected, ok := expectedTypes[step.Name]
		require.True(t, ok, "unexpected step: %s", step.Name)
		assert.Equal(t, expected, step.Type, "step %s has wrong type", step.Name)
	}

	// verify is the only optional step
	for _, step := range tmpl.Steps {
		if step.Name == "verify" {
			assert.False(t, step.Required, "verify step should be optional")
		} else {
			assert.True(t, step.Required, "step %s should be required", step.Name)
		}
	}

	// Check analyze_and_fix config
	aiStep := findStep(tmpl, "analyze_and_fix")
	require.NotNil(t, aiStep)
	assert.Equal(t, "default", aiStep.Config["permission_mode"])
	assert.Equal(t, promptTemplate, aiStep.Config["prompt_template"])
	assert.Equal(t, constants.DefaultAITimeout, aiStep.Timeout)
	assert.Equal(t, 3, aiStep.RetryCount)
}

// assertQualityTemplateTimeouts validates step timeouts.
func assertQualityTemplateTimeouts(t *testing.T, tmpl *domain.Template) {
	t.Helper()

	stepTimeouts := map[string]time.Duration{
		"analyze_and_fix": constants.DefaultAITimeout,
		"verify":          5 * time.Minute,
		"validate":        10 * time.Minute,
		"git_commit":      constants.GitCommitTimeout,
		"git_push":        constants.GitPushTimeout,
		"git_pr":          constants.GitPRTimeout,
		"ci_wait":         constants.DefaultCITimeout,
	}
	for _, step := range tmpl.Steps {
		if expected, ok := stepTimeouts[step.Name]; ok {
			assert.Equal(t, expected, step.Timeout, "step %s has wrong timeout", step.Name)
		}
	}
}

// assertQualityTemplateGitConfig validates git step configurations.
func assertQualityTemplateGitConfig(t *testing.T, tmpl *domain.Template) {
	t.Helper()

	gitCommitStep := findStep(tmpl, "git_commit")
	require.NotNil(t, gitCommitStep)
	assert.Equal(t, "commit", gitCommitStep.Config["operation"])

	gitPushStep := findStep(tmpl, "git_push")
	require.NotNil(t, gitPushStep)
	assert.Equal(t, "push", gitPushStep.Config["operation"])
	assert.Equal(t, constants.GitPushRetryCount, gitPushStep.RetryCount)

	gitPRStep := findStep(tmpl, "git_pr")
	require.NotNil(t, gitPRStep)
	assert.Equal(t, "create_pr", gitPRStep.Config["operation"])
	assert.Equal(t, constants.GitPRRetryCount, gitPRStep.RetryCount)

	ciWaitStep := findStep(tmpl, "ci_wait")
	require.NotNil(t, ciWaitStep)

	reviewStep := findStep(tmpl, "review")
	require.NotNil(t, reviewStep)
	assert.NotEmpty(t, reviewStep.Config["prompt"])
}

// assertQualityTemplateValidation checks ValidationCommands and passes ValidateTemplate.
func assertQualityTemplateValidation(t *testing.T, tmpl *domain.Template) {
	t.Helper()

	expectedCommands := []string{
		"magex format:fix",
		"magex lint",
		"magex test:race",
		"go-pre-commit run --all-files --skip lint",
	}
	assert.Equal(t, expectedCommands, tmpl.ValidationCommands)
	assert.NoError(t, ValidateTemplate(tmpl))
}

// assertQualityTemplateVerifyStep checks verify step configuration.
func assertQualityTemplateVerifyStep(t *testing.T, tmpl *domain.Template) {
	t.Helper()

	verifyStep := findStep(tmpl, "verify")
	require.NotNil(t, verifyStep)
	assert.Equal(t, domain.StepTypeVerify, verifyStep.Type)
	assert.False(t, verifyStep.Required)
	assert.Equal(t, 5*time.Minute, verifyStep.Timeout)
	assert.Equal(t, "gemini", verifyStep.Config["agent"])
	assert.Empty(t, verifyStep.Config["model"])
	checks, ok := verifyStep.Config["checks"].([]string)
	require.True(t, ok, "checks should be a string slice")
	assert.Contains(t, checks, "code_correctness")
}

// --- go-optimize ---

func TestNewGoOptimizeTemplate(t *testing.T) {
	tmpl := NewGoOptimizeTemplate()
	assertQualityTemplateBase(t, tmpl, "go-optimize")
}

func TestGoOptimizeTemplate_Steps(t *testing.T) {
	tmpl := NewGoOptimizeTemplate()
	assertQualityTemplateSteps(t, tmpl, "go-optimize")
}

func TestGoOptimizeTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewGoOptimizeTemplate())
}

func TestGoOptimizeTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewGoOptimizeTemplate())
}

func TestGoOptimizeTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewGoOptimizeTemplate())
}

func TestGoOptimizeTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewGoOptimizeTemplate())
}

func TestGoOptimizeTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("go-optimize")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "go-optimize", tmpl.Name)
}

// --- dedup ---

func TestNewDedupTemplate(t *testing.T) {
	tmpl := NewDedupTemplate()
	assertQualityTemplateBase(t, tmpl, "dedup")
}

func TestDedupTemplate_Steps(t *testing.T) {
	assertQualityTemplateSteps(t, NewDedupTemplate(), "dedup")
}

func TestDedupTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewDedupTemplate())
}

func TestDedupTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewDedupTemplate())
}

func TestDedupTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewDedupTemplate())
}

func TestDedupTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewDedupTemplate())
}

func TestDedupTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("dedup")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "dedup", tmpl.Name)
}

// --- goroutine-leak ---

func TestNewGoroutineLeakTemplate(t *testing.T) {
	tmpl := NewGoroutineLeakTemplate()
	assertQualityTemplateBase(t, tmpl, "goroutine-leak")
}

func TestGoroutineLeakTemplate_Steps(t *testing.T) {
	assertQualityTemplateSteps(t, NewGoroutineLeakTemplate(), "goroutine-leak")
}

func TestGoroutineLeakTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewGoroutineLeakTemplate())
}

func TestGoroutineLeakTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewGoroutineLeakTemplate())
}

func TestGoroutineLeakTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewGoroutineLeakTemplate())
}

func TestGoroutineLeakTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewGoroutineLeakTemplate())
}

func TestGoroutineLeakTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("goroutine-leak")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "goroutine-leak", tmpl.Name)
}

// --- jr-to-sr ---

func TestNewJrToSrTemplate(t *testing.T) {
	tmpl := NewJrToSrTemplate()
	assertQualityTemplateBase(t, tmpl, "jr-to-sr")
}

func TestJrToSrTemplate_Steps(t *testing.T) {
	assertQualityTemplateSteps(t, NewJrToSrTemplate(), "jr-to-sr")
}

func TestJrToSrTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewJrToSrTemplate())
}

func TestJrToSrTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewJrToSrTemplate())
}

func TestJrToSrTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewJrToSrTemplate())
}

func TestJrToSrTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewJrToSrTemplate())
}

func TestJrToSrTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("jr-to-sr")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "jr-to-sr", tmpl.Name)
}

// --- constant-hunter ---

func TestNewConstantHunterTemplate(t *testing.T) {
	tmpl := NewConstantHunterTemplate()
	assertQualityTemplateBase(t, tmpl, "constant-hunter")
}

func TestConstantHunterTemplate_Steps(t *testing.T) {
	assertQualityTemplateSteps(t, NewConstantHunterTemplate(), "constant-hunter")
}

func TestConstantHunterTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewConstantHunterTemplate())
}

func TestConstantHunterTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewConstantHunterTemplate())
}

func TestConstantHunterTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewConstantHunterTemplate())
}

func TestConstantHunterTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewConstantHunterTemplate())
}

func TestConstantHunterTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("constant-hunter")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "constant-hunter", tmpl.Name)
}

// --- config-hunter ---

func TestNewConfigHunterTemplate(t *testing.T) {
	tmpl := NewConfigHunterTemplate()
	assertQualityTemplateBase(t, tmpl, "config-hunter")
}

func TestConfigHunterTemplate_Steps(t *testing.T) {
	assertQualityTemplateSteps(t, NewConfigHunterTemplate(), "config-hunter")
}

func TestConfigHunterTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewConfigHunterTemplate())
}

func TestConfigHunterTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewConfigHunterTemplate())
}

func TestConfigHunterTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewConfigHunterTemplate())
}

func TestConfigHunterTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewConfigHunterTemplate())
}

func TestConfigHunterTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("config-hunter")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "config-hunter", tmpl.Name)
}

// --- test-creator ---

func TestNewTestCreatorTemplate(t *testing.T) {
	tmpl := NewTestCreatorTemplate()
	assertQualityTemplateBase(t, tmpl, "test-creator")
}

func TestTestCreatorTemplate_Steps(t *testing.T) {
	assertQualityTemplateSteps(t, NewTestCreatorTemplate(), "test-creator")
}

func TestTestCreatorTemplate_Timeouts(t *testing.T) {
	assertQualityTemplateTimeouts(t, NewTestCreatorTemplate())
}

func TestTestCreatorTemplate_GitConfig(t *testing.T) {
	assertQualityTemplateGitConfig(t, NewTestCreatorTemplate())
}

func TestTestCreatorTemplate_VerifyStep(t *testing.T) {
	assertQualityTemplateVerifyStep(t, NewTestCreatorTemplate())
}

func TestTestCreatorTemplate_Validation(t *testing.T) {
	assertQualityTemplateValidation(t, NewTestCreatorTemplate())
}

func TestTestCreatorTemplate_RegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()
	tmpl, err := registry.Get("test-creator")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "test-creator", tmpl.Name)
}

// --- cross-cutting ---

func TestQualityTemplates_AllRegisteredInDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()

	qualityTemplates := []string{
		"go-optimize", "dedup", "goroutine-leak", "jr-to-sr",
		"constant-hunter", "config-hunter", "test-creator",
	}
	for _, name := range qualityTemplates {
		tmpl, err := registry.Get(name)
		require.NoError(t, err, "template %q should be registered", name)
		require.NotNil(t, tmpl)
		assert.Equal(t, name, tmpl.Name)
		assert.Equal(t, "quality", tmpl.BranchPrefix)
	}
}

func TestQualityTemplates_AllPassValidation(t *testing.T) {
	constructors := []func() *domain.Template{
		NewGoOptimizeTemplate,
		NewDedupTemplate,
		NewGoroutineLeakTemplate,
		NewJrToSrTemplate,
		NewConstantHunterTemplate,
		NewConfigHunterTemplate,
		NewTestCreatorTemplate,
	}
	for _, ctor := range constructors {
		tmpl := ctor()
		assert.NoError(t, ValidateTemplate(tmpl), "template %q should pass validation", tmpl.Name)
	}
}

func TestQualityTemplates_UniqueNames(t *testing.T) {
	constructors := []func() *domain.Template{
		NewGoOptimizeTemplate,
		NewDedupTemplate,
		NewGoroutineLeakTemplate,
		NewJrToSrTemplate,
		NewConstantHunterTemplate,
		NewConfigHunterTemplate,
		NewTestCreatorTemplate,
	}

	seen := make(map[string]bool)
	for _, ctor := range constructors {
		tmpl := ctor()
		assert.False(t, seen[tmpl.Name], "duplicate template name: %s", tmpl.Name)
		seen[tmpl.Name] = true
	}
}
