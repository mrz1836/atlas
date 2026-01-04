package template

import "fmt"

// NewDefaultRegistry creates a registry with all built-in templates.
// Templates are compiled into the binary (not external files).
func NewDefaultRegistry() *Registry {
	r := NewRegistry()

	// Register built-in templates
	// Errors are ignored as template names are guaranteed unique
	_ = r.Register(NewBugfixTemplate())
	_ = r.Register(NewFeatureTemplate())
	_ = r.Register(NewCommitTemplate())
	_ = r.Register(NewTaskTemplate())

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
