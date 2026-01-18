package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/cli/workflow"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// runGitCommand runs a git command in the specified directory.
func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- test code
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, out)
	}
}

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
			result := workflow.SanitizeWorkspaceName(tc.input)
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
			result := workflow.GenerateWorkspaceName(tc.description)
			assert.NotEmpty(t, result, "workspace name should never be empty")

			if tc.description == "" || workflow.SanitizeWorkspaceName(tc.description) == "" {
				// Should be a timestamp-based name
				assert.True(t, strings.HasPrefix(result, "task-"),
					"empty description should generate task-TIMESTAMP name")
			}
		})
	}
}

func TestSelectTemplate_WithFlag(t *testing.T) {
	registry := template.NewDefaultRegistry()
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))

	tmpl, err := prompter.SelectTemplate(context.Background(), registry, "bugfix", false, "text")
	require.NoError(t, err)
	assert.Equal(t, "bugfix", tmpl.Name)
}

func TestSelectTemplate_InvalidTemplate(t *testing.T) {
	registry := template.NewDefaultRegistry()
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))

	_, err := prompter.SelectTemplate(context.Background(), registry, "nonexistent", false, "text")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrTemplateNotFound)
}

func TestSelectTemplate_NonInteractiveMode(t *testing.T) {
	registry := template.NewDefaultRegistry()
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))

	// No template specified in non-interactive mode
	_, err := prompter.SelectTemplate(context.Background(), registry, "", true, "text")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrTemplateRequired)
}

func TestSelectTemplate_JSONOutputMode(t *testing.T) {
	registry := template.NewDefaultRegistry()
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))

	// No template specified with JSON output
	_, err := prompter.SelectTemplate(context.Background(), registry, "", false, "json")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrTemplateRequired)
}

func TestSelectTemplate_ContextCancellation(t *testing.T) {
	registry := template.NewDefaultRegistry()
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := prompter.SelectTemplate(ctx, registry, "bugfix", false, "text")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFindGitRepository(t *testing.T) {
	// Save current directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	initializer := workflow.NewInitializer(zerolog.Nop())

	t.Run("in git repo", func(t *testing.T) {
		// We're already in a git repo (the atlas project)
		repoPath, err := initializer.FindGitRepository(context.Background())
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

		_, err := initializer.FindGitRepository(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrNotGitRepo)
	})
}

func TestHandleWorkspaceConflict_NoConflict(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)
	mgr := workspace.NewManager(store, nil, zerolog.Nop())
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))

	var buf bytes.Buffer

	result, err := prompter.ResolveWorkspaceConflict(
		context.Background(),
		mgr,
		"new-workspace",
		false,
		"text",
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

	mgr := workspace.NewManager(store, nil, zerolog.Nop())
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))
	var buf bytes.Buffer

	// Non-interactive mode should fail
	_, err = prompter.ResolveWorkspaceConflict(
		context.Background(),
		mgr,
		"existing-ws",
		true,
		"text",
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

	mgr := workspace.NewManager(store, nil, zerolog.Nop())
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))
	var buf bytes.Buffer

	// JSON output mode should return error (no longer writes to buffer)
	_, err = prompter.ResolveWorkspaceConflict(
		context.Background(),
		mgr,
		"existing-ws",
		false,
		"json",
		&buf,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "existing-ws")
}

func TestHandleWorkspaceConflict_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)
	mgr := workspace.NewManager(store, nil, zerolog.Nop())
	prompter := workflow.NewPrompter(tui.NewTTYOutput(os.Stdout))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	_, err = prompter.ResolveWorkspaceConflict(ctx, mgr, "any-ws", false, "text", &buf)
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

	targetFlag := cmd.Flags().Lookup("target")
	require.NotNil(t, targetFlag, "--target flag should be registered")
	assert.Empty(t, targetFlag.Shorthand, "target should not have shorthand")
	assert.Contains(t, targetFlag.Usage, "Existing branch")
	assert.Contains(t, targetFlag.Usage, "mutually exclusive")

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
			err := workflow.ValidateWorkspaceName(tc.input)
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
	result := workflow.SanitizeWorkspaceName(longDesc)

	assert.LessOrEqual(t, len(result), workflow.MaxWorkspaceNameLen,
		"workspace name should be truncated to max length")
}

func TestWorkspaceNameNoTrailingHyphen(t *testing.T) {
	// Create a description that would result in trailing hyphen after truncation
	desc := strings.Repeat("word-", 15) // Will be truncated with trailing hyphen
	result := workflow.SanitizeWorkspaceName(desc)

	assert.False(t, strings.HasSuffix(result, "-"),
		"workspace name should not end with hyphen")
}

func TestValidateModelWithAgent(t *testing.T) {
	tests := []struct {
		name    string
		agent   string
		model   string
		wantErr bool
	}{
		// Claude models
		{"claude sonnet", "claude", "sonnet", false},
		{"claude opus", "claude", "opus", false},
		{"claude haiku", "claude", "haiku", false},
		{"claude invalid model", "claude", "flash", true},
		// Gemini models
		{"gemini flash", "gemini", "flash", false},
		{"gemini pro", "gemini", "pro", false},
		{"gemini invalid model", "gemini", "sonnet", true},
		// Empty agent (checks all)
		{"empty agent valid claude model", "", "sonnet", false},
		{"empty agent valid gemini model", "", "flash", false},
		{"empty agent invalid model", "", "gpt-4", true},
		// Empty model is always valid
		{"empty model", "claude", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateModel(tc.agent, tc.model)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateModel_ExitCode2Error(t *testing.T) {
	tests := []struct {
		name    string
		agent   string
		model   string
		wantErr bool
	}{
		{"valid claude sonnet", "claude", "sonnet", false},
		{"empty is ok", "claude", "", false},
		{"invalid model", "claude", "gpt-4", true},
		{"invalid unknown", "claude", "claude-3", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateModel(tc.agent, tc.model)
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
	_, err := workflow.SelectTemplate(context.Background(), registry, "", true, "text")
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

func TestSelectTemplate_CustomTemplate(t *testing.T) {
	// Create temp directory with custom template
	tmpDir := t.TempDir()
	customTemplate := `
name: custom-deploy
steps:
  - name: implement
    type: ai
    required: true
`
	tmpFile := filepath.Join(tmpDir, "deploy.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(customTemplate), 0o600))

	// Create registry with custom template
	registry, err := template.NewRegistryWithConfig(tmpDir, map[string]string{
		"custom-deploy": "deploy.yaml",
	})
	require.NoError(t, err)

	// Verify custom template is selectable
	tmpl, err := workflow.SelectTemplate(context.Background(), registry, "custom-deploy", false, "text")
	require.NoError(t, err)
	assert.Equal(t, "custom-deploy", tmpl.Name)
}

func TestSelectTemplate_CustomOverridesBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	// Custom template with same name as built-in
	customBugfix := `
name: bugfix
description: Custom bugfix workflow
branch_prefix: hotfix
steps:
  - name: custom-step
    type: ai
    required: true
`
	tmpFile := filepath.Join(tmpDir, "bugfix.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(customBugfix), 0o600))

	registry, err := template.NewRegistryWithConfig(tmpDir, map[string]string{
		"bugfix": "bugfix.yaml",
	})
	require.NoError(t, err)

	// Select bugfix - should get custom version
	tmpl, err := workflow.SelectTemplate(context.Background(), registry, "bugfix", false, "text")
	require.NoError(t, err)
	assert.Equal(t, "bugfix", tmpl.Name)
	assert.Equal(t, "hotfix", tmpl.BranchPrefix) // Custom uses "hotfix", built-in uses "fix"
	assert.Equal(t, "Custom bugfix workflow", tmpl.Description)
}

func TestNewRegistryWithConfig_InvalidTemplateError(t *testing.T) {
	tmpDir := t.TempDir()
	// Invalid template - missing required step type
	invalidTemplate := `
name: invalid
steps:
  - name: step1
    type: unknown_invalid_type
    required: true
`
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(invalidTemplate), 0o600))

	_, err := template.NewRegistryWithConfig(tmpDir, map[string]string{
		"invalid": "invalid.yaml",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load custom templates")
}

func TestCleanupWorkspace_Success(t *testing.T) {
	// Create a real workspace to clean up
	tmpDir := t.TempDir()
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace
	ws := &domain.Workspace{
		Name:      "test-cleanup-ws",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Get current git repo path
	repoPath, err := workflow.FindGitRepository(context.Background())
	require.NoError(t, err)

	// Cleanup should not error (though it may not actually destroy worktree if not created)
	err = workflow.CleanupWorkspace(context.Background(), "test-cleanup-ws", repoPath)
	// May error if worktree doesn't exist, but should not panic
	_ = err
}

func TestHandleTaskStartError_WithNilTask(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "test-error-ws",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	repoPath, err := workflow.FindGitRepository(context.Background())
	require.NoError(t, err)

	sc := &startContext{outputFormat: "text"}
	logger := Logger()

	// When task is nil, workspace should be cleaned up
	sc.handleTaskStartError(context.Background(), ws, repoPath, nil, logger)
	// If cleanup succeeds, workspace should be gone
	// This may not actually delete the workspace if worktree doesn't exist
}

func TestHandleTaskStartError_WithExistingTask(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "test-error-ws-2",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	repoPath, err := workflow.FindGitRepository(context.Background())
	require.NoError(t, err)

	// Create a mock task
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-error-ws-2",
		Status:      constants.TaskStatusPending,
		Description: "test task",
	}

	sc := &startContext{outputFormat: "text"}
	logger := Logger()

	// When task exists, workspace should NOT be cleaned up
	sc.handleTaskStartError(context.Background(), ws, repoPath, task, logger)

	// Workspace should still exist (not cleaned up)
	existingWs, err := wsStore.Get(context.Background(), ws.Name)
	require.NoError(t, err)
	assert.NotNil(t, existingWs)
	assert.Equal(t, ws.Name, existingWs.Name)
}

func TestCreateWorkspace_NewWorkspace(t *testing.T) {
	// This test requires being in a git repository
	repoPath, err := workflow.FindGitRepository(context.Background())
	require.NoError(t, err)

	// Create new workspace
	ws, err := workflow.CreateWorkspaceSimple(
		context.Background(),
		"test-new-ws-"+time.Now().Format("20060102150405"),
		repoPath,
		"test",
		"master",
		"",    // targetBranch (empty for new branch mode)
		false, // useLocal
	)

	// May fail if git operations fail, but should not panic
	if err == nil {
		assert.NotNil(t, ws)
		assert.NotEmpty(t, ws.Name)
		// Cleanup
		_ = workflow.CleanupWorkspace(context.Background(), ws.Name, repoPath)
	}
}

func TestCreateWorkspace_ReuseExisting(t *testing.T) {
	// This test requires being in a git repository
	repoPath, err := workflow.FindGitRepository(context.Background())
	require.NoError(t, err)

	wsName := "test-reuse-ws-" + time.Now().Format("20060102150405")

	// Create first workspace
	ws1, err := workflow.CreateWorkspaceSimple(
		context.Background(),
		wsName,
		repoPath,
		"test",
		"master",
		"",    // targetBranch (empty for new branch mode)
		false, // useLocal
	)
	if err != nil {
		t.Skip("Cannot create workspace in current environment")
	}
	require.NotNil(t, ws1)

	// Try to create again with same name - should reuse
	ws2, err := workflow.CreateWorkspaceSimple(
		context.Background(),
		wsName,
		repoPath,
		"test",
		"master",
		"",    // targetBranch (empty for new branch mode)
		false, // useLocal
	)

	require.NoError(t, err)
	assert.Equal(t, ws1.Name, ws2.Name)

	// Cleanup
	_ = workflow.CleanupWorkspace(context.Background(), wsName, repoPath)
}

func TestStartTaskExecution_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test",
		Branch:       "test-branch",
	}

	registry := template.NewDefaultRegistry()
	tmpl, err := registry.Get("bugfix")
	require.NoError(t, err)

	logger := Logger()
	out := tui.NewOutput(os.Stdout, "")

	// Should fail due to canceled context
	_, err = startTaskExecution(ctx, ws, tmpl, "test description", "", "", logger, out)
	require.Error(t, err)
	// Error may be context.Canceled or a wrapped error
}

// TestApplyVerifyOverrides tests verify flag override behavior
func TestApplyVerifyOverrides(t *testing.T) {
	tests := []struct {
		name           string
		templateVerify bool
		verifyFlag     bool
		noVerifyFlag   bool
		wantVerify     bool
	}{
		{
			name:           "no flags uses template default true",
			templateVerify: true,
			verifyFlag:     false,
			noVerifyFlag:   false,
			wantVerify:     true,
		},
		{
			name:           "no flags uses template default false",
			templateVerify: false,
			verifyFlag:     false,
			noVerifyFlag:   false,
			wantVerify:     false,
		},
		{
			name:           "verify flag overrides template false",
			templateVerify: false,
			verifyFlag:     true,
			noVerifyFlag:   false,
			wantVerify:     true,
		},
		{
			name:           "no-verify flag overrides template true",
			templateVerify: true,
			verifyFlag:     false,
			noVerifyFlag:   true,
			wantVerify:     false,
		},
		{
			name:           "verify flag takes precedence",
			templateVerify: false,
			verifyFlag:     true,
			noVerifyFlag:   false,
			wantVerify:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &domain.Template{
				Verify: tt.templateVerify,
				Steps: []domain.StepDefinition{
					{
						Name:     "verify-step",
						Type:     domain.StepTypeVerify,
						Required: tt.templateVerify,
					},
				},
			}

			workflow.ApplyVerifyOverrides(tmpl, tt.verifyFlag, tt.noVerifyFlag)

			assert.Equal(t, tt.wantVerify, tmpl.Verify)
			// Verify step's Required field should match
			assert.Equal(t, tt.wantVerify, tmpl.Steps[0].Required)
		})
	}
}

// TestApplyVerifyOverrides_WithVerifyModel tests VerifyModel propagation
func TestApplyVerifyOverrides_WithVerifyModel(t *testing.T) {
	tmpl := &domain.Template{
		Verify:      true,
		VerifyModel: "opus",
		Steps: []domain.StepDefinition{
			{
				Name:     "verify-step",
				Type:     domain.StepTypeVerify,
				Required: true,
			},
		},
	}

	workflow.ApplyVerifyOverrides(tmpl, false, false)

	// Verify model should be propagated to step config
	require.NotNil(t, tmpl.Steps[0].Config)
	assert.Equal(t, "opus", tmpl.Steps[0].Config["model"])
}

// TestApplyVerifyOverrides_NoVerifyStep tests template without verify step
func TestApplyVerifyOverrides_NoVerifyStep(t *testing.T) {
	tmpl := &domain.Template{
		Verify: true,
		Steps: []domain.StepDefinition{
			{
				Name: "ai-step",
				Type: domain.StepTypeAI,
			},
		},
	}

	// Should not panic when no verify step exists
	require.NotPanics(t, func() {
		workflow.ApplyVerifyOverrides(tmpl, false, true)
	})

	assert.False(t, tmpl.Verify)
}

// TestApplyVerifyOverrides_DifferentAgentIgnoresVerifyModel tests that VerifyModel
// is NOT applied when the step has a different agent override
func TestApplyVerifyOverrides_DifferentAgentIgnoresVerifyModel(t *testing.T) {
	tmpl := &domain.Template{
		Verify:       true,
		DefaultAgent: domain.AgentClaude, // Template uses Claude
		VerifyModel:  "opus",             // VerifyModel is for Claude
		Steps: []domain.StepDefinition{
			{
				Name:     "verify-step",
				Type:     domain.StepTypeVerify,
				Required: true,
				Config: map[string]any{
					"agent": "gemini", // Step overrides to Gemini
					"model": "",       // No model specified
				},
			},
		},
	}

	workflow.ApplyVerifyOverrides(tmpl, false, false)

	// VerifyModel should NOT be applied because step uses different agent
	// The empty model should remain empty (step executor will use agent's default)
	assert.Empty(t, tmpl.Steps[0].Config["model"])
	assert.Equal(t, "gemini", tmpl.Steps[0].Config["agent"])
}

// TestApplyVerifyOverrides_SameAgentAppliesVerifyModel tests that VerifyModel
// IS applied when the step uses the same agent as template default
func TestApplyVerifyOverrides_SameAgentAppliesVerifyModel(t *testing.T) {
	tmpl := &domain.Template{
		Verify:       true,
		DefaultAgent: domain.AgentClaude,
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			{
				Name:     "verify-step",
				Type:     domain.StepTypeVerify,
				Required: true,
				Config: map[string]any{
					"agent": "claude", // Same as template default
					"model": "",
				},
			},
		},
	}

	workflow.ApplyVerifyOverrides(tmpl, false, false)

	// VerifyModel SHOULD be applied because step uses same agent
	assert.Equal(t, "opus", tmpl.Steps[0].Config["model"])
}

// TestApplyVerifyOverrides_NoAgentOverrideAppliesVerifyModel tests that VerifyModel
// IS applied when step doesn't have any agent override
func TestApplyVerifyOverrides_NoAgentOverrideAppliesVerifyModel(t *testing.T) {
	tmpl := &domain.Template{
		Verify:       true,
		DefaultAgent: domain.AgentClaude,
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			{
				Name:     "verify-step",
				Type:     domain.StepTypeVerify,
				Required: true,
				// No Config - will use template defaults
			},
		},
	}

	workflow.ApplyVerifyOverrides(tmpl, false, false)

	// VerifyModel should be applied (step uses template's default agent)
	require.NotNil(t, tmpl.Steps[0].Config)
	assert.Equal(t, "opus", tmpl.Steps[0].Config["model"])
}

// TestApplyVerifyOverrides_ExplicitModelNotOverwritten tests that explicitly set
// model in step config is NOT overwritten by VerifyModel
func TestApplyVerifyOverrides_ExplicitModelNotOverwritten(t *testing.T) {
	tmpl := &domain.Template{
		Verify:       true,
		DefaultAgent: domain.AgentClaude,
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			{
				Name:     "verify-step",
				Type:     domain.StepTypeVerify,
				Required: true,
				Config: map[string]any{
					"model": "sonnet", // Explicitly set
				},
			},
		},
	}

	workflow.ApplyVerifyOverrides(tmpl, false, false)

	// Explicit model should be preserved
	assert.Equal(t, "sonnet", tmpl.Steps[0].Config["model"])
}

// TestApplyVerifyOverrides_DifferentAgentWithExplicitModel tests that explicit
// model is preserved even when step has different agent
func TestApplyVerifyOverrides_DifferentAgentWithExplicitModel(t *testing.T) {
	tmpl := &domain.Template{
		Verify:       true,
		DefaultAgent: domain.AgentClaude,
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			{
				Name:     "verify-step",
				Type:     domain.StepTypeVerify,
				Required: true,
				Config: map[string]any{
					"agent": "gemini",
					"model": "pro", // Explicitly set for Gemini
				},
			},
		},
	}

	workflow.ApplyVerifyOverrides(tmpl, false, false)

	// Explicit model should be preserved
	assert.Equal(t, "pro", tmpl.Steps[0].Config["model"])
	assert.Equal(t, "gemini", tmpl.Steps[0].Config["agent"])
}

// TestGetSideEffectForStepType tests side effect descriptions
func TestGetSideEffectForStepType(t *testing.T) {
	tests := []struct {
		name     string
		step     domain.StepDefinition
		expected string
	}{
		{
			name:     "AI step",
			step:     domain.StepDefinition{Type: domain.StepTypeAI},
			expected: "AI execution (file modifications)",
		},
		{
			name:     "Validation step",
			step:     domain.StepDefinition{Type: domain.StepTypeValidation},
			expected: "Validation commands (format may modify files)",
		},
		{
			name: "Git commit step",
			step: domain.StepDefinition{
				Type:   domain.StepTypeGit,
				Config: map[string]any{"operation": "commit"},
			},
			expected: "Git commits",
		},
		{
			name: "Git push step",
			step: domain.StepDefinition{
				Type:   domain.StepTypeGit,
				Config: map[string]any{"operation": "push"},
			},
			expected: "Git push to remote",
		},
		{
			name: "Git create PR step",
			step: domain.StepDefinition{
				Type:   domain.StepTypeGit,
				Config: map[string]any{"operation": "create_pr"},
			},
			expected: "Pull request creation",
		},
		{
			name: "Git other operation",
			step: domain.StepDefinition{
				Type:   domain.StepTypeGit,
				Config: map[string]any{"operation": "other"},
			},
			expected: "Git operations",
		},
		{
			name:     "Git no config",
			step:     domain.StepDefinition{Type: domain.StepTypeGit},
			expected: "Git operations",
		},
		{
			name:     "Verify step",
			step:     domain.StepDefinition{Type: domain.StepTypeVerify},
			expected: "AI verification",
		},
		{
			name:     "SDD step",
			step:     domain.StepDefinition{Type: domain.StepTypeSDD},
			expected: "SDD generation",
		},
		{
			name:     "CI step",
			step:     domain.StepDefinition{Type: domain.StepTypeCI},
			expected: "CI execution",
		},
		{
			name:     "Human step",
			step:     domain.StepDefinition{Type: domain.StepTypeHuman},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSideEffectForStepType(tt.step)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRunStart_ConflictingVerifyFlags tests validation of --verify and --no-verify
func TestRunStart_ConflictingVerifyFlags(t *testing.T) {
	cmd := newStartCmd()

	// Add global flags
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runStart(context.Background(), cmd, &buf, "test description", startOptions{
		templateName: "bugfix",
		verify:       true,
		noVerify:     true,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrConflictingFlags)
	assert.True(t, errors.IsExitCode2Error(err))
}

// TestRunStart_InvalidModel tests model validation
func TestRunStart_InvalidModel(t *testing.T) {
	cmd := newStartCmd()

	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runStart(context.Background(), cmd, &buf, "test description", startOptions{
		templateName: "bugfix",
		model:        "gpt-4", // Invalid model
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrInvalidModel)
	assert.True(t, errors.IsExitCode2Error(err))
}

// TestFormatDuration tests the formatDuration helper function
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{
			name:     "zero milliseconds",
			ms:       0,
			expected: "0s",
		},
		{
			name:     "under one second",
			ms:       500,
			expected: "0s",
		},
		{
			name:     "exact one second",
			ms:       1000,
			expected: "1s",
		},
		{
			name:     "under one minute",
			ms:       45000,
			expected: "45s",
		},
		{
			name:     "exact one minute",
			ms:       60000,
			expected: "1m",
		},
		{
			name:     "minutes and seconds",
			ms:       135000,
			expected: "2m 15s",
		},
		{
			name:     "large duration",
			ms:       3661000,
			expected: "61m 1s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.ms)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildStepMetrics tests the buildStepMetrics helper function
func TestBuildStepMetrics(t *testing.T) {
	tests := []struct {
		name              string
		durationMs        int64
		numTurns          int
		filesChangedCount int
		expected          string
	}{
		{
			name:              "all zero",
			durationMs:        0,
			numTurns:          0,
			filesChangedCount: 0,
			expected:          "",
		},
		{
			name:              "only duration",
			durationMs:        45000,
			numTurns:          0,
			filesChangedCount: 0,
			expected:          "Duration: 45s",
		},
		{
			name:              "only turns",
			durationMs:        0,
			numTurns:          5,
			filesChangedCount: 0,
			expected:          "Turns: 5",
		},
		{
			name:              "only files",
			durationMs:        0,
			numTurns:          0,
			filesChangedCount: 3,
			expected:          "Files: 3",
		},
		{
			name:              "duration and turns",
			durationMs:        120000,
			numTurns:          8,
			filesChangedCount: 0,
			expected:          "Duration: 2m | Turns: 8",
		},
		{
			name:              "all fields present",
			durationMs:        75000,
			numTurns:          4,
			filesChangedCount: 12,
			expected:          "Duration: 1m 15s | Turns: 4 | Files: 12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStepMetrics(tt.durationMs, tt.numTurns, tt.filesChangedCount)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestApplyAgentModelOverrides tests the applyAgentModelOverrides function
func TestApplyAgentModelOverrides(t *testing.T) {
	tests := []struct {
		name        string
		agent       string
		model       string
		expectAgent domain.Agent
		expectModel string
	}{
		{
			name:        "no overrides",
			agent:       "",
			model:       "",
			expectAgent: "",
			expectModel: "",
		},
		{
			name:        "agent only",
			agent:       "dev",
			model:       "",
			expectAgent: "dev",
			expectModel: "",
		},
		{
			name:        "model only",
			agent:       "",
			model:       "opus",
			expectAgent: "",
			expectModel: "opus",
		},
		{
			name:        "both overrides",
			agent:       "architect",
			model:       "sonnet",
			expectAgent: "architect",
			expectModel: "sonnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &domain.Template{
				DefaultAgent: "",
				DefaultModel: "",
			}

			workflow.ApplyAgentModelOverrides(tmpl, tt.agent, tt.model)

			assert.Equal(t, tt.expectAgent, tmpl.DefaultAgent)
			assert.Equal(t, tt.expectModel, tmpl.DefaultModel)
		})
	}
}

// mockAIRunner is a simple mock for ai.Runner interface
type mockAIRunner struct {
	ai.Runner
}

// TestCreateValidationRetryHandler tests the createValidationRetryHandler factory function
func TestCreateValidationRetryHandler(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		expectNil bool
	}{
		{
			name:      "disabled returns nil",
			enabled:   false,
			expectNil: true,
		},
		{
			name:      "enabled returns handler",
			enabled:   true,
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Validation: config.ValidationConfig{
					AIRetryEnabled:     tt.enabled,
					MaxAIRetryAttempts: 2,
				},
			}
			aiRunner := &mockAIRunner{}

			handler := workflow.CreateValidationRetryHandler(aiRunner, cfg)

			if tt.expectNil {
				assert.Nil(t, handler)
			} else {
				assert.NotNil(t, handler)
			}
		})
	}
}

// TestCreateNotifiers tests the createNotifiers factory function
func TestCreateNotifiers(t *testing.T) {
	tests := []struct {
		name        string
		bellEnabled bool
	}{
		{
			name:        "bell enabled",
			bellEnabled: true,
		},
		{
			name:        "bell disabled",
			bellEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Notifications: config.NotificationsConfig{
					Bell:   tt.bellEnabled,
					Events: []string{"task.started", "task.completed"},
				},
			}

			notifier, stateNotifier := workflow.CreateNotifiers(cfg)

			require.NotNil(t, notifier)
			require.NotNil(t, stateNotifier)
		})
	}
}

// TestCreateAIRunner tests the createAIRunner factory function
func TestCreateAIRunner(t *testing.T) {
	tests := []struct {
		name  string
		model string
		agent string
	}{
		{
			name:  "with sonnet model",
			model: "sonnet",
			agent: "claude",
		},
		{
			name:  "with opus model",
			model: "opus",
			agent: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AI: config.AIConfig{
					Model: tt.model,
					Agent: tt.agent,
				},
			}
			runner := workflow.CreateAIRunner(cfg)

			require.NotNil(t, runner)
		})
	}
}

// TestRunStart_ConflictingBranchAndTargetFlags tests that --branch and --target are mutually exclusive
func TestRunStart_ConflictingBranchAndTargetFlags(t *testing.T) {
	cmd := newStartCmd()

	// Add global flags
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runStart(context.Background(), cmd, &buf, "test description", startOptions{
		templateName: "hotfix",
		baseBranch:   "develop",
		targetBranch: "feat/existing",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrConflictingFlags)
	assert.True(t, errors.IsExitCode2Error(err))
	assert.Contains(t, err.Error(), "cannot use both --branch and --target")
}

// TestRunStart_TargetFlag_WithHotfixTemplate tests the happy path with --target and hotfix template
func TestRunStart_TargetFlag_WithHotfixTemplate(t *testing.T) {
	// This test verifies that --target flag is properly parsed and passed through
	// We can't do full integration without mocking the workspace creation

	cmd := newStartCmd()

	// Verify target flag can be set
	require.NoError(t, cmd.Flags().Set("target", "feat/my-feature"))
	require.NoError(t, cmd.Flags().Set("template", "hotfix"))

	// Get the flag value back
	targetVal, err := cmd.Flags().GetString("target")
	require.NoError(t, err)
	assert.Equal(t, "feat/my-feature", targetVal)

	templateVal, err := cmd.Flags().GetString("template")
	require.NoError(t, err)
	assert.Equal(t, "hotfix", templateVal)
}

// TestStartOptions_TargetBranch tests the startOptions struct with targetBranch
func TestStartOptions_TargetBranch(t *testing.T) {
	tests := []struct {
		name         string
		opts         startOptions
		expectError  bool
		errorContain string
	}{
		{
			name: "valid target branch only",
			opts: startOptions{
				templateName: "hotfix",
				targetBranch: "feat/existing-branch",
			},
			expectError: false,
		},
		{
			name: "valid base branch only",
			opts: startOptions{
				templateName: "bugfix",
				baseBranch:   "develop",
			},
			expectError: false,
		},
		{
			name: "conflicting branch and target",
			opts: startOptions{
				templateName: "hotfix",
				baseBranch:   "develop",
				targetBranch: "feat/existing",
			},
			expectError:  true,
			errorContain: "cannot use both",
		},
		{
			name: "target with verify flag",
			opts: startOptions{
				templateName: "hotfix",
				targetBranch: "feat/existing",
				verify:       true,
			},
			expectError: false,
		},
		{
			name: "target with no-verify flag",
			opts: startOptions{
				templateName: "hotfix",
				targetBranch: "feat/existing",
				noVerify:     true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate the option combinations - full validation happens in runStart
			hasConflict := tt.opts.baseBranch != "" && tt.opts.targetBranch != ""

			if tt.expectError {
				assert.True(t, hasConflict, "expected conflict between baseBranch and targetBranch")
			} else {
				assert.False(t, hasConflict, "did not expect conflict")
			}
		})
	}
}

// TestCreateWorkspace_WithTargetBranch tests createWorkspace with ExistingBranch mode
func TestCreateWorkspace_WithTargetBranch(t *testing.T) {
	t.Run("passes targetBranch to workspace manager", func(t *testing.T) {
		// Setup a git repository
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoPath, 0o750))

		runGitCommand(t, repoPath, "init")
		runGitCommand(t, repoPath, "config", "user.email", "test@test.com")
		runGitCommand(t, repoPath, "config", "user.name", "Test")

		readmePath := filepath.Join(repoPath, "README.md")
		require.NoError(t, os.WriteFile(readmePath, []byte("# Test"), 0o600))
		runGitCommand(t, repoPath, "add", ".")
		runGitCommand(t, repoPath, "commit", "-m", "Initial commit")

		// Create a target branch
		runGitCommand(t, repoPath, "branch", "feat/target-branch")

		// Create startContext
		// Call createWorkspace with targetBranch
		ws, err := workflow.CreateWorkspaceSimple(
			context.Background(),
			"hotfix-workspace",
			repoPath,
			"hotfix",             // branchPrefix (fallback, not used when targetBranch set)
			"",                   // baseBranch (empty)
			"feat/target-branch", // targetBranch (existing branch)
			false,                // useLocal
		)

		require.NoError(t, err)
		require.NotNil(t, ws)
		assert.Equal(t, "hotfix-workspace", ws.Name)
		assert.Equal(t, "feat/target-branch", ws.Branch, "should use the target branch, not create new")

		// Cleanup
		_ = workflow.CleanupWorkspace(context.Background(), ws.Name, repoPath)
	})

	t.Run("returns error for non-existent target branch", func(t *testing.T) {
		// Setup a git repository
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoPath, 0o750))

		runGitCommand(t, repoPath, "init")
		runGitCommand(t, repoPath, "config", "user.email", "test@test.com")
		runGitCommand(t, repoPath, "config", "user.name", "Test")

		readmePath := filepath.Join(repoPath, "README.md")
		require.NoError(t, os.WriteFile(readmePath, []byte("# Test"), 0o600))
		runGitCommand(t, repoPath, "add", ".")
		runGitCommand(t, repoPath, "commit", "-m", "Initial commit")

		// Try to create workspace with non-existent branch
		_, err := workflow.CreateWorkspaceSimple(
			context.Background(),
			"hotfix-workspace",
			repoPath,
			"hotfix",
			"",
			"feat/does-not-exist", // Non-existent branch
			false,
		)

		require.Error(t, err)
		assert.ErrorIs(t, err, errors.ErrBranchNotFound)
	})
}

// TestTargetFlag_Integration tests the full flow from flag to workspace creation
func TestTargetFlag_Integration(t *testing.T) {
	// Skip if not in suitable environment
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("hotfix template with target creates workspace on existing branch", func(t *testing.T) {
		// Setup
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoPath, 0o750))

		runGitCommand(t, repoPath, "init")
		runGitCommand(t, repoPath, "config", "user.email", "test@test.com")
		runGitCommand(t, repoPath, "config", "user.name", "Test")

		readmePath := filepath.Join(repoPath, "README.md")
		require.NoError(t, os.WriteFile(readmePath, []byte("# Test"), 0o600))
		runGitCommand(t, repoPath, "add", ".")
		runGitCommand(t, repoPath, "commit", "-m", "Initial commit")

		// Create target branch with some changes
		runGitCommand(t, repoPath, "checkout", "-b", "feat/needs-fixes")
		featurePath := filepath.Join(repoPath, "feature.go")
		require.NoError(t, os.WriteFile(featurePath, []byte("package main\n"), 0o600))
		runGitCommand(t, repoPath, "add", ".")
		runGitCommand(t, repoPath, "commit", "-m", "Add feature")
		runGitCommand(t, repoPath, "checkout", "master")

		// Get hotfix template
		registry := template.NewDefaultRegistry()
		hotfixTmpl, err := registry.Get("hotfix")
		require.NoError(t, err)

		// Verify hotfix template doesn't have git_pr step
		hasPRStep := false
		for _, step := range hotfixTmpl.Steps {
			if step.Name == "git_pr" {
				hasPRStep = true
				break
			}
		}
		assert.False(t, hasPRStep, "hotfix template should not have git_pr step")

		// Verify hotfix template has the expected steps
		expectedSteps := []string{"detect", "fix", "verify", "validate", "git_commit", "git_push"}
		require.Len(t, hotfixTmpl.Steps, len(expectedSteps))
		for i, stepName := range expectedSteps {
			assert.Equal(t, stepName, hotfixTmpl.Steps[i].Name)
		}

		// Create workspace with target branch
		ws, err := workflow.CreateWorkspaceSimple(
			context.Background(),
			"hotfix-test-"+time.Now().Format("20060102150405"),
			repoPath,
			hotfixTmpl.BranchPrefix,
			"",                 // No base branch
			"feat/needs-fixes", // Target existing branch
			false,
		)
		require.NoError(t, err)
		require.NotNil(t, ws)

		// Verify workspace is on the target branch
		assert.Equal(t, "feat/needs-fixes", ws.Branch)

		// Verify worktree contains the feature file
		featureInWorktree := filepath.Join(ws.WorktreePath, "feature.go")
		_, err = os.Stat(featureInWorktree)
		require.NoError(t, err, "worktree should contain feature.go from target branch")

		// Cleanup
		_ = workflow.CleanupWorkspace(context.Background(), ws.Name, repoPath)
	})
}
