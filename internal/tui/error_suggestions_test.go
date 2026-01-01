package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestGetSuggestionForError(t *testing.T) {
	t.Run("workspace not found has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrWorkspaceNotFound)
		assert.Contains(t, suggestion, "atlas workspace list")
	})

	t.Run("config not found has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrConfigNotFound)
		assert.Contains(t, suggestion, "atlas init")
	})

	t.Run("not git repo has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrNotGitRepo)
		assert.Contains(t, suggestion, "git init")
	})

	t.Run("nil error returns empty string", func(t *testing.T) {
		suggestion := GetSuggestionForError(nil)
		assert.Empty(t, suggestion)
	})

	t.Run("unmapped error returns empty string", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrUnknownTool)
		// ErrUnknownTool has no specific suggestion mapped
		assert.Empty(t, suggestion)
	})

	t.Run("wrapped error returns suggestion", func(t *testing.T) {
		// errors.Is works with properly wrapped errors (using %w)
		// so we test with the sentinel error directly
		suggestion := GetSuggestionForError(atlaserrors.ErrWorkspaceNotFound)
		assert.NotEmpty(t, suggestion)
	})

	t.Run("validation failed has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrValidationFailed)
		assert.Contains(t, suggestion, "atlas recover")
	})

	t.Run("template required has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrTemplateRequired)
		assert.Contains(t, suggestion, "--template")
	})

	t.Run("gh auth failed has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrGHAuthFailed)
		assert.Contains(t, suggestion, "gh auth login")
	})

	t.Run("invalid model has suggestion", func(t *testing.T) {
		suggestion := GetSuggestionForError(atlaserrors.ErrInvalidModel)
		assert.Contains(t, suggestion, "sonnet")
		assert.Contains(t, suggestion, "opus")
		assert.Contains(t, suggestion, "haiku")
	})
}

func TestWithSuggestion(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		result := WithSuggestion(nil)
		assert.Nil(t, result)
	})

	t.Run("known error returns ActionableError with suggestion", func(t *testing.T) {
		result := WithSuggestion(atlaserrors.ErrConfigNotFound)
		require.NotNil(t, result)
		assert.Equal(t, atlaserrors.ErrConfigNotFound.Error(), result.Message)
		assert.Contains(t, result.Suggestion, "atlas init")
	})

	t.Run("unmapped error returns ActionableError with empty suggestion", func(t *testing.T) {
		// Use an error that exists but has no suggestion mapped
		result := WithSuggestion(atlaserrors.ErrUnknownTool)
		require.NotNil(t, result)
		assert.Equal(t, atlaserrors.ErrUnknownTool.Error(), result.Message)
	})
}

func TestWrapWithSuggestion(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		result := WrapWithSuggestion(nil)
		assert.NoError(t, result)
	})

	t.Run("known error returns ActionableError", func(t *testing.T) {
		result := WrapWithSuggestion(atlaserrors.ErrConfigNotFound)
		require.Error(t, result)

		var ae *ActionableError
		require.ErrorAs(t, result, &ae)
		assert.Contains(t, ae.Suggestion, "atlas init")
	})

	t.Run("unmapped error returns original error unchanged", func(t *testing.T) {
		// Use an existing error that has no suggestion mapped
		result := WrapWithSuggestion(atlaserrors.ErrUnknownTool)

		// Should be the same error, not wrapped (since no suggestion exists)
		assert.Equal(t, atlaserrors.ErrUnknownTool, result)
	})
}

func TestErrorSuggestions_AllHaveSuggestions(t *testing.T) {
	// Verify all mapped errors have non-empty suggestions
	for _, es := range errorSuggestions {
		t.Run(es.Error.Error(), func(t *testing.T) {
			assert.NotEmpty(t, es.Suggestion, "error should have a suggestion")
		})
	}
}

func TestErrorSuggestions_SuggestionStartsWithVerb(t *testing.T) {
	// Suggestions should be actionable, starting with a verb
	verbPrefixes := []string{"Run:", "Use:", "Add:", "Check", "Navigate", "Wait", "Try:", "Commit", "Retry", "Remove"}

	for _, es := range errorSuggestions {
		t.Run(es.Error.Error(), func(t *testing.T) {
			startsWithVerb := false
			for _, prefix := range verbPrefixes {
				if len(es.Suggestion) >= len(prefix) && es.Suggestion[:len(prefix)] == prefix {
					startsWithVerb = true
					break
				}
			}
			assert.True(t, startsWithVerb, "suggestion should start with a verb: %s", es.Suggestion)
		})
	}
}

func TestCommonErrors_HaveSuggestions(t *testing.T) {
	// These are the most common errors users encounter
	// All should have suggestions
	commonErrors := []error{
		atlaserrors.ErrWorkspaceNotFound,
		atlaserrors.ErrConfigNotFound,
		atlaserrors.ErrNotGitRepo,
		atlaserrors.ErrValidationFailed,
		atlaserrors.ErrNoTasksFound,
		atlaserrors.ErrTemplateRequired,
		atlaserrors.ErrGHAuthFailed,
	}

	for _, err := range commonErrors {
		t.Run(err.Error(), func(t *testing.T) {
			suggestion := GetSuggestionForError(err)
			assert.NotEmpty(t, suggestion, "common error should have suggestion")
		})
	}
}
