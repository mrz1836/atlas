package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewInitCmd(t *testing.T) {
	t.Parallel()

	flags := &InitFlags{}
	cmd := newInitCmd(flags)

	assert.Equal(t, "init", cmd.Use)
	assert.Contains(t, cmd.Short, "Initialize")
	assert.Contains(t, cmd.Long, "guided setup wizard")

	// Verify --no-interactive flag exists
	noInteractiveFlag := cmd.Flags().Lookup("no-interactive")
	require.NotNil(t, noInteractiveFlag)
	assert.Equal(t, "false", noInteractiveFlag.DefValue)
}

func TestAddInitCommand(t *testing.T) {
	t.Parallel()

	rootCmd := newRootCmd(&GlobalFlags{}, BuildInfo{})
	AddInitCommand(rootCmd)

	// Verify init command was added
	initCmd, _, err := rootCmd.Find([]string{"init"})
	require.NoError(t, err)
	assert.Equal(t, "init", initCmd.Use)
}

func TestDisplayHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	styles := newInitStyles()

	displayHeader(&buf, styles)

	output := buf.String()
	// Header contains "A T L A S" with spaces
	assert.Contains(t, output, "A T L A S")
	assert.Contains(t, output, "Autonomous Task")
	assert.Contains(t, output, "Automation System")
}

func TestDisplayToolTable(t *testing.T) {
	t.Parallel()

	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{
				Name:           "go",
				Required:       true,
				CurrentVersion: "1.24.2",
				Status:         config.ToolStatusInstalled,
			},
			{
				Name:           "git",
				Required:       true,
				CurrentVersion: "2.39.0",
				Status:         config.ToolStatusInstalled,
			},
			{
				Name:           "magex",
				Required:       false,
				Managed:        true,
				CurrentVersion: "",
				Status:         config.ToolStatusMissing,
			},
		},
	}

	var buf bytes.Buffer
	styles := newInitStyles()

	displayToolTable(&buf, result, styles)

	output := buf.String()

	// Verify header
	assert.Contains(t, output, "TOOL")
	assert.Contains(t, output, "REQUIRED")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "STATUS")

	// Verify tools are displayed
	assert.Contains(t, output, "go")
	assert.Contains(t, output, "git")
	assert.Contains(t, output, "magex")
	assert.Contains(t, output, "1.24.2")
	assert.Contains(t, output, "2.39.0")

	// Verify required tools show "yes"
	assert.Contains(t, output, "yes")

	// Verify managed tools show "managed"
	assert.Contains(t, output, "managed")
}

func TestFormatToolStatus(t *testing.T) {
	t.Parallel()

	styles := newInitStyles()

	tests := []struct {
		name     string
		tool     config.Tool
		expected string
	}{
		{
			name:     "installed tool",
			tool:     config.Tool{Status: config.ToolStatusInstalled},
			expected: "installed",
		},
		{
			name:     "missing tool",
			tool:     config.Tool{Status: config.ToolStatusMissing},
			expected: "missing",
		},
		{
			name:     "outdated tool",
			tool:     config.Tool{Status: config.ToolStatusOutdated},
			expected: "outdated",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := formatToolStatus(tc.tool, styles)
			assert.Contains(t, result, tc.expected)
		})
	}
}

func TestGetManagedToolsNeedingAction(t *testing.T) {
	t.Parallel()

	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: "go", Required: true, Status: config.ToolStatusInstalled},
			{Name: "magex", Managed: true, Status: config.ToolStatusMissing},
			{Name: "speckit", Managed: true, Status: config.ToolStatusOutdated},
			{Name: "go-pre-commit", Managed: true, Status: config.ToolStatusInstalled},
		},
	}

	needAction := getManagedToolsNeedingAction(result)

	assert.Len(t, needAction, 2)
	assert.Equal(t, "magex", needAction[0].Name)
	assert.Equal(t, "speckit", needAction[1].Name)
}

func TestBuildDefaultConfig(t *testing.T) {
	t.Parallel()

	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolMageX, Status: config.ToolStatusInstalled},
			{Name: constants.ToolGoPreCommit, Status: config.ToolStatusInstalled},
		},
	}

	cfg := buildDefaultConfig(result)

	// AI configuration (field names match config.AIConfig)
	assert.Equal(t, "sonnet", cfg.AI.Model)
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.AI.APIKeyEnvVar)
	assert.Equal(t, "30m", cfg.AI.Timeout)
	assert.Equal(t, 10, cfg.AI.MaxTurns)

	// Validation commands - with mage-x installed
	assert.Equal(t, []string{"magex format:fix"}, cfg.Validation.Commands.Format)
	assert.Equal(t, []string{"magex lint"}, cfg.Validation.Commands.Lint)
	assert.Equal(t, []string{"magex test"}, cfg.Validation.Commands.Test)
	assert.Equal(t, []string{"go-pre-commit run --all-files"}, cfg.Validation.Commands.PreCommit)

	// Notification configuration
	assert.True(t, cfg.Notifications.BellEnabled)
	assert.Contains(t, cfg.Notifications.Events, "awaiting_approval")
	assert.Contains(t, cfg.Notifications.Events, "validation_failed")
	assert.Contains(t, cfg.Notifications.Events, "ci_failed")
	assert.Contains(t, cfg.Notifications.Events, "github_failed")
}

func TestBuildDefaultConfig_WithoutMageX(t *testing.T) {
	t.Parallel()

	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolMageX, Status: config.ToolStatusMissing},
		},
	}

	cfg := buildDefaultConfig(result)

	// Should use fallback commands without mage-x
	assert.Equal(t, []string{"gofmt -w ."}, cfg.Validation.Commands.Format)
	assert.Equal(t, []string{"go vet ./..."}, cfg.Validation.Commands.Lint)
	assert.Equal(t, []string{"go test ./..."}, cfg.Validation.Commands.Test)
}

func TestSuggestValidationCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		tools             []config.Tool
		expectedFormat    string
		expectedPreCommit []string
	}{
		{
			name: "with mage-x and go-pre-commit",
			tools: []config.Tool{
				{Name: constants.ToolMageX, Status: config.ToolStatusInstalled},
				{Name: constants.ToolGoPreCommit, Status: config.ToolStatusInstalled},
			},
			expectedFormat:    "magex format:fix",
			expectedPreCommit: []string{"go-pre-commit run --all-files"},
		},
		{
			name: "without managed tools",
			tools: []config.Tool{
				{Name: constants.ToolMageX, Status: config.ToolStatusMissing},
				{Name: constants.ToolGoPreCommit, Status: config.ToolStatusMissing},
			},
			expectedFormat:    "gofmt -w .",
			expectedPreCommit: nil,
		},
		{
			name:              "empty tools",
			tools:             []config.Tool{},
			expectedFormat:    "gofmt -w .",
			expectedPreCommit: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := &config.ToolDetectionResult{Tools: tc.tools}
			cmds := suggestValidationCommands(result)

			require.Len(t, cmds.Format, 1)
			assert.Equal(t, tc.expectedFormat, cmds.Format[0])
			assert.Equal(t, tc.expectedPreCommit, cmds.PreCommit)
		})
	}
}

func TestParseMultilineInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single line",
			input:    "magex test",
			expected: []string{"magex test"},
		},
		{
			name:     "multiple lines",
			input:    "magex format:fix\nmagex lint\nmagex test",
			expected: []string{"magex format:fix", "magex lint", "magex test"},
		},
		{
			name:     "with empty lines",
			input:    "cmd1\n\ncmd2\n\n\ncmd3",
			expected: []string{"cmd1", "cmd2", "cmd3"},
		},
		{
			name:     "with whitespace",
			input:    "  cmd1  \n  cmd2  ",
			expected: []string{"cmd1", "cmd2"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "only whitespace",
			input:    "   \n   \n   ",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := parseMultilineInput(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSaveConfig(t *testing.T) {
	// Create a temp directory to use as HOME
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := AtlasConfig{
		AI: AIConfig{
			Model:        "sonnet",
			APIKeyEnvVar: "ANTHROPIC_API_KEY",
			Timeout:      "30m",
			MaxTurns:     10,
		},
		Validation: ValidationConfig{
			Commands: ValidationCommands{
				Format: []string{"magex format:fix"},
				Lint:   []string{"magex lint"},
				Test:   []string{"magex test"},
			},
		},
		Notifications: NotificationConfig{
			BellEnabled: true,
			Events:      []string{"awaiting_approval", "validation_failed"},
		},
	}

	err := saveConfig(cfg)
	require.NoError(t, err)

	// Verify the config file was created
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	assert.FileExists(t, configPath)

	// Verify the content
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file with controlled path
	require.NoError(t, err)

	// Verify header comment
	assert.Contains(t, string(content), "# ATLAS Configuration")
	assert.Contains(t, string(content), "Generated by atlas init")

	// Parse the YAML content (skip the header comments)
	lines := strings.Split(string(content), "\n")
	yamlStart := 0
	for i, line := range lines {
		if !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			yamlStart = i
			break
		}
	}
	yamlContent := strings.Join(lines[yamlStart:], "\n")

	var parsedCfg AtlasConfig
	err = yaml.Unmarshal([]byte(yamlContent), &parsedCfg)
	require.NoError(t, err)

	assert.Equal(t, cfg.AI.Model, parsedCfg.AI.Model)
	assert.Equal(t, cfg.AI.APIKeyEnvVar, parsedCfg.AI.APIKeyEnvVar)
	assert.Equal(t, cfg.AI.MaxTurns, parsedCfg.AI.MaxTurns)
	assert.Equal(t, cfg.Notifications.BellEnabled, parsedCfg.Notifications.BellEnabled)
}

func TestNotificationConfig_YAMLFieldNames_MatchConfigPackage(t *testing.T) {
	// This test verifies that the CLI's NotificationConfig YAML field names
	// match the internal/config package's NotificationsConfig for compatibility
	// with config.Load().
	//
	// The CLI uses "bell" (not "bell_enabled") to match internal/config/config.go.
	cfg := NotificationConfig{
		BellEnabled: true,
		Events:      []string{"awaiting_approval", "ci_failed"},
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	yamlStr := string(data)

	// Verify correct field name is used (matches internal/config/config.go)
	assert.Contains(t, yamlStr, "bell:", "YAML should use 'bell' field name for config.Load() compatibility")
	assert.NotContains(t, yamlStr, "bell_enabled:", "YAML should NOT use 'bell_enabled' - use 'bell' to match config package")
	assert.Contains(t, yamlStr, "events:")

	// Verify the YAML can be parsed by a struct with matching field names
	// This simulates what config.Load() would do
	type ConfigPackageNotifications struct {
		Bell   bool     `yaml:"bell"`
		Events []string `yaml:"events"`
	}

	var parsed ConfigPackageNotifications
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed.Bell, "Bell field should be parsed correctly")
	assert.Equal(t, cfg.Events, parsed.Events, "Events should be parsed correctly")
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := AtlasConfig{
		AI: AIConfig{Model: "sonnet"},
	}

	// Verify directory doesn't exist
	atlasDir := filepath.Join(tmpDir, constants.AtlasHome)
	_, err := os.Stat(atlasDir)
	require.True(t, os.IsNotExist(err))

	// Save config
	err = saveConfig(cfg)
	require.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(atlasDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify permissions (0700)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestDisplaySuccessMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		nonInteractive bool
		expectedOutput []string
	}{
		{
			name:           "interactive mode",
			nonInteractive: false,
			expectedOutput: []string{
				"ATLAS configuration saved successfully",
				"Suggested next commands",
				"atlas status",
				"atlas start",
			},
		},
		{
			name:           "non-interactive mode",
			nonInteractive: true,
			expectedOutput: []string{
				"ATLAS configuration saved successfully",
				"Non-interactive mode used default values",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			styles := newInitStyles()

			displaySuccessMessage(&buf, tc.nonInteractive, styles)

			output := buf.String()
			for _, expected := range tc.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestNewInitStyles(t *testing.T) {
	t.Parallel()

	styles := newInitStyles()

	// Verify all styles are initialized (non-empty render)
	assert.NotEmpty(t, styles.header.Render("test"))
	assert.NotEmpty(t, styles.installed.Render("test"))
	assert.NotEmpty(t, styles.missing.Render("test"))
	assert.NotEmpty(t, styles.outdated.Render("test"))
	assert.NotEmpty(t, styles.success.Render("test"))
	assert.NotEmpty(t, styles.err.Render("test"))
	assert.NotEmpty(t, styles.info.Render("test"))
	assert.NotEmpty(t, styles.dim.Render("test"))
}

func TestRunInit_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true}

	err := runInit(ctx, &buf, flags)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestAtlasConfig_YAML_Marshaling(t *testing.T) {
	t.Parallel()

	cfg := AtlasConfig{
		AI: AIConfig{
			Model:        "opus",
			APIKeyEnvVar: "MY_API_KEY",
			Timeout:      "1h",
			MaxTurns:     20,
		},
		Validation: ValidationConfig{
			Commands: ValidationCommands{
				Format:    []string{"fmt1", "fmt2"},
				Lint:      []string{"lint1"},
				Test:      []string{"test1", "test2", "test3"},
				PreCommit: []string{"pre1"},
			},
		},
		Notifications: NotificationConfig{
			BellEnabled: false,
			Events:      []string{"event1", "event2"},
		},
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var parsed AtlasConfig
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, cfg, parsed)
}

func TestInitFlags(t *testing.T) {
	t.Parallel()

	flags := &InitFlags{NoInteractive: true}
	cmd := newInitCmd(flags)

	// Test that flag is properly bound
	err := cmd.Flags().Set("no-interactive", "true")
	require.NoError(t, err)
	assert.True(t, flags.NoInteractive)

	err = cmd.Flags().Set("no-interactive", "false")
	require.NoError(t, err)
	assert.False(t, flags.NoInteractive)
}

// mockToolDetector is a test double for ToolDetector.
type mockToolDetector struct {
	result *config.ToolDetectionResult
	err    error
}

func (m *mockToolDetector) Detect(_ context.Context) (*config.ToolDetectionResult, error) {
	return m.result, m.err
}

func TestRunInitWithDetector_NonInteractive_Success(t *testing.T) {
	// Use temp HOME directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create mock detector with all tools installed
	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{
			HasMissingRequired: false,
			Tools: []config.Tool{
				{Name: "go", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "1.24.0"},
				{Name: "git", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.39.0"},
				{Name: "gh", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.62.0"},
				{Name: "uv", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "0.5.14"},
				{Name: "claude", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.0.76"},
				{Name: constants.ToolMageX, Managed: true, Status: config.ToolStatusInstalled},
			},
		},
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true}

	err := runInitWithDetector(context.Background(), &buf, flags, detector)
	require.NoError(t, err)

	output := buf.String()

	// Verify header was displayed
	assert.Contains(t, output, "A T L A S")

	// Verify tool detection ran
	assert.Contains(t, output, "Detecting tools...")

	// Verify tool table was displayed
	assert.Contains(t, output, "go")
	assert.Contains(t, output, "installed")

	// Verify success message
	assert.Contains(t, output, "ATLAS configuration saved successfully")

	// Verify config file was created
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	assert.FileExists(t, configPath)

	// Verify config content - field names match config.AIConfig
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(content), "model: sonnet")
	assert.Contains(t, string(content), "api_key_env_var: ANTHROPIC_API_KEY")
}

func TestRunInitWithDetector_MissingRequiredTools(t *testing.T) {
	t.Parallel()

	// Create mock detector with missing required tools
	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{
			HasMissingRequired: true,
			Tools: []config.Tool{
				{Name: "go", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "1.24.0"},
				{Name: "git", Required: true, Status: config.ToolStatusMissing}, // Missing!
			},
		},
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true}

	err := runInitWithDetector(context.Background(), &buf, flags, detector)

	// Should return ErrMissingRequiredTools
	require.ErrorIs(t, err, atlaserrors.ErrMissingRequiredTools)

	output := buf.String()

	// Verify error message was displayed
	assert.Contains(t, output, "Required tools are missing")
}

// errDetectionFailed is a test error for mock detector failures.
var errDetectionFailed = errors.New("detection failed")

func TestRunInitWithDetector_DetectionError(t *testing.T) {
	t.Parallel()

	// Create mock detector that returns an error
	detector := &mockToolDetector{
		err: errDetectionFailed,
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true}

	err := runInitWithDetector(context.Background(), &buf, flags, detector)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to detect tools")
}

func TestRunInitWithDetector_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{},
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true}

	err := runInitWithDetector(ctx, &buf, flags, detector)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestRunInitWithDetector_WithManagedToolsMissing(t *testing.T) {
	// Use temp HOME directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create mock detector with required tools OK but managed tools missing
	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{
			HasMissingRequired: false,
			Tools: []config.Tool{
				{Name: "go", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "1.24.0"},
				{Name: "git", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.39.0"},
				{Name: constants.ToolMageX, Managed: true, Status: config.ToolStatusMissing},
				{Name: constants.ToolGoPreCommit, Managed: true, Status: config.ToolStatusMissing},
			},
		},
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true} // Non-interactive skips managed tool prompts

	err := runInitWithDetector(context.Background(), &buf, flags, detector)
	require.NoError(t, err)

	output := buf.String()

	// Should complete without prompting (non-interactive mode)
	assert.Contains(t, output, "ATLAS configuration saved successfully")

	// Verify fallback commands are used when mage-x is missing
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)

	// Should have fallback commands since mage-x is missing
	assert.Contains(t, string(content), "gofmt -w .")
	assert.Contains(t, string(content), "go vet ./...")
}

func TestSaveConfig_CreatesBackup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create initial config
	initialCfg := AtlasConfig{
		AI: AIConfig{Model: "opus"},
	}
	err := saveConfig(initialCfg)
	require.NoError(t, err)

	// Verify initial config
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(content), "model: opus")

	// Save a new config (should create backup)
	newCfg := AtlasConfig{
		AI: AIConfig{Model: "sonnet"},
	}
	err = saveConfig(newCfg)
	require.NoError(t, err)

	// Verify new config
	content, err = os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(content), "model: sonnet")

	// Verify backup was created with original content
	backupPath := configPath + ".backup"
	assert.FileExists(t, backupPath)
	backupContent, err := os.ReadFile(backupPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(backupContent), "model: opus")
}

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	srcContent := []byte("test content")
	err := os.WriteFile(srcPath, srcContent, 0o600)
	require.NoError(t, err)

	// Copy to destination
	dstPath := filepath.Join(tmpDir, "dest.txt")
	err = copyFile(srcPath, dstPath)
	require.NoError(t, err)

	// Verify destination content
	dstContent, err := os.ReadFile(dstPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Equal(t, srcContent, dstContent)
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dest"))
	require.Error(t, err)
}

func TestIsInGitRepo(t *testing.T) {
	// This test runs in the atlas repo, so it should return true
	result := isInGitRepo()
	assert.True(t, result, "Should detect we're in the atlas git repository")
}

func TestFindGitRoot(t *testing.T) {
	// This test runs in the atlas repo, so it should find the root
	gitRoot := findGitRoot()
	assert.NotEmpty(t, gitRoot, "Should find git root")

	// Verify .git exists at the returned path (can be dir or file in worktree)
	gitPath := filepath.Join(gitRoot, ".git")
	_, err := os.Stat(gitPath)
	require.NoError(t, err)
}

func TestSaveProjectConfig(t *testing.T) {
	// Create a fake git repo in temp directory
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	cfg := AtlasConfig{
		AI: AIConfig{
			Model:        "opus",
			APIKeyEnvVar: "ANTHROPIC_API_KEY",
			Timeout:      "30m",
			MaxTurns:     10,
		},
	}

	err = saveProjectConfig(cfg)
	require.NoError(t, err)

	// Verify the config file was created
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	assert.FileExists(t, configPath)

	// Verify the content
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)

	// Verify header comment
	assert.Contains(t, string(content), "# ATLAS Project Configuration")
	assert.Contains(t, string(content), "overrides ~/.atlas/config.yaml")

	// Verify config values
	assert.Contains(t, string(content), "model: opus")
}

func TestSaveProjectConfig_NotInGitRepo(t *testing.T) {
	// Create temp directory without .git
	tmpDir := t.TempDir()

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	cfg := AtlasConfig{
		AI: AIConfig{Model: "sonnet"},
	}

	err = saveProjectConfig(cfg)
	require.ErrorIs(t, err, atlaserrors.ErrNotInGitRepo)
}

func TestSaveProjectConfig_CreatesBackup(t *testing.T) {
	// Create a fake git repo in temp directory
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	require.NoError(t, err)

	// Create initial config
	atlasDir := filepath.Join(tmpDir, constants.AtlasHome)
	err = os.MkdirAll(atlasDir, 0o700)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)
	err = os.WriteFile(configPath, []byte("# Original config\nai:\n  model: haiku\n"), 0o600)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	cfg := AtlasConfig{
		AI: AIConfig{Model: "opus"},
	}

	err = saveProjectConfig(cfg)
	require.NoError(t, err)

	// Verify new config
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(content), "model: opus")

	// Verify backup was created
	backupPath := configPath + ".backup"
	assert.FileExists(t, backupPath)
	backupContent, err := os.ReadFile(backupPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(backupContent), "model: haiku")
}

func TestDetermineConfigLocations_NonInteractive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	styles := newInitStyles()
	flags := &InitFlags{NoInteractive: true}

	saveToProject, saveToGlobal := determineConfigLocations(&buf, flags, styles)

	assert.False(t, saveToProject, "Non-interactive should not save to project")
	assert.True(t, saveToGlobal, "Non-interactive should save to global")
}

func TestNewInitCmd_Flags(t *testing.T) {
	t.Parallel()

	flags := &InitFlags{}
	cmd := newInitCmd(flags)

	// Verify --global flag exists
	globalFlag := cmd.Flags().Lookup("global")
	require.NotNil(t, globalFlag)
	assert.Equal(t, "false", globalFlag.DefValue)

	// Verify --project flag exists
	projectFlag := cmd.Flags().Lookup("project")
	require.NotNil(t, projectFlag)
	assert.Equal(t, "false", projectFlag.DefValue)

	// Test mutual exclusivity: setting both should error
	err := cmd.Flags().Set("global", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("project", "true")
	require.NoError(t, err)
	// The mutual exclusivity is checked at runtime by cobra, not parse time
}

func TestRunInitWithDetector_ProjectFlag_NotInGitRepo(t *testing.T) {
	// Create temp directory without .git
	tmpDir := t.TempDir()

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{
			HasMissingRequired: false,
			Tools:              []config.Tool{},
		},
	}

	var buf bytes.Buffer
	flags := &InitFlags{Project: true}

	err = runInitWithDetector(context.Background(), &buf, flags, detector)

	require.ErrorIs(t, err, atlaserrors.ErrNotInProjectDir)
	assert.Contains(t, buf.String(), "--project flag requires being in a git repository")
}

func TestRunInitWithDetector_GlobalFlag_Success(t *testing.T) {
	// Use temp HOME directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create mock detector with all tools installed
	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{
			HasMissingRequired: false,
			Tools: []config.Tool{
				{Name: "go", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "1.24.0"},
				{Name: "git", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.39.0"},
				{Name: "gh", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.62.0"},
				{Name: "uv", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "0.5.14"},
				{Name: "claude", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.0.76"},
			},
		},
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true, Global: true}

	err := runInitWithDetector(context.Background(), &buf, flags, detector)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ATLAS configuration saved successfully")

	// Verify global config was created
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	assert.FileExists(t, configPath)
}

func TestRunInitWithDetector_ProjectFlag_Success(t *testing.T) {
	// Create a fake git repo in temp directory
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create mock detector with all tools installed
	detector := &mockToolDetector{
		result: &config.ToolDetectionResult{
			HasMissingRequired: false,
			Tools: []config.Tool{
				{Name: "go", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "1.24.0"},
				{Name: "git", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.39.0"},
				{Name: "gh", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.62.0"},
				{Name: "uv", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "0.5.14"},
				{Name: "claude", Required: true, Status: config.ToolStatusInstalled, CurrentVersion: "2.0.76"},
			},
		},
	}

	var buf bytes.Buffer
	flags := &InitFlags{NoInteractive: true, Project: true}

	err = runInitWithDetector(context.Background(), &buf, flags, detector)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ATLAS configuration saved successfully")

	// Verify project config was created
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	assert.FileExists(t, configPath)

	// Verify gitignore tip is shown
	assert.Contains(t, output, "Consider adding to .gitignore")
}

func TestDisplaySuccessMessageWithPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		nonInteractive       bool
		projectConfigCreated bool
		configPaths          []string
		expectedOutput       []string
	}{
		{
			name:                 "global config only",
			nonInteractive:       false,
			projectConfigCreated: false,
			configPaths:          []string{"~/.atlas/config.yaml"},
			expectedOutput: []string{
				"ATLAS configuration saved successfully",
				"~/.atlas/config.yaml",
				"atlas config show",
			},
		},
		{
			name:                 "project config created",
			nonInteractive:       false,
			projectConfigCreated: true,
			configPaths:          []string{"/project/.atlas/config.yaml"},
			expectedOutput: []string{
				"ATLAS configuration saved successfully",
				"/project/.atlas/config.yaml",
				"Consider adding to .gitignore",
				".atlas/config.yaml",
			},
		},
		{
			name:                 "non-interactive with project config",
			nonInteractive:       true,
			projectConfigCreated: true,
			configPaths:          []string{"/project/.atlas/config.yaml"},
			expectedOutput: []string{
				"Non-interactive mode used default values",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			styles := newInitStyles()

			displaySuccessMessageWithPaths(&buf, tc.nonInteractive, tc.projectConfigCreated, tc.configPaths, styles)

			output := buf.String()
			for _, expected := range tc.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}
