package backlog

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateTaskDescription(t *testing.T) {
	t.Parallel()

	t.Run("generates full description with all fields", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title:  "Missing error handling in payment processor",
			Status: StatusPending,
			Content: Content{
				Description: "The API endpoint doesn't handle network failures properly",
				Category:    CategoryBug,
				Severity:    SeverityHigh,
				Tags:        []string{"error-handling", "network"},
			},
			Location: &Location{
				File: "cmd/api.go",
				Line: 47,
			},
			Context: Context{
				DiscoveredAt: time.Now(),
				DiscoveredBy: "ai:claude:sonnet",
			},
		}

		result := GenerateTaskDescription(d)

		// Check title with severity badge
		assert.Contains(t, result, "Missing error handling in payment processor")
		assert.Contains(t, result, "[HIGH]")

		// Check category
		assert.Contains(t, result, "Category: bug")

		// Check description
		assert.Contains(t, result, "The API endpoint doesn't handle network failures properly")

		// Check location
		assert.Contains(t, result, "Location: cmd/api.go:47")

		// Check tags
		assert.Contains(t, result, "Tags: error-handling, network")
	})

	t.Run("handles discovery without optional fields", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title:  "Simple issue",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
				// No description, no tags
			},
			// No location
		}

		result := GenerateTaskDescription(d)

		assert.Contains(t, result, "Simple issue")
		assert.Contains(t, result, "[LOW]")
		assert.NotContains(t, result, "Location:")
		assert.NotContains(t, result, "Tags:")
	})

	t.Run("handles location without line number", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "File-level issue",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
			Location: &Location{
				File: "main.go",
				// No line number
			},
		}

		result := GenerateTaskDescription(d)

		assert.Contains(t, result, "Location: main.go")
		assert.NotContains(t, result, "main.go:")
	})

	t.Run("handles empty title", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
		}

		result := GenerateTaskDescription(d)

		// Should still include severity badge
		assert.Contains(t, result, "[HIGH]")
	})
}

func TestGenerateTaskDescriptionWithConfig(t *testing.T) {
	t.Parallel()

	baseDiscovery := &Discovery{
		Title:  "Test issue",
		Status: StatusPending,
		Content: Content{
			Description: "Detailed description here",
			Category:    CategoryBug,
			Severity:    SeverityCritical,
			Tags:        []string{"tag1", "tag2"},
		},
		Location: &Location{
			File: "test.go",
			Line: 100,
		},
	}

	t.Run("nil config uses defaults", func(t *testing.T) {
		t.Parallel()
		result := GenerateTaskDescriptionWithConfig(baseDiscovery, nil)

		// Default includes everything
		assert.Contains(t, result, "[CRITICAL]")
		assert.Contains(t, result, "Category:")
		assert.Contains(t, result, "Location:")
		assert.Contains(t, result, "Tags:")
	})

	t.Run("excludes severity badge when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := &DescriptionConfig{
			IncludeSeverityBadge: false,
			IncludeDescription:   true,
			IncludeLocation:      true,
			IncludeTags:          true,
			IncludeCategory:      true,
		}

		result := GenerateTaskDescriptionWithConfig(baseDiscovery, cfg)

		assert.Contains(t, result, "Test issue")
		assert.NotContains(t, result, "[CRITICAL]")
	})

	t.Run("excludes description when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := &DescriptionConfig{
			IncludeSeverityBadge: true,
			IncludeDescription:   false,
			IncludeLocation:      true,
			IncludeTags:          true,
			IncludeCategory:      true,
		}

		result := GenerateTaskDescriptionWithConfig(baseDiscovery, cfg)

		assert.NotContains(t, result, "Detailed description here")
	})

	t.Run("excludes location when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := &DescriptionConfig{
			IncludeSeverityBadge: true,
			IncludeDescription:   true,
			IncludeLocation:      false,
			IncludeTags:          true,
			IncludeCategory:      true,
		}

		result := GenerateTaskDescriptionWithConfig(baseDiscovery, cfg)

		assert.NotContains(t, result, "Location:")
	})

	t.Run("excludes tags when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := &DescriptionConfig{
			IncludeSeverityBadge: true,
			IncludeDescription:   true,
			IncludeLocation:      true,
			IncludeTags:          false,
			IncludeCategory:      true,
		}

		result := GenerateTaskDescriptionWithConfig(baseDiscovery, cfg)

		assert.NotContains(t, result, "Tags:")
	})

	t.Run("excludes category when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := &DescriptionConfig{
			IncludeSeverityBadge: true,
			IncludeDescription:   true,
			IncludeLocation:      true,
			IncludeTags:          true,
			IncludeCategory:      false,
		}

		result := GenerateTaskDescriptionWithConfig(baseDiscovery, cfg)

		assert.NotContains(t, result, "Category:")
	})

	t.Run("truncates long description", func(t *testing.T) {
		t.Parallel()
		longDesc := strings.Repeat("a", 200)
		d := &Discovery{
			Title: "Issue",
			Content: Content{
				Description: longDesc,
				Category:    CategoryBug,
				Severity:    SeverityLow,
			},
		}

		cfg := &DescriptionConfig{
			IncludeSeverityBadge: false,
			IncludeDescription:   true,
			MaxDescriptionLength: 50,
		}

		result := GenerateTaskDescriptionWithConfig(d, cfg)

		// Description should be truncated to 50 chars + "..."
		assert.Contains(t, result, strings.Repeat("a", 50)+"...")
		assert.NotContains(t, result, strings.Repeat("a", 51))
	})

	t.Run("minimal config produces minimal output", func(t *testing.T) {
		t.Parallel()
		cfg := &DescriptionConfig{
			IncludeSeverityBadge: false,
			IncludeDescription:   false,
			IncludeLocation:      false,
			IncludeTags:          false,
			IncludeCategory:      false,
		}

		result := GenerateTaskDescriptionWithConfig(baseDiscovery, cfg)

		// Should only contain the title
		assert.Equal(t, "Test issue", result)
	})
}

func TestDefaultDescriptionConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultDescriptionConfig()

	t.Run("all options enabled by default", func(t *testing.T) {
		t.Parallel()
		assert.True(t, cfg.IncludeSeverityBadge)
		assert.True(t, cfg.IncludeDescription)
		assert.True(t, cfg.IncludeLocation)
		assert.True(t, cfg.IncludeTags)
		assert.True(t, cfg.IncludeCategory)
	})

	t.Run("no length limit by default", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, cfg.MaxDescriptionLength)
	})
}

func TestGenerateWorkspaceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "simple title",
			title:    "fix login bug",
			expected: "fix-login-bug",
		},
		{
			name:     "title with special characters",
			title:    "fix null pointer in parseConfig()",
			expected: "fix-null-pointer-parseconfig", // "in" is a stop word
		},
		{
			name:     "uppercase title",
			title:    "Add User Authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "title with multiple spaces",
			title:    "fix    multiple   spaces",
			expected: "fix-multiple-spaces",
		},
		{
			name:     "title with leading/trailing spaces",
			title:    "  fix bug  ",
			expected: "fix-bug",
		},
		{
			name:     "title with numbers",
			title:    "add feature v2",
			expected: "add-feature-v2",
		},
		{
			name:     "title with hyphens",
			title:    "fix-this-bug",
			expected: "fix-this-bug", // hyphenated words are not split
		},
		{
			name:     "empty title",
			title:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			title:    "!!!@@@###",
			expected: "",
		},
		{
			name:     "removes stop words",
			title:    "I found that the login is broken",
			expected: "login-broken", // "I", "found", "that", "the", "is" are stop words
		},
		{
			name:     "file path with stop words",
			title:    `I found that "atlas / internal / constants / status.go" is missing 100% test coverage`,
			expected: "atlas-internal-constants-statusgo-missin", // stop words removed, truncated to 40 chars
		},
		{
			name:     "removes stop words but keeps meaningful content",
			title:    "Add a new feature to the application",
			expected: "add-new-feature-application", // "a", "to", "the" removed
		},
		{
			name:     "only stop words",
			title:    "I found that it is there",
			expected: "", // all words are stop words
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateWorkspaceName(tc.title)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSanitizeWorkspaceName(t *testing.T) {
	t.Parallel()

	t.Run("truncates long names", func(t *testing.T) {
		t.Parallel()
		longTitle := strings.Repeat("a", 100)
		result := SanitizeWorkspaceName(longTitle)

		assert.LessOrEqual(t, len(result), maxWorkspaceNameLen)
		assert.Len(t, result, maxWorkspaceNameLen)
	})

	t.Run("removes trailing hyphen after truncation", func(t *testing.T) {
		t.Parallel()
		// Create a string that will end with hyphen after truncation
		title := strings.Repeat("word-", 15) // Will be truncated
		result := SanitizeWorkspaceName(title)

		assert.False(t, strings.HasSuffix(result, "-"))
	})

	t.Run("handles emojis", func(t *testing.T) {
		t.Parallel()
		result := SanitizeWorkspaceName("fix 🐛 bug")
		assert.Equal(t, "fix-bug", result)
	})

	t.Run("handles unicode", func(t *testing.T) {
		t.Parallel()
		// Test with non-ASCII characters (Chinese for "fix")
		result := SanitizeWorkspaceName("\u4fee\u590d bug")
		assert.Equal(t, "bug", result)
	})

	t.Run("collapses multiple hyphens", func(t *testing.T) {
		t.Parallel()
		result := SanitizeWorkspaceName("fix--multiple---hyphens")
		assert.NotContains(t, result, "--")
	})

	t.Run("trims leading and trailing hyphens", func(t *testing.T) {
		t.Parallel()
		result := SanitizeWorkspaceName("--fix bug--")
		assert.False(t, strings.HasPrefix(result, "-"))
		assert.False(t, strings.HasSuffix(result, "-"))
	})

	t.Run("filters stop words from verbose titles", func(t *testing.T) {
		t.Parallel()
		result := SanitizeWorkspaceName("I found that there is a bug in the system")
		// "I", "found", "that", "there", "is", "a", "in", "the" are stop words
		assert.Equal(t, "bug-system", result)
	})

	t.Run("preserves file paths while filtering stop words", func(t *testing.T) {
		t.Parallel()
		result := SanitizeWorkspaceName("fix bug in internal/config/parser.go")
		// "in" is a stop word, but file path components are preserved
		assert.Equal(t, "fix-bug-internal-config-parsergo", result)
	})

	t.Run("handles mixed path separators", func(t *testing.T) {
		t.Parallel()
		result := SanitizeWorkspaceName(`add test for src\utils\helper.go`)
		assert.Equal(t, "add-test-src-utils-helpergo", result)
	})
}

func TestRemoveStopWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes common stop words",
			input:    "I found that the bug is there",
			expected: "bug",
		},
		{
			name:     "preserves meaningful words",
			input:    "fix authentication error",
			expected: "fix authentication error",
		},
		{
			name:     "handles path separators",
			input:    "internal / config / parser",
			expected: "internal config parser",
		},
		{
			name:     "handles backslash paths",
			input:    `src\utils\helper`,
			expected: "src utils helper",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "only stop words",
			input:    "I have found that it is",
			expected: "",
		},
		{
			name:     "filters stop words within quoted content",
			input:    `fix "the bug" in parser`,
			expected: `fix bug" parser`, // "the is a stop word, quotes are just punctuation
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := removeStopWords(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFormatLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		location *Location
		expected string
	}{
		{
			name:     "nil location",
			location: nil,
			expected: "",
		},
		{
			name:     "empty file",
			location: &Location{File: "", Line: 10},
			expected: "",
		},
		{
			name:     "file without line",
			location: &Location{File: "main.go", Line: 0},
			expected: "main.go",
		},
		{
			name:     "file with line",
			location: &Location{File: "main.go", Line: 42},
			expected: "main.go:42",
		},
		{
			name:     "path with directories",
			location: &Location{File: "internal/cli/start.go", Line: 100},
			expected: "internal/cli/start.go:100",
		},
		{
			name:     "negative line number is ignored",
			location: &Location{File: "main.go", Line: -1},
			expected: "main.go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := FormatLocation(tc.location)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateBranchName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		prefix        string
		workspaceName string
		expected      string
	}{
		{
			name:          "standard branch name",
			prefix:        "fix",
			workspaceName: "null-pointer-bug",
			expected:      "fix/null-pointer-bug",
		},
		{
			name:          "feature branch",
			prefix:        "feat",
			workspaceName: "add-user-auth",
			expected:      "feat/add-user-auth",
		},
		{
			name:          "patch branch",
			prefix:        "patch",
			workspaceName: "critical-security-fix",
			expected:      "patch/critical-security-fix",
		},
		{
			name:          "empty prefix returns workspace name only",
			prefix:        "",
			workspaceName: "just-workspace",
			expected:      "just-workspace",
		},
		{
			name:          "empty workspace with prefix",
			prefix:        "fix",
			workspaceName: "",
			expected:      "fix/",
		},
		{
			name:          "both empty",
			prefix:        "",
			workspaceName: "",
			expected:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateBranchName(tc.prefix, tc.workspaceName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateTaskDescriptionSeverityLevels(t *testing.T) {
	t.Parallel()

	severities := ValidSeverities()
	for _, sev := range severities {
		t.Run("severity_"+string(sev), func(t *testing.T) {
			t.Parallel()
			d := &Discovery{
				Title: "Test",
				Content: Content{
					Category: CategoryBug,
					Severity: sev,
				},
			}

			result := GenerateTaskDescription(d)

			expected := "[" + strings.ToUpper(string(sev)) + "]"
			assert.Contains(t, result, expected)
		})
	}
}
