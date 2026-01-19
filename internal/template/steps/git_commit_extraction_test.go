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

func TestExtractCommitMessages_UsesMetadataOnly(t *testing.T) {
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
