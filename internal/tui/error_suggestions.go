// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"errors"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ErrorSuggestion maps a sentinel error to its suggested fix.
// These provide actionable guidance when errors occur.
type ErrorSuggestion struct {
	Error      error
	Suggestion string
}

// errorSuggestions maps common errors to helpful suggestions.
// Each suggestion should be actionable and start with a verb.
//
//nolint:gochecknoglobals // Intentional package-level constant for error suggestions
var errorSuggestions = []ErrorSuggestion{
	// Workspace errors
	{atlaserrors.ErrWorkspaceNotFound, "Run: atlas workspace list"},
	{atlaserrors.ErrWorkspaceExists, "Run: atlas start --workspace <new-name>"},
	{atlaserrors.ErrWorkspaceCorrupted, "Try: atlas workspace destroy <name> && atlas start"},
	{atlaserrors.ErrWorkspaceHasRunningTasks, "Wait for tasks to complete or run: atlas status"},

	// Git errors
	{atlaserrors.ErrNotGitRepo, "Run: git init"},
	{atlaserrors.ErrNotInGitRepo, "Navigate to a git repository directory"},
	{atlaserrors.ErrBranchExists, "Use: atlas start --workspace <different-name>"},
	{atlaserrors.ErrWorktreeExists, "Run: git worktree prune"},
	{atlaserrors.ErrWorktreeDirty, "Commit or stash uncommitted changes"},

	// Configuration errors
	{atlaserrors.ErrConfigNotFound, "Run: atlas init"},
	{atlaserrors.ErrConfigInvalidAI, "Run: atlas config ai"},
	{atlaserrors.ErrConfigInvalidGit, "Check your git configuration"},
	{atlaserrors.ErrConfigInvalidValidation, "Run: atlas config validation"},
	{atlaserrors.ErrMissingRequiredTools, "Run: atlas upgrade"},

	// Task errors
	{atlaserrors.ErrNoTasksFound, "Run: atlas start \"description\""},
	{atlaserrors.ErrTaskNotFound, "Run: atlas status"},
	{atlaserrors.ErrValidationFailed, "Run: atlas resume"},
	{atlaserrors.ErrMaxRetriesExceeded, "Run: atlas resume"},

	// Template errors
	{atlaserrors.ErrTemplateNotFound, "Use: --template bugfix, feature, or commit"},
	{atlaserrors.ErrTemplateRequired, "Add: --template <template-name>"},

	// GitHub/CI errors
	{atlaserrors.ErrPushAuthFailed, "Check your git credentials or SSH keys"},
	{atlaserrors.ErrPushNetworkFailed, "Check your network connection"},
	{atlaserrors.ErrPRCreationFailed, "Run: atlas resume"},
	{atlaserrors.ErrGHAuthFailed, "Run: gh auth login"},
	{atlaserrors.ErrGHRateLimited, "Wait a few minutes and try again"},
	{atlaserrors.ErrCIFailed, "Run: atlas resume"},
	{atlaserrors.ErrCITimeout, "Run: atlas resume"},

	// AI errors
	{atlaserrors.ErrClaudeInvocation, "Check that Claude CLI is installed: claude --version"},
	{atlaserrors.ErrAIError, "Retry the operation"},
	{atlaserrors.ErrAIEmptyResponse, "Retry the operation"},

	// User input errors
	{atlaserrors.ErrNonInteractiveMode, "Add: --force to confirm"},
	{atlaserrors.ErrApprovalRequired, "Add: --auto-approve to skip confirmation"},
	{atlaserrors.ErrConflictingFlags, "Remove one of the conflicting flags"},

	// Model errors
	{atlaserrors.ErrInvalidModel, "Use: --model sonnet, opus, or haiku"},
}

// SuggestionForError returns a suggestion for the given error.
// Returns empty string if no suggestion is available.
func SuggestionForError(err error) string {
	if err == nil {
		return ""
	}

	for _, es := range errorSuggestions {
		if errors.Is(err, es.Error) {
			return es.Suggestion
		}
	}
	return ""
}

// WithSuggestion wraps an error with its suggestion if one exists.
// If no suggestion is available, returns an ActionableError with empty suggestion.
// This allows consistent error handling in CLI commands.
func WithSuggestion(err error) *ActionableError {
	if err == nil {
		return nil
	}

	suggestion := SuggestionForError(err)
	return NewActionableError(err.Error(), suggestion)
}

// WrapWithSuggestion creates an ActionableError from an error if a suggestion exists.
// Returns the original error unchanged if no suggestion is available.
// This is useful for preserving error types when suggestions don't exist.
func WrapWithSuggestion(err error) error {
	if err == nil {
		return nil
	}

	suggestion := SuggestionForError(err)
	if suggestion == "" {
		return err
	}
	return NewActionableError(err.Error(), suggestion)
}
