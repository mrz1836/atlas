package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestExtractAbandonInfo(t *testing.T) {
	tests := []struct {
		name            string
		task            *domain.Task
		workspace       *domain.Workspace
		expectWorkspace string
		expectBranch    string
		expectPath      string
		expectTaskID    string
	}{
		{
			name: "extracts all info correctly",
			task: &domain.Task{
				ID:     "task-abc-xyz",
				Status: constants.TaskStatusValidationFailed,
			},
			workspace: &domain.Workspace{
				Name:         "auth-fix",
				Branch:       "fix/auth-fix",
				WorktreePath: "/home/user/repos/auth-fix",
			},
			expectWorkspace: "auth-fix",
			expectBranch:    "fix/auth-fix",
			expectPath:      "/home/user/repos/auth-fix",
			expectTaskID:    "task-abc-xyz",
		},
		{
			name: "handles empty branch",
			task: &domain.Task{
				ID:     "task-test-id",
				Status: constants.TaskStatusGHFailed,
			},
			workspace: &domain.Workspace{
				Name:         "test-ws",
				Branch:       "",
				WorktreePath: "/tmp/worktree",
			},
			expectWorkspace: "test-ws",
			expectBranch:    "",
			expectPath:      "/tmp/worktree",
			expectTaskID:    "task-test-id",
		},
		{
			name: "handles ci_failed state",
			task: &domain.Task{
				ID:     "task-ci-fail",
				Status: constants.TaskStatusCIFailed,
			},
			workspace: &domain.Workspace{
				Name:         "ci-test",
				Branch:       "feat/ci-test",
				WorktreePath: "/projects/ci-test",
			},
			expectWorkspace: "ci-test",
			expectBranch:    "feat/ci-test",
			expectPath:      "/projects/ci-test",
			expectTaskID:    "task-ci-fail",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ExtractAbandonInfo(tc.task, tc.workspace)

			require.NotNil(t, info)
			assert.Equal(t, tc.expectWorkspace, info.WorkspaceName)
			assert.Equal(t, tc.expectBranch, info.BranchName)
			assert.Equal(t, tc.expectPath, info.WorktreePath)
			assert.Equal(t, tc.expectTaskID, info.TaskID)
		})
	}
}

func TestDisplayAbandonmentSuccess(t *testing.T) {
	tests := []struct {
		name           string
		task           *domain.Task
		workspace      *domain.Workspace
		expectContains []string
	}{
		{
			name: "displays all abandonment info",
			task: &domain.Task{
				ID:     "task-abandon-abc",
				Status: constants.TaskStatusAbandoned,
			},
			workspace: &domain.Workspace{
				Name:         "auth-fix",
				Branch:       "fix/auth-fix",
				WorktreePath: "/home/user/repos/auth-fix",
			},
			expectContains: []string{
				"Task Abandoned",                   // Header
				"task-abandon-abc",                 // Task ID
				"fix/auth-fix",                     // Branch (preserved)
				"/home/user/repos/auth-fix",        // Worktree path (preserved)
				"preserved",                        // Indicates preservation
				"Next Steps",                       // Instructions header
				"Navigate to the worktree path",    // Next step instruction
				"atlas start",                      // Suggestion for new task
				"atlas workspace destroy auth-fix", // Cleanup command
			},
		},
		{
			name: "displays correct workspace name in cleanup command",
			task: &domain.Task{
				ID:     "task-xyz",
				Status: constants.TaskStatusAbandoned,
			},
			workspace: &domain.Workspace{
				Name:         "my-custom-workspace",
				Branch:       "feat/custom",
				WorktreePath: "/path/to/worktree",
			},
			expectContains: []string{
				"atlas workspace destroy my-custom-workspace",
				"my-custom-workspace",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := NewTTYOutput(&buf)

			DisplayAbandonmentSuccess(out, tc.task, tc.workspace)

			output := buf.String()

			for _, expected := range tc.expectContains {
				assert.Contains(t, output, expected, "output should contain: %s", expected)
			}
		})
	}
}

func TestAbandonInfo_StructFields(t *testing.T) {
	// Verify the struct can be created with all fields
	info := &AbandonInfo{
		WorkspaceName: "test-ws",
		BranchName:    "fix/test",
		WorktreePath:  "/home/user/test",
		TaskID:        "task-test-id",
	}

	assert.Equal(t, "test-ws", info.WorkspaceName)
	assert.Equal(t, "fix/test", info.BranchName)
	assert.Equal(t, "/home/user/test", info.WorktreePath)
	assert.Equal(t, "task-test-id", info.TaskID)
}
