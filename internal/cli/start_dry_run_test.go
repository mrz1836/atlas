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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initGitRepo creates a temporary git repository for testing
func initGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize git repo
	cmd := exec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "failed to init git repo")

	// Configure git user for commits
	cmd = exec.CommandContext(context.Background(), "git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.CommandContext(context.Background(), "git", "config", "user.name", "Test User")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Create initial commit
	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o600))

	cmd = exec.CommandContext(context.Background(), "git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	return dir
}

func TestDryRun_NoWorkspaceCreated(t *testing.T) {
	// Setup: create a git repo
	repoDir := initGitRepo(t)

	// Change to repo dir
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	// Count worktree directories before
	parentDir := filepath.Dir(repoDir)
	entriesBefore, err := os.ReadDir(parentDir)
	require.NoError(t, err)

	// Run dry-run with JSON output to verify it completes successfully
	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test description", "--template", "bug", "--dry-run"})

	// Mock the output flag with JSON (easier to capture and parse)
	cmd.PersistentFlags().String("output", "json", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	// Verify no new worktree directories were created
	entriesAfter, err := os.ReadDir(parentDir)
	require.NoError(t, err)
	require.Len(t, entriesAfter, len(entriesBefore), "no new directories should be created in dry-run mode")

	// Verify JSON response indicates dry-run
	var response dryRunResponse
	err = json.Unmarshal(buf.Bytes(), &response)
	require.NoError(t, err, "should return valid JSON")
	assert.True(t, response.DryRun)
	assert.True(t, response.Workspace.WouldCreate)
}

func TestDryRun_OutputFormat_TTY(t *testing.T) {
	// TTY output goes to os.Stdout, not the buffer
	// This test verifies the command completes successfully with text output
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"fix null pointer bug", "--template", "bug", "--dry-run"})
	cmd.PersistentFlags().String("output", "text", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err, "dry-run with text output should succeed")
	// Actual text output verification would require capturing os.Stdout
}

func TestDryRun_OutputFormat_JSON(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"fix null pointer bug", "--template", "bug", "--dry-run"})
	cmd.PersistentFlags().String("output", "json", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	// Parse JSON output
	var response dryRunResponse
	err = json.Unmarshal(buf.Bytes(), &response)
	require.NoError(t, err, "output should be valid JSON")

	// Verify JSON structure
	assert.True(t, response.DryRun)
	assert.Equal(t, "bug", response.Template)
	assert.True(t, response.Workspace.WouldCreate)
	assert.Equal(t, "fix-null-pointer-bug", response.Workspace.Name)
	assert.NotEmpty(t, response.Steps)
	assert.NotEmpty(t, response.Summary.SideEffectsPrevented)
}

func TestDryRun_StepDetails(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"implement feature", "--template", "feature", "--dry-run"})
	cmd.PersistentFlags().String("output", "json", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	var response dryRunResponse
	err = json.Unmarshal(buf.Bytes(), &response)
	require.NoError(t, err)

	// Verify steps have proper structure
	for _, step := range response.Steps {
		assert.NotEmpty(t, step.Name)
		assert.NotEmpty(t, step.Type)
		assert.Equal(t, "would_execute", step.Status)
		assert.NotEmpty(t, step.WouldDo, "each step should describe what it would do")
	}
}

func TestDryRun_ValidationError_StillCaught(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	// Invalid template name should still error
	cmd.SetArgs([]string{"test", "--template", "nonexistent-template", "--dry-run"})
	cmd.PersistentFlags().String("output", "text", "")

	err = cmd.ExecuteContext(context.Background())
	require.Error(t, err, "dry-run should still catch validation errors")
	assert.Contains(t, err.Error(), "not found")
}

func TestDryRun_InvalidModelFlag_StillCaught(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--model", "invalid-model", "--dry-run"})
	cmd.PersistentFlags().String("output", "text", "")

	err = cmd.ExecuteContext(context.Background())
	require.Error(t, err, "dry-run should still catch invalid model flag")
}

func TestDryRun_WorkspaceName_Custom(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--workspace", "my-custom-ws", "--dry-run"})
	cmd.PersistentFlags().String("output", "json", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	var response dryRunResponse
	err = json.Unmarshal(buf.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "my-custom-ws", response.Workspace.Name)
	assert.Contains(t, response.Workspace.Branch, "my-custom-ws")
}

func TestDryRun_VerifyFlag(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--verify", "--dry-run"})
	cmd.PersistentFlags().String("output", "json", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	var response dryRunResponse
	err = json.Unmarshal(buf.Bytes(), &response)
	require.NoError(t, err)

	// Check if verify step is present and required
	hasVerifyStep := false
	for _, step := range response.Steps {
		if step.Type == "verify" {
			hasVerifyStep = true
			assert.True(t, step.Required, "verify step should be required when --verify flag is used")
		}
	}
	assert.True(t, hasVerifyStep, "should have verify step when --verify flag is used")
}

func TestDryRun_NoVerifyFlag(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--no-verify", "--dry-run"})
	cmd.PersistentFlags().String("output", "json", "")

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	var response dryRunResponse
	err = json.Unmarshal(buf.Bytes(), &response)
	require.NoError(t, err)

	// Check that verify step is not required
	for _, step := range response.Steps {
		if step.Type == "verify" {
			assert.False(t, step.Required, "verify step should not be required when --no-verify flag is used")
		}
	}
}

func TestDryRun_ConflictingVerifyFlags_StillCaught(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--verify", "--no-verify", "--dry-run"})
	cmd.PersistentFlags().String("output", "text", "")

	err = cmd.ExecuteContext(context.Background())
	require.Error(t, err, "conflicting verify flags should still error in dry-run mode")
	assert.Contains(t, err.Error(), "cannot use both")
}

func TestDryRun_NotInGitRepo_StillErrors(t *testing.T) {
	// Use a temp dir that is NOT a git repo
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--dry-run"})
	cmd.PersistentFlags().String("output", "text", "")

	err = cmd.ExecuteContext(context.Background())
	require.Error(t, err, "dry-run should still fail when not in a git repo")
	assert.Contains(t, err.Error(), "git repository")
}

func TestDryRun_BranchPrefix(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	// Default templates use prefix without trailing slash (e.g., "fix" not "fix/")
	tests := []struct {
		template       string
		expectedPrefix string
	}{
		{"bug", "fix"},
		{"feature", "feat"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			var buf bytes.Buffer
			cmd := newStartCmd()
			cmd.SetOut(&buf)
			cmd.SetArgs([]string{"test feature", "--template", tt.template, "--dry-run"})
			cmd.PersistentFlags().String("output", "json", "")

			err := cmd.ExecuteContext(context.Background())
			require.NoError(t, err)

			var response dryRunResponse
			err = json.Unmarshal(buf.Bytes(), &response)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(response.Workspace.Branch, tt.expectedPrefix),
				"branch %s should have prefix %s", response.Workspace.Branch, tt.expectedPrefix)
		})
	}
}

func TestDryRun_CommandHelp_ShowsDryRun(t *testing.T) {
	cmd := newStartCmd()

	// Verify --dry-run is in the help output
	help := cmd.UsageString()
	assert.Contains(t, help, "--dry-run")
	assert.Contains(t, help, "Show what would happen")
}

func TestDryRunResponse_Types(t *testing.T) {
	// Test the response type structures are correct
	response := dryRunResponse{
		DryRun:   true,
		Template: "bug",
		Workspace: dryRunWorkspaceInfo{
			Name:        "test",
			Branch:      "fix/test",
			WouldCreate: true,
		},
		Steps: []dryRunStepInfo{
			{
				Index:       0,
				Name:        "implement",
				Type:        "ai",
				Description: "Implement the fix",
				Required:    true,
				Status:      "would_execute",
				WouldDo:     []string{"Execute AI"},
				Config:      map[string]any{"model": "claude-sonnet-4-20250514"},
			},
		},
		Summary: dryRunSummary{
			TotalSteps:           1,
			SideEffectsPrevented: []string{"Workspace creation"},
		},
	}

	// Marshal and unmarshal to verify structure
	data, err := json.Marshal(response)
	require.NoError(t, err)

	var unmarshaled dryRunResponse
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, response.DryRun, unmarshaled.DryRun)
	assert.Equal(t, response.Template, unmarshaled.Template)
	assert.Equal(t, response.Workspace.Name, unmarshaled.Workspace.Name)
	assert.Len(t, unmarshaled.Steps, 1)
}

// TestDryRun_ContextCancellation verifies dry-run respects context cancellation
func TestDryRun_ContextCancellation(t *testing.T) {
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldWd) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	cmd := newStartCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test", "--template", "bug", "--dry-run"})
	cmd.PersistentFlags().String("output", "text", "")

	err = cmd.ExecuteContext(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}
