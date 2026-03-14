package template

import "github.com/mrz1836/atlas/internal/domain"

// findStep finds a step by name in a template.
// This helper is used across multiple test files.
func findStep(tmpl *domain.Template, name string) *domain.StepDefinition {
	for i := range tmpl.Steps {
		if tmpl.Steps[i].Name == name {
			return &tmpl.Steps[i]
		}
	}
	return nil
}
