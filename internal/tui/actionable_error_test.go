package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionableError_Error(t *testing.T) {
	tests := []struct {
		name       string
		msg        string
		suggestion string
		context    string
		expected   string
	}{
		{
			name:       "basic error without context",
			msg:        "file not found",
			suggestion: "Check the file path",
			context:    "",
			expected:   "file not found",
		},
		{
			name:       "error with context",
			msg:        "file not found",
			suggestion: "Check the file path",
			context:    "/path/to/file",
			expected:   "file not found (/path/to/file)",
		},
		{
			name:       "empty message",
			msg:        "",
			suggestion: "Do something",
			context:    "",
			expected:   "",
		},
		{
			name:       "empty context treated as no context",
			msg:        "error occurred",
			suggestion: "Try again",
			context:    "",
			expected:   "error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewActionableError(tt.msg, tt.suggestion)
			if tt.context != "" {
				err = err.WithContext(tt.context)
			}

			assert.Equal(t, tt.expected, err.Error())
		})
	}
}

func TestNewActionableError(t *testing.T) {
	msg := "workspace not found"
	suggestion := "Run: atlas workspace list"

	err := NewActionableError(msg, suggestion)

	require.NotNil(t, err)
	assert.Equal(t, msg, err.Message)
	assert.Equal(t, suggestion, err.Suggestion)
	assert.Empty(t, err.Context)
}

func TestActionableError_WithContext(t *testing.T) {
	err := NewActionableError("config not found", "Run: atlas init")
	ctx := "~/.atlas/config.yaml"

	result := err.WithContext(ctx)

	// Should return the same error for chaining
	assert.Same(t, err, result)
	assert.Equal(t, ctx, err.Context)
}

func TestActionableError_GetSuggestion(t *testing.T) {
	suggestion := "Run: atlas init"
	err := NewActionableError("test error", suggestion)

	assert.Equal(t, suggestion, err.GetSuggestion())
}

func TestActionableError_GetContext(t *testing.T) {
	ctx := "/some/path"
	err := NewActionableError("test error", "suggestion").WithContext(ctx)

	assert.Equal(t, ctx, err.GetContext())
}

func TestActionableError_Chaining(t *testing.T) {
	err := NewActionableError("config missing", "Run: atlas init").
		WithContext("~/.atlas/config.yaml")

	assert.Equal(t, "config missing", err.Message)
	assert.Equal(t, "Run: atlas init", err.Suggestion)
	assert.Equal(t, "~/.atlas/config.yaml", err.Context)
	assert.Equal(t, "config missing (~/.atlas/config.yaml)", err.Error())
}

func TestActionableError_ImplementsError(t *testing.T) {
	var err error = NewActionableError("test", "suggestion")

	// Should be usable as a standard error
	require.Error(t, err)
	assert.Equal(t, "test", err.Error())
}

func TestActionableError_ErrorsAs(t *testing.T) {
	original := NewActionableError("config not found", "Run: atlas init").
		WithContext("/path/to/config")

	// Direct errors.As check
	var ae *ActionableError
	err := original // Use the original directly
	require.ErrorAs(t, err, &ae, "errors.As should find ActionableError")
	assert.Equal(t, "config not found", ae.Message)
	assert.Equal(t, "Run: atlas init", ae.Suggestion)
	assert.Equal(t, "/path/to/config", ae.Context)
}

func TestActionableError_EmptySuggestion(t *testing.T) {
	err := NewActionableError("something went wrong", "")

	assert.Equal(t, "something went wrong", err.Error())
	assert.Empty(t, err.GetSuggestion())
}
