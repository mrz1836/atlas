package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestApplyConfig_NilTemplate(t *testing.T) {
	cfg := &config.Config{}
	result := ApplyConfig(nil, cfg)
	assert.Nil(t, result)
}

func TestApplyConfig_NilConfig(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
	}

	result := ApplyConfig(tmpl, nil)
	require.NotNil(t, result)
	assert.Equal(t, "sonnet", result.DefaultModel)
}

func TestApplyConfig_ModelOverride(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
	}
	cfg := &config.Config{
		AI: config.AIConfig{
			Model: "opus",
		},
	}

	result := ApplyConfig(tmpl, cfg)
	require.NotNil(t, result)
	assert.Equal(t, "opus", result.DefaultModel)
}

func TestApplyConfig_EmptyModelKeepsDefault(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
	}
	cfg := &config.Config{
		AI: config.AIConfig{
			Model: "",
		},
	}

	result := ApplyConfig(tmpl, cfg)
	require.NotNil(t, result)
	assert.Equal(t, "sonnet", result.DefaultModel)
}

func TestApplyConfig_OriginalUnmodified(t *testing.T) {
	original := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
	}
	cfg := &config.Config{
		AI: config.AIConfig{
			Model: "opus",
		},
	}

	_ = ApplyConfig(original, cfg)
	assert.Equal(t, "sonnet", original.DefaultModel)
}

func TestApplyOverrides_NilTemplate(t *testing.T) {
	overrides := Overrides{Model: "opus"}
	result := ApplyOverrides(nil, overrides)
	assert.Nil(t, result)
}

func TestApplyOverrides_ModelOverride(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
	}
	overrides := Overrides{
		Model: "opus",
	}

	result := ApplyOverrides(tmpl, overrides)
	require.NotNil(t, result)
	assert.Equal(t, "opus", result.DefaultModel)
}

func TestApplyOverrides_BranchPrefixOverride(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		BranchPrefix: "feat/",
	}
	overrides := Overrides{
		BranchPrefix: "feature/",
	}

	result := ApplyOverrides(tmpl, overrides)
	require.NotNil(t, result)
	assert.Equal(t, "feature/", result.BranchPrefix)
}

func TestApplyOverrides_AutoProceedGit(t *testing.T) {
	tmpl := &domain.Template{
		Name: "test",
		Steps: []domain.StepDefinition{
			{
				Name: "git_commit",
				Type: domain.StepTypeGit,
			},
			{
				Name: "validate",
				Type: domain.StepTypeValidation,
			},
			{
				Name:   "git_push",
				Type:   domain.StepTypeGit,
				Config: map[string]any{"operation": "push"},
			},
		},
	}
	overrides := Overrides{
		AutoProceedGit: true,
	}

	result := ApplyOverrides(tmpl, overrides)
	require.NotNil(t, result)

	// Git steps should have auto_proceed set
	assert.Equal(t, true, result.Steps[0].Config["auto_proceed"])
	assert.Equal(t, true, result.Steps[2].Config["auto_proceed"])

	// Non-git steps should be unchanged
	assert.Nil(t, result.Steps[1].Config)
}

func TestApplyOverrides_EmptyOverridesKeepsDefaults(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
		BranchPrefix: "fix/",
	}
	overrides := Overrides{}

	result := ApplyOverrides(tmpl, overrides)
	require.NotNil(t, result)
	assert.Equal(t, "sonnet", result.DefaultModel)
	assert.Equal(t, "fix/", result.BranchPrefix)
}

func TestApplyOverrides_MultipleOverrides(t *testing.T) {
	tmpl := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
		BranchPrefix: "fix/",
	}
	overrides := Overrides{
		Model:        "opus",
		BranchPrefix: "hotfix/",
	}

	result := ApplyOverrides(tmpl, overrides)
	require.NotNil(t, result)
	assert.Equal(t, "opus", result.DefaultModel)
	assert.Equal(t, "hotfix/", result.BranchPrefix)
}

func TestApplyOverrides_OriginalUnmodified(t *testing.T) {
	original := &domain.Template{
		Name:         "test",
		DefaultModel: "sonnet",
		BranchPrefix: "fix/",
		Steps: []domain.StepDefinition{
			{
				Name: "git_commit",
				Type: domain.StepTypeGit,
			},
		},
	}
	overrides := Overrides{
		Model:          "opus",
		BranchPrefix:   "hotfix/",
		AutoProceedGit: true,
	}

	_ = ApplyOverrides(original, overrides)

	assert.Equal(t, "sonnet", original.DefaultModel)
	assert.Equal(t, "fix/", original.BranchPrefix)
	assert.Nil(t, original.Steps[0].Config)
}

func TestWithConfig_Success(t *testing.T) {
	r := NewDefaultRegistry()
	cfg := &config.Config{
		AI: config.AIConfig{
			Model: "haiku",
		},
	}

	tmpl, err := WithConfig(r, "bugfix", cfg)
	require.NoError(t, err)
	assert.Equal(t, "bugfix", tmpl.Name)
	assert.Equal(t, "haiku", tmpl.DefaultModel)
}

func TestWithConfig_NotFound(t *testing.T) {
	r := NewDefaultRegistry()
	cfg := &config.Config{}

	_, err := WithConfig(r, "nonexistent", cfg)
	require.Error(t, err)
}

func TestWithConfig_NilConfig(t *testing.T) {
	r := NewDefaultRegistry()

	tmpl, err := WithConfig(r, "bugfix", nil)
	require.NoError(t, err)
	assert.Equal(t, "bugfix", tmpl.Name)
	assert.Equal(t, "sonnet", tmpl.DefaultModel) // Original default
}
