package template

import (
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

// Overrides contains template-specific configuration overrides.
type Overrides struct {
	// Agent overrides the template's default AI agent (claude, gemini).
	Agent domain.Agent

	// Model overrides the template's default AI model.
	Model string

	// BranchPrefix overrides the template's branch prefix.
	BranchPrefix string

	// AutoProceedGit indicates whether git operations should proceed automatically.
	AutoProceedGit bool
}

// ApplyConfig applies configuration overrides to a template.
// Returns a new template with the overrides applied (original is not modified).
func ApplyConfig(t *domain.Template, cfg *config.Config) *domain.Template {
	if t == nil {
		return nil
	}

	// Clone the template to avoid modifying the original
	result := cloneTemplate(t)

	if cfg == nil {
		return result
	}

	// Apply AI agent override from config
	if cfg.AI.Agent != "" {
		result.DefaultAgent = domain.Agent(cfg.AI.Agent)
	}

	// Apply AI model override from config
	if cfg.AI.Model != "" {
		result.DefaultModel = cfg.AI.Model
	}

	return result
}

// ApplyOverrides applies explicit overrides to a template.
// This is useful when CLI flags or other sources provide overrides.
// Returns a new template with the overrides applied (original is not modified).
func ApplyOverrides(t *domain.Template, overrides Overrides) *domain.Template {
	if t == nil {
		return nil
	}

	// Clone the template to avoid modifying the original
	result := cloneTemplate(t)

	// Apply agent override
	if overrides.Agent != "" {
		result.DefaultAgent = overrides.Agent
	}

	// Apply model override
	if overrides.Model != "" {
		result.DefaultModel = overrides.Model
	}

	// Apply branch prefix override
	if overrides.BranchPrefix != "" {
		result.BranchPrefix = overrides.BranchPrefix
	}

	// Store auto_proceed_git in step configs that need it
	if overrides.AutoProceedGit {
		for i := range result.Steps {
			if result.Steps[i].Type == domain.StepTypeGit {
				if result.Steps[i].Config == nil {
					result.Steps[i].Config = make(map[string]any)
				}
				result.Steps[i].Config["auto_proceed"] = true
			}
		}
	}

	return result
}

// WithConfig retrieves a template from the registry and applies config overrides.
// This is the main entry point for getting a ready-to-use template.
func WithConfig(r *Registry, name string, cfg *config.Config) (*domain.Template, error) {
	tmpl, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	return ApplyConfig(tmpl, cfg), nil
}
