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
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

func TestRunWorkspaceList_EmptyState(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()

	// Create store (empty)
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create manager
	mgr := workspace.NewManager(store, nil)

	// Verify empty list
	workspaces, err := mgr.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, workspaces)
}

func TestRunWorkspaceList_WithWorkspaces(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now.Add(-2 * time.Hour),
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create manager and list
	mgr := workspace.NewManager(store, nil)
	workspaces, err := mgr.List(context.Background())
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	assert.Equal(t, "test-ws", workspaces[0].Name)
	assert.Equal(t, "feat/test", workspaces[0].Branch)
	assert.Equal(t, constants.WorkspaceStatusActive, workspaces[0].Status)
}

func TestRunWorkspaceList_MultipleWorkspaces(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()

	// Create multiple workspaces
	workspaceData := []struct {
		name   string
		branch string
		status constants.WorkspaceStatus
		age    time.Duration
	}{
		{"auth", "feat/auth", constants.WorkspaceStatusActive, 2 * time.Hour},
		{"payment", "fix/payment", constants.WorkspaceStatusPaused, 24 * time.Hour},
		{"old-feat", "feat/old", constants.WorkspaceStatusClosed, 3 * 24 * time.Hour},
	}

	for _, data := range workspaceData {
		ws := &domain.Workspace{
			Name:         data.name,
			WorktreePath: "/tmp/" + data.name,
			Branch:       data.branch,
			Status:       data.status,
			Tasks:        []domain.TaskRef{},
			CreatedAt:    now.Add(-data.age),
			UpdatedAt:    now,
		}
		require.NoError(t, store.Create(context.Background(), ws))
	}

	// Create manager and list
	mgr := workspace.NewManager(store, nil)
	workspaces, err := mgr.List(context.Background())
	require.NoError(t, err)
	require.Len(t, workspaces, 3)
}

func TestOutputWorkspacesJSON(t *testing.T) {
	now := time.Now()
	workspaces := []*domain.Workspace{
		{
			Name:         "test-ws",
			Path:         "/tmp/atlas/workspaces/test-ws",
			WorktreePath: "/tmp/repo-test-ws",
			Branch:       "feat/test",
			Status:       constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{
				{ID: "task-1", Status: constants.TaskStatusCompleted},
			},
			CreatedAt:     now.Add(-2 * time.Hour),
			UpdatedAt:     now,
			SchemaVersion: 1,
		},
	}

	// Use buffer to capture output
	var buf bytes.Buffer
	err := outputWorkspacesJSON(&buf, workspaces)
	require.NoError(t, err)

	output := buf.String()

	// Verify valid JSON
	var parsed []domain.Workspace
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, "test-ws", parsed[0].Name)
	assert.Equal(t, "feat/test", parsed[0].Branch)
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "just now",
			input:    now.Add(-30 * time.Second),
			expected: "just now",
		},
		{
			name:     "1 minute ago",
			input:    now.Add(-1 * time.Minute),
			expected: "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			input:    now.Add(-5 * time.Minute),
			expected: "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			input:    now.Add(-1 * time.Hour),
			expected: "1 hour ago",
		},
		{
			name:     "2 hours ago",
			input:    now.Add(-2 * time.Hour),
			expected: "2 hours ago",
		},
		{
			name:     "1 day ago",
			input:    now.Add(-24 * time.Hour),
			expected: "1 day ago",
		},
		{
			name:     "3 days ago",
			input:    now.Add(-3 * 24 * time.Hour),
			expected: "3 days ago",
		},
		{
			name:     "1 week ago",
			input:    now.Add(-7 * 24 * time.Hour),
			expected: "1 week ago",
		},
		{
			name:     "2 weeks ago",
			input:    now.Add(-14 * 24 * time.Hour),
			expected: "2 weeks ago",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tui.RelativeTime(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestWorkspaceListCommand_Integration(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Test using runWorkspaceList directly with a buffer
	var buf bytes.Buffer

	// Create a mock command to get the output flag
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	listCmd := &cobra.Command{Use: "list"}
	rootCmd.AddCommand(listCmd)

	// Execute with buffer
	err := runWorkspaceList(context.Background(), listCmd, &buf)
	require.NoError(t, err)

	// Verify empty message
	assert.Contains(t, buf.String(), "No workspaces")
}

func TestWorkspaceListCommand_JSONOutput(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Test using runWorkspaceList directly with a buffer
	var buf bytes.Buffer

	// Create a mock command with output flag set to json
	rootCmd := &cobra.Command{Use: "atlas"}
	flags := &GlobalFlags{Output: OutputJSON}
	AddGlobalFlags(rootCmd, flags)

	listCmd := &cobra.Command{Use: "list"}
	rootCmd.AddCommand(listCmd)

	// Set the output flag value
	_ = rootCmd.PersistentFlags().Set("output", "json")

	// Execute with buffer
	err := runWorkspaceList(context.Background(), listCmd, &buf)
	require.NoError(t, err)

	// Should output empty JSON array
	assert.Equal(t, "[]\n", buf.String())
}

func TestWorkspaceListCommand_Alias(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Verify 'ls' alias exists
	wsCmd, _, err := rootCmd.Find([]string{"workspace", "ls"})
	require.NoError(t, err)
	assert.NotNil(t, wsCmd)
	assert.Equal(t, "list", wsCmd.Name())
}

func TestRunWorkspaceList_ContextCancellation(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	listCmd := &cobra.Command{Use: "list"}
	rootCmd.AddCommand(listCmd)

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with canceled context
	err := runWorkspaceList(ctx, listCmd, &buf)

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestColorOffset(t *testing.T) {
	tests := []struct {
		name     string
		rendered string
		plain    string
		expected int
	}{
		{
			name:     "no color",
			rendered: "active",
			plain:    "active",
			expected: 0,
		},
		{
			name:     "with ANSI codes",
			rendered: "\x1b[34mactive\x1b[0m",
			plain:    "active",
			expected: 9, // len("\x1b[34m") + len("\x1b[0m") = 5 + 4 = 9
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tui.ColorOffset(tc.rendered, tc.plain)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOutputWorkspacesTable(t *testing.T) {
	now := time.Now()
	workspaces := []*domain.Workspace{
		{
			Name:         "test-ws",
			WorktreePath: "/tmp/test",
			Branch:       "feat/test",
			Status:       constants.WorkspaceStatusActive,
			Tasks:        []domain.TaskRef{},
			CreatedAt:    now.Add(-2 * time.Hour),
			UpdatedAt:    now,
		},
	}

	// Set NO_COLOR to ensure consistent output
	t.Setenv("NO_COLOR", "1")

	// Use buffer to capture output
	var buf bytes.Buffer
	err := outputWorkspacesTable(&buf, workspaces)
	require.NoError(t, err)

	output := buf.String()

	// Verify table structure
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "CREATED")
	assert.Contains(t, output, "TASKS")
	assert.Contains(t, output, "test-ws")
	assert.Contains(t, output, "feat/test")
	assert.Contains(t, output, "active")
}

func TestStatusColors(t *testing.T) {
	// Verify all workspace statuses have colors defined
	statuses := []constants.WorkspaceStatus{
		constants.WorkspaceStatusActive,
		constants.WorkspaceStatusPaused,
		constants.WorkspaceStatusClosed,
	}

	colors := getStatusColors()
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			color, ok := colors[status]
			assert.True(t, ok, "color should be defined for status %s", status)
			assert.NotEmpty(t, color.Light, "light color should be defined")
			assert.NotEmpty(t, color.Dark, "dark color should be defined")
		})
	}
}
