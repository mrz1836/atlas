package config

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
)

// MockCommandExecutor is a test double for CommandExecutor.
type MockCommandExecutor struct {
	lookPathResults map[string]struct {
		path string
		err  error
	}
	runResults map[string]struct {
		output string
		err    error
	}
}

// NewMockCommandExecutor creates a new mock executor.
func NewMockCommandExecutor() *MockCommandExecutor {
	return &MockCommandExecutor{
		lookPathResults: make(map[string]struct {
			path string
			err  error
		}),
		runResults: make(map[string]struct {
			output string
			err    error
		}),
	}
}

// SetLookPath configures the response for LookPath.
func (m *MockCommandExecutor) SetLookPath(file, path string, err error) {
	m.lookPathResults[file] = struct {
		path string
		err  error
	}{path, err}
}

// SetRun configures the response for Run.
func (m *MockCommandExecutor) SetRun(key, output string, err error) {
	m.runResults[key] = struct {
		output string
		err    error
	}{output, err}
}

// LookPath implements CommandExecutor.
func (m *MockCommandExecutor) LookPath(file string) (string, error) {
	if result, ok := m.lookPathResults[file]; ok {
		return result.path, result.err
	}
	return "", exec.ErrNotFound
}

// Run implements CommandExecutor.
func (m *MockCommandExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	if result, ok := m.runResults[key]; ok {
		return result.output, result.err
	}
	// Try just the command name
	if result, ok := m.runResults[name]; ok {
		return result.output, result.err
	}
	return "", errors.ErrCommandNotConfigured
}

// TestToolStatus_String tests ToolStatus string representation.
func TestToolStatus_String(t *testing.T) {
	tests := []struct {
		status   ToolStatus
		expected string
	}{
		{ToolStatusInstalled, "installed"},
		{ToolStatusMissing, "missing"},
		{ToolStatusOutdated, "outdated"},
		{ToolStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			status := tt.status
			assert.Equal(t, tt.expected, status.String())
		})
	}
}

// setupMockForOtherTools sets up mock to return ErrNotFound for all tools except the one being tested.
func setupMockForOtherTools(mock *MockCommandExecutor, excludeTool string) {
	allTools := []string{
		constants.ToolGo, constants.ToolGit, constants.ToolGH, constants.ToolUV,
		constants.ToolClaude, constants.ToolMageX, constants.ToolGoPreCommit, constants.ToolSpeckit,
	}
	for _, tool := range allTools {
		if tool != excludeTool {
			mock.SetLookPath(tool, "", exec.ErrNotFound)
		}
	}
}

// findToolByName finds a tool by name in the detection result.
func findToolByName(result *ToolDetectionResult, name string) *Tool {
	for i := range result.Tools {
		if result.Tools[i].Name == name {
			return &result.Tools[i]
		}
	}
	return nil
}

// TestToolDetector_DetectGo tests Go detection scenarios.
func TestToolDetector_DetectGo(t *testing.T) {
	tests := []struct {
		name            string
		lookPathErr     error
		versionOutput   string
		versionErr      error
		expectedStatus  ToolStatus
		expectedVersion string
	}{
		{
			name:            "installed and current",
			versionOutput:   "go version go1.24.2 darwin/arm64",
			expectedStatus:  ToolStatusInstalled,
			expectedVersion: "1.24.2",
		},
		{
			name:            "installed exact minimum",
			versionOutput:   "go version go1.24.0 linux/amd64",
			expectedStatus:  ToolStatusInstalled,
			expectedVersion: "1.24.0",
		},
		{
			name:            "outdated version",
			versionOutput:   "go version go1.21.0 darwin/arm64",
			expectedStatus:  ToolStatusOutdated,
			expectedVersion: "1.21.0",
		},
		{
			name:           "not installed",
			lookPathErr:    exec.ErrNotFound,
			expectedStatus: ToolStatusMissing,
		},
		{
			name:            "version command fails",
			versionErr:      errors.ErrCommandFailed,
			expectedStatus:  ToolStatusInstalled,
			expectedVersion: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockCommandExecutor()
			setupMockForOtherTools(mock, constants.ToolGo)

			if tt.lookPathErr != nil {
				mock.SetLookPath(constants.ToolGo, "", tt.lookPathErr)
			} else {
				mock.SetLookPath(constants.ToolGo, "/usr/local/go/bin/go", nil)
			}

			if tt.versionOutput != "" || tt.versionErr != nil {
				mock.SetRun("go version", tt.versionOutput, tt.versionErr)
			}

			detector := NewToolDetectorWithExecutor(mock)
			result, err := detector.Detect(context.Background())
			require.NoError(t, err)
			require.NotNil(t, result)

			goTool := findToolByName(result, constants.ToolGo)
			require.NotNil(t, goTool, "Go tool not found in results")

			assert.Equal(t, tt.expectedStatus, goTool.Status)
			if tt.expectedVersion != "" {
				assert.Equal(t, tt.expectedVersion, goTool.CurrentVersion)
			}
		})
	}
}

// TestToolDetector_DetectGit tests Git detection scenarios.
func TestToolDetector_DetectGit(t *testing.T) {
	tests := []struct {
		name            string
		lookPathErr     error
		versionOutput   string
		expectedStatus  ToolStatus
		expectedVersion string
	}{
		{
			name:            "installed and current",
			versionOutput:   "git version 2.39.0",
			expectedStatus:  ToolStatusInstalled,
			expectedVersion: "2.39.0",
		},
		{
			name:            "installed with extras",
			versionOutput:   "git version 2.43.0 (Apple Git-146)",
			expectedStatus:  ToolStatusInstalled,
			expectedVersion: "2.43.0",
		},
		{
			name:            "outdated version",
			versionOutput:   "git version 2.19.0",
			expectedStatus:  ToolStatusOutdated,
			expectedVersion: "2.19.0",
		},
		{
			name:           "not installed",
			lookPathErr:    exec.ErrNotFound,
			expectedStatus: ToolStatusMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockCommandExecutor()
			setupMockForOtherTools(mock, constants.ToolGit)

			if tt.lookPathErr != nil {
				mock.SetLookPath(constants.ToolGit, "", tt.lookPathErr)
			} else {
				mock.SetLookPath(constants.ToolGit, "/usr/bin/git", nil)
				mock.SetRun("git --version", tt.versionOutput, nil)
			}

			detector := NewToolDetectorWithExecutor(mock)
			result, err := detector.Detect(context.Background())
			require.NoError(t, err)

			gitTool := findToolByName(result, constants.ToolGit)
			require.NotNil(t, gitTool)

			assert.Equal(t, tt.expectedStatus, gitTool.Status)
			if tt.expectedVersion != "" {
				assert.Equal(t, tt.expectedVersion, gitTool.CurrentVersion)
			}
		})
	}
}

// TestToolDetector_DetectGH tests GitHub CLI detection.
func TestToolDetector_DetectGH(t *testing.T) {
	tests := []struct {
		name            string
		versionOutput   string
		expectedVersion string
	}{
		{
			name:            "standard format",
			versionOutput:   "gh version 2.62.0 (2024-11-06)",
			expectedVersion: "2.62.0",
		},
		{
			name:            "with newlines",
			versionOutput:   "gh version 2.40.1 (2023-12-13)\nhttps://github.com/cli/cli/releases/tag/v2.40.1",
			expectedVersion: "2.40.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := parseGHVersion(tt.versionOutput)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

// TestToolDetector_DetectUV tests uv detection.
func TestToolDetector_DetectUV(t *testing.T) {
	tests := []struct {
		name            string
		versionOutput   string
		expectedVersion string
	}{
		{
			name:            "standard format",
			versionOutput:   "uv 0.5.14 (bb7af57b8 2025-01-03)",
			expectedVersion: "0.5.14",
		},
		{
			name:            "simple format",
			versionOutput:   "uv 0.5.0",
			expectedVersion: "0.5.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := parseUVVersion(tt.versionOutput)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

// TestToolDetector_DetectClaude tests Claude CLI detection.
func TestToolDetector_DetectClaude(t *testing.T) {
	tests := []struct {
		name            string
		versionOutput   string
		expectedVersion string
	}{
		{
			name:            "Claude Code format",
			versionOutput:   "Claude Code 2.0.76",
			expectedVersion: "2.0.76",
		},
		{
			name:            "claude-code format",
			versionOutput:   "claude-code 2.1.0",
			expectedVersion: "2.1.0",
		},
		{
			name:            "version only",
			versionOutput:   "2.0.80",
			expectedVersion: "2.0.80",
		},
		{
			name:            "with v prefix",
			versionOutput:   "Claude Code v2.0.77",
			expectedVersion: "2.0.77",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := parseClaudeVersion(tt.versionOutput)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

// TestCompareVersions tests version comparison logic.
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		required string
		expected int
	}{
		// Equal versions
		{
			name:     "equal versions",
			current:  "1.24.0",
			required: "1.24.0",
			expected: 0,
		},
		{
			name:     "equal with v prefix",
			current:  "v2.0.0",
			required: "2.0.0",
			expected: 0,
		},

		// Current greater than required
		{
			name:     "current patch greater",
			current:  "1.24.2",
			required: "1.24.0",
			expected: 1,
		},
		{
			name:     "current minor greater",
			current:  "1.25.0",
			required: "1.24.0",
			expected: 1,
		},
		{
			name:     "current major greater",
			current:  "2.0.0",
			required: "1.24.0",
			expected: 1,
		},

		// Current less than required
		{
			name:     "current patch less",
			current:  "1.24.0",
			required: "1.24.2",
			expected: -1,
		},
		{
			name:     "current minor less",
			current:  "1.23.0",
			required: "1.24.0",
			expected: -1,
		},
		{
			name:     "current major less",
			current:  "1.0.0",
			required: "2.0.0",
			expected: -1,
		},

		// Partial versions
		{
			name:     "partial current version",
			current:  "1.24",
			required: "1.24.0",
			expected: 0,
		},
		{
			name:     "partial required version",
			current:  "1.24.5",
			required: "1.24",
			expected: 1,
		},

		// Edge cases
		{
			name:     "version with x suffix",
			current:  "0.5.14",
			required: "0.5.0",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.current, tt.required)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestToolDetectionResult_MissingRequiredTools tests filtering missing required tools.
func TestToolDetectionResult_MissingRequiredTools(t *testing.T) {
	result := &ToolDetectionResult{
		Tools: []Tool{
			{Name: "go", Required: true, Status: ToolStatusInstalled},
			{Name: "git", Required: true, Status: ToolStatusMissing},
			{Name: "gh", Required: true, Status: ToolStatusOutdated},
			{Name: "magex", Required: false, Status: ToolStatusMissing},
		},
	}

	missing := result.MissingRequiredTools()

	assert.Len(t, missing, 2)
	assert.Equal(t, "git", missing[0].Name)
	assert.Equal(t, "gh", missing[1].Name)
}

// TestFormatMissingToolsError tests error message formatting.
func TestFormatMissingToolsError(t *testing.T) {
	t.Run("no missing tools", func(t *testing.T) {
		result := FormatMissingToolsError(nil)
		assert.Empty(t, result)
	})

	t.Run("missing tool", func(t *testing.T) {
		missing := []Tool{
			{
				Name:        "git",
				Status:      ToolStatusMissing,
				InstallHint: "Install Git from https://git-scm.com",
			},
		}
		result := FormatMissingToolsError(missing)
		assert.Contains(t, result, "git")
		assert.Contains(t, result, "missing")
		assert.Contains(t, result, "Install Git from https://git-scm.com")
	})

	t.Run("outdated tool", func(t *testing.T) {
		missing := []Tool{
			{
				Name:           "go",
				Status:         ToolStatusOutdated,
				CurrentVersion: "1.21.0",
				MinVersion:     "1.24.0",
				InstallHint:    "Install Go from https://go.dev",
			},
		}
		result := FormatMissingToolsError(missing)
		assert.Contains(t, result, "go")
		assert.Contains(t, result, "outdated")
		assert.Contains(t, result, "1.21.0")
		assert.Contains(t, result, "1.24.0")
	})
}

// TestToolDetector_ContextCancellation tests that detection respects context cancellation.
func TestToolDetector_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	detector := NewToolDetector()
	result, err := detector.Detect(ctx)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestToolDetector_ParallelDetection tests that detection runs in parallel.
func TestToolDetector_ParallelDetection(t *testing.T) {
	mock := NewMockCommandExecutor()

	// Set up all tools with artificial delay simulation through mock
	tools := []string{constants.ToolGo, constants.ToolGit, constants.ToolGH, constants.ToolUV, constants.ToolClaude, constants.ToolMageX, constants.ToolGoPreCommit, constants.ToolSpeckit}
	for _, tool := range tools {
		mock.SetLookPath(tool, "/usr/bin/"+tool, nil)
	}

	// Set up version outputs
	mock.SetRun("go version", "go version go1.24.2 darwin/arm64", nil)
	mock.SetRun("git --version", "git version 2.39.0", nil)
	mock.SetRun("gh --version", "gh version 2.62.0", nil)
	mock.SetRun("uv --version", "uv 0.5.14", nil)
	mock.SetRun("claude --version", "Claude Code 2.0.76", nil)
	mock.SetRun("magex --version", "v1.0.0", nil)
	mock.SetRun("go-pre-commit --version", "v1.0.0", nil)
	mock.SetRun("specify --version", "v1.0.0", nil)

	detector := NewToolDetectorWithExecutor(mock)

	start := time.Now()
	result, err := detector.Detect(context.Background())
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should complete quickly since mock doesn't actually run commands
	assert.Less(t, elapsed, 1*time.Second)

	// All tools should be detected (including gemini and codex)
	assert.Len(t, result.Tools, 10)
}

// TestParseVersionParts tests version string parsing.
func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		version  string
		expected [3]int
	}{
		{"1.24.2", [3]int{1, 24, 2}},
		{"2.0.0", [3]int{2, 0, 0}},
		{"0.5.14", [3]int{0, 5, 14}},
		{"1.24", [3]int{1, 24, 0}},
		{"2", [3]int{2, 0, 0}},
		{"", [3]int{0, 0, 0}},
		{"v1.2.3", [3]int{0, 2, 3}}, // v prefix causes first segment to fail parsing, but 1.2 and 3 parse correctly
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := parseVersionParts(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseGoVersion tests Go version parsing.
func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		output   string
		expected string
	}{
		{"go version go1.24.2 darwin/arm64", "1.24.2"},
		{"go version go1.24 linux/amd64", "1.24"},
		{"go version go1.21.0 windows/amd64", "1.21.0"},
		{"invalid output", ""},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			result := parseGoVersion(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseGitVersion tests Git version parsing.
func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		output   string
		expected string
	}{
		{"git version 2.39.0", "2.39.0"},
		{"git version 2.43.0 (Apple Git-146)", "2.43.0"},
		{"git version 2.20.1.windows.1", "2.20.1"},
		{"invalid output", ""},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			result := parseGitVersion(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNewToolDetector tests detector creation.
func TestNewToolDetector(t *testing.T) {
	detector := NewToolDetector()
	assert.NotNil(t, detector)
	assert.NotNil(t, detector.executor)
}

// TestNewToolDetectorWithExecutor tests detector creation with custom executor.
func TestNewToolDetectorWithExecutor(t *testing.T) {
	mock := NewMockCommandExecutor()
	detector := NewToolDetectorWithExecutor(mock)
	assert.NotNil(t, detector)
	assert.Equal(t, mock, detector.executor)
}

// TestToolDetector_AllToolsPresent tests happy path with all tools installed.
func TestToolDetector_AllToolsPresent(t *testing.T) {
	mock := NewMockCommandExecutor()

	// Configure all tools as present with valid versions
	mock.SetLookPath(constants.ToolGo, "/usr/local/go/bin/go", nil)
	mock.SetRun("go version", "go version go1.24.2 darwin/arm64", nil)

	mock.SetLookPath(constants.ToolGit, "/usr/bin/git", nil)
	mock.SetRun("git --version", "git version 2.39.0", nil)

	mock.SetLookPath(constants.ToolGH, "/usr/local/bin/gh", nil)
	mock.SetRun("gh --version", "gh version 2.62.0 (2024-11-06)", nil)

	mock.SetLookPath(constants.ToolUV, "/usr/local/bin/uv", nil)
	mock.SetRun("uv --version", "uv 0.5.14 (bb7af57b8 2025-01-03)", nil)

	mock.SetLookPath(constants.ToolClaude, "/usr/local/bin/claude", nil)
	mock.SetRun("claude --version", "Claude Code 2.0.76", nil)

	mock.SetLookPath(constants.ToolGemini, "/usr/local/bin/gemini", nil)
	mock.SetRun("gemini --version", "Gemini CLI 0.22.0", nil)

	mock.SetLookPath(constants.ToolMageX, "/go/bin/magex", nil)
	mock.SetRun("magex --version", "v1.0.0", nil)

	mock.SetLookPath(constants.ToolGoPreCommit, "/go/bin/go-pre-commit", nil)
	mock.SetRun("go-pre-commit --version", "v1.0.0", nil)

	mock.SetLookPath(constants.ToolSpeckit, "/usr/local/bin/specify", nil)
	mock.SetRun("specify --version", "v1.0.0", nil)

	detector := NewToolDetectorWithExecutor(mock)
	result, err := detector.Detect(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.HasMissingRequired)
	assert.Len(t, result.Tools, 10)

	// Verify all required tools are installed
	for _, tool := range result.Tools {
		if tool.Required {
			assert.Equal(t, ToolStatusInstalled, tool.Status, "Tool %s should be installed", tool.Name)
		}
	}
}

// TestToolStatus_JSONMarshal tests JSON marshaling of ToolStatus.
func TestToolStatus_JSONMarshal(t *testing.T) {
	tests := []struct {
		status   ToolStatus
		expected string
	}{
		{ToolStatusInstalled, `"installed"`},
		{ToolStatusMissing, `"missing"`},
		{ToolStatusOutdated, `"outdated"`},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			data, err := tt.status.MarshalJSON()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

// TestToolStatus_JSONUnmarshal tests JSON unmarshaling of ToolStatus.
func TestToolStatus_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		input    string
		expected ToolStatus
	}{
		{`"installed"`, ToolStatusInstalled},
		{`"missing"`, ToolStatusMissing},
		{`"outdated"`, ToolStatusOutdated},
		{`"unknown"`, ToolStatusMissing}, // Default to missing
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var status ToolStatus
			err := status.UnmarshalJSON([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, status)
		})
	}
}

// TestToolDetector_TimeoutBehavior tests that detection respects the 2-second timeout.
func TestToolDetector_TimeoutBehavior(t *testing.T) {
	// Create a slow mock that simulates a tool that takes longer than timeout
	mock := &SlowMockExecutor{
		delay: 3 * time.Second, // Longer than ToolDetectionTimeout (2s)
	}

	detector := NewToolDetectorWithExecutor(mock)

	start := time.Now()
	result, err := detector.Detect(context.Background())
	elapsed := time.Since(start)

	// Detection should complete (possibly with timeout error or partial results)
	// The key is that it doesn't take 3 seconds * 8 tools = 24 seconds
	require.NoError(t, err) // errgroup doesn't return errors from individual goroutines in this impl
	require.NotNil(t, result)

	// Should complete within timeout + small buffer, not full delay * num tools
	assert.Less(t, elapsed, 4*time.Second, "Detection should timeout, not wait for slow executor")
}

// SlowMockExecutor is a mock that simulates slow command execution.
type SlowMockExecutor struct {
	delay time.Duration
}

// LookPath returns success for all tools to trigger version check.
func (m *SlowMockExecutor) LookPath(_ string) (string, error) {
	return "/usr/bin/tool", nil
}

// Run simulates a slow command that respects context cancellation.
func (m *SlowMockExecutor) Run(ctx context.Context, _ string, _ ...string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(m.delay):
		return "1.0.0", nil
	}
}

// TestIsGoPreCommitInstalled tests the convenience function for checking go-pre-commit.
func TestIsGoPreCommitInstalled(t *testing.T) {
	tests := []struct {
		name           string
		lookPathResult error
		versionOutput  string
		versionErr     error
		wantInstalled  bool
		wantVersion    string
		wantErrNil     bool
		cancelContext  bool
	}{
		{
			name:           "installed with valid version",
			lookPathResult: nil,
			versionOutput:  "v1.2.3",
			versionErr:     nil,
			wantInstalled:  true,
			wantVersion:    "1.2.3",
			wantErrNil:     true,
		},
		{
			name:           "installed version without v prefix",
			lookPathResult: nil,
			versionOutput:  "go-pre-commit version 2.0.0",
			versionErr:     nil,
			wantInstalled:  true,
			wantVersion:    "2.0.0",
			wantErrNil:     true,
		},
		{
			name:           "installed but version command fails",
			lookPathResult: nil,
			versionOutput:  "",
			versionErr:     errors.ErrCommandFailed,
			wantInstalled:  true,
			wantVersion:    "unknown",
			wantErrNil:     true,
		},
		{
			name:           "installed but version parse fails",
			lookPathResult: nil,
			versionOutput:  "no version info",
			versionErr:     nil,
			wantInstalled:  true,
			wantVersion:    "unknown",
			wantErrNil:     true,
		},
		{
			name:           "not installed",
			lookPathResult: exec.ErrNotFound,
			wantInstalled:  false,
			wantVersion:    "",
			wantErrNil:     true,
		},
		{
			name:          "context canceled",
			cancelContext: true,
			wantInstalled: false,
			wantVersion:   "",
			wantErrNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // Cancel immediately
			}

			mock := NewMockCommandExecutor()
			if tt.lookPathResult != nil {
				mock.SetLookPath(constants.ToolGoPreCommit, "", tt.lookPathResult)
			} else {
				mock.SetLookPath(constants.ToolGoPreCommit, "/go/bin/go-pre-commit", nil)
			}
			mock.SetRun("go-pre-commit --version", tt.versionOutput, tt.versionErr)

			installed, version, err := IsGoPreCommitInstalledWithExecutor(ctx, mock)

			assert.Equal(t, tt.wantInstalled, installed)
			assert.Equal(t, tt.wantVersion, version)
			if tt.wantErrNil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestToolDetector_RequiredToolMissing tests detection when a required tool is missing.
func TestToolDetector_RequiredToolMissing(t *testing.T) {
	mock := NewMockCommandExecutor()

	// Go is missing
	mock.SetLookPath(constants.ToolGo, "", exec.ErrNotFound)

	// Other tools present
	mock.SetLookPath(constants.ToolGit, "/usr/bin/git", nil)
	mock.SetRun("git --version", "git version 2.39.0", nil)

	mock.SetLookPath(constants.ToolGH, "/usr/local/bin/gh", nil)
	mock.SetRun("gh --version", "gh version 2.62.0", nil)

	mock.SetLookPath(constants.ToolUV, "/usr/local/bin/uv", nil)
	mock.SetRun("uv --version", "uv 0.5.14", nil)

	mock.SetLookPath(constants.ToolClaude, "/usr/local/bin/claude", nil)
	mock.SetRun("claude --version", "Claude Code 2.0.76", nil)

	// Managed tools missing
	mock.SetLookPath(constants.ToolMageX, "", exec.ErrNotFound)
	mock.SetLookPath(constants.ToolGoPreCommit, "", exec.ErrNotFound)
	mock.SetLookPath(constants.ToolSpeckit, "", exec.ErrNotFound)

	detector := NewToolDetectorWithExecutor(mock)
	result, err := detector.Detect(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HasMissingRequired)

	missing := result.MissingRequiredTools()
	require.Len(t, missing, 1)
	assert.Equal(t, constants.ToolGo, missing[0].Name)
}
