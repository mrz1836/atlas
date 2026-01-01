// Package tui provides terminal user interface components for ATLAS.
package tui

// ActionableError wraps an error with an actionable suggestion.
// Used to provide users with clear next steps when errors occur.
//
// Example usage:
//
//	err := NewActionableError("configuration not found", "Run: atlas init")
//	output.Error(err)
//	// Outputs: ✗ configuration not found
//	//          ▸ Try: atlas init
type ActionableError struct {
	// Message is the primary error message.
	Message string

	// Suggestion provides actionable guidance for resolving the error.
	// Should start with a verb (e.g., "Run: atlas init", "Check the file path").
	Suggestion string

	// Context provides optional additional information about the error.
	// When present, it is appended to the message in parentheses.
	Context string
}

// NewActionableError creates a new ActionableError with message and suggestion.
// The suggestion should be actionable guidance for the user.
//
// Example:
//
//	err := NewActionableError("workspace not found", "Run: atlas workspace list")
func NewActionableError(msg, suggestion string) *ActionableError {
	return &ActionableError{
		Message:    msg,
		Suggestion: suggestion,
	}
}

// Error implements the error interface.
// Returns the message with context if provided, e.g., "file not found (/path/to/file)".
func (e *ActionableError) Error() string {
	if e.Context != "" {
		return e.Message + " (" + e.Context + ")"
	}
	return e.Message
}

// WithContext adds optional context to the error.
// Returns the same error for method chaining.
//
// Example:
//
//	err := NewActionableError("file not found", "Check the path").
//	    WithContext("/path/to/missing/file")
func (e *ActionableError) WithContext(ctx string) *ActionableError {
	e.Context = ctx
	return e
}

// GetSuggestion returns the suggestion for this error.
// Used by output formatters to extract the suggestion for display.
func (e *ActionableError) GetSuggestion() string {
	return e.Suggestion
}

// GetContext returns the context for this error.
// Used by output formatters to extract the context for structured output.
func (e *ActionableError) GetContext() string {
	return e.Context
}
