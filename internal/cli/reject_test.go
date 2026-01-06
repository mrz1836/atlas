// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockTaskStoreForReject implements task.Store interface for testing.
type mockTaskStoreForReject struct {
	tasks       map[string][]*domain.Task // workspaceName -> tasks
	artifacts   map[string][]byte         // "workspace:task:filename" -> data
	getErr      error
	listErr     error
	updateErr   error
	artifactErr error
	saveArtErr  error
}

func (m *mockTaskStoreForReject) List(_ context.Context, workspaceName string) ([]*domain.Task, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		return tasks, nil
	}
	return []*domain.Task{}, nil
}

func (m *mockTaskStoreForReject) Get(_ context.Context, workspaceName, taskID string) (*domain.Task, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		for _, t := range tasks {
			if t.ID == taskID {
				return t, nil
			}
		}
	}
	return nil, atlaserrors.ErrTaskNotFound
}

func (m *mockTaskStoreForReject) Create(_ context.Context, _ string, _ *domain.Task) error {
	return nil
}

func (m *mockTaskStoreForReject) Update(_ context.Context, workspaceName string, t *domain.Task) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		for i, existing := range tasks {
			if existing.ID == t.ID {
				m.tasks[workspaceName][i] = t
				return nil
			}
		}
	}
	return nil
}

func (m *mockTaskStoreForReject) Delete(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockTaskStoreForReject) AppendLog(_ context.Context, _, _ string, _ []byte) error {
	return nil
}

func (m *mockTaskStoreForReject) ReadLog(_ context.Context, _, _ string) ([]byte, error) {
	return nil, nil
}

func (m *mockTaskStoreForReject) SaveArtifact(_ context.Context, workspaceName, taskID, filename string, data []byte) error {
	if m.saveArtErr != nil {
		return m.saveArtErr
	}
	if m.artifacts == nil {
		m.artifacts = make(map[string][]byte)
	}
	key := workspaceName + ":" + taskID + ":" + filename
	m.artifacts[key] = data
	return nil
}

func (m *mockTaskStoreForReject) SaveVersionedArtifact(_ context.Context, _, _, _ string, _ []byte) (string, error) {
	return "", nil
}

func (m *mockTaskStoreForReject) GetArtifact(_ context.Context, workspaceName, taskID, filename string) ([]byte, error) {
	if m.artifactErr != nil {
		return nil, m.artifactErr
	}
	key := workspaceName + ":" + taskID + ":" + filename
	if data, ok := m.artifacts[key]; ok {
		return data, nil
	}
	return nil, atlaserrors.ErrArtifactNotFound
}

func (m *mockTaskStoreForReject) ListArtifacts(_ context.Context, _, _ string) ([]string, error) {
	return []string{}, nil
}

// TestAddRejectCommand tests that reject command is properly added to root.
func TestAddRejectCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddRejectCommand(root)

	// Find the reject command
	rejectCmd, _, err := root.Find([]string{"reject"})
	require.NoError(t, err)
	require.NotNil(t, rejectCmd)
	assert.Equal(t, "reject", rejectCmd.Name())
}

// TestRejectCommand_CommandHelp tests command help text.
func TestRejectCommand_CommandHelp(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddRejectCommand(root)

	rejectCmd, _, err := root.Find([]string{"reject"})
	require.NoError(t, err)

	// Check usage line
	assert.Equal(t, "reject [workspace]", rejectCmd.Use)

	// Check short description
	assert.Contains(t, rejectCmd.Short, "Reject")
	assert.Contains(t, rejectCmd.Short, "feedback")

	// Check long description
	assert.Contains(t, rejectCmd.Long, "Reject a task")
	assert.Contains(t, rejectCmd.Long, "awaiting approval")
}

// TestRejectCommand_MaxArgs tests that reject accepts at most 1 argument.
func TestRejectCommand_MaxArgs(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddRejectCommand(root)

	rejectCmd, _, err := root.Find([]string{"reject"})
	require.NoError(t, err)

	// Args function should be set
	require.NotNil(t, rejectCmd.Args)
}

// TestRejectCommand_Flags tests command flags.
func TestRejectCommand_Flags(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddRejectCommand(root)

	rejectCmd, _, err := root.Find([]string{"reject"})
	require.NoError(t, err)

	// Check flags exist
	retryFlag := rejectCmd.Flags().Lookup("retry")
	require.NotNil(t, retryFlag)
	assert.Equal(t, "false", retryFlag.DefValue)

	doneFlag := rejectCmd.Flags().Lookup("done")
	require.NotNil(t, doneFlag)
	assert.Equal(t, "false", doneFlag.DefValue)

	feedbackFlag := rejectCmd.Flags().Lookup("feedback")
	require.NotNil(t, feedbackFlag)
	assert.Empty(t, feedbackFlag.DefValue)

	stepFlag := rejectCmd.Flags().Lookup("step")
	require.NotNil(t, stepFlag)
	assert.Equal(t, "0", stepFlag.DefValue)
}

// TestRejectResponse_JSONStructure tests JSON response structure.
func TestRejectResponse_JSONStructure(t *testing.T) {
	t.Parallel()

	resp := rejectResponse{
		Success:       true,
		Action:        "retry",
		WorkspaceName: "test-ws",
		TaskID:        "task-1",
		Feedback:      "Fix the auth flow",
		ResumeStep:    3,
		BranchName:    "feat/test",
		WorktreePath:  "/path/to/worktree",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify JSON structure
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed["success"].(bool))
	assert.Equal(t, "retry", parsed["action"])
	assert.Equal(t, "test-ws", parsed["workspace_name"])
	assert.Equal(t, "task-1", parsed["task_id"])
	assert.Equal(t, "Fix the auth flow", parsed["feedback"])
	assert.InDelta(t, 3, parsed["resume_step"], 0.001)
	assert.Equal(t, "feat/test", parsed["branch_name"])
	assert.Equal(t, "/path/to/worktree", parsed["worktree_path"])
}

// TestRejectResponse_JSONDone tests JSON response for done action.
func TestRejectResponse_JSONDone(t *testing.T) {
	t.Parallel()

	resp := rejectResponse{
		Success:       true,
		Action:        "done",
		WorkspaceName: "test-ws",
		TaskID:        "task-1",
		BranchName:    "feat/test",
		WorktreePath:  "/path/to/worktree",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	jsonStr := string(data)
	// Feedback and resume_step should be omitted for done action
	assert.NotContains(t, jsonStr, "feedback")
	assert.NotContains(t, jsonStr, "resume_step")
}

// TestRejectResponse_JSONError tests JSON error response structure.
func TestRejectResponse_JSONError(t *testing.T) {
	t.Parallel()

	resp := rejectResponse{
		Success:       false,
		WorkspaceName: "test-ws",
		TaskID:        "task-1",
		Error:         "failed to reject task",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify error is present
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.False(t, parsed["success"].(bool))
	assert.Equal(t, "failed to reject task", parsed["error"])
}

// TestOutputRejectErrorJSON tests JSON error output.
func TestOutputRejectErrorJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := outputRejectErrorJSON(&buf, "test-ws", "task-1", "test error message")

	// Should return ErrJSONErrorOutput
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON output
	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)

	assert.False(t, resp.Success)
	assert.Equal(t, "test-ws", resp.WorkspaceName)
	assert.Equal(t, "task-1", resp.TaskID)
	assert.Equal(t, "test error message", resp.Error)
}

// TestHandleRejectError tests error handling based on output format.
func TestHandleRejectError(t *testing.T) {
	t.Parallel()

	testErr := atlaserrors.ErrWorkspaceNotFound

	t.Run("text format returns error directly", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := handleRejectError(OutputText, &buf, "test-ws", testErr)
		require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
		assert.Empty(t, buf.String(), "text format should not write to buffer")
	})

	t.Run("json format outputs JSON error", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := handleRejectError(OutputJSON, &buf, "test-ws", testErr)
		require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)
		assert.NotEmpty(t, buf.String(), "json format should write to buffer")

		var resp rejectResponse
		jsonErr := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, jsonErr)
		assert.False(t, resp.Success)
	})
}

// TestRejectAction_Constants tests reject action constants.
func TestRejectAction_Constants(t *testing.T) {
	t.Parallel()

	// Verify all action constants are defined
	assert.Equal(t, rejectActionRetry, rejectAction("retry"))
	assert.Equal(t, rejectActionDone, rejectAction("done"))
}

// TestFindDefaultResumeStep tests finding the default resume step.
func TestFindDefaultResumeStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		task     *domain.Task
		expected int
	}{
		{
			name:     "empty steps",
			task:     &domain.Task{Steps: []domain.Step{}},
			expected: 0,
		},
		{
			name: "no implement step",
			task: &domain.Task{
				Steps: []domain.Step{
					{Name: "analyze"},
					{Name: "validate"},
				},
			},
			expected: 0,
		},
		{
			name: "has implement step",
			task: &domain.Task{
				Steps: []domain.Step{
					{Name: "analyze"},
					{Name: "implement"},
					{Name: "validate"},
				},
			},
			expected: 1,
		},
		{
			name: "has implementation step",
			task: &domain.Task{
				Steps: []domain.Step{
					{Name: "plan"},
					{Name: "implementation"},
					{Name: "test"},
				},
			},
			expected: 1,
		},
		{
			name: "has code step",
			task: &domain.Task{
				Steps: []domain.Step{
					{Name: "design"},
					{Name: "code"},
					{Name: "review"},
				},
			},
			expected: 1,
		},
		{
			name: "has develop step",
			task: &domain.Task{
				Steps: []domain.Step{
					{Name: "setup"},
					{Name: "develop"},
					{Name: "deploy"},
				},
			},
			expected: 1,
		},
		{
			name: "case insensitive",
			task: &domain.Task{
				Steps: []domain.Step{
					{Name: "analyze"},
					{Name: "IMPLEMENT"},
					{Name: "validate"},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := findDefaultResumeStep(tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSaveRejectionFeedback tests saving rejection feedback as artifact.
func TestSaveRejectionFeedback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{},
		artifacts: make(map[string][]byte),
	}

	err := saveRejectionFeedback(ctx, mockStore, "test-ws", "task-1", "Fix the authentication", 2)
	require.NoError(t, err)

	// Verify artifact was saved
	key := "test-ws:task-1:rejection-feedback.md"
	data, ok := mockStore.artifacts[key]
	require.True(t, ok, "artifact should be saved")

	content := string(data)
	assert.Contains(t, content, "Rejection Feedback")
	assert.Contains(t, content, "Fix the authentication")
	assert.Contains(t, content, "Resume From: Step 3") // resumeStep + 1
}

// TestSaveRejectionFeedback_Error tests error handling when saving fails.
func TestSaveRejectionFeedback_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockStore := &mockTaskStoreForReject{
		saveArtErr: atlaserrors.ErrArtifactNotFound,
	}

	err := saveRejectionFeedback(ctx, mockStore, "test-ws", "task-1", "feedback", 0)
	require.Error(t, err)
}

// TestAwaitingTaskStructure tests the awaitingTask struct fields.
// The selectWorkspaceForReject function requires interactive TUI input and cannot be unit tested.
func TestAwaitingTaskStructure(t *testing.T) {
	t.Parallel()

	ws := &domain.Workspace{Name: "test-ws", Branch: "feat/test"}
	task := &domain.Task{ID: "task-1", Description: "Test task"}

	at := awaitingTask{
		workspace: ws,
		task:      task,
	}

	assert.Equal(t, "test-ws", at.workspace.Name)
	assert.Equal(t, "feat/test", at.workspace.Branch)
	assert.Equal(t, "task-1", at.task.ID)
	assert.Equal(t, "Test task", at.task.Description)
}

// TestRejectOptions_Structure tests rejectOptions structure.
func TestRejectOptions_Structure(t *testing.T) {
	t.Parallel()

	opts := rejectOptions{
		workspace: "my-workspace",
		retry:     true,
		done:      false,
		feedback:  "Fix the issue",
		step:      3,
	}

	assert.Equal(t, "my-workspace", opts.workspace)
	assert.True(t, opts.retry)
	assert.False(t, opts.done)
	assert.Equal(t, "Fix the issue", opts.feedback)
	assert.Equal(t, 3, opts.step)
}

// TestRunReject_ContextCancellation tests context cancellation handling.
func TestRunReject_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	rejectCmd := &cobra.Command{Use: "reject"}
	rootCmd.AddCommand(rejectCmd)

	opts := &rejectOptions{}
	err := runReject(ctx, rejectCmd, &buf, opts)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestRunReject_JSONModeRequiresWorkspace tests JSON mode workspace requirement.
func TestRunReject_JSONModeRequiresWorkspace(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	flags := &GlobalFlags{Output: OutputJSON}
	AddGlobalFlags(rootCmd, flags)
	_ = rootCmd.PersistentFlags().Set("output", "json")

	rejectCmd := &cobra.Command{Use: "reject"}
	rootCmd.AddCommand(rejectCmd)

	ctx := context.Background()
	opts := &rejectOptions{} // No workspace
	err := runReject(ctx, rejectCmd, &buf, opts)

	// Should return ErrJSONErrorOutput because error is written as JSON
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON error output
	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "workspace argument required")
}

// TestRunReject_JSONModeRequiresRetryOrDone tests JSON mode requires --retry or --done.
func TestRunReject_JSONModeRequiresRetryOrDone(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	flags := &GlobalFlags{Output: OutputJSON}
	AddGlobalFlags(rootCmd, flags)
	_ = rootCmd.PersistentFlags().Set("output", "json")

	rejectCmd := &cobra.Command{Use: "reject"}
	rootCmd.AddCommand(rejectCmd)

	ctx := context.Background()
	opts := &rejectOptions{workspace: "test-ws"} // No --retry or --done
	err := runReject(ctx, rejectCmd, &buf, opts)

	// Should return ErrJSONErrorOutput
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON error output
	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "--retry or --done flag required")
}

// TestRunReject_JSONModeCannotUseBothRetryAndDone tests mutual exclusion of flags.
func TestRunReject_JSONModeCannotUseBothRetryAndDone(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	flags := &GlobalFlags{Output: OutputJSON}
	AddGlobalFlags(rootCmd, flags)
	_ = rootCmd.PersistentFlags().Set("output", "json")

	rejectCmd := &cobra.Command{Use: "reject"}
	rootCmd.AddCommand(rejectCmd)

	ctx := context.Background()
	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		done:      true, // Both set
	}
	err := runReject(ctx, rejectCmd, &buf, opts)

	// Should return ErrJSONErrorOutput
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON error output
	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "cannot use both --retry and --done")
}

// TestProcessJSONRejectRetry_RequiresFeedback tests that retry requires feedback.
func TestProcessJSONRejectRetry_RequiresFeedback(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "implement"}},
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "", // Empty feedback
		step:      0,
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "feedback required")
}

// TestProcessJSONRejectRetry_InvalidStep tests that retry validates step.
func TestProcessJSONRejectRetry_InvalidStep(t *testing.T) {
	t.Parallel()

	t.Run("step too high", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		ctx := context.Background()

		ws := &domain.Workspace{
			Name:         "test-ws",
			Branch:       "feat/test",
			WorktreePath: "/path/to/worktree",
		}
		task := &domain.Task{
			ID:          "task-1",
			WorkspaceID: "test-ws",
			Status:      constants.TaskStatusAwaitingApproval,
			Steps:       []domain.Step{{Name: "implement"}, {Name: "validate"}},
		}

		mockStore := &mockTaskStoreForReject{
			tasks:     map[string][]*domain.Task{"test-ws": {task}},
			artifacts: make(map[string][]byte),
		}

		opts := &rejectOptions{
			workspace: "test-ws",
			retry:     true,
			feedback:  "Fix the issue",
			step:      10, // Invalid step (out of range, 1-indexed)
		}

		err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
		require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

		var resp rejectResponse
		jsonErr := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, jsonErr)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "invalid step")
	})

	t.Run("negative step", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		ctx := context.Background()

		ws := &domain.Workspace{
			Name:         "test-ws",
			Branch:       "feat/test",
			WorktreePath: "/path/to/worktree",
		}
		task := &domain.Task{
			ID:          "task-1",
			WorkspaceID: "test-ws",
			Status:      constants.TaskStatusAwaitingApproval,
			Steps:       []domain.Step{{Name: "implement"}, {Name: "validate"}},
		}

		mockStore := &mockTaskStoreForReject{
			tasks:     map[string][]*domain.Task{"test-ws": {task}},
			artifacts: make(map[string][]byte),
		}

		opts := &rejectOptions{
			workspace: "test-ws",
			retry:     true,
			feedback:  "Fix the issue",
			step:      -1, // Invalid negative step
		}

		err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
		require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

		var resp rejectResponse
		jsonErr := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, jsonErr)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "invalid step")
	})
}

// TestProcessJSONRejectRetry_Success tests successful JSON retry rejection.
func TestProcessJSONRejectRetry_Success(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "analyze"}, {Name: "implement"}, {Name: "validate"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the authentication flow",
		step:      2, // 1-indexed: resume from step 2 (implement)
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.NoError(t, err)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.True(t, resp.Success)
	assert.Equal(t, "retry", resp.Action)
	assert.Equal(t, "test-ws", resp.WorkspaceName)
	assert.Equal(t, "task-1", resp.TaskID)
	assert.Equal(t, "Fix the authentication flow", resp.Feedback)
	assert.Equal(t, 2, resp.ResumeStep) // 1-indexed output

	// Verify task state (internal is 0-indexed)
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
	assert.Equal(t, 1, task.CurrentStep) // internal 0-indexed: step 2 -> index 1
	assert.Equal(t, "Fix the authentication flow", task.Metadata["rejection_feedback"])
}

// TestProcessJSONRejectDone_Success tests successful JSON done rejection.
func TestProcessJSONRejectDone_Success(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks: map[string][]*domain.Task{"test-ws": {task}},
	}

	err := processJSONRejectDone(ctx, &buf, mockStore, ws, task)
	require.NoError(t, err)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.True(t, resp.Success)
	assert.Equal(t, "done", resp.Action)
	assert.Equal(t, "test-ws", resp.WorkspaceName)
	assert.Equal(t, "task-1", resp.TaskID)
	assert.Equal(t, "feat/test", resp.BranchName)
	assert.Equal(t, "/path/to/worktree", resp.WorktreePath)

	// Verify task state
	assert.Equal(t, constants.TaskStatusRejected, task.Status)
}

// TestRejectCommand_Examples tests that command has examples.
func TestRejectCommand_Examples(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddRejectCommand(root)

	rejectCmd, _, err := root.Find([]string{"reject"})
	require.NoError(t, err)

	// Check examples in long description
	assert.Contains(t, rejectCmd.Long, "atlas reject")
	assert.Contains(t, rejectCmd.Long, "--output json")
	assert.Contains(t, rejectCmd.Long, "--retry")
	assert.Contains(t, rejectCmd.Long, "--done")
	assert.Contains(t, rejectCmd.Long, "--feedback")
	assert.Contains(t, rejectCmd.Long, "--step")
}

// TestRejectResponseFields tests that all expected fields are present in response.
func TestRejectResponseFields(t *testing.T) {
	t.Parallel()

	resp := rejectResponse{
		Success:       true,
		Action:        "retry",
		WorkspaceName: "test-ws",
		TaskID:        "task-1",
		Feedback:      "Fix the auth",
		ResumeStep:    2,
		BranchName:    "feat/test",
		WorktreePath:  "/path/to/worktree",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify all expected fields are present
	jsonStr := string(data)
	expectedFields := []string{
		"success",
		"action",
		"workspace_name",
		"task_id",
		"feedback",
		"resume_step",
		"branch_name",
		"worktree_path",
	}

	for _, field := range expectedFields {
		assert.Contains(t, jsonStr, field, "JSON should contain field %q", field)
	}
}

// TestWorkspaceLookupInAwaitingTasks tests workspace name lookup logic.
// The findAndSelectTaskForReject function is tested indirectly through JSON mode tests.
func TestWorkspaceLookupInAwaitingTasks(t *testing.T) {
	t.Parallel()

	awaitingTasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "ws1", Branch: "feat/ws1"},
			task:      &domain.Task{ID: "task-1", Description: "Task 1"},
		},
		{
			workspace: &domain.Workspace{Name: "ws2", Branch: "feat/ws2"},
			task:      &domain.Task{ID: "task-2", Description: "Task 2"},
		},
	}

	t.Run("finds existing workspace", func(t *testing.T) {
		t.Parallel()
		var found *awaitingTask
		for i := range awaitingTasks {
			if awaitingTasks[i].workspace.Name == "ws2" {
				found = &awaitingTasks[i]
				break
			}
		}
		require.NotNil(t, found)
		assert.Equal(t, "ws2", found.workspace.Name)
		assert.Equal(t, "task-2", found.task.ID)
	})

	t.Run("returns nil for nonexistent workspace", func(t *testing.T) {
		t.Parallel()
		var found *awaitingTask
		for i := range awaitingTasks {
			if awaitingTasks[i].workspace.Name == "nonexistent" {
				found = &awaitingTasks[i]
				break
			}
		}
		assert.Nil(t, found)
	})
}

// TestRejectAction_String tests rejectAction string conversion.
func TestRejectAction_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "retry", string(rejectActionRetry))
	assert.Equal(t, "done", string(rejectActionDone))
}

// TestStateTransition_RejectToRunning tests task state transition on reject with retry.
func TestStateTransition_RejectToRunning(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "implement"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the issue",
		step:      0,
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.NoError(t, err)

	// Verify state changed to running
	assert.Equal(t, constants.TaskStatusRunning, task.Status)

	// Verify transition was recorded
	assert.Len(t, task.Transitions, 1)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Transitions[0].FromStatus)
	assert.Equal(t, constants.TaskStatusRunning, task.Transitions[0].ToStatus)
}

// TestStateTransition_RejectToRejected tests task state transition on reject done.
func TestStateTransition_RejectToRejected(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks: map[string][]*domain.Task{"test-ws": {task}},
	}

	err := processJSONRejectDone(ctx, &buf, mockStore, ws, task)
	require.NoError(t, err)

	// Verify state changed to rejected
	assert.Equal(t, constants.TaskStatusRejected, task.Status)

	// Verify transition was recorded
	assert.Len(t, task.Transitions, 1)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Transitions[0].FromStatus)
	assert.Equal(t, constants.TaskStatusRejected, task.Transitions[0].ToStatus)
}

// TestFeedbackArtifact_Format tests the format of saved feedback artifact.
func TestFeedbackArtifact_Format(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{},
		artifacts: make(map[string][]byte),
	}

	feedback := `The authentication flow should use OAuth2 instead of basic auth.
Please also add proper error handling for network timeouts.`

	err := saveRejectionFeedback(ctx, mockStore, "payment", "task-abc", feedback, 2)
	require.NoError(t, err)

	// Verify artifact content
	key := "payment:task-abc:rejection-feedback.md"
	data := mockStore.artifacts[key]
	content := string(data)

	assert.Contains(t, content, "# Rejection Feedback")
	assert.Contains(t, content, "Date:")
	assert.Contains(t, content, "Resume From: Step 3")
	assert.Contains(t, content, "## Feedback")
	assert.Contains(t, content, "OAuth2")
	assert.Contains(t, content, "network timeouts")
}

// TestProcessJSONReject_RoutesToRetry tests routing to retry path.
func TestProcessJSONReject_RoutesToRetry(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "implement"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the issue",
		step:      0,
	}

	err := processJSONReject(ctx, &buf, mockStore, ws, task, opts)
	require.NoError(t, err)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.True(t, resp.Success)
	assert.Equal(t, "retry", resp.Action)
}

// TestProcessJSONReject_RoutesToDone tests routing to done path.
func TestProcessJSONReject_RoutesToDone(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks: map[string][]*domain.Task{"test-ws": {task}},
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		done:      true,
	}

	err := processJSONReject(ctx, &buf, mockStore, ws, task, opts)
	require.NoError(t, err)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.True(t, resp.Success)
	assert.Equal(t, "done", resp.Action)
}

// TestProcessJSONRejectDone_TransitionError tests transition failure.
func TestProcessJSONRejectDone_TransitionError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	// Task with invalid status for transition
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCompleted, // Cannot transition from completed to rejected
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks: map[string][]*domain.Task{"test-ws": {task}},
	}

	err := processJSONRejectDone(ctx, &buf, mockStore, ws, task)
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "failed to transition task")
}

// TestProcessJSONRejectDone_UpdateError tests update failure.
func TestProcessJSONRejectDone_UpdateError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		updateErr: atlaserrors.ErrTaskNotFound,
	}

	err := processJSONRejectDone(ctx, &buf, mockStore, ws, task)
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "failed to save task")
}

// TestProcessJSONRejectRetry_AutoSelectStep tests auto-selection of step (step=0).
func TestProcessJSONRejectRetry_AutoSelectStep(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "analyze"}, {Name: "implement"}, {Name: "validate"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the issue",
		step:      0, // Auto-select
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.NoError(t, err)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.True(t, resp.Success)
	assert.Equal(t, 2, resp.ResumeStep) // Should auto-select "implement" step (index 1, displayed as 2)
}

// TestProcessJSONRejectRetry_SaveArtifactError tests artifact save failure.
func TestProcessJSONRejectRetry_SaveArtifactError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "implement"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:      map[string][]*domain.Task{"test-ws": {task}},
		saveArtErr: atlaserrors.ErrArtifactNotFound,
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the issue",
		step:      1,
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "failed to save feedback")
}

// TestProcessJSONRejectRetry_TransitionError tests transition failure.
func TestProcessJSONRejectRetry_TransitionError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCompleted, // Invalid status for transition
		Steps:       []domain.Step{{Name: "implement"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the issue",
		step:      1,
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "failed to transition task")
}

// TestProcessJSONRejectRetry_UpdateError tests update failure.
func TestProcessJSONRejectRetry_UpdateError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/path/to/worktree",
	}
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       []domain.Step{{Name: "implement"}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForReject{
		tasks:     map[string][]*domain.Task{"test-ws": {task}},
		artifacts: make(map[string][]byte),
		updateErr: atlaserrors.ErrTaskNotFound,
	}

	opts := &rejectOptions{
		workspace: "test-ws",
		retry:     true,
		feedback:  "Fix the issue",
		step:      1,
	}

	err := processJSONRejectRetry(ctx, &buf, mockStore, ws, task, opts)
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var resp rejectResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "failed to save task")
}
