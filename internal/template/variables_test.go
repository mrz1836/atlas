package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewVariableExpander(t *testing.T) {
	e := NewVariableExpander()
	require.NotNil(t, e)
}

func TestVariableExpander_Expand_NilTemplate(t *testing.T) {
	e := NewVariableExpander()

	_, err := e.Expand(nil, map[string]string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNil)
}

func TestVariableExpander_Expand_NoVariables(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "A test template",
	}

	result, err := e.Expand(tmpl, nil)
	require.NoError(t, err)
	assert.Equal(t, "A test template", result.Description)
}

func TestVariableExpander_Expand_DescriptionVariable(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "Fix bug in {{component}}",
	}

	result, err := e.Expand(tmpl, map[string]string{"component": "auth"})
	require.NoError(t, err)
	assert.Equal(t, "Fix bug in auth", result.Description)
}

func TestVariableExpander_Expand_MultipleVariables(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "{{action}} the {{component}} in {{location}}",
	}

	result, err := e.Expand(tmpl, map[string]string{
		"action":    "Update",
		"component": "logger",
		"location":  "utils",
	})
	require.NoError(t, err)
	assert.Equal(t, "Update the logger in utils", result.Description)
}

func TestVariableExpander_Expand_StepDescription(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name: "test",
		Steps: []domain.StepDefinition{
			{
				Name:        "implement",
				Description: "Implement changes for {{task_id}}",
			},
		},
	}

	result, err := e.Expand(tmpl, map[string]string{"task_id": "TASK-123"})
	require.NoError(t, err)
	require.Len(t, result.Steps, 1)
	assert.Equal(t, "Implement changes for TASK-123", result.Steps[0].Description)
}

func TestVariableExpander_Expand_StepConfig(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name: "test",
		Steps: []domain.StepDefinition{
			{
				Name: "commit",
				Config: map[string]any{
					"message": "feat({{scope}}): {{description}}",
					"author":  "{{author}}",
				},
			},
		},
	}

	result, err := e.Expand(tmpl, map[string]string{
		"scope":       "cli",
		"description": "add new command",
		"author":      "Bot",
	})
	require.NoError(t, err)
	require.Len(t, result.Steps, 1)
	assert.Equal(t, "feat(cli): add new command", result.Steps[0].Config["message"])
	assert.Equal(t, "Bot", result.Steps[0].Config["author"])
}

func TestVariableExpander_Expand_RequiredVariableMissing(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "Fix {{bug_id}}",
		Variables: map[string]domain.TemplateVariable{
			"bug_id": {
				Required: true,
			},
		},
	}

	_, err := e.Expand(tmpl, map[string]string{})
	require.ErrorIs(t, err, atlaserrors.ErrVariableRequired)
	assert.Contains(t, err.Error(), "bug_id")
}

func TestVariableExpander_Expand_RequiredVariableWithDefault(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "Priority: {{priority}}",
		Variables: map[string]domain.TemplateVariable{
			"priority": {
				Required: true,
				Default:  "medium",
			},
		},
	}

	result, err := e.Expand(tmpl, map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "Priority: medium", result.Description)
}

func TestVariableExpander_Expand_RequiredVariableProvided(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "Priority: {{priority}}",
		Variables: map[string]domain.TemplateVariable{
			"priority": {
				Required: true,
			},
		},
	}

	result, err := e.Expand(tmpl, map[string]string{"priority": "high"})
	require.NoError(t, err)
	assert.Equal(t, "Priority: high", result.Description)
}

func TestVariableExpander_Expand_DefaultOverride(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "Model: {{model}}",
		Variables: map[string]domain.TemplateVariable{
			"model": {
				Required: false,
				Default:  "sonnet",
			},
		},
	}

	result, err := e.Expand(tmpl, map[string]string{"model": "opus"})
	require.NoError(t, err)
	assert.Equal(t, "Model: opus", result.Description)
}

func TestVariableExpander_Expand_UnknownVariable(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name:        "test",
		Description: "Unknown: {{unknown_var}}",
	}

	result, err := e.Expand(tmpl, map[string]string{})
	require.NoError(t, err)
	// Unknown variables are left unexpanded
	assert.Equal(t, "Unknown: {{unknown_var}}", result.Description)
}

func TestVariableExpander_Expand_OriginalUnmodified(t *testing.T) {
	e := NewVariableExpander()
	original := &domain.Template{
		Name:        "test",
		Description: "Original: {{value}}",
		Steps: []domain.StepDefinition{
			{
				Name:        "step1",
				Description: "Step: {{value}}",
				Config: map[string]any{
					"key": "{{value}}",
				},
			},
		},
	}

	_, err := e.Expand(original, map[string]string{"value": "expanded"})
	require.NoError(t, err)

	// Original should be unchanged
	assert.Equal(t, "Original: {{value}}", original.Description)
	assert.Equal(t, "Step: {{value}}", original.Steps[0].Description)
	assert.Equal(t, "{{value}}", original.Steps[0].Config["key"])
}

func TestVariableExpander_Expand_PreservesOtherFields(t *testing.T) {
	e := NewVariableExpander()
	original := &domain.Template{
		Name:         "test",
		Description:  "Test",
		BranchPrefix: "feat/",
		DefaultModel: "opus",
		Steps: []domain.StepDefinition{
			{
				Name:       "step1",
				Type:       domain.StepTypeAI,
				Required:   true,
				Timeout:    30 * time.Minute,
				RetryCount: 3,
			},
		},
		ValidationCommands: []string{"lint", "test"},
	}

	result, err := e.Expand(original, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, "test", result.Name)
	assert.Equal(t, "feat/", result.BranchPrefix)
	assert.Equal(t, "opus", result.DefaultModel)
	assert.Equal(t, []string{"lint", "test"}, result.ValidationCommands)

	require.Len(t, result.Steps, 1)
	assert.Equal(t, "step1", result.Steps[0].Name)
	assert.Equal(t, domain.StepTypeAI, result.Steps[0].Type)
	assert.True(t, result.Steps[0].Required)
	assert.Equal(t, 30*time.Minute, result.Steps[0].Timeout)
	assert.Equal(t, 3, result.Steps[0].RetryCount)
}

func TestVariableExpander_Expand_NonStringConfigValues(t *testing.T) {
	e := NewVariableExpander()
	tmpl := &domain.Template{
		Name: "test",
		Steps: []domain.StepDefinition{
			{
				Name: "step1",
				Config: map[string]any{
					"string_val":  "{{name}}",
					"int_val":     42,
					"bool_val":    true,
					"slice_val":   []string{"a", "b"},
					"timeout_val": 5 * time.Minute,
				},
			},
		},
	}

	result, err := e.Expand(tmpl, map[string]string{"name": "test"})
	require.NoError(t, err)

	// String values should be expanded
	assert.Equal(t, "test", result.Steps[0].Config["string_val"])

	// Non-string values should be preserved
	assert.Equal(t, 42, result.Steps[0].Config["int_val"])
	assert.Equal(t, true, result.Steps[0].Config["bool_val"])
	assert.Equal(t, []string{"a", "b"}, result.Steps[0].Config["slice_val"])
	assert.Equal(t, 5*time.Minute, result.Steps[0].Config["timeout_val"])
}

func Test_expandString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		values map[string]string
		want   string
	}{
		{
			name:   "no variables",
			input:  "plain text",
			values: nil,
			want:   "plain text",
		},
		{
			name:   "single variable",
			input:  "Hello {{name}}",
			values: map[string]string{"name": "World"},
			want:   "Hello World",
		},
		{
			name:   "multiple same variable",
			input:  "{{x}} and {{x}}",
			values: map[string]string{"x": "Y"},
			want:   "Y and Y",
		},
		{
			name:   "variable not in map",
			input:  "{{missing}}",
			values: map[string]string{},
			want:   "{{missing}}",
		},
		{
			name:   "empty value",
			input:  "before{{x}}after",
			values: map[string]string{"x": ""},
			want:   "beforeafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandString(tt.input, tt.values)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTemplate_Clone(t *testing.T) {
	original := &domain.Template{
		Name:               "original",
		Description:        "Original description",
		BranchPrefix:       "feat/",
		DefaultModel:       "opus",
		ValidationCommands: []string{"lint", "test"},
		Steps: []domain.StepDefinition{
			{
				Name:       "step1",
				Type:       domain.StepTypeAI,
				Required:   true,
				Timeout:    10 * time.Minute,
				RetryCount: 2,
				Config:     map[string]any{"key": "value"},
			},
		},
		Variables: map[string]domain.TemplateVariable{
			"var1": {Required: true, Default: "default1"},
		},
	}

	clone := original.Clone()

	// Verify clone has same values
	assert.Equal(t, original.Name, clone.Name)
	assert.Equal(t, original.Description, clone.Description)
	assert.Equal(t, original.BranchPrefix, clone.BranchPrefix)
	assert.Equal(t, original.DefaultModel, clone.DefaultModel)
	assert.Equal(t, original.ValidationCommands, clone.ValidationCommands)
	require.Len(t, clone.Steps, 1)
	assert.Equal(t, original.Steps[0].Name, clone.Steps[0].Name)
	assert.Equal(t, original.Variables["var1"], clone.Variables["var1"])

	// Modify clone and verify original is unchanged
	clone.Name = "modified"
	clone.ValidationCommands[0] = "modified"
	clone.Steps[0].Name = "modified"
	clone.Steps[0].Config["key"] = "modified"
	clone.Variables["var2"] = domain.TemplateVariable{Required: false}

	assert.Equal(t, "original", original.Name)
	assert.Equal(t, "lint", original.ValidationCommands[0])
	assert.Equal(t, "step1", original.Steps[0].Name)
	assert.Equal(t, "value", original.Steps[0].Config["key"])
	_, exists := original.Variables["var2"]
	assert.False(t, exists)
}
