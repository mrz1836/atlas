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
