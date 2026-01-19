package backlog

// TemplateMapping defines the mapping from discovery category/severity to task template.
// This struct allows customization of the mapping logic.
type TemplateMapping struct {
	// CategoryMappings maps category names to template names.
	// Used when severity doesn't require special handling.
	CategoryMappings map[Category]string

	// CriticalSecurityTemplate is used for critical security issues.
	// This takes precedence over CategoryMappings for critical severity security issues.
	CriticalSecurityTemplate string

	// DefaultTemplate is used when no specific mapping exists.
	DefaultTemplate string
}

// DefaultTemplateMapping returns the default category → template mapping configuration.
func DefaultTemplateMapping() *TemplateMapping {
	return &TemplateMapping{
		CategoryMappings: map[Category]string{
			CategoryBug:             "bugfix",
			CategorySecurity:        "bugfix", // non-critical security
			CategoryPerformance:     "task",
			CategoryMaintainability: "task",
			CategoryTesting:         "task",
			CategoryDocumentation:   "task",
		},
		CriticalSecurityTemplate: "hotfix",
		DefaultTemplate:          "task",
	}
}

// MapCategoryToTemplate returns the best template for a discovery based on its category and severity.
// The mapping follows these rules:
//  1. Critical security issues → hotfix (immediate attention required)
//  2. Bug/Security → bugfix (code fixes)
//  3. Other categories → task (general work)
//
// The mapping can be customized by providing a custom TemplateMapping.
func MapCategoryToTemplate(category Category, severity Severity) string {
	return MapCategoryToTemplateWithMapping(category, severity, nil)
}

// MapCategoryToTemplateWithMapping returns the template name using a custom mapping.
// If mapping is nil, the default mapping is used.
func MapCategoryToTemplateWithMapping(category Category, severity Severity, mapping *TemplateMapping) string {
	if mapping == nil {
		mapping = DefaultTemplateMapping()
	}

	// Special case: Critical security issues get hotfix
	if category == CategorySecurity && severity == SeverityCritical {
		if mapping.CriticalSecurityTemplate != "" {
			return mapping.CriticalSecurityTemplate
		}
	}

	// Look up in category mappings
	if template, ok := mapping.CategoryMappings[category]; ok {
		return template
	}

	// Fall back to default
	return mapping.DefaultTemplate
}

// ValidTemplateNames returns all valid template names that can be used.
// This is useful for validation and help text.
func ValidTemplateNames() []string {
	return []string{"bugfix", "feature", "task", "fix", "hotfix", "commit"}
}

// IsValidTemplateName checks if a template name is valid.
func IsValidTemplateName(name string) bool {
	for _, valid := range ValidTemplateNames() {
		if name == valid {
			return true
		}
	}
	return false
}
