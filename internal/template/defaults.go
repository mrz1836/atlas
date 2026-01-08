package template

import (
	"fmt"

	"github.com/mrz1836/atlas/internal/domain"
)

// mustRegister registers a template and panics on error.
// This is used during registry initialization where registration
// errors indicate programming bugs (duplicate names, nil templates).
func mustRegister(r *Registry, t *domain.Template) {
	if err := r.Register(t); err != nil {
		panic(fmt.Sprintf("failed to register %s template: %v", t.Name, err))
	}
}

// NewDefaultRegistry creates a registry with all built-in templates.
// Templates are compiled into the binary (not external files).
// Panics if any template registration fails (indicates programming error).
func NewDefaultRegistry() *Registry {
	r := NewRegistry()

	// Register built-in templates
	mustRegister(r, NewBugfixTemplate())
	mustRegister(r, NewFeatureTemplate())
	mustRegister(r, NewCommitTemplate())
	mustRegister(r, NewTaskTemplate())
	mustRegister(r, NewFixTemplate())

	return r
}

// NewRegistryWithConfig creates a registry with built-in templates and custom templates from config.
// Custom templates are loaded from file paths specified in the customTemplates map.
// If a custom template has the same name as a built-in, the custom template takes precedence.
//
// basePath is used to resolve relative template paths (typically the project root).
// customTemplates maps template names to their file paths.
//
// Returns an error on the first template loading failure (fail-fast behavior).
func NewRegistryWithConfig(basePath string, customTemplates map[string]string) (*Registry, error) {
	r := NewDefaultRegistry()

	// If no custom templates, return early
	if len(customTemplates) == 0 {
		return r, nil
	}

	// Load custom templates
	loader := NewLoader(basePath)
	customs, err := loader.LoadAll(customTemplates)
	if err != nil {
		return nil, fmt.Errorf("failed to load custom templates: %w", err)
	}

	// Register custom templates using RegisterOrReplace (allows override of built-ins)
	for _, tmpl := range customs {
		if err := r.RegisterOrReplace(tmpl); err != nil {
			return nil, fmt.Errorf("failed to register custom template %q: %w", tmpl.Name, err)
		}
	}

	return r, nil
}
