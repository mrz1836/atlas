// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/workspace"
)

// createTestWorkspaceWithLogs creates a test workspace with a task and log file.
//
//nolint:unparam // wsName varies in some tests via direct store.Create
func createTestWorkspaceWithLogs(t *testing.T, tmpDir, wsName string, tasks []domain.TaskRef, logContent string) {
	t.Helper()

	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      wsName,
		Status:    constants.WorkspaceStatusActive,
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create task directory and log file for each task
	for _, task := range tasks {
		taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, wsName, constants.TasksDir, task.ID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		if logContent != "" {
			logPath := filepath.Join(taskDir, constants.TaskLogFileName)
			require.NoError(t, os.WriteFile(logPath, []byte(logContent), 0o600))
		}
	}
}

func TestRunWorkspaceLogs_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"test log","step_name":"implement"}
{"ts":"2025-12-27T10:01:00Z","level":"error","event":"test error","step_name":"validate","error":"lint failed"}
`
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "implement")
	assert.Contains(t, output, "test log")
	assert.Contains(t, output, "validate")
	assert.Contains(t, output, "test error")
}

func TestRunWorkspaceLogs_NoTasks(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "empty-ws",
		Status:    constants.WorkspaceStatusActive,
		Tasks:     []domain.TaskRef{}, // No tasks
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err = runWorkspaceLogs(context.Background(), cmd, &buf, "empty-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "No logs found for workspace 'empty-ws'")
}

func TestRunWorkspaceLogs_ClosedWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"closed workspace log","step_name":"implement"}
`
	// Create workspace
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "closed-ws",
		Status:    constants.WorkspaceStatusClosed, // Closed status
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create task directory and log file
	taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "closed-ws", constants.TasksDir, "task-20251227-100000")
	require.NoError(t, os.MkdirAll(taskDir, 0o750))
	logPath := filepath.Join(taskDir, constants.TaskLogFileName)
	require.NoError(t, os.WriteFile(logPath, []byte(logContent), 0o600))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err = runWorkspaceLogs(context.Background(), cmd, &buf, "closed-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	// Logs should still be viewable for closed workspace
	assert.Contains(t, buf.String(), "closed workspace log")
}

func TestRunWorkspaceLogs_WorkspaceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "nonexistent", logsOptions{}, tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Workspace 'nonexistent' not found")
}

func TestRunWorkspaceLogs_TaskFilter(t *testing.T) {
	tmpDir := t.TempDir()

	time1 := time.Now().Add(-2 * time.Hour)
	time2 := time.Now().Add(-1 * time.Hour)
	tasks := []domain.TaskRef{
		{ID: "task-older", Status: constants.TaskStatusCompleted, StartedAt: &time1},
		{ID: "task-newer", Status: constants.TaskStatusCompleted, StartedAt: &time2},
	}

	// Create workspace with two tasks
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "multi-task-ws",
		Status:    constants.WorkspaceStatusActive,
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create log files for each task
	for i, task := range tasks {
		taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "multi-task-ws", constants.TasksDir, task.ID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		logPath := filepath.Join(taskDir, constants.TaskLogFileName)
		var logContent string
		if i == 0 {
			logContent = `{"ts":"2025-12-27T08:00:00Z","level":"info","event":"older task log","step_name":"implement"}
`
		} else {
			logContent = `{"ts":"2025-12-27T09:00:00Z","level":"info","event":"newer task log","step_name":"implement"}
`
		}
		require.NoError(t, os.WriteFile(logPath, []byte(logContent), 0o600))
	}

	// Test showing specific task logs
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err = runWorkspaceLogs(context.Background(), cmd, &buf, "multi-task-ws", logsOptions{
		taskID: "task-older",
	}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "older task log")
	assert.NotContains(t, output, "newer task log")
}

func TestRunWorkspaceLogs_TaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "existing-task", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"log"}
`)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{
		taskID: "non-existent-task",
	}, tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Task 'non-existent-task' not found")
}

func TestRunWorkspaceLogs_StepFilter(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"implement started","step_name":"implement"}
{"ts":"2025-12-27T10:01:00Z","level":"info","event":"implement complete","step_name":"implement"}
{"ts":"2025-12-27T10:02:00Z","level":"info","event":"validate started","step_name":"validate"}
{"ts":"2025-12-27T10:03:00Z","level":"error","event":"validate failed","step_name":"validate","error":"lint failed"}
`
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{
		stepName: "validate",
	}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "validate started")
	assert.Contains(t, output, "validate failed")
	assert.NotContains(t, output, "implement started")
	assert.NotContains(t, output, "implement complete")
}

func TestRunWorkspaceLogs_StepFilterCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"Implement started","step_name":"Implement"}
{"ts":"2025-12-27T10:01:00Z","level":"info","event":"validate started","step_name":"validate"}
`
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	// Search with lowercase should find capitalized step name
	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{
		stepName: "impl",
	}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Implement started")
	assert.NotContains(t, output, "validate started")
}

func TestRunWorkspaceLogs_StepNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"implement started","step_name":"implement"}
`
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{
		stepName: "nonexistent-step",
	}, tmpDir)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "No logs found for step 'nonexistent-step'")
}

func TestRunWorkspaceLogs_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"test log","step_name":"implement"}
{"ts":"2025-12-27T10:01:00Z","level":"error","event":"test error","step_name":"validate"}
`
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer

	err := runWorkspaceLogsWithOutput(context.Background(), &buf, "test-ws", logsOptions{}, tmpDir, OutputJSON)
	require.NoError(t, err)

	output := buf.String()
	// Should be valid JSON array
	assert.True(t, strings.HasPrefix(strings.TrimSpace(output), "["))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(output), "]"))
	assert.Contains(t, output, "test log")
	assert.Contains(t, output, "test error")
}

func TestRunWorkspaceLogs_JSONOutputEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "empty-ws",
		Status:    constants.WorkspaceStatusActive,
		Tasks:     []domain.TaskRef{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	err = runWorkspaceLogsWithOutput(context.Background(), &buf, "empty-ws", logsOptions{}, tmpDir, OutputJSON)
	require.NoError(t, err)

	assert.Equal(t, "[]\n", buf.String())
}

func TestRunWorkspaceLogs_TailFlag(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	// Create log with 10 lines
	var logLines []string
	for i := 0; i < 10; i++ {
		logLines = append(logLines, `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"line `+string(rune('0'+i))+`","step_name":"test"}`)
	}
	logContent := strings.Join(logLines, "\n") + "\n"
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{
		tail: 3,
	}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	// Should only have last 3 lines
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 3)
}

func TestRunWorkspaceLogs_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"log"}
`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(ctx, cmd, &buf, "test-ws", logsOptions{}, tmpDir)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestRunWorkspaceLogs_MostRecentTask(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two tasks with different start times
	time1 := time.Now().Add(-2 * time.Hour)
	time2 := time.Now().Add(-1 * time.Hour)
	tasks := []domain.TaskRef{
		{ID: "task-older", Status: constants.TaskStatusCompleted, StartedAt: &time1},
		{ID: "task-newer", Status: constants.TaskStatusCompleted, StartedAt: &time2},
	}

	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "multi-task-ws",
		Status:    constants.WorkspaceStatusActive,
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create log files for each task
	for i, task := range tasks {
		taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "multi-task-ws", constants.TasksDir, task.ID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		logPath := filepath.Join(taskDir, constants.TaskLogFileName)
		var logContent string
		if i == 0 {
			logContent = `{"ts":"2025-12-27T08:00:00Z","level":"info","event":"older task log","step_name":"implement"}
`
		} else {
			logContent = `{"ts":"2025-12-27T09:00:00Z","level":"info","event":"newer task log","step_name":"implement"}
`
		}
		require.NoError(t, os.WriteFile(logPath, []byte(logContent), 0o600))
	}

	// Without specifying a task, should show most recent
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err = runWorkspaceLogs(context.Background(), cmd, &buf, "multi-task-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "newer task log")
	assert.NotContains(t, output, "older task log")
}

func TestRunWorkspaceLogs_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	// Log content with invalid JSON lines mixed in
	logContent := `{"ts":"2025-12-27T10:00:00Z","level":"info","event":"valid log 1","step_name":"implement"}
invalid json here
{"ts":"2025-12-27T10:01:00Z","level":"info","event":"valid log 2","step_name":"implement"}
`
	createTestWorkspaceWithLogs(t, tmpDir, "test-ws", tasks, logContent)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err := runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "valid log 1")
	assert.Contains(t, output, "valid log 2")
	// Invalid JSON line should be shown as-is
	assert.Contains(t, output, "invalid json here")
}

func TestRunWorkspaceLogs_EmptyLogFile(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	// Create workspace with empty log file
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "test-ws",
		Status:    constants.WorkspaceStatusActive,
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create task directory with empty log file
	taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, "task-20251227-100000")
	require.NoError(t, os.MkdirAll(taskDir, 0o750))
	logPath := filepath.Join(taskDir, constants.TaskLogFileName)
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o600)) // Empty file

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err = runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	// Empty log file should produce no output (empty formatted output)
	assert.Empty(t, strings.TrimSpace(buf.String()))
}

func TestRunWorkspaceLogs_NoLogFile(t *testing.T) {
	tmpDir := t.TempDir()

	startTime := time.Now()
	tasks := []domain.TaskRef{
		{ID: "task-20251227-100000", Status: constants.TaskStatusCompleted, StartedAt: &startTime},
	}

	// Create workspace without log file (just the task directory)
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "test-ws",
		Status:    constants.WorkspaceStatusActive,
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create task directory but NO log file
	taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, "task-20251227-100000")
	require.NoError(t, os.MkdirAll(taskDir, 0o750))
	// Intentionally no log file created

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")

	err = runWorkspaceLogs(context.Background(), cmd, &buf, "test-ws", logsOptions{}, tmpDir)
	require.NoError(t, err)

	// Should show no logs found message
	assert.Contains(t, buf.String(), "No logs found for workspace 'test-ws'")
}

func TestRunWorkspaceLogs_JSONOutputError(t *testing.T) {
	tmpDir := t.TempDir()

	var buf bytes.Buffer

	err := runWorkspaceLogsWithOutput(context.Background(), &buf, "nonexistent", logsOptions{}, tmpDir, OutputJSON)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, `"status"`)
	assert.Contains(t, output, `"error"`)
	assert.Contains(t, output, "workspace not found")
}

func TestFormatLogLine(t *testing.T) {
	styles := newLogStyles()

	tests := []struct {
		name     string
		line     []byte
		contains []string
	}{
		{
			name:     "info level",
			line:     []byte(`{"ts":"2025-12-27T10:00:00Z","level":"info","event":"test event","step_name":"implement"}`),
			contains: []string{"INFO", "implement", "test event"},
		},
		{
			name:     "error level with error field",
			line:     []byte(`{"ts":"2025-12-27T10:00:00Z","level":"error","event":"failed","step_name":"validate","error":"lint error"}`),
			contains: []string{"ERROR", "validate", "failed", "lint error"},
		},
		{
			name:     "with duration",
			line:     []byte(`{"ts":"2025-12-27T10:00:00Z","level":"info","event":"complete","step_name":"test","duration_ms":5000}`),
			contains: []string{"INFO", "5000ms"},
		},
		{
			name:     "invalid JSON",
			line:     []byte(`not valid json`),
			contains: []string{"not valid json"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatLogLine(tc.line, styles)
			for _, expected := range tc.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestFilterByStep(t *testing.T) {
	lines := [][]byte{
		[]byte(`{"ts":"2025-12-27T10:00:00Z","level":"info","event":"e1","step_name":"implement"}`),
		[]byte(`{"ts":"2025-12-27T10:01:00Z","level":"info","event":"e2","step_name":"validate"}`),
		[]byte(`{"ts":"2025-12-27T10:02:00Z","level":"info","event":"e3","step_name":"implement"}`),
		[]byte(`{"ts":"2025-12-27T10:03:00Z","level":"info","event":"e4","step_name":"Validate"}`),
	}

	// Filter for implement
	filtered := filterByStep(lines, "implement")
	assert.Len(t, filtered, 2)

	// Filter for validate (case-insensitive)
	filtered = filterByStep(lines, "val")
	assert.Len(t, filtered, 2)

	// Filter for non-existent
	filtered = filterByStep(lines, "nonexistent")
	assert.Empty(t, filtered)
}

func TestFindMostRecentTask(t *testing.T) {
	time1 := time.Now().Add(-3 * time.Hour)
	time2 := time.Now().Add(-1 * time.Hour)
	time3 := time.Now().Add(-2 * time.Hour)

	tests := []struct {
		name     string
		tasks    []domain.TaskRef
		expected string
	}{
		{
			name:     "empty tasks",
			tasks:    []domain.TaskRef{},
			expected: "",
		},
		{
			name: "single task",
			tasks: []domain.TaskRef{
				{ID: "task-1", StartedAt: &time1},
			},
			expected: "task-1",
		},
		{
			name: "multiple tasks",
			tasks: []domain.TaskRef{
				{ID: "task-oldest", StartedAt: &time1},
				{ID: "task-newest", StartedAt: &time2},
				{ID: "task-middle", StartedAt: &time3},
			},
			expected: "task-newest",
		},
		{
			name: "task with nil StartedAt",
			tasks: []domain.TaskRef{
				{ID: "task-nil", StartedAt: nil},
				{ID: "task-has-time", StartedAt: &time1},
			},
			expected: "task-has-time",
		},
		{
			name: "all nil StartedAt - uses ID as tiebreaker",
			tasks: []domain.TaskRef{
				{ID: "task-aaa", StartedAt: nil},
				{ID: "task-zzz", StartedAt: nil},
				{ID: "task-mmm", StartedAt: nil},
			},
			expected: "task-zzz", // Lexicographically largest ID
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := findMostRecentTask(tc.tasks)
			if tc.expected == "" {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tc.expected, result.ID)
			}
		})
	}
}

func TestGetTaskLogPath(t *testing.T) {
	tmpDir := "/tmp/test-atlas"

	path, err := getTaskLogPath(tmpDir, "my-workspace", "task-123")
	require.NoError(t, err)
	expected := filepath.Join(tmpDir, constants.WorkspacesDir, "my-workspace", constants.TasksDir, "task-123", constants.TaskLogFileName)
	assert.Equal(t, expected, path)
}

func TestGetTaskLogPath_DefaultBaseDir(t *testing.T) {
	// When storeBaseDir is empty, should use home directory
	path, err := getTaskLogPath("", "my-workspace", "task-123")
	require.NoError(t, err)
	assert.Contains(t, path, "my-workspace")
	assert.Contains(t, path, "task-123")
}

func TestFollowLogs_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create initial log file
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	styles := newLogStyles()

	// Start followLogs in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- followLogs(ctx, logPath, &buf, styles, "", "")
	}()

	// Wait for followLogs to start and seek to end
	time.Sleep(100 * time.Millisecond)

	// Append new content to the log file
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600) //#nosec G304 -- test file path
	require.NoError(t, err)
	_, err = f.WriteString(`{"ts":"2025-12-27T10:00:00Z","level":"info","event":"new log entry","step_name":"test"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Wait for poll cycle (500ms) plus buffer
	time.Sleep(700 * time.Millisecond)

	// Cancel context to stop followLogs
	cancel()

	// Wait for followLogs to complete
	err = <-done
	require.NoError(t, err)

	// Verify output contains the new log entry
	output := buf.String()
	assert.Contains(t, output, "Watching for new log entries")
	assert.Contains(t, output, "new log entry")
}

func TestFollowLogs_WithStepFilter(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create initial log file
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	styles := newLogStyles()

	// Start followLogs with step filter
	done := make(chan error, 1)
	go func() {
		done <- followLogs(ctx, logPath, &buf, styles, "", "validate")
	}()

	// Wait for followLogs to start
	time.Sleep(100 * time.Millisecond)

	// Append log entries - one matching, one not matching
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600) //#nosec G304 -- test file path
	require.NoError(t, err)
	_, err = f.WriteString(`{"ts":"2025-12-27T10:00:00Z","level":"info","event":"should be filtered","step_name":"implement"}` + "\n")
	require.NoError(t, err)
	_, err = f.WriteString(`{"ts":"2025-12-27T10:00:01Z","level":"info","event":"should appear","step_name":"validate"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Wait for poll cycle
	time.Sleep(700 * time.Millisecond)

	cancel()
	err = <-done
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Watching for new log entries (step: validate)")
	assert.Contains(t, output, "should appear")
	assert.NotContains(t, output, "should be filtered")
}

func TestLogStylesLevelColor(t *testing.T) {
	styles := newLogStyles()

	tests := []struct {
		level    string
		expected lipgloss.Style
	}{
		{"info", styles.info},
		{"INFO", styles.info},
		{"warn", styles.warn},
		{"warning", styles.warn},
		{"error", styles.errorSty},
		{"fatal", styles.errorSty},
		{"panic", styles.errorSty},
		{"debug", styles.debug},
		{"trace", styles.debug},
		{"unknown", styles.info}, // default
	}

	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			result := styles.levelColor(tc.level)
			// Compare style string representation
			assert.Equal(t, tc.expected.String(), result.String())
		})
	}
}
