package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewCommitTemplate(t *testing.T) {
	tmpl := NewCommitTemplate()

	require.NotNil(t, tmpl)
	assert.Equal(t, "commit", tmpl.Name)
	assert.Equal(t, "chore", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.NotEmpty(t, tmpl.Description)
}

func TestCommitTemplate_StepOrder(t *testing.T) {
	tmpl := NewCommitTemplate()

	expectedSteps := []string{
		"analyze_changes", "smart_commit", "git_push",
	}

	require.Len(t, tmpl.Steps, len(expectedSteps), "expected %d steps", len(expectedSteps))
	for i, name := range expectedSteps {
		assert.Equal(t, name, tmpl.Steps[i].Name, "step %d should be %s", i, name)
	}
}

func TestCommitTemplate_StepTypes(t *testing.T) {
	tmpl := NewCommitTemplate()

	expectedTypes := map[string]domain.StepType{
		"analyze_changes": domain.StepTypeAI,
		"smart_commit":    domain.StepTypeGit,
		"git_push":        domain.StepTypeGit,
	}

	for _, step := range tmpl.Steps {
		expected, ok := expectedTypes[step.Name]
		require.True(t, ok, "unexpected step: %s", step.Name)
		assert.Equal(t, expected, step.Type, "step %s has wrong type", step.Name)
	}
}

func TestCommitTemplate_AllStepsRequired(t *testing.T) {
	tmpl := NewCommitTemplate()

	for _, step := range tmpl.Steps {
		assert.True(t, step.Required, "step %s should be required", step.Name)
	}
}

func TestCommitTemplate_Timeouts(t *testing.T) {
	tmpl := NewCommitTemplate()

	stepTimeouts := map[string]time.Duration{
		"analyze_changes": 5 * time.Minute,
		"smart_commit":    2 * time.Minute,
		"git_push":        2 * time.Minute,
	}

	for _, step := range tmpl.Steps {
		expected, ok := stepTimeouts[step.Name]
		require.True(t, ok, "no expected timeout for %s", step.Name)
		assert.Equal(t, expected, step.Timeout, "step %s has wrong timeout", step.Name)
	}
}

func TestCommitTemplate_AnalyzeChangesConfig(t *testing.T) {
	tmpl := NewCommitTemplate()

	step := findStep(tmpl, "analyze_changes")
	require.NotNil(t, step)

	assert.Equal(t, "plan", step.Config["permission_mode"])
	assert.Equal(t, true, step.Config["detect_garbage"])

	patterns, ok := step.Config["garbage_patterns"].([]string)
	require.True(t, ok, "garbage_patterns should be []string")
	assert.NotEmpty(t, patterns)

	// Verify common garbage patterns are present
	expectedPatterns := []string{"*.tmp", "*.bak", ".DS_Store", ".env*"}
	for _, expected := range expectedPatterns {
		assert.Contains(t, patterns, expected, "missing pattern: %s", expected)
	}
}

func TestCommitTemplate_SmartCommitConfig(t *testing.T) {
	tmpl := NewCommitTemplate()

	step := findStep(tmpl, "smart_commit")
	require.NotNil(t, step)

	assert.Equal(t, "smart_commit", step.Config["operation"])
	assert.Equal(t, true, step.Config["group_by_package"])
	assert.Equal(t, true, step.Config["conventional"])
}

func TestCommitTemplate_GitPushConfig(t *testing.T) {
	tmpl := NewCommitTemplate()

	step := findStep(tmpl, "git_push")
	require.NotNil(t, step)

	assert.Equal(t, "push", step.Config["operation"])
	assert.Equal(t, 3, step.RetryCount)
}

func TestCommitTemplate_NoValidationCommands(t *testing.T) {
	tmpl := NewCommitTemplate()

	// Commit template typically doesn't run validation
	// as it's just committing existing changes
	assert.Empty(t, tmpl.ValidationCommands)
}
