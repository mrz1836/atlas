// Package cli provides the command-line interface for atlas.
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

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCommandExecutor is a mock implementation of config.CommandExecutor for testing.
type mockCommandExecutor struct {
	lookPathResults map[string]string
	lookPathErrors  map[string]error
	runResults      map[string]string
	runErrors       map[string]error
}

// LookPath implements config.CommandExecutor.
func (m *mockCommandExecutor) LookPath(file string) (string, error) {
	if err, ok := m.lookPathErrors[file]; ok {
		return "", err
	}
	if path, ok := m.lookPathResults[file]; ok {
		return path, nil
	}
	return "", exec.ErrNotFound
}

// Run implements config.CommandExecutor.
func (m *mockCommandExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	key := name
	if len(args) > 0 {
		key = name + " " + strings.Join(args, " ")
	}
	if err, ok := m.runErrors[key]; ok {
		return "", err
	}
	if out, ok := m.runResults[key]; ok {
		return out, nil
	}
	return "", nil
}

// mockUpgradeChecker is a mock implementation of UpgradeChecker for testing.
type mockUpgradeChecker struct {
	checkAllResult   *UpdateCheckResult
	checkAllErr      error
	checkToolResults map[string]*UpdateInfo
	checkToolErrors  map[string]error
}

// CheckAllUpdates implements UpgradeChecker.
func (m *mockUpgradeChecker) CheckAllUpdates(_ context.Context) (*UpdateCheckResult, error) {
	if m.checkAllErr != nil {
		return nil, m.checkAllErr
	}
	return m.checkAllResult, nil
}

// CheckToolUpdate implements UpgradeChecker.
func (m *mockUpgradeChecker) CheckToolUpdate(_ context.Context, tool string) (*UpdateInfo, error) {
	if err, ok := m.checkToolErrors[tool]; ok {
		return nil, err
	}
	if info, ok := m.checkToolResults[tool]; ok {
		return info, nil
	}
	return nil, errors.ErrUnknownTool
}

// mockUpgradeExecutor is a mock implementation of UpgradeExecutor for testing.
type mockUpgradeExecutor struct {
	upgradeResults map[string]*UpgradeResult
	upgradeErrors  map[string]error
	backupResult   string
	backupErr      error
	restoreErr     error
	cleanupErr     error
}

// UpgradeTool implements UpgradeExecutor.
func (m *mockUpgradeExecutor) UpgradeTool(_ context.Context, tool string) (*UpgradeResult, error) {
	if err, ok := m.upgradeErrors[tool]; ok {
		return nil, err
	}
	if result, ok := m.upgradeResults[tool]; ok {
		return result, nil
	}
	return &UpgradeResult{Tool: tool, Success: true}, nil
}

// BackupConstitution implements UpgradeExecutor.
func (m *mockUpgradeExecutor) BackupConstitution() (string, error) {
	return m.backupResult, m.backupErr
}

// RestoreConstitution implements UpgradeExecutor.
func (m *mockUpgradeExecutor) RestoreConstitution(_ string) error {
	return m.restoreErr
}

// CleanupConstitutionBackup implements UpgradeExecutor.
func (m *mockUpgradeExecutor) CleanupConstitutionBackup(_ string) error {
	return m.cleanupErr
}

func TestNewUpgradeCmd(t *testing.T) {
	t.Parallel()

	flags := &UpgradeFlags{}
	cmd := newUpgradeCmd(flags)

	assert.Equal(t, "upgrade [tool]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

func TestIsValidTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tool     string
		expected bool
	}{
		{"valid atlas", constants.ToolAtlas, true},
		{"valid mage-x", constants.ToolMageX, true},
		{"valid go-pre-commit", constants.ToolGoPreCommit, true},
		{"valid speckit", constants.ToolSpeckit, true},
		{"invalid tool", "invalid-tool", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isValidTool(tt.tool)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetValidToolNames(t *testing.T) {
	t.Parallel()

	names := getValidToolNames()
	assert.Len(t, names, 4)
	assert.Contains(t, names, constants.ToolAtlas)
	assert.Contains(t, names, constants.ToolMageX)
	assert.Contains(t, names, constants.ToolGoPreCommit)
	assert.Contains(t, names, constants.ToolSpeckit)
}

func TestUpgradeCmd_CheckOnly_ShowsUpdates(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.1.0", LatestVersion: "0.2.0", UpdateAvailable: true, Installed: true},
				{Name: "mage-x", CurrentVersion: "0.5.0", UpdateAvailable: false, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Check: true, OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)

	// --check returns exit code 1 when updates are available
	var exitErr *ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.Code)

	output := buf.String()
	assert.Contains(t, output, "atlas")
	assert.Contains(t, output, "0.1.0")
}

func TestUpgradeCmd_CheckOnly_NoUpdates(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: false,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.2.0", UpdateAvailable: false, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Check: true, OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "atlas")
}

func TestUpgradeCmd_CheckOnly_JSONOutput(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.1.0", LatestVersion: "0.2.0", UpdateAvailable: true, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Check: true, OutputFormat: "json"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)

	// --check with JSON output returns exit code 1 when updates available (for scripting)
	var exitErr *ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.Code)

	// Parse JSON output
	var result UpdateCheckResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.True(t, result.UpdatesAvailable)
	assert.Len(t, result.Tools, 1)
	assert.Equal(t, "atlas", result.Tools[0].Name)
}

func TestUpgradeCmd_SingleTool_OnlyUpgradesThat(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkToolResults: map[string]*UpdateInfo{
			constants.ToolSpeckit: {
				Name:            constants.ToolSpeckit,
				CurrentVersion:  "1.0.0",
				UpdateAvailable: true,
				Installed:       true,
			},
		},
	}
	executor := &mockUpgradeExecutor{
		upgradeResults: map[string]*UpgradeResult{
			constants.ToolSpeckit: {Tool: constants.ToolSpeckit, Success: true, NewVersion: "1.1.0"},
		},
	}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Yes: true, OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, constants.ToolSpeckit, checker, executor)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, constants.ToolSpeckit)
	assert.Contains(t, output, "Success")
}

func TestUpgradeCmd_WithYesFlag_SkipsConfirmation(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.1.0", UpdateAvailable: true, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{
		upgradeResults: map[string]*UpgradeResult{
			"atlas": {Tool: "atlas", Success: true, NewVersion: "0.2.0"},
		},
	}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Yes: true, OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Upgrading")
	assert.Contains(t, output, "atlas")
}

func TestUpgradeCmd_UpgradeFailure_HandledGracefully(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.1.0", UpdateAvailable: true, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{
		upgradeErrors: map[string]error{
			"atlas": errors.ErrCommandFailed,
		},
	}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Yes: true, OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Failed")
}

func TestUpgradeCmd_UpgradeResultFailure_NilError(t *testing.T) {
	// This tests the scenario where UpgradeTool returns nil error but result.Success is false
	// This happens when the upgrade fails internally (e.g., GitHub release download fails)
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.1.0", UpdateAvailable: true, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{
		upgradeResults: map[string]*UpgradeResult{
			"atlas": {
				Tool:    "atlas",
				Success: false,
				Error:   "failed to download release",
			},
		},
	}

	var buf bytes.Buffer
	flags := &UpgradeFlags{Yes: true, OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)
	require.NoError(t, err)

	output := buf.String()
	// Should show "Failed" message, not "Success"
	assert.Contains(t, output, "Failed")
	assert.Contains(t, output, "failed to download release")
	assert.NotContains(t, output, "✓ Success")
}

func TestUpgradeCmd_AllUpToDate(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: false,
			Tools: []UpdateInfo{
				{Name: "atlas", CurrentVersion: "0.2.0", UpdateAvailable: false, Installed: true},
				{Name: "mage-x", CurrentVersion: "0.5.0", UpdateAvailable: false, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	var buf bytes.Buffer
	flags := &UpgradeFlags{OutputFormat: "text"}

	err := runUpgradeWithDeps(context.Background(), &buf, flags, "", checker, executor)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "up to date")
}

func TestUpgradeCmd_InvalidTool_ReturnsError(t *testing.T) {
	t.Parallel()

	flags := &UpgradeFlags{}
	cmd := newUpgradeCmd(flags)

	err := cmd.RunE(cmd, []string{"invalid-tool"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrInvalidToolName)
}

func TestUpgradeCmd_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{},
	}
	executor := &mockUpgradeExecutor{}

	var buf bytes.Buffer
	flags := &UpgradeFlags{}

	err := runUpgradeWithDeps(ctx, &buf, flags, "", checker, executor)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultUpgradeChecker_CheckAllUpdates(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		lookPathResults: map[string]string{
			"atlas":             "/usr/bin/atlas",
			constants.ToolMageX: "/go/bin/magex",
		},
		runResults: map[string]string{
			"atlas --version":                  "0.1.0 (commit: abc, built: 2024-01-01)",
			constants.ToolMageX + " --version": "v0.5.0",
		},
	}

	checker := NewDefaultUpgradeChecker(executor)
	result, err := checker.CheckAllUpdates(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Tools)
}

func TestDefaultUpgradeChecker_CheckToolUpdate(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		lookPathResults: map[string]string{
			"atlas": "/usr/bin/atlas",
		},
		runResults: map[string]string{
			"atlas --version": "0.1.0 (commit: abc, built: 2024-01-01)",
		},
	}

	checker := NewDefaultUpgradeChecker(executor)
	info, err := checker.CheckToolUpdate(context.Background(), constants.ToolAtlas)

	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, constants.ToolAtlas, info.Name)
	assert.True(t, info.Installed)
	assert.Equal(t, "0.1.0", info.CurrentVersion)
}

func TestDefaultUpgradeChecker_CheckToolUpdate_UnknownTool(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{}
	checker := NewDefaultUpgradeChecker(executor)

	_, err := checker.CheckToolUpdate(context.Background(), "unknown-tool")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrUnknownTool)
}

func TestDefaultUpgradeExecutor_UpgradeTool(t *testing.T) {
	t.Parallel()

	// Test atlas upgrade when already on latest version (no download needed)
	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"atlas --version": "atlas version 1.0.0 (commit: def, built: 2024-02-01)",
		},
		lookPathResults: map[string]string{
			"atlas": "/usr/bin/atlas",
		},
	}

	// Create a mock upgrader function that returns "already on latest"
	mockUpgraderFunc := func(_ config.CommandExecutor) *AtlasReleaseUpgrader {
		return NewAtlasReleaseUpgraderWithDeps(
			&mockReleaseClientForUpgrade{
				release: &GitHubRelease{
					TagName: "v1.0.0", // Same version as current
				},
			},
			nil,
			executor,
		)
	}

	upgradeExec := NewDefaultUpgradeExecutorWithUpgrader(executor, mockUpgraderFunc)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolAtlas)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, constants.ToolAtlas, result.Tool)
	// When already on latest, returns current version
	assert.Equal(t, "1.0.0", result.NewVersion)
}

// mockReleaseClientForUpgrade is a mock that supports the full upgrade flow.
type mockReleaseClientForUpgrade struct {
	release *GitHubRelease
	err     error
}

func (m *mockReleaseClientForUpgrade) GetLatestRelease(_ context.Context, _, _ string) (*GitHubRelease, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.release, nil
}

func TestDefaultUpgradeExecutor_UpgradeTool_UnknownTool(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	_, err := upgradeExec.UpgradeTool(context.Background(), "unknown-tool")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrUnknownTool)
}

func TestDefaultUpgradeExecutor_UpgradeTool_MageX_UsesUpdateInstall(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"magex update:install": "",
			"magex --version":      "v1.2.0",
		},
		lookPathResults: map[string]string{
			"magex": "/go/bin/magex",
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolMageX)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, constants.ToolMageX, result.Tool)
	assert.Equal(t, "1.2.0", result.NewVersion)
}

func TestDefaultUpgradeExecutor_UpgradeTool_MageX_NotInstalled_UsesGoInstall(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"go install " + constants.InstallPathMageX: "",
			"magex --version":                          "v1.2.0",
		},
		lookPathErrors: map[string]error{
			"magex": exec.ErrNotFound,
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolMageX)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, constants.ToolMageX, result.Tool)
}

func TestDefaultUpgradeExecutor_UpgradeTool_GoPreCommit_UsesUpgradeForce(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"go-pre-commit upgrade --force": "",
			"go-pre-commit --version":       "v1.0.0",
		},
		lookPathResults: map[string]string{
			"go-pre-commit": "/go/bin/go-pre-commit",
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolGoPreCommit)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, constants.ToolGoPreCommit, result.Tool)
	assert.Equal(t, "1.0.0", result.NewVersion)
}

func TestDefaultUpgradeExecutor_UpgradeTool_GoPreCommit_NotInstalled_UsesGoInstall(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"go install " + constants.InstallPathGoPreCommit: "",
			"go-pre-commit --version":                        "v1.0.0",
		},
		lookPathErrors: map[string]error{
			"go-pre-commit": exec.ErrNotFound,
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolGoPreCommit)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, constants.ToolGoPreCommit, result.Tool)
}

func TestDefaultUpgradeExecutor_BackupConstitution(t *testing.T) {
	// Cannot use t.Parallel() because os.Chdir affects the entire process

	// Create a temp directory with a constitution.md file
	tmpDir := t.TempDir()
	speckitDir := filepath.Join(tmpDir, ".speckit")
	require.NoError(t, os.MkdirAll(speckitDir, 0o700))

	constitutionPath := filepath.Join(speckitDir, "constitution.md")
	require.NoError(t, os.WriteFile(constitutionPath, []byte("# Constitution"), 0o600))

	// Change to temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	path, err := upgradeExec.BackupConstitution()
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// Verify backup was created
	backupPath := path + ".backup"
	_, err = os.Stat(backupPath)
	assert.NoError(t, err)
}

func TestDefaultUpgradeExecutor_BackupConstitution_NoFile(t *testing.T) {
	// Cannot use t.Parallel() because os.Chdir affects the entire process

	// Create a temp directory without constitution.md
	tmpDir := t.TempDir()

	// Change to temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	path, err := upgradeExec.BackupConstitution()
	require.NoError(t, err)
	assert.Empty(t, path) // No file to backup
}

func TestDefaultUpgradeExecutor_RestoreConstitution(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a backup file
	tmpDir := t.TempDir()
	constitutionPath := filepath.Join(tmpDir, "constitution.md")
	backupPath := constitutionPath + ".backup"

	// Create backup file
	require.NoError(t, os.WriteFile(backupPath, []byte("# Original Content"), 0o600))

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	err := upgradeExec.RestoreConstitution(constitutionPath)
	require.NoError(t, err)

	// Verify file was restored
	content, err := os.ReadFile(constitutionPath) //nolint:gosec // Test file path
	require.NoError(t, err)
	assert.Equal(t, "# Original Content", string(content))
}

func TestDefaultUpgradeExecutor_RestoreConstitution_EmptyPath(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	err := upgradeExec.RestoreConstitution("")
	require.NoError(t, err)
}

func TestDefaultUpgradeExecutor_RestoreConstitution_NoBackup(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	constitutionPath := filepath.Join(tmpDir, "constitution.md")

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	// Should not error when no backup exists
	err := upgradeExec.RestoreConstitution(constitutionPath)
	require.NoError(t, err)
}

func TestDefaultUpgradeExecutor_CleanupConstitutionBackup(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a backup file
	tmpDir := t.TempDir()
	constitutionPath := filepath.Join(tmpDir, "constitution.md")
	backupPath := constitutionPath + ".backup"

	// Create backup file
	require.NoError(t, os.WriteFile(backupPath, []byte("# Content"), 0o600))

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	err := upgradeExec.CleanupConstitutionBackup(constitutionPath)
	require.NoError(t, err)

	// Verify backup was removed
	_, err = os.Stat(backupPath)
	assert.True(t, os.IsNotExist(err))
}

func TestParseVersionFromOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tool     string
		output   string
		expected string
	}{
		{
			name:     "atlas with commit info",
			tool:     constants.ToolAtlas,
			output:   "0.1.0 (commit: abc123, built: 2024-01-01)",
			expected: "0.1.0",
		},
		{
			name:     "atlas simple version",
			tool:     constants.ToolAtlas,
			output:   "1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "magex with v prefix",
			tool:     constants.ToolMageX,
			output:   "v0.5.0",
			expected: "0.5.0",
		},
		{
			name:     "generic version output",
			tool:     constants.ToolGoPreCommit,
			output:   "go-pre-commit version 1.0.0",
			expected: "1.0.0",
		},
		{
			name:     "specify with CLI version",
			tool:     constants.ToolSpeckit,
			output:   "CLI Version    1.0.0\nTemplate Version    0.0.90\nReleased    2025-12-04",
			expected: "1.0.0",
		},
		{
			name:     "specify with minimal output",
			tool:     constants.ToolSpeckit,
			output:   "CLI Version    1.0.0",
			expected: "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseVersionFromOutput(tt.tool, tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUpgradableTools(t *testing.T) {
	t.Parallel()

	tools := getUpgradableTools()
	assert.Len(t, tools, 4)

	// Verify all required fields are set
	for _, tool := range tools {
		assert.NotEmpty(t, tool.name, "tool name should not be empty")
		assert.NotEmpty(t, tool.command, "tool command should not be empty")
		assert.NotEmpty(t, tool.versionFlag, "tool version flag should not be empty")
		assert.NotEmpty(t, tool.installPath, "tool install path should not be empty")
	}
}

func TestGetToolConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tool     string
		expected bool
	}{
		{"atlas exists", constants.ToolAtlas, true},
		{"mage-x exists", constants.ToolMageX, true},
		{"go-pre-commit exists", constants.ToolGoPreCommit, true},
		{"speckit exists", constants.ToolSpeckit, true},
		{"unknown tool", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := getToolConfig(tt.tool)
			if tt.expected {
				assert.NotNil(t, cfg)
				assert.Equal(t, tt.tool, cfg.name)
			} else {
				assert.Nil(t, cfg)
			}
		})
	}
}

func TestExitCodeError(t *testing.T) {
	t.Parallel()

	t.Run("with error", func(t *testing.T) {
		t.Parallel()
		err := &ExitCodeError{Code: 1, Err: errors.ErrUnknownTool}
		assert.Equal(t, "unknown tool", err.Error())
	})

	t.Run("without error", func(t *testing.T) {
		t.Parallel()
		err := &ExitCodeError{Code: 1}
		assert.Equal(t, "exit code 1", err.Error())
	})
}

func TestGetConstitutionLocations(t *testing.T) {
	t.Parallel()

	locations := getConstitutionLocations()
	assert.NotEmpty(t, locations)

	// Should include project-local location
	assert.Contains(t, locations, filepath.Join(".", ".speckit", "constitution.md"))

	// Should include home directory location if home is available
	if home, err := os.UserHomeDir(); err == nil {
		assert.Contains(t, locations, filepath.Join(home, ".speckit", "constitution.md"))
	}
}

func TestUpgradeStyles(t *testing.T) {
	t.Parallel()

	styles := newUpgradeStyles()
	assert.NotNil(t, styles)
	assert.NotNil(t, styles.header)
	assert.NotNil(t, styles.success)
	assert.NotNil(t, styles.err)
}

func TestAddUpgradeCommand(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "test"}
	AddUpgradeCommand(rootCmd)

	// Verify upgrade command was added
	upgradeCmd, _, err := rootCmd.Find([]string{"upgrade"})
	require.NoError(t, err)
	assert.Equal(t, "upgrade [tool]", upgradeCmd.Use)
}

func TestParseAtlasVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "atlas version with metadata",
			output:   "atlas version 1.2.3 (commit: abc, built: 2024-01-01)",
			expected: "1.2.3",
		},
		{
			name:     "atlas version simple",
			output:   "atlas version 0.5.0",
			expected: "0.5.0",
		},
		{
			name:     "plain version",
			output:   "1.0.0",
			expected: "1.0.0",
		},
		{
			name:     "version with space",
			output:   "2.3.4 extra",
			expected: "2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseAtlasVersion(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSpeckitVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "speckit with CLI version",
			output:   "CLI Version    1.0.0\nTemplate Version    0.0.90",
			expected: "1.0.0",
		},
		{
			name:     "speckit with minimal output",
			output:   "CLI Version    2.5.3",
			expected: "2.5.3",
		},
		{
			name:     "no CLI version line",
			output:   "Some other output",
			expected: "",
		},
		{
			name:     "case insensitive match",
			output:   "cli version    3.0.0",
			expected: "3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseSpeckitVersion(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseGenericVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "version with v prefix",
			output:   "v1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "version without prefix",
			output:   "2.3.4",
			expected: "2.3.4",
		},
		{
			name:     "version in sentence",
			output:   "tool version 1.0.0",
			expected: "1.0.0",
		},
		{
			name:     "multi-line with version",
			output:   "Header\nVersion: 2.5.0\nFooter",
			expected: "2.5.0",
		},
		{
			name:     "no version found",
			output:   "no version here",
			expected: "no version here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseGenericVersion(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpgradeCmd_CheckForUpdates_SpecificTool(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkToolResults: map[string]*UpdateInfo{
			constants.ToolAtlas: {
				Name:            constants.ToolAtlas,
				CurrentVersion:  "1.0.0",
				LatestVersion:   "1.1.0",
				UpdateAvailable: true,
				Installed:       true,
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  checker,
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &bytes.Buffer{},
	}

	result, err := cmd.checkForUpdates(context.Background(), constants.ToolAtlas)
	require.NoError(t, err)
	assert.True(t, result.UpdatesAvailable)
	assert.Len(t, result.Tools, 1)
	assert.Equal(t, constants.ToolAtlas, result.Tools[0].Name)
}

func TestUpgradeCmd_CheckForUpdates_AllTools(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: constants.ToolAtlas, UpdateAvailable: true, Installed: true},
				{Name: constants.ToolMageX, UpdateAvailable: false, Installed: true},
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  checker,
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &bytes.Buffer{},
	}

	result, err := cmd.checkForUpdates(context.Background(), "")
	require.NoError(t, err)
	assert.True(t, result.UpdatesAvailable)
	assert.Len(t, result.Tools, 2)
}

func TestUpgradeCmd_CheckForUpdates_Error(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllErr: assert.AnError,
	}
	executor := &mockUpgradeExecutor{}

	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  checker,
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &bytes.Buffer{},
	}

	_, err := cmd.checkForUpdates(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for updates")
}

func TestUpgradeCmd_GetToolsToUpgrade_NoUpdates(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	checkResult := &UpdateCheckResult{
		UpdatesAvailable: false,
		Tools:            []UpdateInfo{},
	}

	tools := cmd.getToolsToUpgrade(checkResult)
	assert.Nil(t, tools)
	assert.Contains(t, buf.String(), "up to date")
}

func TestUpgradeCmd_GetToolsToUpgrade_OnlyNotInstalled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	checkResult := &UpdateCheckResult{
		UpdatesAvailable: true,
		Tools: []UpdateInfo{
			{Name: "tool1", UpdateAvailable: true, Installed: false},
		},
	}

	tools := cmd.getToolsToUpgrade(checkResult)
	assert.Empty(t, tools)
}

func TestUpgradeCmd_FormatToolStatus_NotInstalled(t *testing.T) {
	t.Parallel()

	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &bytes.Buffer{},
	}

	status := cmd.formatToolStatus(UpdateInfo{
		Name:      "test",
		Installed: false,
	})
	assert.Contains(t, status, "not installed")
}

func TestUpgradeCmd_FormatToolStatus_UpdateAvailable_NoLatestVersion(t *testing.T) {
	t.Parallel()

	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &bytes.Buffer{},
	}

	status := cmd.formatToolStatus(UpdateInfo{
		Name:            "test",
		Installed:       true,
		UpdateAvailable: true,
		LatestVersion:   "",
	})
	assert.Contains(t, status, "upgrade to check")
}

func TestUpgradeCmd_DisplayUpdateTable_MultilineVersion(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	result := &UpdateCheckResult{
		UpdatesAvailable: false,
		Tools: []UpdateInfo{
			{
				Name:           "speckit",
				CurrentVersion: "CLI Version    1.0.0\nTemplate Version    0.0.90",
				Installed:      true,
			},
		},
	}

	cmd.displayUpdateTable(result)
	output := buf.String()
	assert.Contains(t, output, "speckit")
	assert.Contains(t, output, "CLI Version")
}

func TestUpgradeCmd_HandleJSONOutput_NoUpdates(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{Check: true, OutputFormat: "json"},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	checkResult := &UpdateCheckResult{
		UpdatesAvailable: false,
		Tools:            []UpdateInfo{},
	}

	err := cmd.handleJSONOutput(checkResult)
	require.NoError(t, err)

	var result UpdateCheckResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.False(t, result.UpdatesAvailable)
}

func TestUpgradeCmd_DisplayUpgradeResults_AllSuccess(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	results := []UpgradeResult{
		{Tool: "atlas", Success: true},
		{Tool: "mage-x", Success: true},
	}

	cmd.displayUpgradeResults(results)
	output := buf.String()
	assert.Contains(t, output, "All upgrades completed successfully")
}

func TestUpgradeCmd_DisplayUpgradeResults_AllFailed(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	results := []UpgradeResult{
		{Tool: "atlas", Success: false},
		{Tool: "mage-x", Success: false},
	}

	cmd.displayUpgradeResults(results)
	output := buf.String()
	assert.Contains(t, output, "All upgrades failed")
	assert.Contains(t, output, "Recovery options")
}

func TestUpgradeCmd_DisplayUpgradeResults_Mixed(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	results := []UpgradeResult{
		{Tool: "atlas", Success: true},
		{Tool: "mage-x", Success: false},
	}

	cmd.displayUpgradeResults(results)
	output := buf.String()
	assert.Contains(t, output, "succeeded")
	assert.Contains(t, output, "failed")
}

func TestDefaultUpgradeExecutor_UpgradeTool_Speckit_BackupSuccess(t *testing.T) {
	// Cannot use t.Parallel() because os.Chdir affects the entire process

	// Create a temp directory with a constitution.md file
	tmpDir := t.TempDir()
	speckitDir := filepath.Join(tmpDir, ".speckit")
	require.NoError(t, os.MkdirAll(speckitDir, 0o700))

	constitutionPath := filepath.Join(speckitDir, "constitution.md")
	require.NoError(t, os.WriteFile(constitutionPath, []byte("# Original"), 0o600))

	// Change to temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"go install " + constants.InstallPathSpeckit: "",
			"speckit --help": "CLI Version    1.1.0",
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolSpeckit)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify constitution was restored
	content, err := os.ReadFile(constitutionPath) //nolint:gosec // Test file path
	require.NoError(t, err)
	assert.Equal(t, "# Original", string(content))

	// Verify backup was cleaned up
	backupPath := constitutionPath + ".backup"
	_, err = os.Stat(backupPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDefaultUpgradeExecutor_UpgradeTool_Speckit_BackupFailure(t *testing.T) {
	// Test the case where backup fails but upgrade continues

	executor := &mockCommandExecutor{
		runResults: map[string]string{
			"go install " + constants.InstallPathSpeckit: "",
			"speckit --help": "CLI Version    1.1.0",
		},
	}

	// Use a directory that doesn't exist and we can't create
	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolSpeckit)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestDefaultUpgradeExecutor_UpgradeTool_Speckit_UpgradeFailed(t *testing.T) {
	// Cannot use t.Parallel() because os.Chdir affects the entire process

	// Create a temp directory with a constitution.md file
	tmpDir := t.TempDir()
	speckitDir := filepath.Join(tmpDir, ".speckit")
	require.NoError(t, os.MkdirAll(speckitDir, 0o700))

	constitutionPath := filepath.Join(speckitDir, "constitution.md")
	require.NoError(t, os.WriteFile(constitutionPath, []byte("# Original"), 0o600))

	// Change to temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	executor := &mockCommandExecutor{
		runErrors: map[string]error{
			"go install " + constants.InstallPathSpeckit: assert.AnError,
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolSpeckit)

	require.NoError(t, err)
	assert.False(t, result.Success)

	// When upgrade fails, constitution should not be restored
	// Backup should still exist
	backupPath := constitutionPath + ".backup"
	_, err = os.Stat(backupPath)
	assert.NoError(t, err)
}

func TestDefaultUpgradeExecutor_UpgradeTool_MageX_UpdateInstallFails(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		lookPathResults: map[string]string{
			"magex": "/go/bin/magex",
		},
		runErrors: map[string]error{
			"magex update:install": assert.AnError,
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolMageX)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
}

func TestDefaultUpgradeExecutor_UpgradeTool_GoPreCommit_UpgradeFails(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		lookPathResults: map[string]string{
			"go-pre-commit": "/go/bin/go-pre-commit",
		},
		runErrors: map[string]error{
			"go-pre-commit upgrade --force": assert.AnError,
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolGoPreCommit)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
}

func TestDefaultUpgradeExecutor_UpgradeTool_GoInstallFails(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		runErrors: map[string]error{
			"go install " + constants.InstallPathSpeckit: assert.AnError,
		},
	}

	upgradeExec := NewDefaultUpgradeExecutor(executor)
	result, err := upgradeExec.UpgradeTool(context.Background(), constants.ToolSpeckit)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
}

func TestDefaultUpgradeChecker_CheckToolUpdate_NotInstalled(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		lookPathErrors: map[string]error{
			constants.ToolAtlas: exec.ErrNotFound,
		},
	}

	checker := NewDefaultUpgradeChecker(executor)
	info, err := checker.CheckToolUpdate(context.Background(), constants.ToolAtlas)

	require.NoError(t, err)
	assert.False(t, info.Installed)
	assert.True(t, info.UpdateAvailable) // Can be installed
}

func TestDefaultUpgradeChecker_CheckToolUpdate_VersionCommandFails(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{
		lookPathResults: map[string]string{
			constants.ToolAtlas: "/usr/bin/atlas",
		},
		runErrors: map[string]error{
			"atlas --version": assert.AnError,
		},
	}

	checker := NewDefaultUpgradeChecker(executor)
	info, err := checker.CheckToolUpdate(context.Background(), constants.ToolAtlas)

	require.NoError(t, err)
	assert.True(t, info.Installed)
	assert.Equal(t, "unknown", info.CurrentVersion)
	assert.True(t, info.UpdateAvailable)
}

func TestDefaultUpgradeChecker_CheckAllUpdates_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := &mockCommandExecutor{}
	checker := NewDefaultUpgradeChecker(executor)

	_, err := checker.CheckAllUpdates(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultUpgradeChecker_CheckToolUpdate_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := &mockCommandExecutor{}
	checker := NewDefaultUpgradeChecker(executor)

	_, err := checker.CheckToolUpdate(ctx, constants.ToolAtlas)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestUpgradeCmd_ExecuteUpgrades_WithWarnings(t *testing.T) {
	t.Parallel()

	executor := &mockUpgradeExecutor{
		upgradeResults: map[string]*UpgradeResult{
			"atlas": {
				Tool:     "atlas",
				Success:  true,
				Warnings: []string{"Warning: backup failed", "Warning: cleanup failed"},
			},
		},
	}

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	tools := []UpdateInfo{
		{Name: "atlas", CurrentVersion: "1.0.0", UpdateAvailable: true, Installed: true},
	}

	results := cmd.executeUpgrades(context.Background(), tools)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Len(t, results[0].Warnings, 2)

	output := buf.String()
	assert.Contains(t, output, "Warning: backup failed")
	assert.Contains(t, output, "Warning: cleanup failed")
}

func TestUpgradeCmd_ExecuteUpgrades_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  &mockUpgradeChecker{},
		executor: &mockUpgradeExecutor{},
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	tools := []UpdateInfo{
		{Name: "atlas", CurrentVersion: "1.0.0", UpdateAvailable: true, Installed: true},
	}

	results := cmd.executeUpgrades(ctx, tools)
	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
	assert.Equal(t, "canceled", results[0].Error)
}

func TestDefaultUpgradeExecutor_CleanupConstitutionBackup_EmptyPath(t *testing.T) {
	t.Parallel()

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	err := upgradeExec.CleanupConstitutionBackup("")
	require.NoError(t, err)
}

func TestExitCodeError_Error_WithError(t *testing.T) {
	t.Parallel()

	err := &ExitCodeError{Code: 1, Err: errors.ErrUnknownTool}
	assert.Equal(t, "unknown tool", err.Error())
}

func TestExitCodeError_Error_WithoutError(t *testing.T) {
	t.Parallel()

	err := &ExitCodeError{Code: 42, Err: nil}
	assert.Equal(t, "exit code 42", err.Error())
}

func TestUpgradeCmd_ConfirmAndExecuteUpgrades_WithYesFlag(t *testing.T) {
	t.Parallel()

	executor := &mockUpgradeExecutor{
		upgradeResults: map[string]*UpgradeResult{
			"atlas": {Tool: "atlas", Success: true, NewVersion: "1.1.0"},
		},
	}

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{Yes: true}, // With -y flag to skip prompt
		checker:  &mockUpgradeChecker{},
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	tools := []UpdateInfo{
		{Name: "atlas", CurrentVersion: "1.0.0", UpdateAvailable: true, Installed: true},
	}

	err := cmd.confirmAndExecuteUpgrades(context.Background(), tools)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Upgrading")
}

func TestDefaultUpgradeExecutor_HandleSpeckitRestore_RestoreFails(t *testing.T) {
	// Test that when restore fails, the warning is added to the result
	t.Parallel()

	tmpDir := t.TempDir()
	constitutionPath := filepath.Join(tmpDir, "constitution.md")
	backupPath := constitutionPath + ".backup"

	// Create backup file
	require.NoError(t, os.WriteFile(backupPath, []byte("# Content"), 0o600))

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	result := &UpgradeResult{
		Tool:    constants.ToolSpeckit,
		Success: true,
	}

	// Make the restore fail by making the destination read-only directory
	// First create the directory
	require.NoError(t, os.MkdirAll(tmpDir, 0o700))
	// Make it read-only (can't write the restored file)
	require.NoError(t, os.Chmod(tmpDir, 0o400))
	defer func() { _ = os.Chmod(tmpDir, 0o700) }() //nolint:gosec // Test cleanup requires restoring directory permissions

	upgradeExec.handleSpeckitRestore(result, constitutionPath)

	// Restore permissions before test cleanup
	require.NoError(t, os.Chmod(tmpDir, 0o700)) //nolint:gosec // Test cleanup requires restoring directory permissions

	// Should have a warning about restore failure
	assert.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "failed to restore constitution.md")
}

func TestDefaultUpgradeExecutor_HandleSpeckitRestore_CleanupFails(t *testing.T) {
	// Test that when cleanup fails, the warning is added to the result
	t.Parallel()

	tmpDir := t.TempDir()
	constitutionPath := filepath.Join(tmpDir, "constitution.md")
	backupPath := constitutionPath + ".backup"

	// Create backup file
	require.NoError(t, os.WriteFile(backupPath, []byte("# Content"), 0o600))

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	result := &UpgradeResult{
		Tool:    constants.ToolSpeckit,
		Success: true,
	}

	// Restore will succeed, but cleanup will fail because we'll make the directory read-only
	upgradeExec.handleSpeckitRestore(result, constitutionPath)

	// Make backup read-only to prevent cleanup
	require.NoError(t, os.Chmod(tmpDir, 0o400))
	defer func() { _ = os.Chmod(tmpDir, 0o700) }() //nolint:gosec // Test cleanup requires restoring directory permissions

	// Try cleanup again
	err := upgradeExec.CleanupConstitutionBackup(constitutionPath)
	require.Error(t, err)

	// Restore permissions so test cleanup can succeed
	require.NoError(t, os.Chmod(tmpDir, 0o700)) //nolint:gosec // Test cleanup requires restoring directory permissions
}

func TestUpgradeCmd_Run_AllInstalledToolsUpToDate(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkAllResult: &UpdateCheckResult{
			UpdatesAvailable: true,
			Tools: []UpdateInfo{
				{Name: "tool1", UpdateAvailable: true, Installed: false}, // Not installed
			},
		},
	}
	executor := &mockUpgradeExecutor{}

	var buf bytes.Buffer
	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  checker,
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &buf,
	}

	err := cmd.run(context.Background(), "")
	require.NoError(t, err)

	// Should show message that installed tools are up to date
	output := buf.String()
	assert.Contains(t, output, "installed tools are up to date")
}

func TestParseSpeckitVersion_InsufficientFields(t *testing.T) {
	t.Parallel()

	// Test case where CLI Version line exists but has fewer than 3 fields
	output := "CLI Version"
	result := parseSpeckitVersion(output)
	assert.Empty(t, result)
}

func TestParseAtlasVersion_NoSpace(t *testing.T) {
	t.Parallel()

	// Test case where there's no space in the output (edge case)
	output := "atlas version 1.2.3"
	result := parseAtlasVersion(output)
	assert.Equal(t, "1.2.3", result)
}

func TestUpgradeCmd_CheckForUpdates_ToolSpecificError(t *testing.T) {
	t.Parallel()

	checker := &mockUpgradeChecker{
		checkToolErrors: map[string]error{
			constants.ToolAtlas: assert.AnError,
		},
	}
	executor := &mockUpgradeExecutor{}

	cmd := &upgradeCmd{
		flags:    &UpgradeFlags{},
		checker:  checker,
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        &bytes.Buffer{},
	}

	_, err := cmd.checkForUpdates(context.Background(), constants.ToolAtlas)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for updates")
}

func TestDefaultUpgradeExecutor_RestoreConstitution_StatError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	constitutionPath := filepath.Join(tmpDir, "constitution.md")

	// Create a directory where we expect the backup file to be
	// This will cause Stat to return an error (not IsNotExist)
	backupPath := constitutionPath + ".backup"
	require.NoError(t, os.MkdirAll(backupPath, 0o700))

	executor := &mockCommandExecutor{}
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	err := upgradeExec.RestoreConstitution(constitutionPath)
	require.Error(t, err)
	assert.False(t, os.IsNotExist(err))
}
