package steps

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestExtractCommitMessages_FromMetadata(t *testing.T) {
	results := []domain.StepResult{
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []string{
					"feat(api): add user authentication\n\nImplements JWT-based auth with refresh tokens.",
					"fix(db): handle connection timeout\n\nAdds retry logic for database connections.",
				},
			},
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 2)
	assert.Equal(t, "feat(api): add user authentication\n\nImplements JWT-based auth with refresh tokens.", messages[0])
	assert.Equal(t, "fix(db): handle connection timeout\n\nAdds retry logic for database connections.", messages[1])
}

func TestExtractCommitMessages_FromMetadataAnySlice(t *testing.T) {
	// Test when metadata contains []any instead of []string (can happen with JSON unmarshaling)
	results := []domain.StepResult{
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []any{
					"feat(api): add endpoint",
					"docs(readme): update installation",
				},
			},
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 2)
	assert.Equal(t, "feat(api): add endpoint", messages[0])
	assert.Equal(t, "docs(readme): update installation", messages[1])
}

func TestExtractCommitMessages_FallbackToOutput(t *testing.T) {
	// When no metadata, should fallback to Output if it looks like a commit message
	results := []domain.StepResult{
		{
			Status: "success",
			Output: "feat(api): add user authentication",
		},
		{
			Status: "success",
			Output: "fix: handle edge case",
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 2)
	assert.Equal(t, "feat(api): add user authentication", messages[0])
	assert.Equal(t, "fix: handle edge case", messages[1])
}

func TestExtractCommitMessages_IgnoresNonCommitOutput(t *testing.T) {
	// Should NOT include outputs that don't look like commit messages
	results := []domain.StepResult{
		{
			Status: "success",
			Output: "Created 1 commit(s), 5 files changed", // Not a commit message
		},
		{
			Status: "success",
			Output: "Pushed to origin/main", // Not a commit message
		},
		{
			Status: "success",
			Output: "feat(api): add endpoint", // This IS a commit message
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 1, "should only extract the actual commit message")
	assert.Equal(t, "feat(api): add endpoint", messages[0])
}

func TestExtractCommitMessages_PreferMetadataOverOutput(t *testing.T) {
	// When both metadata and output exist, should use metadata
	results := []domain.StepResult{
		{
			Status: "success",
			Output: "feat(wrong): this should be ignored",
			Metadata: map[string]any{
				"commit_messages": []string{
					"feat(correct): use this from metadata",
				},
			},
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 1)
	assert.Equal(t, "feat(correct): use this from metadata", messages[0])
}

func TestExtractCommitMessages_IgnoresFailedSteps(t *testing.T) {
	results := []domain.StepResult{
		{
			Status: "failed",
			Metadata: map[string]any{
				"commit_messages": []string{"feat: this failed"},
			},
		},
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []string{"feat: this succeeded"},
			},
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 1, "should only extract from successful steps")
	assert.Equal(t, "feat: this succeeded", messages[0])
}

func TestExtractCommitMessages_MultipleStepsWithMultipleCommits(t *testing.T) {
	results := []domain.StepResult{
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []string{
					"feat(api): add endpoint",
					"feat(api): add tests",
				},
			},
		},
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []string{
					"docs: update readme",
				},
			},
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 3, "should extract all commit messages from all steps")
	assert.Equal(t, "feat(api): add endpoint", messages[0])
	assert.Equal(t, "feat(api): add tests", messages[1])
	assert.Equal(t, "docs: update readme", messages[2])
}

func TestExtractCommitMessages_EmptyResults(t *testing.T) {
	messages := extractCommitMessages([]domain.StepResult{})
	assert.Empty(t, messages)
}

func TestExtractCommitMessages_NoCommitMessages(t *testing.T) {
	results := []domain.StepResult{
		{
			Status: "success",
			Output: "Some random output",
		},
		{
			Status: "success",
			Metadata: map[string]any{
				"other_data": "not commit messages",
			},
		},
	}

	messages := extractCommitMessages(results)
	assert.Empty(t, messages, "should return empty slice when no commit messages found")
}

func TestLooksLikeCommitMessage_ValidFormats(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		// Valid conventional commit types
		{"feat without scope", "feat: add new feature", true},
		{"feat with scope", "feat(api): add endpoint", true},
		{"fix without scope", "fix: resolve bug", true},
		{"fix with scope", "fix(auth): handle timeout", true},
		{"docs", "docs: update readme", true},
		{"docs with scope", "docs(api): add examples", true},
		{"style", "style: format code", true},
		{"refactor", "refactor: simplify logic", true},
		{"refactor with scope", "refactor(core): extract helper", true},
		{"test", "test: add unit tests", true},
		{"chore", "chore: update dependencies", true},
		{"chore with scope", "chore(deps): bump version", true},
		{"build", "build: update webpack config", true},
		{"ci", "ci: add github actions", true},
		{"ci with scope", "ci(actions): add test workflow", true},
		{"perf", "perf: optimize query", true},
		{"perf with scope", "perf(db): add index", true},
		{"revert", "revert: undo previous commit", true},

		// Invalid formats
		{"no type prefix", "add new feature", false},
		{"wrong type", "feature: add something", false},
		{"missing colon", "feat add feature", false},
		{"step summary", "Created 1 commit(s), 5 files changed", false},
		{"push output", "Pushed to origin/main", false},
		{"empty string", "", false},
		{"just type", "feat", false},
		{"type with space", "feat : add feature", false},
		{"random text", "This is some random text", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeCommitMessage(tt.message)
			assert.Equal(t, tt.want, got, "message: %q", tt.message)
		})
	}
}

func TestLooksLikeCommitMessage_EdgeCases(t *testing.T) {
	// Very short strings
	assert.False(t, looksLikeCommitMessage("f"))
	assert.False(t, looksLikeCommitMessage("fe"))
	assert.False(t, looksLikeCommitMessage("fea"))

	// Just the type prefix
	assert.False(t, looksLikeCommitMessage("feat:"))
	assert.False(t, looksLikeCommitMessage("fix:"))

	// Minimum valid length
	assert.True(t, looksLikeCommitMessage("feat: x"))
	assert.True(t, looksLikeCommitMessage("fix: x"))

	// With newlines and extra content
	assert.True(t, looksLikeCommitMessage("feat: add feature\n\nThis is the body."))
	assert.True(t, looksLikeCommitMessage("fix(api): resolve bug\n\nAdds error handling."))

	// Scopes with various characters
	assert.True(t, looksLikeCommitMessage("feat(my-scope): description"))
	assert.True(t, looksLikeCommitMessage("feat(my_scope): description"))
	assert.True(t, looksLikeCommitMessage("feat(scope123): description"))

	// Case sensitivity - should be case-sensitive
	assert.False(t, looksLikeCommitMessage("Feat: add feature"))
	assert.False(t, looksLikeCommitMessage("FEAT: add feature"))
	assert.False(t, looksLikeCommitMessage("Fix: resolve bug"))
}

func TestExtractCommitMessages_MixedMetadataTypes(t *testing.T) {
	// Test handling of invalid metadata types
	results := []domain.StepResult{
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": "not an array", // Wrong type
			},
		},
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []string{"feat: valid message"},
			},
		},
		{
			Status: "success",
			Metadata: map[string]any{
				"commit_messages": []any{
					123,              // Not a string
					"fix: valid msg", // Valid string
					nil,              // Nil value
				},
			},
		},
	}

	messages := extractCommitMessages(results)

	// Should only extract the valid messages
	assert.Len(t, messages, 2)
	assert.Equal(t, "feat: valid message", messages[0])
	assert.Equal(t, "fix: valid msg", messages[1])
}

func TestExtractCommitMessages_BackwardCompatibility(t *testing.T) {
	// Test that old step results without metadata still work
	results := []domain.StepResult{
		{
			Status: "success",
			Output: "feat(api): add endpoint",
			// No metadata field
		},
		{
			Status: "success",
			Output: "Created 1 commit(s)", // Should be filtered out
		},
	}

	messages := extractCommitMessages(results)

	assert.Len(t, messages, 1)
	assert.Equal(t, "feat(api): add endpoint", messages[0])
}
