package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewFeatureTemplate(t *testing.T) {
	tmpl := NewFeatureTemplate()

	require.NotNil(t, tmpl)
	assert.Equal(t, "feature", tmpl.Name)
	assert.Equal(t, "feat/", tmpl.BranchPrefix)
	assert.Equal(t, "opus", tmpl.DefaultModel)
	assert.NotEmpty(t, tmpl.Description)
}

func TestFeatureTemplate_StepOrder(t *testing.T) {
	tmpl := NewFeatureTemplate()

	expectedSteps := []string{
		"specify", "review_spec", "plan", "tasks", "implement", "verify",
		"validate", "checklist", "git_commit", "git_push", "git_pr",
		"ci_wait", "review",
	}

	require.Len(t, tmpl.Steps, len(expectedSteps), "expected %d steps", len(expectedSteps))
	for i, name := range expectedSteps {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}
}

func TestFeatureTemplate_StepTypes(t *testing.T) {
	tmpl := NewFeatureTemplate()

	expectedTypes := map[string]domain.StepType{
		"specify":     domain.StepTypeSDD,
		"review_spec": domain.StepTypeHuman,
		"plan":        domain.StepTypeSDD,
		"tasks":       domain.StepTypeSDD,
		"implement":   domain.StepTypeSDD,
		"verify":      domain.StepTypeVerify,
		"validate":    domain.StepTypeValidation,
		"checklist":   domain.StepTypeSDD,
		"git_commit":  domain.StepTypeGit,
		"git_push":    domain.StepTypeGit,
		"git_pr":      domain.StepTypeGit,
		"ci_wait":     domain.StepTypeCI,
		"review":      domain.StepTypeHuman,
	}

	for _, step := range tmpl.Steps {
		expected, ok := expectedTypes[step.Name]
		require.True(t, ok, "unexpected step: %s", step.Name)
		assert.Equal(t, expected, step.Type, "step %s has wrong type", step.Name)
	}
}

func TestFeatureTemplate_SDDSteps(t *testing.T) {
	tmpl := NewFeatureTemplate()

	sddSteps := []struct {
		name    string
		command string
	}{
		{"specify", "specify"},
		{"plan", "plan"},
		{"tasks", "tasks"},
		{"checklist", "checklist"},
	}

	for _, tc := range sddSteps {
		step := findStep(tmpl, tc.name)
		require.NotNil(t, step, "step %s not found", tc.name)
		assert.Equal(t, domain.StepTypeSDD, step.Type, "step %s should be SDD type", tc.name)
		assert.Equal(t, tc.command, step.Config["sdd_command"], "step %s has wrong SDD command", tc.name)
	}
}

func TestFeatureTemplate_HumanReviewCheckpoints(t *testing.T) {
	tmpl := NewFeatureTemplate()

	humanSteps := []string{"review_spec", "review"}

	for _, name := range humanSteps {
		step := findStep(tmpl, name)
		require.NotNil(t, step, "step %s not found", name)
		assert.Equal(t, domain.StepTypeHuman, step.Type, "step %s should be human type", name)
		assert.NotEmpty(t, step.Config["prompt"], "step %s should have a prompt", name)
	}
}

func TestFeatureTemplate_AllStepsRequired(t *testing.T) {
	tmpl := NewFeatureTemplate()

	for _, step := range tmpl.Steps {
		assert.True(t, step.Required, "step %s should be required", step.Name)
	}
}

func TestFeatureTemplate_Timeouts(t *testing.T) {
	tmpl := NewFeatureTemplate()

	stepTimeouts := map[string]time.Duration{
		"specify":    20 * time.Minute,
		"plan":       15 * time.Minute,
		"tasks":      15 * time.Minute,
		"implement":  45 * time.Minute,
		"validate":   10 * time.Minute,
		"checklist":  10 * time.Minute,
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

func TestFeatureTemplate_LongerAITimeouts(t *testing.T) {
	tmpl := NewFeatureTemplate()

	// Feature template should have longer timeouts for AI spec steps
	specifyStep := findStep(tmpl, "specify")
	require.NotNil(t, specifyStep)
	assert.Equal(t, 20*time.Minute, specifyStep.Timeout)

	implementStep := findStep(tmpl, "implement")
	require.NotNil(t, implementStep)
	assert.Equal(t, 45*time.Minute, implementStep.Timeout)
}

func TestFeatureTemplate_RetryConfiguration(t *testing.T) {
	tmpl := NewFeatureTemplate()

	stepRetries := map[string]int{
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

func TestFeatureTemplate_ValidationCommands(t *testing.T) {
	tmpl := NewFeatureTemplate()

	expectedCommands := []string{
		"magex format:fix",
		"magex lint",
		"magex test",
	}

	assert.Equal(t, expectedCommands, tmpl.ValidationCommands)
}

func TestFeatureTemplate_ImplementStep(t *testing.T) {
	tmpl := NewFeatureTemplate()

	implementStep := findStep(tmpl, "implement")
	require.NotNil(t, implementStep)
	assert.Equal(t, domain.StepTypeSDD, implementStep.Type)
	assert.Equal(t, "implement", implementStep.Config["sdd_command"])
}
