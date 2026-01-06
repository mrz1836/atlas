package template

import (
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// varPattern matches {{variable}} patterns for template expansion.
// This is a package-level compiled regex for performance (immutable after init).
var varPattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

// VariableExpander handles template variable substitution.
type VariableExpander struct{}

// NewVariableExpander creates a new variable expander.
func NewVariableExpander() *VariableExpander {
	return &VariableExpander{}
}

// Expand substitutes variables in a template with provided values.
// Missing required variables result in an error.
// Optional variables without values use their defaults.
// Returns a cloned template with expanded values.
func (e *VariableExpander) Expand(t *domain.Template, values map[string]string) (*domain.Template, error) {
	if t == nil {
		return nil, atlaserrors.ErrTemplateNil
	}

	// Validate required variables are provided (or have defaults)
	for name, v := range t.Variables {
		if v.Required {
			if _, ok := values[name]; !ok {
				if v.Default == "" {
					return nil, fmt.Errorf("%w: %s", atlaserrors.ErrVariableRequired, name)
				}
			}
		}
	}

	// Merge provided values with defaults
	merged := make(map[string]string)
	for name, v := range t.Variables {
		if v.Default != "" {
			merged[name] = v.Default
		}
	}
	maps.Copy(merged, values)

	// Clone template before modifying
	result := cloneTemplate(t)

	// Expand in description
	result.Description = expandString(result.Description, merged)

	// Expand in step descriptions and configs
	for i := range result.Steps {
		result.Steps[i].Description = expandString(result.Steps[i].Description, merged)
		result.Steps[i].Config = expandConfig(result.Steps[i].Config, merged)
	}

	return result, nil
}

// expandString replaces {{variable}} patterns with values from the map.
// Unmatched patterns are left as-is.
func expandString(s string, values map[string]string) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.Trim(match, "{}")
		if val, ok := values[name]; ok {
			return val
		}
		return match // Leave unexpanded if not found
	})
}

// expandConfig recursively expands string values in a config map.
func expandConfig(cfg map[string]any, values map[string]string) map[string]any {
	if cfg == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range cfg {
		switch val := v.(type) {
		case string:
			result[k] = expandString(val, values)
		case map[string]any:
			result[k] = expandConfig(val, values)
		default:
			result[k] = v
		}
	}
	return result
}

// cloneTemplate creates a deep copy of a template.
func cloneTemplate(t *domain.Template) *domain.Template {
	result := &domain.Template{
		Name:         t.Name,
		Description:  t.Description,
		BranchPrefix: t.BranchPrefix,
		DefaultAgent: t.DefaultAgent,
		DefaultModel: t.DefaultModel,
		Verify:       t.Verify,
		VerifyModel:  t.VerifyModel,
	}

	// Clone validation commands
	if len(t.ValidationCommands) > 0 {
		result.ValidationCommands = make([]string, len(t.ValidationCommands))
		copy(result.ValidationCommands, t.ValidationCommands)
	}

	// Clone steps
	if len(t.Steps) > 0 {
		result.Steps = make([]domain.StepDefinition, len(t.Steps))
		for i, s := range t.Steps {
			result.Steps[i] = domain.StepDefinition{
				Name:        s.Name,
				Type:        s.Type,
				Description: s.Description,
				Required:    s.Required,
				Timeout:     s.Timeout,
				RetryCount:  s.RetryCount,
			}
			if s.Config != nil {
				result.Steps[i].Config = cloneConfig(s.Config)
			}
		}
	}

	// Clone variables
	if t.Variables != nil {
		result.Variables = make(map[string]domain.TemplateVariable)
		maps.Copy(result.Variables, t.Variables)
	}

	return result
}

// cloneConfig creates a shallow copy of a config map.
// Nested maps and slices are shared references, not deep cloned.
// This is acceptable because expandConfig creates new maps during expansion,
// and templates should be treated as immutable after registration.
func cloneConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	result := make(map[string]any)
	maps.Copy(result, cfg)
	return result
}
