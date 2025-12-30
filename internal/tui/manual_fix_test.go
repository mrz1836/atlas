package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestExtractManualFixInfo(t *testing.T) {
	tests := []struct {
		name        string
		task        *domain.Task
		workspace   *domain.Workspace
		expectPath  string
		expectStep  string
		expectError string
		expectCmd   string
	}{
		{
			name: "extracts all info from task with metadata",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 1,
				Steps: []domain.Step{
					{Name: "ai_execute"},
					{Name: "validate"},
				},
				Metadata: map[string]any{
					"last_error": "golangci-lint: undefined: foo",
				},
			},
			workspace: &domain.Workspace{
				Name:         "test-ws",
				WorktreePath: "/home/user/repos/test-ws",
			},
			expectPath:  "/home/user/repos/test-ws",
			expectStep:  "validate",
			expectError: "golangci-lint: undefined: foo",
			expectCmd:   "atlas resume test-ws",
		},
		{
			name: "handles empty metadata",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "validate"},
				},
				Metadata: nil,
			},
			workspace: &domain.Workspace{
				Name:         "auth-fix",
				WorktreePath: "/tmp/worktree",
			},
			expectPath:  "/tmp/worktree",
			expectStep:  "validate",
			expectError: "",
			expectCmd:   "atlas resume auth-fix",
		},
		{
			name: "handles retired workspace with empty worktree path",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "lint"},
				},
			},
			workspace: &domain.Workspace{
				Name:         "retired-ws",
				WorktreePath: "",
			},
			expectPath:  "(workspace retired - worktree not available)",
			expectStep:  "lint",
			expectError: "",
			expectCmd:   "atlas resume retired-ws",
		},
		{
			name: "handles current step beyond steps array",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 5, // out of bounds
				Steps: []domain.Step{
					{Name: "validate"},
				},
			},
			workspace: &domain.Workspace{
				Name:         "bounds-test",
				WorktreePath: "/path",
			},
			expectPath:  "/path",
			expectStep:  "", // no step name when out of bounds
			expectError: "",
			expectCmd:   "atlas resume bounds-test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ExtractManualFixInfo(tc.task, tc.workspace)

			require.NotNil(t, info)
			assert.Equal(t, tc.workspace.Name, info.WorkspaceName)
			assert.Equal(t, tc.expectPath, info.WorktreePath)
			assert.Equal(t, tc.expectStep, info.FailedStep)
			assert.Equal(t, tc.expectError, info.ErrorSummary)
			assert.Equal(t, tc.expectCmd, info.ResumeCommand)
		})
	}
}

func TestDisplayManualFixInstructions(t *testing.T) {
	tests := []struct {
		name              string
		task              *domain.Task
		workspace         *domain.Workspace
		expectContains    []string
		expectNotContains []string
	}{
		{
			name: "displays all sections for validation failure",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 1,
				Steps: []domain.Step{
					{Name: "ai_execute"},
					{Name: "validate"},
				},
				Metadata: map[string]any{
					"last_error": "golangci-lint: undefined: foo",
				},
			},
			workspace: &domain.Workspace{
				Name:         "test-ws",
				WorktreePath: "/home/user/repos/test-ws",
			},
			expectContains: []string{
				"/home/user/repos/test-ws",      // AC #1 - worktree path
				"golangci-lint: undefined: foo", // AC #2 - error details
				"atlas resume test-ws",          // AC #3 - resume command
				"Failed Step: validate",         // Step name shown
				"Navigate to the worktree path", // Instructions
				"Fix the validation errors",     // Instructions
				"Run the resume command",        // Instructions
			},
		},
		{
			name: "displays info without error details when none present",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "lint"},
				},
			},
			workspace: &domain.Workspace{
				Name:         "auth-fix",
				WorktreePath: "/tmp/worktree",
			},
			expectContains: []string{
				"/tmp/worktree",
				"atlas resume auth-fix",
				"Failed Step: lint",
			},
			expectNotContains: []string{
				"Error Details:", // Should not show this section when no error
			},
		},
		{
			name: "handles multiline error message",
			task: &domain.Task{
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "test"},
				},
				Metadata: map[string]any{
					"last_error": "test failed:\n  line 1: assertion error\n  line 2: expected true",
				},
			},
			workspace: &domain.Workspace{
				Name:         "multi-line",
				WorktreePath: "/path/to/worktree",
			},
			expectContains: []string{
				"test failed:",
				"assertion error",
				"expected true",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := NewTTYOutput(&buf)

			DisplayManualFixInstructions(out, tc.task, tc.workspace)

			output := buf.String()

			for _, expected := range tc.expectContains {
				assert.Contains(t, output, expected, "output should contain: %s", expected)
			}

			for _, notExpected := range tc.expectNotContains {
				assert.NotContains(t, output, notExpected, "output should not contain: %s", notExpected)
			}
		})
	}
}

func TestManualFixInfo_ResumeCommand(t *testing.T) {
	// Test that resume command is correctly formatted for various workspace names
	tests := []struct {
		workspaceName string
		expectCmd     string
	}{
		{"simple", "atlas resume simple"},
		{"with-hyphens", "atlas resume with-hyphens"},
		{"workspace123", "atlas resume workspace123"},
	}

	for _, tc := range tests {
		t.Run(tc.workspaceName, func(t *testing.T) {
			task := &domain.Task{
				CurrentStep: 0,
				Steps:       []domain.Step{{Name: "test"}},
			}
			ws := &domain.Workspace{
				Name:         tc.workspaceName,
				WorktreePath: "/path",
			}

			info := ExtractManualFixInfo(task, ws)

			assert.Equal(t, tc.expectCmd, info.ResumeCommand)
		})
	}
}

func TestExtractManualFixInfo_RetryContext(t *testing.T) {
	// Test that retry_context metadata is not used (only last_error is)
	// This documents the current behavior - retry_context is mentioned in dev notes
	// but only last_error is actually extracted

	task := &domain.Task{
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "validate"}},
		Metadata: map[string]any{
			"last_error":    "golangci-lint failed with exit code 1",
			"retry_context": "Additional context for AI retry",
		},
	}
	ws := &domain.Workspace{
		Name:         "retry-test",
		WorktreePath: "/tmp/retry",
	}

	info := ExtractManualFixInfo(task, ws)

	// Verify last_error is extracted
	assert.Equal(t, "golangci-lint failed with exit code 1", info.ErrorSummary)
	// retry_context is NOT extracted into ManualFixInfo (only last_error is used)
	// This is by design - retry_context is for AI retry, not manual fix display
}

func TestExtractManualFixInfo_NonStringMetadata(t *testing.T) {
	// Test handling of non-string last_error metadata
	task := &domain.Task{
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "test"}},
		Metadata: map[string]any{
			"last_error": 12345, // Non-string value
		},
	}
	ws := &domain.Workspace{
		Name:         "non-string-test",
		WorktreePath: "/tmp/test",
	}

	info := ExtractManualFixInfo(task, ws)

	// Non-string last_error should result in empty ErrorSummary
	assert.Empty(t, info.ErrorSummary)
}
