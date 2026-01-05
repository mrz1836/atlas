package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCmd_Help(t *testing.T) {
	t.Parallel()

	flags := &GlobalFlags{}
	cmd := newRootCmd(flags, BuildInfo{Version: "test"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ATLAS")
	assert.Contains(t, output, "--output")
	assert.Contains(t, output, "--verbose")
	assert.Contains(t, output, "--quiet")
	assert.Contains(t, output, "--version")
}

func TestRootCmd_Version(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		info           BuildInfo
		expectContains []string
	}{
		{
			name: "full version info",
			info: BuildInfo{
				Version: "1.0.0",
				Commit:  "abc1234",
				Date:    "2025-01-01",
			},
			expectContains: []string{"1.0.0", "abc1234", "2025-01-01"},
		},
		{
			name:           "default dev version",
			info:           BuildInfo{},
			expectContains: []string{"dev", "none", "unknown"},
		},
		{
			name: "partial version info",
			info: BuildInfo{
				Version: "2.0.0-beta",
			},
			expectContains: []string{"2.0.0-beta", "none", "unknown"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flags := &GlobalFlags{}
			cmd := newRootCmd(flags, tc.info)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"--version"})

			err := cmd.Execute()
			require.NoError(t, err)

			output := buf.String()
			for _, expected := range tc.expectContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestRootCmd_OutputFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		expectedValue string
		expectError   bool
	}{
		{
			name:          "text output",
			args:          []string{"--output", "text"},
			expectedValue: OutputText,
			expectError:   false,
		},
		{
			name:          "json output",
			args:          []string{"--output", "json"},
			expectedValue: OutputJSON,
			expectError:   false,
		},
		{
			name:          "shorthand output",
			args:          []string{"-o", "json"},
			expectedValue: OutputJSON,
			expectError:   false,
		},
		{
			name:        "invalid output format",
			args:        []string{"--output", "xml"},
			expectError: true,
		},
		{
			name:        "empty output format",
			args:        []string{"--output", ""},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flags := &GlobalFlags{}
			cmd := newRootCmd(flags, BuildInfo{})
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedValue, flags.Output)
			}
		})
	}
}

func TestRootCmd_VerboseQuietMutuallyExclusive(t *testing.T) {
	t.Parallel()

	flags := &GlobalFlags{}
	cmd := newRootCmd(flags, BuildInfo{})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--verbose", "--quiet"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verbose")
	assert.Contains(t, err.Error(), "quiet")
}

func TestRootCmd_VerboseFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "verbose long form",
			args:     []string{"--verbose"},
			expected: true,
		},
		{
			name:     "verbose short form",
			args:     []string{"-v"},
			expected: true,
		},
		{
			name:     "no verbose",
			args:     []string{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flags := &GlobalFlags{}
			cmd := newRootCmd(flags, BuildInfo{})
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, flags.Verbose)
		})
	}
}

func TestRootCmd_QuietFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "quiet long form",
			args:     []string{"--quiet"},
			expected: true,
		},
		{
			name:     "quiet short form",
			args:     []string{"-q"},
			expected: true,
		},
		{
			name:     "no quiet",
			args:     []string{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flags := &GlobalFlags{}
			cmd := newRootCmd(flags, BuildInfo{})
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, flags.Quiet)
		})
	}
}

func TestRootCmd_SilencesUsageOnError(t *testing.T) {
	t.Parallel()

	flags := &GlobalFlags{}
	cmd := newRootCmd(flags, BuildInfo{})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--output", "invalid"})

	err := cmd.Execute()
	require.Error(t, err)

	// Usage should not be in output (SilenceUsage is set)
	output := buf.String()
	assert.NotContains(t, output, "Usage:")
}

func TestExecute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	info := BuildInfo{
		Version: "test",
		Commit:  "test123",
		Date:    "today",
	}

	// Execute should not error with valid args
	err := Execute(ctx, info)
	require.NoError(t, err)
}

func TestFormatVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		info     BuildInfo
		expected string
	}{
		{
			name: "all fields set",
			info: BuildInfo{
				Version: "1.0.0",
				Commit:  "abc123",
				Date:    "2025-01-01",
			},
			expected: "1.0.0 (commit: abc123, built: 2025-01-01)",
		},
		{
			name:     "empty info uses defaults",
			info:     BuildInfo{},
			expected: "dev (commit: none, built: unknown)",
		},
		{
			name: "partial info fills defaults",
			info: BuildInfo{
				Version: "2.0.0",
			},
			expected: "2.0.0 (commit: none, built: unknown)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, formatVersion(tc.info))
		})
	}
}

func TestGetLogger(t *testing.T) {
	t.Parallel()

	// Execute a command to initialize the logger
	flags := &GlobalFlags{}
	cmd := newRootCmd(flags, BuildInfo{})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	// GetLogger should return a valid logger after execution
	logger := GetLogger()
	assert.NotNil(t, logger)
}
