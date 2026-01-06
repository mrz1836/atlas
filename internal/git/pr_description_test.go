// Package git provides Git operations for ATLAS.
package git

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// prMockAIRunner is a test double for ai.Runner used in PR description tests.
type prMockAIRunner struct {
	runFunc func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}

func (m *prMockAIRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, req)
	}
	return &domain.AIResult{}, nil
}

func TestPRDescription_Validate(t *testing.T) {
	tests := []struct {
		name    string
		desc    *PRDescription
		wantErr error
	}{
		{
			name: "valid description",
			desc: &PRDescription{
				Title: "fix(config): handle nil options",
				Body:  "## Summary\nFix null pointer.\n\n## Changes\n- file.go\n\n## Test Plan\nTests pass.",
			},
			wantErr: nil,
		},
		{
			name: "empty title",
			desc: &PRDescription{
				Title: "",
				Body:  "## Summary\nSomething\n\n## Changes\n- file\n\n## Test Plan\nPasses.",
			},
			wantErr: atlaserrors.ErrEmptyValue,
		},
		{
			name: "empty body",
			desc: &PRDescription{
				Title: "feat: add feature",
				Body:  "",
			},
			wantErr: atlaserrors.ErrEmptyValue,
		},
		{
			name: "invalid title format",
			desc: &PRDescription{
				Title: "just a plain title",
				Body:  "## Summary\nSomething\n\n## Changes\n- file\n\n## Test Plan\nPasses.",
			},
			wantErr: atlaserrors.ErrAIInvalidFormat,
		},
		{
			name: "missing required sections",
			desc: &PRDescription{
				Title: "fix: something",
				Body:  "Just some text without proper sections.",
			},
			wantErr: atlaserrors.ErrAIInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.desc.Validate()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidConventionalTitle(t *testing.T) {
	validTitles := []string{
		"feat: add new feature",
		"fix(config): handle nil",
		"docs: update README",
		"style: format code",
		"refactor(api): restructure handlers",
		"test: add unit tests",
		"chore: update deps",
		"build: update makefile",
		"ci: add workflow",
		"perf: optimize query",
		"revert: undo change",
	}

	invalidTitles := []string{
		"just a plain title",
		"Feature: something",  // Wrong case
		"fix - something",     // Wrong separator
		"feat():empty scope:", // Empty scope
		"feat",                // No description
		"feat:",               // No description
	}

	for _, title := range validTitles {
		t.Run("valid: "+title, func(t *testing.T) {
			assert.True(t, isValidConventionalTitle(title))
		})
	}

	for _, title := range invalidTitles {
		t.Run("invalid: "+title, func(t *testing.T) {
			assert.False(t, isValidConventionalTitle(title))
		})
	}
}

func TestHasRequiredSections(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "all sections present",
			body:     "## Summary\nSome summary.\n\n## Changes\n- file.go\n\n## Test Plan\nTests pass.",
			expected: true,
		},
		{
			name:     "case insensitive",
			body:     "## summary\nSome summary.\n\n## changes\n- file.go\n\n## test plan\nTests pass.",
			expected: true,
		},
		{
			name:     "missing summary",
			body:     "## Changes\n- file.go\n\n## Test Plan\nTests pass.",
			expected: false,
		},
		{
			name:     "missing changes",
			body:     "## Summary\nSome summary.\n\n## Test Plan\nTests pass.",
			expected: false,
		},
		{
			name:     "missing test plan",
			body:     "## Summary\nSome summary.\n\n## Changes\n- file.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasRequiredSections(tt.body))
		})
	}
}

func TestTemplateDescriptionGenerator_Generate(t *testing.T) {
	t.Run("generates description from task description", func(t *testing.T) {
		gen := NewTemplateDescriptionGenerator()

		desc, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Fix the null pointer in config parsing",
			FilesChanged: []PRFileChange{
				{Path: "internal/config/parser.go", Insertions: 5, Deletions: 2},
			},
			TemplateName:  "bugfix",
			TaskID:        "task-atlas-test-abc",
			WorkspaceName: "fix/null-pointer",
		})

		require.NoError(t, err)
		assert.Contains(t, desc.Title, "fix")
		assert.Contains(t, desc.Body, "## Summary")
		assert.Contains(t, desc.Body, "## Changes")
		assert.Contains(t, desc.Body, "## Test Plan")
		assert.Equal(t, "fix", desc.ConventionalType)
	})

	t.Run("uses commit messages if no description", func(t *testing.T) {
		gen := NewTemplateDescriptionGenerator()

		desc, err := gen.Generate(context.Background(), PRDescOptions{
			CommitMessages: []string{"fix: handle null options in parseConfig"},
			TemplateName:   "bugfix",
		})

		require.NoError(t, err)
		assert.Contains(t, desc.Body, "handle null options in parseConfig")
	})

	t.Run("handles empty files list", func(t *testing.T) {
		gen := NewTemplateDescriptionGenerator()

		desc, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Test task",
			TemplateName:    "feature",
		})

		require.NoError(t, err)
		assert.Contains(t, desc.Body, "No files listed")
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		gen := NewTemplateDescriptionGenerator()

		_, err := gen.Generate(ctx, PRDescOptions{
			TaskDescription: "Test",
			TemplateName:    "feature",
		})

		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestAIDescriptionGenerator_Generate(t *testing.T) {
	t.Run("successful generation", func(t *testing.T) {
		mockRunner := &prMockAIRunner{
			runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{
					Success: true,
					Output: `TITLE: fix(config): handle nil options in parser
BODY:
## Summary

Fixed null pointer exception when parsing configuration with nil options.

## Changes

- internal/config/parser.go - Added nil check

## Test Plan

- Unit tests pass
- Integration tests pass`,
				}, nil
			},
		}

		gen := NewAIDescriptionGenerator(mockRunner,
			WithAIDescTimeout(time.Minute),
		)

		desc, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Fix null pointer in config",
			FilesChanged: []PRFileChange{
				{Path: "internal/config/parser.go", Insertions: 5, Deletions: 2},
			},
			TemplateName:  "bugfix",
			TaskID:        "task-test-abc",
			WorkspaceName: "fix/null",
		})

		require.NoError(t, err)
		assert.Equal(t, "fix(config): handle nil options in parser", desc.Title)
		assert.Contains(t, desc.Body, "## Summary")
		assert.Equal(t, "fix", desc.ConventionalType)
		assert.Equal(t, "config", desc.Scope)
		// Verify hidden metadata is appended
		assert.Contains(t, desc.Body, "<!-- ATLAS_METADATA:")
		assert.Contains(t, desc.Body, `"task_id":"task-test-abc"`)
		assert.Contains(t, desc.Body, `"workspace":"fix/null"`)
	})

	t.Run("empty response fallback to template", func(t *testing.T) {
		mockRunner := &prMockAIRunner{
			runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{
					Success: true,
					Output:  "",
				}, nil
			},
		}

		gen := NewAIDescriptionGenerator(mockRunner)

		_, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Test task",
			TemplateName:    "feature",
		})

		// Should error because empty response
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrAIEmptyResponse)
	})

	t.Run("validation failure", func(t *testing.T) {
		mockRunner := &prMockAIRunner{
			runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return nil, assert.AnError
			},
		}

		gen := NewAIDescriptionGenerator(mockRunner)

		_, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Test",
			TemplateName:    "feature",
		})

		require.Error(t, err)
	})

	t.Run("requires task description or commits", func(t *testing.T) {
		mockRunner := &prMockAIRunner{}
		gen := NewAIDescriptionGenerator(mockRunner)

		_, err := gen.Generate(context.Background(), PRDescOptions{
			TemplateName: "feature",
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mockRunner := &prMockAIRunner{}
		gen := NewAIDescriptionGenerator(mockRunner)

		_, err := gen.Generate(ctx, PRDescOptions{
			TaskDescription: "Test",
			TemplateName:    "feature",
		})

		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("missing TITLE marker falls back to template", func(t *testing.T) {
		mockRunner := &prMockAIRunner{
			runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{
					Success: true,
					Output:  "Just some text without proper markers",
				}, nil
			},
		}

		gen := NewAIDescriptionGenerator(mockRunner)

		// Should fall back to template generator since TITLE: marker is missing
		desc, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Test task description",
			TemplateName:    "feature",
		})

		// Falls back to template, which should succeed
		require.NoError(t, err)
		assert.Contains(t, desc.Title, "feat")
	})

	t.Run("missing BODY marker falls back to template", func(t *testing.T) {
		mockRunner := &prMockAIRunner{
			runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{
					Success: true,
					Output:  "TITLE: feat: add something\nBut no BODY marker",
				}, nil
			},
		}

		gen := NewAIDescriptionGenerator(mockRunner)

		// Should fall back to template generator since BODY: marker is missing
		desc, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Test task",
			TemplateName:    "feature",
		})

		// Falls back to template, which should succeed
		require.NoError(t, err)
		assert.Contains(t, desc.Body, "## Summary")
	})

	t.Run("invalid conventional commits title falls back to template", func(t *testing.T) {
		mockRunner := &prMockAIRunner{
			runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{
					Success: true,
					Output: `TITLE: invalid title without type
BODY:
## Summary
Some content
## Changes
- file.go
## Test Plan
Tests`,
				}, nil
			},
		}

		gen := NewAIDescriptionGenerator(mockRunner)

		// The title doesn't match conventional commits, validation should fail and fall back
		desc, err := gen.Generate(context.Background(), PRDescOptions{
			TaskDescription: "Test task",
			TemplateName:    "feature",
		})

		require.NoError(t, err)
		// Falls back to template generator which produces valid conventional commits
		assert.True(t, isValidConventionalTitle(desc.Title))
	})
}

func TestTypeFromTemplate(t *testing.T) {
	tests := []struct {
		template string
		expected string
	}{
		{"bugfix", "fix"},
		{"bug", "fix"},
		{"hotfix", "fix"},
		{"feature", "feat"},
		{"feat", "feat"},
		{"docs", "docs"},
		{"documentation", "docs"},
		{"refactor", "refactor"},
		{"refactoring", "refactor"},
		{"test", "test"},
		{"testing", "test"},
		{"chore", "chore"},
		{"maintenance", "chore"},
		{"style", "style"},
		{"formatting", "style"},
		{"build", "build"},
		{"ci", "ci"},
		{"perf", "perf"},
		{"performance", "perf"},
		{"unknown", "feat"},
		{"", "feat"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			assert.Equal(t, tt.expected, typeFromTemplate(tt.template))
		})
	}
}

func TestScopeFromFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []PRFileChange
		expected string
	}{
		{
			name:     "empty files",
			files:    []PRFileChange{},
			expected: "",
		},
		{
			name: "single file",
			files: []PRFileChange{
				{Path: "internal/config/parser.go"},
			},
			expected: "config",
		},
		{
			name: "multiple files same dir",
			files: []PRFileChange{
				{Path: "internal/git/runner.go"},
				{Path: "internal/git/runner_test.go"},
			},
			expected: "git",
		},
		{
			name: "most common directory wins",
			files: []PRFileChange{
				{Path: "internal/config/parser.go"},
				{Path: "internal/git/runner.go"},
				{Path: "internal/git/types.go"},
			},
			expected: "git",
		},
		{
			name: "skips src directory",
			files: []PRFileChange{
				{Path: "src/components/button.tsx"},
			},
			expected: "components",
		},
		{
			name: "skips lib directory",
			files: []PRFileChange{
				{Path: "lib/utils/helpers.js"},
			},
			expected: "utils",
		},
		{
			name: "skips test directory",
			files: []PRFileChange{
				{Path: "test/unit/parser_test.go"},
			},
			expected: "unit",
		},
		{
			name: "skips tests directory",
			files: []PRFileChange{
				{Path: "tests/integration/api_test.go"},
			},
			expected: "integration",
		},
		{
			name: "skips vendor directory",
			files: []PRFileChange{
				{Path: "vendor/github.com/pkg/errors/errors.go"},
			},
			expected: "github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, scopeFromFiles(tt.files))
		})
	}
}

func TestFormatPRTitle(t *testing.T) {
	tests := []struct {
		commitType  string
		scope       string
		description string
		expected    string
	}{
		{"fix", "config", "handle nil", "fix(config): handle nil"},
		{"feat", "", "add feature", "feat: add feature"},
		{"docs", "readme", "update docs", "docs(readme): update docs"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatPRTitle(tt.commitType, tt.scope, tt.description)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSummarizeDescription(t *testing.T) {
	tests := []struct {
		name        string
		taskDesc    string
		commits     []string
		minContains string
	}{
		{
			name:        "uses task description",
			taskDesc:    "Fix the null pointer exception",
			commits:     []string{"some commit"},
			minContains: "fix the null pointer",
		},
		{
			name:        "truncates long description",
			taskDesc:    "This is a very long description that exceeds fifty characters and needs truncation",
			commits:     nil,
			minContains: "...",
		},
		{
			name:        "falls back to commit",
			taskDesc:    "",
			commits:     []string{"fix: handle null options"},
			minContains: "handle null options",
		},
		{
			name:        "strips conventional prefix from commit",
			taskDesc:    "",
			commits:     []string{"feat(api): add endpoint"},
			minContains: "add endpoint",
		},
		{
			name:        "defaults when empty",
			taskDesc:    "",
			commits:     nil,
			minContains: "update code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeDescription(tt.taskDesc, tt.commits)
			assert.Contains(t, strings.ToLower(result), strings.ToLower(tt.minContains))
		})
	}
}

func TestSummarizeDescription_PreservesCase(t *testing.T) {
	// Test that acronyms and proper nouns are preserved (only first char lowercased)
	tests := []struct {
		name     string
		taskDesc string
		expected string
	}{
		{
			name:     "preserves API acronym",
			taskDesc: "Update API endpoint",
			expected: "update API endpoint",
		},
		{
			name:     "preserves HTTP acronym",
			taskDesc: "Add HTTP client support",
			expected: "add HTTP client support",
		},
		{
			name:     "preserves mixed case",
			taskDesc: "Fix OAuth2 token refresh",
			expected: "fix OAuth2 token refresh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeDescription(tt.taskDesc, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLowercaseFirst(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello", "hello"},
		{"HELLO", "hELLO"},
		{"API endpoint", "aPI endpoint"},
		{"a", "a"},
		{"A", "a"},
		{"", ""},
		{"123abc", "123abc"},
		{"Über", "über"}, // UTF-8 support
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, lowercaseFirst(tt.input))
		})
	}
}

func TestWriteSummarySection(t *testing.T) {
	t.Run("with task description", func(t *testing.T) {
		var sb strings.Builder
		writeSummarySection(&sb, PRDescOptions{
			TaskDescription: "Fix the bug",
		})
		assert.Contains(t, sb.String(), "Fix the bug")
	})

	t.Run("with commit message fallback", func(t *testing.T) {
		var sb strings.Builder
		writeSummarySection(&sb, PRDescOptions{
			CommitMessages: []string{"First commit"},
		})
		assert.Contains(t, sb.String(), "First commit")
	})

	t.Run("default message", func(t *testing.T) {
		var sb strings.Builder
		writeSummarySection(&sb, PRDescOptions{})
		assert.Contains(t, sb.String(), "No description provided")
	})
}

func TestWriteChangesSection(t *testing.T) {
	t.Run("with files", func(t *testing.T) {
		var sb strings.Builder
		writeChangesSection(&sb, PRDescOptions{
			FilesChanged: []PRFileChange{
				{Path: "file.go", Insertions: 10, Deletions: 5},
			},
		})
		assert.Contains(t, sb.String(), "`file.go`")
		assert.Contains(t, sb.String(), "(+10, -5)")
	})

	t.Run("no stats when zero", func(t *testing.T) {
		var sb strings.Builder
		writeChangesSection(&sb, PRDescOptions{
			FilesChanged: []PRFileChange{
				{Path: "file.go"},
			},
		})
		assert.Contains(t, sb.String(), "`file.go`")
		assert.NotContains(t, sb.String(), "(+0")
	})

	t.Run("empty files", func(t *testing.T) {
		var sb strings.Builder
		writeChangesSection(&sb, PRDescOptions{})
		assert.Contains(t, sb.String(), "No files listed")
	})
}

func TestWriteTestPlanSection(t *testing.T) {
	t.Run("with validation results", func(t *testing.T) {
		var sb strings.Builder
		writeTestPlanSection(&sb, PRDescOptions{
			ValidationResults: "All 42 tests pass",
		})
		assert.Contains(t, sb.String(), "All 42 tests pass")
	})

	t.Run("default checklist", func(t *testing.T) {
		var sb strings.Builder
		writeTestPlanSection(&sb, PRDescOptions{})
		assert.Contains(t, sb.String(), "Tests pass")
		assert.Contains(t, sb.String(), "Lint passes")
	})
}

func TestWriteMetadataSection(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		var sb strings.Builder
		writeMetadataSection(&sb, PRDescOptions{
			TaskID:        "task-abc-xyz",
			TemplateName:  "bugfix",
			WorkspaceName: "fix/test",
		})
		result := sb.String()
		assert.Contains(t, result, "<!-- ATLAS_METADATA:")
		assert.Contains(t, result, "-->")
		assert.Contains(t, result, `"task_id":"task-abc-xyz"`)
		assert.Contains(t, result, `"template":"bugfix"`)
		assert.Contains(t, result, `"workspace":"fix/test"`)
	})

	t.Run("no fields", func(t *testing.T) {
		var sb strings.Builder
		writeMetadataSection(&sb, PRDescOptions{})
		assert.Empty(t, sb.String())
	})

	t.Run("partial fields", func(t *testing.T) {
		var sb strings.Builder
		writeMetadataSection(&sb, PRDescOptions{
			TaskID: "task-only",
		})
		result := sb.String()
		assert.Contains(t, result, "<!-- ATLAS_METADATA:")
		assert.Contains(t, result, `"task_id":"task-only"`)
		assert.NotContains(t, result, `"template"`)
		assert.NotContains(t, result, `"workspace"`)
	})
}

func TestNewAIDescriptionGenerator_Options(t *testing.T) {
	mockRunner := &prMockAIRunner{}
	logger := zerolog.Nop()
	timeout := 5 * time.Minute

	gen := NewAIDescriptionGenerator(mockRunner,
		WithAIDescLogger(logger),
		WithAIDescTimeout(timeout),
		WithAIDescAgent("gemini"),
		WithAIDescModel("flash"),
	)

	assert.Equal(t, timeout, gen.timeout)
	assert.Equal(t, "gemini", gen.agent)
	assert.Equal(t, "flash", gen.model)
}

func TestNewTemplateDescriptionGenerator_Options(t *testing.T) {
	logger := zerolog.Nop()

	gen := NewTemplateDescriptionGenerator(
		WithTemplateDescLogger(logger),
	)

	assert.NotNil(t, gen)
}

func TestBuildPrompt_AllFields(t *testing.T) {
	mockRunner := &prMockAIRunner{
		runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			// Verify all fields are in the prompt
			prompt := req.Prompt
			assert.Contains(t, prompt, "## Task Description")
			assert.Contains(t, prompt, "Test task description")
			assert.Contains(t, prompt, "## Commits")
			assert.Contains(t, prompt, "feat: first commit")
			assert.Contains(t, prompt, "## Files Changed")
			assert.Contains(t, prompt, "file.go")
			assert.Contains(t, prompt, "## Diff Summary")
			assert.Contains(t, prompt, "Added 10 lines")
			assert.Contains(t, prompt, "## Validation Results")
			assert.Contains(t, prompt, "All tests pass")
			assert.Contains(t, prompt, "## Template Type")
			assert.Contains(t, prompt, "feature")
			assert.Contains(t, prompt, "## Task ID")
			assert.Contains(t, prompt, "task-test-id")
			assert.Contains(t, prompt, "## Workspace")
			assert.Contains(t, prompt, "feat/test-ws")

			return &domain.AIResult{
				Success: true,
				Output: `TITLE: feat(test): add feature
BODY:
## Summary
Test summary.

## Changes
- file.go

## Test Plan
Tests pass.`,
			}, nil
		},
	}

	gen := NewAIDescriptionGenerator(mockRunner)

	_, err := gen.Generate(context.Background(), PRDescOptions{
		TaskDescription:   "Test task description",
		CommitMessages:    []string{"feat: first commit", "fix: second commit"},
		FilesChanged:      []PRFileChange{{Path: "file.go", Insertions: 10, Deletions: 5}},
		DiffSummary:       "Added 10 lines, removed 5 lines",
		ValidationResults: "All tests pass",
		TemplateName:      "feature",
		TaskID:            "task-test-id",
		WorkspaceName:     "feat/test-ws",
	})

	require.NoError(t, err)
}

func TestBuildPrompt_MinimalFields(t *testing.T) {
	mockRunner := &prMockAIRunner{
		runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			prompt := req.Prompt
			// Verify fallback text for empty fields (task description is empty)
			assert.Contains(t, prompt, "(Not provided)")
			// We ARE providing commits, so this should NOT appear
			assert.NotContains(t, prompt, "(No commit messages provided)")
			// We are NOT providing files
			assert.Contains(t, prompt, "(No file changes provided)")
			// Should NOT contain optional sections
			assert.NotContains(t, prompt, "## Diff Summary")
			assert.NotContains(t, prompt, "## Validation Results")
			assert.NotContains(t, prompt, "## Task ID")
			assert.NotContains(t, prompt, "## Workspace")

			return &domain.AIResult{
				Success: true,
				Output: `TITLE: feat: update code
BODY:
## Summary
Update.

## Changes
- code

## Test Plan
Ok.`,
			}, nil
		},
	}

	gen := NewAIDescriptionGenerator(mockRunner)

	// Use commits instead of task description to satisfy validation
	_, err := gen.Generate(context.Background(), PRDescOptions{
		CommitMessages: []string{"feat: minimal"},
		TemplateName:   "feature",
	})

	// This will work because we have commits
	require.NoError(t, err)
}

func TestBuildPrompt_NoCommitsOrFiles(t *testing.T) {
	mockRunner := &prMockAIRunner{
		runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			prompt := req.Prompt
			// Verify all fallback text appears when no commits or files
			assert.Contains(t, prompt, "(No commit messages provided)")
			assert.Contains(t, prompt, "(No file changes provided)")

			return &domain.AIResult{
				Success: true,
				Output: `TITLE: feat: update code
BODY:
## Summary
Update.

## Changes
- code

## Test Plan
Ok.`,
			}, nil
		},
	}

	gen := NewAIDescriptionGenerator(mockRunner)

	// Provide task description to satisfy validation, but no commits or files
	_, err := gen.Generate(context.Background(), PRDescOptions{
		TaskDescription: "Test task",
		TemplateName:    "feature",
	})

	require.NoError(t, err)
}
