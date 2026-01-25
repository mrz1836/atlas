package template

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewDefaultRegistry(t *testing.T) {
	r := NewDefaultRegistry()
	require.NotNil(t, r)
}

func TestDefaultRegistry_ContainsAllTemplates(t *testing.T) {
	r := NewDefaultRegistry()

	templates := r.List()
	assert.Len(t, templates, 5)

	// Verify all five templates are present
	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
	}

	assert.True(t, names["bug"], "missing bug template")
	assert.True(t, names["patch"], "missing patch template")
	assert.True(t, names["feature"], "missing feature template")
	assert.True(t, names["commit"], "missing commit template")
	assert.True(t, names["task"], "missing task template")
}

func TestDefaultRegistry_Aliases(t *testing.T) {
	r := NewDefaultRegistry()

	// Verify aliases are registered
	aliases := r.Aliases()
	assert.Len(t, aliases, 3)
	assert.Equal(t, "bug", aliases["fix"])
	assert.Equal(t, "bug", aliases["bugfix"])
	assert.Equal(t, "patch", aliases["hotfix"])
}

func TestDefaultRegistry_AliasResolution(t *testing.T) {
	r := NewDefaultRegistry()

	// Test "fix" alias resolves to "bug" template
	fixTmpl, err := r.Get("fix")
	require.NoError(t, err)
	assert.Equal(t, "bug", fixTmpl.Name)

	// Test "bugfix" alias resolves to "bug" template
	bugfixTmpl, err := r.Get("bugfix")
	require.NoError(t, err)
	assert.Equal(t, "bug", bugfixTmpl.Name)

	// Test "hotfix" alias resolves to "patch" template
	hotfixTmpl, err := r.Get("hotfix")
	require.NoError(t, err)
	assert.Equal(t, "patch", hotfixTmpl.Name)
}

func TestDefaultRegistry_IsAlias(t *testing.T) {
	r := NewDefaultRegistry()

	// Aliases should be detected
	assert.True(t, r.IsAlias("fix"))
	assert.True(t, r.IsAlias("bugfix"))
	assert.True(t, r.IsAlias("hotfix"))

	// Templates should not be detected as aliases
	assert.False(t, r.IsAlias("bug"))
	assert.False(t, r.IsAlias("patch"))
	assert.False(t, r.IsAlias("feature"))
	assert.False(t, r.IsAlias("task"))
	assert.False(t, r.IsAlias("commit"))
}

func TestDefaultRegistry_GetBug(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := r.Get("bug")
	require.NoError(t, err)
	assert.Equal(t, "bug", tmpl.Name)
	assert.Equal(t, "fix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
}

func TestDefaultRegistry_GetPatch(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := r.Get("patch")
	require.NoError(t, err)
	assert.Equal(t, "patch", tmpl.Name)
	assert.Equal(t, "patch", tmpl.BranchPrefix)
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

func TestDefaultRegistry_GetFixAlias(t *testing.T) {
	r := NewDefaultRegistry()

	// "fix" is now an alias for "bug"
	tmpl, err := r.Get("fix")
	require.NoError(t, err)
	assert.Equal(t, "bug", tmpl.Name, "fix alias should resolve to bug template")
	assert.Equal(t, "fix", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
}

func TestDefaultRegistry_TemplatesAreCompiledIn(t *testing.T) {
	// This test verifies that templates are Go code, not loaded from files.
	// If templates were loaded from files, this would fail or require file I/O.
	r := NewDefaultRegistry()

	// All templates (and aliases) should be immediately available without file loading
	for _, name := range []string{"bug", "patch", "feature", "commit", "task", "fix", "bugfix", "hotfix"} {
		tmpl, err := r.Get(name)
		require.NoError(t, err, "template/alias %s should be available", name)
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

// Helper to create a valid custom template YAML file.
func createCustomTemplateFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const validCustomTemplate = `
name: custom-workflow
description: A custom workflow template
branch_prefix: custom
default_model: haiku

steps:
  - name: implement
    type: ai
    required: true
    timeout: 20m

  - name: validate
    type: validation
    required: true
    timeout: 5m
`

const customBugfixOverride = `
name: bugfix
description: Custom bugfix workflow
branch_prefix: hotfix
default_model: opus

steps:
  - name: quick-fix
    type: ai
    required: true
    timeout: 10m
`

func TestNewRegistryWithConfig_NoCustom(t *testing.T) {
	r, err := NewRegistryWithConfig("/tmp", nil)
	require.NoError(t, err)
	require.NotNil(t, r)

	// Should have all 5 built-in templates
	assert.Len(t, r.List(), 5)
}

func TestNewRegistryWithConfig_EmptyCustom(t *testing.T) {
	r, err := NewRegistryWithConfig("/tmp", map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, r)

	// Should have all 5 built-in templates
	assert.Len(t, r.List(), 5)
}

func TestNewRegistryWithConfig_CustomAdded(t *testing.T) {
	tmpDir := t.TempDir()
	createCustomTemplateFile(t, tmpDir, "custom.yaml", validCustomTemplate)

	r, err := NewRegistryWithConfig(tmpDir, map[string]string{
		"custom-workflow": "custom.yaml",
	})
	require.NoError(t, err)
	require.NotNil(t, r)

	// Should have 6 templates (5 built-in + 1 custom)
	assert.Len(t, r.List(), 6)

	// Verify custom template is available
	custom, err := r.Get("custom-workflow")
	require.NoError(t, err)
	assert.Equal(t, "custom-workflow", custom.Name)
	assert.Equal(t, "custom", custom.BranchPrefix)
	assert.Equal(t, "haiku", custom.DefaultModel)
}

func TestNewRegistryWithConfig_OverrideBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	createCustomTemplateFile(t, tmpDir, "bug.yaml", customBugfixOverride)

	r, err := NewRegistryWithConfig(tmpDir, map[string]string{
		"bug": "bug.yaml",
	})
	require.NoError(t, err)
	require.NotNil(t, r)

	// Should still have 5 templates (custom replaces built-in)
	assert.Len(t, r.List(), 5)

	// Verify the bug template was replaced (note: custom has name="bugfix" in YAML but we override with config key)
	bug, err := r.Get("bug")
	require.NoError(t, err)
	assert.Equal(t, "bug", bug.Name)
	assert.Equal(t, "Custom bugfix workflow", bug.Description)
	assert.Equal(t, "hotfix", bug.BranchPrefix)
	assert.Equal(t, "opus", bug.DefaultModel)

	// Verify other built-ins are unchanged
	feature, err := r.Get("feature")
	require.NoError(t, err)
	assert.Equal(t, "feat", feature.BranchPrefix)
}

func TestNewRegistryWithConfig_LoadFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file
	_, err := NewRegistryWithConfig(tmpDir, map[string]string{
		"missing": "nonexistent.yaml",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load custom templates")
}

func TestNewRegistryWithConfig_InvalidTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	invalidTemplate := `
name: invalid
steps:
  - name: step1
    type: unknown_type
    required: true
`
	createCustomTemplateFile(t, tmpDir, "invalid.yaml", invalidTemplate)

	_, err := NewRegistryWithConfig(tmpDir, map[string]string{
		"invalid": "invalid.yaml",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not valid")
}

func TestNewRegistryWithConfig_MultipleCustom(t *testing.T) {
	tmpDir := t.TempDir()

	template1 := `
name: workflow1
steps:
  - name: step1
    type: ai
    required: true
`
	template2 := `
name: workflow2
steps:
  - name: step1
    type: validation
    required: true
`
	createCustomTemplateFile(t, tmpDir, "w1.yaml", template1)
	createCustomTemplateFile(t, tmpDir, "w2.yaml", template2)

	r, err := NewRegistryWithConfig(tmpDir, map[string]string{
		"workflow1": "w1.yaml",
		"workflow2": "w2.yaml",
	})
	require.NoError(t, err)

	// Should have 7 templates (5 built-in + 2 custom)
	assert.Len(t, r.List(), 7)

	// Verify both custom templates are available
	w1, err := r.Get("workflow1")
	require.NoError(t, err)
	assert.Equal(t, domain.StepTypeAI, w1.Steps[0].Type)

	w2, err := r.Get("workflow2")
	require.NoError(t, err)
	assert.Equal(t, domain.StepTypeValidation, w2.Steps[0].Type)
}

func TestNewRegistryWithConfig_ConfigNameOverridesFileName(t *testing.T) {
	tmpDir := t.TempDir()
	template := `
name: original-name
steps:
  - name: step1
    type: ai
    required: true
`
	createCustomTemplateFile(t, tmpDir, "template.yaml", template)

	r, err := NewRegistryWithConfig(tmpDir, map[string]string{
		"config-name": "template.yaml",
	})
	require.NoError(t, err)

	// Should be able to get by config name
	tmpl, err := r.Get("config-name")
	require.NoError(t, err)
	assert.Equal(t, "config-name", tmpl.Name)

	// Original name should not exist
	_, err = r.Get("original-name")
	require.Error(t, err)
}

func TestNewRegistryWithConfig_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	absPath := createCustomTemplateFile(t, tmpDir, "template.yaml", validCustomTemplate)

	// Use a different base path to prove absolute path works
	r, err := NewRegistryWithConfig("/some/other/path", map[string]string{
		"custom-workflow": absPath,
	})
	require.NoError(t, err)

	tmpl, err := r.Get("custom-workflow")
	require.NoError(t, err)
	assert.Equal(t, "custom-workflow", tmpl.Name)
}
