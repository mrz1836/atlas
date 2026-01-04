package template

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// validTemplate returns a valid template for testing.
func validTemplate() *domain.Template {
	return &domain.Template{
		Name:         "test-template",
		Description:  "A test template",
		BranchPrefix: "test",
		DefaultModel: "sonnet",
		Steps: []domain.StepDefinition{
			{
				Name:       "implement",
				Type:       domain.StepTypeAI,
				Required:   true,
				Timeout:    30 * time.Minute,
				RetryCount: 3,
			},
		},
	}
}

func TestValidateTemplate_Valid(t *testing.T) {
	tmpl := validTemplate()
	err := ValidateTemplate(tmpl)
	assert.NoError(t, err)
}

func TestValidateTemplate_ValidWithAllFields(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "full-template",
		Description:  "A complete template with all fields",
		BranchPrefix: "feat",
		DefaultModel: "opus",
		Verify:       true,
		VerifyModel:  "sonnet",
		Steps: []domain.StepDefinition{
			{
				Name:        "analyze",
				Type:        domain.StepTypeAI,
				Description: "Analyze the problem",
				Required:    true,
				Timeout:     15 * time.Minute,
				RetryCount:  2,
				Config: map[string]any{
					"permission_mode": "plan",
				},
			},
			{
				Name:     "validate",
				Type:     domain.StepTypeValidation,
				Required: true,
				Timeout:  10 * time.Minute,
			},
			{
				Name:     "commit",
				Type:     domain.StepTypeGit,
				Required: true,
				Config: map[string]any{
					"operation": "commit",
				},
			},
		},
		ValidationCommands: []string{"make lint", "make test"},
		Variables: map[string]domain.TemplateVariable{
			"ticket_id": {
				Description: "JIRA ticket ID",
				Required:    true,
			},
			"component": {
				Description: "Component name",
				Default:     "core",
				Required:    false,
			},
		},
	}

	err := ValidateTemplate(tmpl)
	assert.NoError(t, err)
}

func TestValidateTemplate_Nil(t *testing.T) {
	err := ValidateTemplate(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNil)
}

func TestValidateTemplate_EmptyName(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Name = ""
	err := ValidateTemplate(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestValidateTemplate_WhitespaceName(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Name = "   \t\n  "
	err := ValidateTemplate(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestValidateTemplate_NoSteps(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps = nil
	err := ValidateTemplate(tmpl)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
	assert.Contains(t, err.Error(), "at least one step")
}

func TestValidateTemplate_EmptySteps(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps = []domain.StepDefinition{}
	err := ValidateTemplate(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
}

func TestValidateTemplate_EmptyStepName(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].Name = ""
	err := ValidateTemplate(tmpl)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
	assert.Contains(t, err.Error(), "step 0")
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateTemplate_WhitespaceStepName(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].Name = "   "
	err := ValidateTemplate(tmpl)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
}

func TestValidateTemplate_InvalidStepType(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].Type = "invalid_type"
	err := ValidateTemplate(tmpl)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
	assert.Contains(t, err.Error(), "invalid type")
	assert.Contains(t, err.Error(), "must be one of")
}

func TestValidateTemplate_NegativeTimeout(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].Timeout = -1 * time.Minute
	err := ValidateTemplate(tmpl)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
	assert.Contains(t, err.Error(), "timeout cannot be negative")
}

func TestValidateTemplate_NegativeRetryCount(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].RetryCount = -1
	err := ValidateTemplate(tmpl)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
	assert.Contains(t, err.Error(), "retry_count cannot be negative")
}

func TestValidateTemplate_ZeroTimeoutAllowed(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].Timeout = 0
	err := ValidateTemplate(tmpl)
	assert.NoError(t, err)
}

func TestValidateTemplate_ZeroRetryCountAllowed(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps[0].RetryCount = 0
	err := ValidateTemplate(tmpl)
	assert.NoError(t, err)
}

func TestValidateTemplate_MultipleStepsOneInvalid(t *testing.T) {
	tmpl := validTemplate()
	tmpl.Steps = append(tmpl.Steps, domain.StepDefinition{
		Name:       "step2",
		Type:       domain.StepTypeValidation,
		RetryCount: -5, // Invalid
	})
	err := ValidateTemplate(tmpl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 1")
	assert.Contains(t, err.Error(), "step2")
}

func TestValidateStep_AllValidTypes(t *testing.T) {
	validTypes := []domain.StepType{
		domain.StepTypeAI,
		domain.StepTypeValidation,
		domain.StepTypeGit,
		domain.StepTypeHuman,
		domain.StepTypeSDD,
		domain.StepTypeCI,
		domain.StepTypeVerify,
	}

	for _, stepType := range validTypes {
		t.Run(string(stepType), func(t *testing.T) {
			tmpl := validTemplate()
			tmpl.Steps[0].Type = stepType
			err := ValidateTemplate(tmpl)
			assert.NoError(t, err, "step type %q should be valid", stepType)
		})
	}
}

func TestIsValidStepType_Valid(t *testing.T) {
	tests := []struct {
		stepType domain.StepType
		want     bool
	}{
		{domain.StepTypeAI, true},
		{domain.StepTypeValidation, true},
		{domain.StepTypeGit, true},
		{domain.StepTypeHuman, true},
		{domain.StepTypeSDD, true},
		{domain.StepTypeCI, true},
		{domain.StepTypeVerify, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.stepType), func(t *testing.T) {
			got := IsValidStepType(tt.stepType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidStepType_Invalid(t *testing.T) {
	invalidTypes := []domain.StepType{
		"invalid",
		"AI",    // Case-sensitive check
		"HUMAN", // Case-sensitive check
		"unknown",
		"",
	}

	for _, stepType := range invalidTypes {
		t.Run(string(stepType), func(t *testing.T) {
			got := IsValidStepType(stepType)
			assert.False(t, got, "step type %q should be invalid", stepType)
		})
	}
}

func TestParseStepType_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  domain.StepType
	}{
		{"ai", domain.StepTypeAI},
		{"AI", domain.StepTypeAI},
		{"Ai", domain.StepTypeAI},
		{"validation", domain.StepTypeValidation},
		{"VALIDATION", domain.StepTypeValidation},
		{"git", domain.StepTypeGit},
		{"GIT", domain.StepTypeGit},
		{"human", domain.StepTypeHuman},
		{"HUMAN", domain.StepTypeHuman},
		{"sdd", domain.StepTypeSDD},
		{"SDD", domain.StepTypeSDD},
		{"ci", domain.StepTypeCI},
		{"CI", domain.StepTypeCI},
		{"verify", domain.StepTypeVerify},
		{"VERIFY", domain.StepTypeVerify},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseStepType(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseStepType_WithWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  domain.StepType
	}{
		{"  ai  ", domain.StepTypeAI},
		{"\tgit\n", domain.StepTypeGit},
		{" validation ", domain.StepTypeValidation},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseStepType(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseStepType_Invalid(t *testing.T) {
	invalidInputs := []string{
		"invalid",
		"unknown",
		"magic",
		"",
		"   ",
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			_, err := ParseStepType(input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "is not valid")
			assert.Contains(t, err.Error(), "must be one of")
		})
	}
}

func TestValidStepTypes_ContainsAllTypes(t *testing.T) {
	// Verify ValidStepTypes matches the domain constants
	expectedTypes := map[domain.StepType]bool{
		domain.StepTypeAI:         true,
		domain.StepTypeValidation: true,
		domain.StepTypeGit:        true,
		domain.StepTypeHuman:      true,
		domain.StepTypeSDD:        true,
		domain.StepTypeCI:         true,
		domain.StepTypeVerify:     true,
	}

	assert.Len(t, ValidStepTypes(), len(expectedTypes), "ValidStepTypes should have all step types")

	for _, stepType := range ValidStepTypes() {
		assert.True(t, expectedTypes[stepType], "ValidStepTypes contains unexpected type: %s", stepType)
	}
}
