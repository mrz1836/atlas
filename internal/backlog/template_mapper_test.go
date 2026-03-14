package backlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapCategoryToTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category Category
		severity Severity
		expected string
	}{
		// Bug category tests
		{
			name:     "bug with low severity maps to bug",
			category: CategoryBug,
			severity: SeverityLow,
			expected: "bug",
		},
		{
			name:     "bug with medium severity maps to bug",
			category: CategoryBug,
			severity: SeverityMedium,
			expected: "bug",
		},
		{
			name:     "bug with high severity maps to bug",
			category: CategoryBug,
			severity: SeverityHigh,
			expected: "bug",
		},
		{
			name:     "bug with critical severity maps to bug",
			category: CategoryBug,
			severity: SeverityCritical,
			expected: "bug",
		},

		// Security category tests
		{
			name:     "security with low severity maps to bug",
			category: CategorySecurity,
			severity: SeverityLow,
			expected: "bug",
		},
		{
			name:     "security with medium severity maps to bug",
			category: CategorySecurity,
			severity: SeverityMedium,
			expected: "bug",
		},
		{
			name:     "security with high severity maps to bug",
			category: CategorySecurity,
			severity: SeverityHigh,
			expected: "bug",
		},
		{
			name:     "critical security maps to patch",
			category: CategorySecurity,
			severity: SeverityCritical,
			expected: "patch",
		},

		// Performance category tests
		{
			name:     "performance with low severity maps to task",
			category: CategoryPerformance,
			severity: SeverityLow,
			expected: "task",
		},
		{
			name:     "performance with critical severity maps to task",
			category: CategoryPerformance,
			severity: SeverityCritical,
			expected: "task",
		},

		// Maintainability category tests
		{
			name:     "maintainability with low severity maps to task",
			category: CategoryMaintainability,
			severity: SeverityLow,
			expected: "task",
		},
		{
			name:     "maintainability with high severity maps to task",
			category: CategoryMaintainability,
			severity: SeverityHigh,
			expected: "task",
		},

		// Testing category tests
		{
			name:     "testing with low severity maps to task",
			category: CategoryTesting,
			severity: SeverityLow,
			expected: "task",
		},
		{
			name:     "testing with critical severity maps to task",
			category: CategoryTesting,
			severity: SeverityCritical,
			expected: "task",
		},

		// Documentation category tests
		{
			name:     "documentation with low severity maps to task",
			category: CategoryDocumentation,
			severity: SeverityLow,
			expected: "task",
		},
		{
			name:     "documentation with high severity maps to task",
			category: CategoryDocumentation,
			severity: SeverityHigh,
			expected: "task",
		},

		// Unknown/invalid category tests
		{
			name:     "unknown category maps to default task",
			category: Category("unknown"),
			severity: SeverityMedium,
			expected: "task",
		},
		{
			name:     "empty category maps to default task",
			category: Category(""),
			severity: SeverityHigh,
			expected: "task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := MapCategoryToTemplate(tc.category, tc.severity)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMapCategoryToTemplateWithMapping_CustomMapping(t *testing.T) {
	t.Parallel()

	t.Run("custom mapping overrides defaults", func(t *testing.T) {
		t.Parallel()
		customMapping := &TemplateMapping{
			CategoryMappings: map[Category]string{
				CategoryBug:         "custom-bug",
				CategoryPerformance: "custom-perf",
			},
			CriticalSecurityTemplate: "custom-patch",
			DefaultTemplate:          "custom-task",
		}

		tests := []struct {
			category Category
			severity Severity
			expected string
		}{
			{CategoryBug, SeverityLow, "custom-bug"},
			{CategoryPerformance, SeverityHigh, "custom-perf"},
			{CategorySecurity, SeverityCritical, "custom-patch"},
			{CategoryDocumentation, SeverityLow, "custom-task"},
		}

		for _, tc := range tests {
			result := MapCategoryToTemplateWithMapping(tc.category, tc.severity, customMapping)
			assert.Equal(t, tc.expected, result)
		}
	})

	t.Run("nil mapping uses defaults", func(t *testing.T) {
		t.Parallel()
		result := MapCategoryToTemplateWithMapping(CategoryBug, SeverityHigh, nil)
		assert.Equal(t, "bug", result)
	})

	t.Run("empty critical security template falls back to category mapping", func(t *testing.T) {
		t.Parallel()
		customMapping := &TemplateMapping{
			CategoryMappings: map[Category]string{
				CategorySecurity: "security-fix",
			},
			CriticalSecurityTemplate: "", // Empty
			DefaultTemplate:          "task",
		}

		result := MapCategoryToTemplateWithMapping(CategorySecurity, SeverityCritical, customMapping)
		assert.Equal(t, "security-fix", result)
	})

	t.Run("missing category mapping uses default", func(t *testing.T) {
		t.Parallel()
		customMapping := &TemplateMapping{
			CategoryMappings:         map[Category]string{}, // Empty mappings
			CriticalSecurityTemplate: "hotfix",
			DefaultTemplate:          "fallback-task",
		}

		result := MapCategoryToTemplateWithMapping(CategoryBug, SeverityHigh, customMapping)
		assert.Equal(t, "fallback-task", result)
	})
}

func TestDefaultTemplateMapping(t *testing.T) {
	t.Parallel()

	mapping := DefaultTemplateMapping()

	t.Run("has all categories mapped", func(t *testing.T) {
		t.Parallel()
		expectedCategories := ValidCategories()
		for _, cat := range expectedCategories {
			_, ok := mapping.CategoryMappings[cat]
			assert.True(t, ok, "category %s should have a mapping", cat)
		}
	})

	t.Run("has critical security template set", func(t *testing.T) {
		t.Parallel()
		assert.NotEmpty(t, mapping.CriticalSecurityTemplate)
		assert.Equal(t, "patch", mapping.CriticalSecurityTemplate)
	})

	t.Run("has default template set", func(t *testing.T) {
		t.Parallel()
		assert.NotEmpty(t, mapping.DefaultTemplate)
		assert.Equal(t, "task", mapping.DefaultTemplate)
	})
}

func TestValidTemplateNames(t *testing.T) {
	t.Parallel()

	names := ValidTemplateNames()

	t.Run("contains expected templates", func(t *testing.T) {
		t.Parallel()
		expected := []string{"bug", "patch", "feature", "task", "commit", "fix", "bugfix", "hotfix"}
		assert.ElementsMatch(t, expected, names)
	})

	t.Run("is not empty", func(t *testing.T) {
		t.Parallel()
		assert.NotEmpty(t, names)
	})
}

func TestIsValidTemplateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid templates (primary)
		{"bug is valid", "bug", true},
		{"patch is valid", "patch", true},
		{"feature is valid", "feature", true},
		{"task is valid", "task", true},
		{"commit is valid", "commit", true},

		// Valid templates (aliases for backward compatibility)
		{"fix is valid (alias)", "fix", true},
		{"bugfix is valid (alias)", "bugfix", true},
		{"hotfix is valid (alias)", "hotfix", true},

		// Invalid templates
		{"empty string is invalid", "", false},
		{"unknown is invalid", "unknown", false},
		{"typo is invalid", "bugfi", false},
		{"case sensitive is invalid", "Bug", false},
		{"uppercase is invalid", "BUG", false},
		{"with spaces is invalid", "bug fix", false},
		{"special chars is invalid", "bug-fix", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := IsValidTemplateName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMappingConsistency(t *testing.T) {
	t.Parallel()

	// Ensure all mapped templates are valid template names
	t.Run("all mapped templates are valid", func(t *testing.T) {
		t.Parallel()
		mapping := DefaultTemplateMapping()

		for cat, tmpl := range mapping.CategoryMappings {
			assert.True(t, IsValidTemplateName(tmpl),
				"category %s maps to invalid template %s", cat, tmpl)
		}

		if mapping.CriticalSecurityTemplate != "" {
			assert.True(t, IsValidTemplateName(mapping.CriticalSecurityTemplate),
				"critical security template %s is invalid", mapping.CriticalSecurityTemplate)
		}

		assert.True(t, IsValidTemplateName(mapping.DefaultTemplate),
			"default template %s is invalid", mapping.DefaultTemplate)
	})
}

func TestAllCategorySeverityCombinations(t *testing.T) {
	t.Parallel()

	// Comprehensive test of all category/severity combinations
	categories := ValidCategories()
	severities := ValidSeverities()

	for _, cat := range categories {
		for _, sev := range severities {
			t.Run(string(cat)+"_"+string(sev), func(t *testing.T) {
				t.Parallel()
				result := MapCategoryToTemplate(cat, sev)

				// Result should always be non-empty
				assert.NotEmpty(t, result, "mapping should return non-empty template")

				// Result should always be a valid template
				assert.True(t, IsValidTemplateName(result),
					"mapping should return valid template, got %s", result)
			})
		}
	}
}
