package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

func TestSanitizeWorkspaceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple description",
			input:    "fix login bug",
			expected: "fix-login-bug",
		},
		{
			name:     "with special characters",
			input:    "fix null pointer in parseConfig()",
			expected: "fix-null-pointer-in-parseconfig",
		},
		{
			name:     "uppercase conversion",
			input:    "Add User Authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "multiple spaces",
			input:    "fix    multiple   spaces",
			expected: "fix-multiple-spaces",
		},
		{
			name:     "leading/trailing spaces",
			input:    "  fix bug  ",
			expected: "fix-bug",
		},
		{
			name:     "numbers preserved",
			input:    "add feature v2",
			expected: "add-feature-v2",
		},
		{
			name:     "hyphens preserved",
			input:    "fix-this-bug",
			expected: "fix-this-bug",
		},
		{
			name:     "emojis removed",
			input:    "fix 🐛 bug",
			expected: "fix-bug",
		},
		{
			name:     "unicode removed",
			input:    "\u4fee\u590d bug",
			expected: "bug",
		},
		{
			name:     "truncate long names",
			input:    "this is a very long description that should be truncated to fit within the maximum workspace name length limit",
			expected: "this-is-a-very-long-description-that-should-be-tru",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "!!!@@@###",
			expected: "",
		},
		{
			name:     "special chars with spaces",
			input:    "fix: parse error (URGENT)",
			expected: "fix-parse-error-urgent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeWorkspaceName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateWorkspaceName(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expectEmpty bool
	}{
		{
			name:        "normal description",
			description: "fix login bug",
			expectEmpty: false,
		},
		{
			name:        "empty description generates timestamp",
			description: "",
			expectEmpty: false,
		},
		{
			name:        "only special chars generates timestamp",
			description: "!!!@@@",
			expectEmpty: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := generateWorkspaceName(tc.description)
			assert.NotEmpty(t, result, "workspace name should never be empty")

			if tc.description == "" || sanitizeWorkspaceName(tc.description) == "" {
				// Should be a timestamp-based name
				assert.True(t, strings.HasPrefix(result, "task-"),
					"empty description should generate task-TIMESTAMP name")
			}
		})
	}
}

func TestSelectTemplate_WithFlag(t *testing.T) {
	registry := template.NewDefaultRegistry()

	tmpl, err := selectTemplate(context.Background(), registry, "bugfix", false, "text")
	require.NoError(t, err)
	assert.Equal(t, "bugfix", tmpl.Name)
}

func TestSelectTemplate_InvalidTemplate(t *testing.T) {
	registry := template.NewDefaultRegistry()

	_, err := selectTemplate(context.Background(), registry, "nonexistent", false, "text")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrTemplateNotFound)
}

func TestSelectTemplate_NonInteractiveMode(t *testing.T) {
	registry := template.NewDefaultRegistry()

	// No template specified in non-interactive mode
	_, err := selectTemplate(context.Background(), registry, "", true, "text")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrTemplateRequired)
}

func TestSelectTemplate_JSONOutputMode(t *testing.T) {
	registry := template.NewDefaultRegistry()

	// No template specified with JSON output
	_, err := selectTemplate(context.Background(), registry, "", false, "json")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrTemplateRequired)
}

func TestSelectTemplate_ContextCancellation(t *testing.T) {
	registry := template.NewDefaultRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := selectTemplate(ctx, registry, "bugfix", false, "text")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFindGitRepository(t *testing.T) {
	// Save current directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	t.Run("in git repo", func(t *testing.T) {
		// We're already in a git repo (the atlas project)
		repoPath, err := findGitRepository(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, repoPath)

		// Verify .git exists
		gitPath := filepath.Join(repoPath, ".git")
		_, err = os.Stat(gitPath)
		require.NoError(t, err)
	})

	t.Run("not in git repo", func(t *testing.T) {
		// Create temp directory outside git
		tmpDir := t.TempDir()
		require.NoError(t, os.Chdir(tmpDir))

		_, err := findGitRepository(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrNotGitRepo)
	})
}

func TestHandleWorkspaceConflict_NoConflict(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)
	mgr := workspace.NewManager(store, nil)

	var buf bytes.Buffer

	result, err := handleWorkspaceConflict(
		context.Background(),
		mgr,
		"new-workspace",
		false,
		"text",
		nil,
		&buf,
	)
	require.NoError(t, err)
	assert.Equal(t, "new-workspace", result)
}

func TestHandleWorkspaceConflict_ExistsNonInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create existing workspace
	ws := &domain.Workspace{
		Name:      "existing-ws",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	mgr := workspace.NewManager(store, nil)
	var buf bytes.Buffer

	// Non-interactive mode should fail
	_, err = handleWorkspaceConflict(
		context.Background(),
		mgr,
		"existing-ws",
		true,
		"text",
		nil,
		&buf,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrWorkspaceExists)
}

func TestHandleWorkspaceConflict_ExistsJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create existing workspace
	ws := &domain.Workspace{
		Name:      "existing-ws",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Create(context.Background(), ws))

	mgr := workspace.NewManager(store, nil)
	var buf bytes.Buffer

	// JSON output mode should output JSON error
	_, _ = handleWorkspaceConflict(
		context.Background(),
		mgr,
		"existing-ws",
		false,
		"json",
		nil,
		&buf,
	)

	// Verify JSON error was output
	var resp startResponse
	err = json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "existing-ws")
}

func TestHandleWorkspaceConflict_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)
	mgr := workspace.NewManager(store, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	_, err = handleWorkspaceConflict(ctx, mgr, "any-ws", false, "text", nil, &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestStartResponse_JSONStructure(t *testing.T) {
	resp := startResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name:         "test-ws",
			Branch:       "feat/test",
			WorktreePath: "/tmp/repo-test-ws",
			Status:       "active",
		},
		Task: taskInfo{
			ID:           "task-20240101-120000",
			TemplateName: "bugfix",
			Description:  "fix login bug",
			Status:       "running",
			CurrentStep:  0,
			TotalSteps:   5,
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify structure
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.True(t, parsed["success"].(bool))
	assert.NotNil(t, parsed["workspace"])
	assert.NotNil(t, parsed["task"])

	ws := parsed["workspace"].(map[string]any)
	assert.Equal(t, "test-ws", ws["name"])
	assert.Equal(t, "feat/test", ws["branch"])

	task := parsed["task"].(map[string]any)
	assert.Equal(t, "task-20240101-120000", task["task_id"])
	assert.Equal(t, "bugfix", task["template_name"])
}

func TestStartResponse_ErrorCase(t *testing.T) {
	resp := startResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: "test-ws",
		},
		Task:  taskInfo{},
		Error: "workspace creation failed",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.False(t, parsed["success"].(bool))
	assert.Equal(t, "workspace creation failed", parsed["error"])
}

func TestOutputStartErrorJSON(t *testing.T) {
	var buf bytes.Buffer
	err := outputStartErrorJSON(&buf, "my-workspace", "", "something went wrong")
	require.NoError(t, err)

	var resp startResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))

	assert.False(t, resp.Success)
	assert.Equal(t, "my-workspace", resp.Workspace.Name)
	assert.Equal(t, "something went wrong", resp.Error)
}

func TestNewStartCmd(t *testing.T) {
	cmd := newStartCmd()

	assert.Equal(t, "start <description>", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify flags
	templateFlag := cmd.Flags().Lookup("template")
	require.NotNil(t, templateFlag)
	assert.Equal(t, "t", templateFlag.Shorthand)

	workspaceFlag := cmd.Flags().Lookup("workspace")
	require.NotNil(t, workspaceFlag)
	assert.Equal(t, "w", workspaceFlag.Shorthand)

	modelFlag := cmd.Flags().Lookup("model")
	require.NotNil(t, modelFlag)
	assert.Equal(t, "m", modelFlag.Shorthand)

	branchFlag := cmd.Flags().Lookup("branch")
	require.NotNil(t, branchFlag)
	assert.Equal(t, "b", branchFlag.Shorthand)
	assert.Contains(t, branchFlag.Usage, "Base branch")

	noInteractiveFlag := cmd.Flags().Lookup("no-interactive")
	require.NotNil(t, noInteractiveFlag)
}

func TestAddStartCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddStartCommand(root)

	// Verify start command was added
	startCmd, _, err := root.Find([]string{"start"})
	require.NoError(t, err)
	assert.NotNil(t, startCmd)
	assert.Equal(t, "start", startCmd.Name())
}

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid name",
			input:   "my-workspace",
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWorkspaceName(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, errors.ErrEmptyValue)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStartContext_HandleError_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	sc := &startContext{
		ctx:          context.Background(),
		outputFormat: "text",
		w:            &buf,
	}

	testErr := errors.ErrWorkspaceExists
	result := sc.handleError("test-ws", testErr)

	// Should return the error directly
	assert.Equal(t, testErr, result)
	// Buffer should be empty (no JSON output)
	assert.Empty(t, buf.String())
}

func TestStartContext_HandleError_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	sc := &startContext{
		ctx:          context.Background(),
		outputFormat: "json",
		w:            &buf,
	}

	testErr := errors.ErrWorkspaceExists
	err := sc.handleError("test-ws", testErr)

	// Should return nil (JSON was output)
	assert.NoError(t, err)

	// Buffer should contain JSON
	var resp startResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "test-ws", resp.Workspace.Name)
	assert.Contains(t, resp.Error, errors.ErrWorkspaceExists.Error())
}

func TestRunStart_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newStartCmd()
	cmd.SetContext(ctx)

	// Add output flag to root
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runStart(ctx, cmd, &buf, "test description", startOptions{
		templateName: "bugfix",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWorkspaceNameMaxLength(t *testing.T) {
	// Generate a very long string
	longDesc := strings.Repeat("a", 100)
	result := sanitizeWorkspaceName(longDesc)

	assert.LessOrEqual(t, len(result), maxWorkspaceNameLen,
		"workspace name should be truncated to max length")
}

func TestWorkspaceNameNoTrailingHyphen(t *testing.T) {
	// Create a description that would result in trailing hyphen after truncation
	desc := strings.Repeat("word-", 15) // Will be truncated with trailing hyphen
	result := sanitizeWorkspaceName(desc)

	assert.False(t, strings.HasSuffix(result, "-"),
		"workspace name should not end with hyphen")
}

func TestIsValidModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"valid sonnet", "sonnet", true},
		{"valid opus", "opus", true},
		{"valid haiku", "haiku", true},
		{"invalid model", "gpt-4", false},
		{"empty string", "", false},
		{"uppercase", "SONNET", false},
		{"mixed case", "Opus", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidModel(tc.model)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestValidateModel(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{"valid sonnet", "sonnet", false},
		{"valid opus", "opus", false},
		{"valid haiku", "haiku", false},
		{"empty is ok", "", false},
		{"invalid model", "gpt-4", true},
		{"invalid unknown", "claude-3", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateModel(tc.model)
			if tc.wantErr {
				require.Error(t, err)
				// Should be exit code 2 error
				assert.True(t, errors.IsExitCode2Error(err),
					"invalid model should return ExitCode2Error")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExitCode2Error_Integration(t *testing.T) {
	registry := template.NewDefaultRegistry()

	// Test that non-interactive mode without template returns exit code 2 error
	_, err := selectTemplate(context.Background(), registry, "", true, "text")
	require.Error(t, err)
	assert.True(t, errors.IsExitCode2Error(err),
		"missing template in non-interactive mode should return ExitCode2Error")
	assert.ErrorIs(t, err, errors.ErrTemplateRequired)
}

func TestExitCodeForError_WithExitCode2Error(t *testing.T) {
	// Test that ExitCode2Error returns exit code 2
	wrappedErr := errors.NewExitCode2Error(errors.ErrTemplateRequired)
	code := ExitCodeForError(wrappedErr)
	assert.Equal(t, ExitInvalidInput, code, "ExitCode2Error should return exit code 2")
}

func TestDisplayTaskStatus_TTYOutput(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/tmp/test-ws",
		Status:       constants.WorkspaceStatusActive,
	}

	task := &domain.Task{
		ID:          "task-123",
		TemplateID:  "bugfix",
		Description: "fix a bug",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 1,
		Steps:       make([]domain.Step, 5),
	}

	err := displayTaskStatus(out, "text", ws, task, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "task-123")
	assert.Contains(t, output, "test-ws")
	assert.Contains(t, output, "bugfix")
}

func TestDisplayTaskStatus_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "json")

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/tmp/test-ws",
		Status:       constants.WorkspaceStatusActive,
	}

	task := &domain.Task{
		ID:          "task-123",
		TemplateID:  "bugfix",
		Description: "fix a bug",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 1,
		Steps:       make([]domain.Step, 5),
	}

	err := displayTaskStatus(out, "json", ws, task, nil)
	require.NoError(t, err)

	var resp startResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "test-ws", resp.Workspace.Name)
	assert.Equal(t, "task-123", resp.Task.ID)
	assert.Equal(t, 5, resp.Task.TotalSteps)
}

func TestDisplayTaskStatus_WithError(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	ws := &domain.Workspace{
		Name:   "test-ws",
		Status: constants.WorkspaceStatusActive,
	}

	task := &domain.Task{
		ID:     "task-123",
		Status: constants.TaskStatusRunning,
		Steps:  make([]domain.Step, 3),
	}

	execErr := errors.ErrValidationFailed
	err := displayTaskStatus(out, "text", ws, task, execErr)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Execution paused")
}
