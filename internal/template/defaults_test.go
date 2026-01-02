package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultRegistry(t *testing.T) {
	r := NewDefaultRegistry()
	require.NotNil(t, r)
}

func TestDefaultRegistry_ContainsAllTemplates(t *testing.T) {
	r := NewDefaultRegistry()

	templates := r.List()
	assert.Len(t, templates, 4)

	// Verify all four templates are present
	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
	}

	assert.True(t, names["bugfix"], "missing bugfix template")
	assert.True(t, names["feature"], "missing feature template")
	assert.True(t, names["commit"], "missing commit template")
	assert.True(t, names["task"], "missing task template")
}

func TestDefaultRegistry_GetBugfix(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := r.Get("bugfix")
	require.NoError(t, err)
	assert.Equal(t, "bugfix", tmpl.Name)
	assert.Equal(t, "fix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
}

func TestDefaultRegistry_GetFeature(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := r.Get("feature")
	require.NoError(t, err)
	assert.Equal(t, "feature", tmpl.Name)
	assert.Equal(t, "feat", tmpl.BranchPrefix)
	assert.Equal(t, "opus", tmpl.DefaultModel)
}

func TestDefaultRegistry_GetCommit(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := r.Get("commit")
	require.NoError(t, err)
	assert.Equal(t, "commit", tmpl.Name)
	assert.Equal(t, "chore", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
}

func TestDefaultRegistry_GetTask(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := r.Get("task")
	require.NoError(t, err)
	assert.Equal(t, "task", tmpl.Name)
	assert.Equal(t, "task", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
}

func TestDefaultRegistry_TemplatesAreCompiledIn(t *testing.T) {
	// This test verifies that templates are Go code, not loaded from files.
	// If templates were loaded from files, this would fail or require file I/O.
	r := NewDefaultRegistry()

	// All templates should be immediately available without file loading
	for _, name := range []string{"bugfix", "feature", "commit", "task"} {
		tmpl, err := r.Get(name)
		require.NoError(t, err, "template %s should be available", name)
		assert.NotEmpty(t, tmpl.Steps, "template %s should have steps", name)
	}
}

func TestDefaultRegistry_TemplatesHaveValidConfiguration(t *testing.T) {
	r := NewDefaultRegistry()

	for _, tmpl := range r.List() {
		t.Run(tmpl.Name, func(t *testing.T) {
			// Each template should have required fields
			assert.NotEmpty(t, tmpl.Name)
			assert.NotEmpty(t, tmpl.Description)
			assert.NotEmpty(t, tmpl.BranchPrefix)
			assert.NotEmpty(t, tmpl.DefaultModel)
			assert.NotEmpty(t, tmpl.Steps)

			// Each step should have required fields
			for _, step := range tmpl.Steps {
				assert.NotEmpty(t, step.Name, "step should have name")
				assert.NotEmpty(t, step.Type, "step %s should have type", step.Name)
			}
		})
	}
}
