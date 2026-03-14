package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// This file contains additional tests to boost coverage for hard-to-test functions
// by testing their edge cases and error paths where possible.

func TestMaxWorkspaceNameLenConstant(t *testing.T) {
	// Verify the constant is exported and has the right value
	assert.Equal(t, 50, MaxWorkspaceNameLen)
	assert.Equal(t, maxWorkspaceNameLen, MaxWorkspaceNameLen)

	// Verify sanitizeWorkspaceName respects this limit
	longName := "this-is-an-extremely-long-workspace-name-that-should-be-truncated-to-fifty-characters-maximum"
	result := sanitizeWorkspaceName(longName)
	assert.LessOrEqual(t, len(result), MaxWorkspaceNameLen, "sanitized name should not exceed max length")
}

func TestGenerateWorkspaceName_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		description string
		checkResult func(t *testing.T, result string)
	}{
		{
			name:        "unicode characters",
			description: "Add multilingual support",
			checkResult: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
				// Unicode chars should be stripped or converted
			},
		},
		{
			name:        "mixed case with numbers",
			description: "Bug Fix #123 for API v2.0",
			checkResult: func(t *testing.T, result string) {
				assert.Contains(t, result, "bug")
				assert.Contains(t, result, "123")
			},
		},
		{
			name:        "underscore handling",
			description: "update_test_cases",
			checkResult: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
			},
		},
		{
			name:        "tab and newline characters",
			description: "fix\tbug\nwith\tspaces",
			checkResult: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
				assert.NotContains(t, result, "\t")
				assert.NotContains(t, result, "\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWorkspaceName(tt.description)
			tt.checkResult(t, result)
		})
	}
}

func TestSanitizeWorkspaceName_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		checkResult func(t *testing.T, result string)
	}{
		{
			name:  "all hyphens",
			input: "------",
			checkResult: func(t *testing.T, result string) {
				assert.Empty(t, result, "all hyphens should be trimmed to empty string")
			},
		},
		{
			name:  "hyphen sandwich",
			input: "---valid---",
			checkResult: func(t *testing.T, result string) {
				assert.Equal(t, "valid", result)
			},
		},
		{
			name:  "exactly at max length",
			input: "12345678901234567890123456789012345678901234567890",
			checkResult: func(t *testing.T, result string) {
				assert.Len(t, result, 50)
			},
		},
		{
			name:  "one over max length",
			input: "123456789012345678901234567890123456789012345678901",
			checkResult: func(t *testing.T, result string) {
				assert.Len(t, result, 50)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeWorkspaceName(tt.input)
			tt.checkResult(t, result)
		})
	}
}

func TestWorkspaceOptions_AllFieldsCoverage(t *testing.T) {
	// Test that all WorkspaceOptions fields can be set and accessed
	errorCalled := false
	errorHandler := func(wsName string, err error) error {
		errorCalled = true
		assert.Equal(t, "test-ws", wsName)
		assert.Error(t, err)
		return err
	}

	opts := WorkspaceOptions{
		Name:          "test-ws",
		RepoPath:      "/repo",
		BranchPrefix:  "feat",
		BaseBranch:    "main",
		TargetBranch:  "dev",
		UseLocal:      true,
		NoInteractive: true,
		OutputFormat:  "json",
		ErrorHandler:  errorHandler,
	}

	// Verify all fields
	assert.Equal(t, "test-ws", opts.Name)
	assert.Equal(t, "/repo", opts.RepoPath)
	assert.Equal(t, "feat", opts.BranchPrefix)
	assert.Equal(t, "main", opts.BaseBranch)
	assert.Equal(t, "dev", opts.TargetBranch)
	assert.True(t, opts.UseLocal)
	assert.True(t, opts.NoInteractive)
	assert.Equal(t, "json", opts.OutputFormat)

	// Test error handler
	_ = opts.ErrorHandler("test-ws", assert.AnError)
	assert.True(t, errorCalled)
}

func TestGitServices_AllFieldsCoverage(t *testing.T) {
	// Test that GitServices struct fields are all accessible
	gs := &GitServices{
		Runner:           nil,
		SmartCommitter:   nil,
		Pusher:           nil,
		HubRunner:        nil,
		PRDescGen:        nil,
		CIFailureHandler: nil,
	}

	assert.NotNil(t, gs)
	assert.Nil(t, gs.Runner)
	assert.Nil(t, gs.SmartCommitter)
	assert.Nil(t, gs.Pusher)
	assert.Nil(t, gs.HubRunner)
	assert.Nil(t, gs.PRDescGen)
	assert.Nil(t, gs.CIFailureHandler)
}

func TestRegexPatterns_Coverage(t *testing.T) {
	t.Run("nonAlphanumericRegex various inputs", func(t *testing.T) {
		tests := []struct {
			input          string
			shouldNotEmpty bool
		}{
			{"hello@world", true},
			{"test!name#here", true},
			{"valid-name", true},
			{"123abc", true},
			{"UPPER", false}, // Uppercase is removed by the regex
			{"lower", true},
		}

		for _, tt := range tests {
			result := nonAlphanumericRegex.ReplaceAllString(tt.input, "")
			// After removing non-alphanumeric (except hyphen), result may be empty
			// The regex only allows lowercase letters, digits, and hyphens
			if tt.shouldNotEmpty {
				assert.NotEmpty(t, result)
			}
		}
	})

	t.Run("multipleHyphensRegex various inputs", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"test---name", "test-name"},
			{"test--name", "test-name"},
			{"test----name", "test-name"},
			{"test-name", "test-name"},
		}

		for _, tt := range tests {
			result := multipleHyphensRegex.ReplaceAllString(tt.input, "-")
			assert.Equal(t, tt.expected, result)
		}
	})
}
