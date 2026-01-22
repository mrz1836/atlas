package cli

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/errors"
)

func TestExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitSuccess", ExitSuccess, 0},
		{"ExitError", ExitError, 1},
		{"ExitInvalidInput", ExitInvalidInput, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.code)
		})
	}
}

func TestOutputFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{"OutputText", OutputText, "text"},
		{"OutputJSON", OutputJSON, "json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.format)
		})
	}
}

func TestGlobalFlags_Defaults(t *testing.T) {
	t.Parallel()

	flags := &GlobalFlags{}
	cmd := &cobra.Command{Use: "test"}
	AddGlobalFlags(cmd, flags)

	// Check defaults
	assert.Equal(t, OutputText, flags.Output)
	assert.False(t, flags.Verbose)
	assert.False(t, flags.Quiet)
}

func TestAddGlobalFlags(t *testing.T) {
	t.Parallel()

	flags := &GlobalFlags{}
	cmd := &cobra.Command{Use: "test"}
	AddGlobalFlags(cmd, flags)

	// Verify flags are registered
	outputFlag := cmd.PersistentFlags().Lookup("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, "o", outputFlag.Shorthand)
	assert.Equal(t, OutputText, outputFlag.DefValue)

	verboseFlag := cmd.PersistentFlags().Lookup("verbose")
	require.NotNil(t, verboseFlag)
	assert.Equal(t, "v", verboseFlag.Shorthand)
	assert.Equal(t, "false", verboseFlag.DefValue)

	quietFlag := cmd.PersistentFlags().Lookup("quiet")
	require.NotNil(t, quietFlag)
	assert.Equal(t, "q", quietFlag.Shorthand)
	assert.Equal(t, "false", quietFlag.DefValue)
}

func TestAddGlobalFlags_ParsesCorrectly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		args            []string
		expectedOutput  string
		expectedVerbose bool
		expectedQuiet   bool
	}{
		{
			name:            "default values",
			args:            []string{},
			expectedOutput:  OutputText,
			expectedVerbose: false,
			expectedQuiet:   false,
		},
		{
			name:            "output json",
			args:            []string{"--output", "json"},
			expectedOutput:  OutputJSON,
			expectedVerbose: false,
			expectedQuiet:   false,
		},
		{
			name:            "output shorthand",
			args:            []string{"-o", "json"},
			expectedOutput:  OutputJSON,
			expectedVerbose: false,
			expectedQuiet:   false,
		},
		{
			name:            "verbose flag",
			args:            []string{"--verbose"},
			expectedOutput:  OutputText,
			expectedVerbose: true,
			expectedQuiet:   false,
		},
		{
			name:            "verbose shorthand",
			args:            []string{"-v"},
			expectedOutput:  OutputText,
			expectedVerbose: true,
			expectedQuiet:   false,
		},
		{
			name:            "quiet flag",
			args:            []string{"--quiet"},
			expectedOutput:  OutputText,
			expectedVerbose: false,
			expectedQuiet:   true,
		},
		{
			name:            "quiet shorthand",
			args:            []string{"-q"},
			expectedOutput:  OutputText,
			expectedVerbose: false,
			expectedQuiet:   true,
		},
		{
			name:            "combined flags",
			args:            []string{"-o", "json", "-v"},
			expectedOutput:  OutputJSON,
			expectedVerbose: true,
			expectedQuiet:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flags := &GlobalFlags{}
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(_ *cobra.Command, _ []string) error {
					return nil
				},
			}
			AddGlobalFlags(cmd, flags)

			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			require.NoError(t, err)

			assert.Equal(t, tc.expectedOutput, flags.Output)
			assert.Equal(t, tc.expectedVerbose, flags.Verbose)
			assert.Equal(t, tc.expectedQuiet, flags.Quiet)
		})
	}
}

func TestBindGlobalFlags(t *testing.T) {
	t.Parallel()

	flags := &GlobalFlags{}
	v := viper.New()
	cmd := &cobra.Command{Use: "test"}
	AddGlobalFlags(cmd, flags)

	err := BindGlobalFlags(v, cmd)
	require.NoError(t, err)

	// Verify Viper is configured with env prefix
	// We can't directly test AutomaticEnv, but we can verify SetEnvPrefix was called
	// by checking that environment variable binding works

	// Set a test value via flag
	require.NoError(t, cmd.PersistentFlags().Set("output", "json"))

	// Verify Viper can read it
	assert.Equal(t, "json", v.GetString("output"))
}

func TestValidOutputFormats(t *testing.T) {
	t.Parallel()

	formats := ValidOutputFormats()
	assert.Len(t, formats, 2)
	assert.Contains(t, formats, OutputText)
	assert.Contains(t, formats, OutputJSON)
}

func TestIsValidOutputFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   string
		expected bool
	}{
		{"text is valid", OutputText, true},
		{"json is valid", OutputJSON, true},
		{"xml is invalid", "xml", false},
		{"empty is invalid", "", false},
		{"uppercase TEXT is invalid", "TEXT", false},
		{"uppercase JSON is invalid", "JSON", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, IsValidOutputFormat(tc.format))
		})
	}
}

//nolint:err113 // Test cases intentionally use dynamic errors to simulate Cobra error messages
func TestExitCodeForError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{
			name:         "nil error returns success",
			err:          nil,
			expectedCode: ExitSuccess,
		},
		{
			name:         "ErrInvalidOutputFormat returns invalid input",
			err:          errors.ErrInvalidOutputFormat,
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "wrapped ErrInvalidOutputFormat returns invalid input",
			err:          fmt.Errorf("validation failed: %w", errors.ErrInvalidOutputFormat),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "unknown flag error returns invalid input",
			err:          stderrors.New("unknown flag: --foo"),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "unknown shorthand flag error returns invalid input",
			err:          stderrors.New("unknown shorthand flag: 'x'"),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "flag needs argument error returns invalid input",
			err:          stderrors.New("flag needs an argument: --output"),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "invalid argument error returns invalid input",
			err:          stderrors.New(`invalid argument "foo" for "--count"`),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "mutually exclusive flags error returns invalid input",
			err:          stderrors.New("if any flags in the group [verbose quiet] are set none of the others can be"),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "required flag error returns invalid input",
			err:          stderrors.New(`required flag "--config" not set`),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "unknown command error returns invalid input",
			err:          stderrors.New(`unknown command "foo"`),
			expectedCode: ExitInvalidInput,
		},
		{
			name:         "generic error returns error code",
			err:          stderrors.New("something went wrong"),
			expectedCode: ExitError,
		},
		{
			name:         "file not found returns error code",
			err:          stderrors.New("config file not found"),
			expectedCode: ExitError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expectedCode, ExitCodeForError(tc.err))
		})
	}
}

func TestIsInvalidInputError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"unknown flag", "unknown flag: --foo", true},
		{"unknown shorthand", "unknown shorthand flag: 'x'", true},
		{"flag needs argument", "flag needs an argument: --output", true},
		{"invalid argument", "invalid argument \"foo\"", true},
		{"mutually exclusive", "if any flags in the group [a b]", true},
		{"required flag", "required flag \"--config\" not set", true},
		{"unknown command", "unknown command \"bar\"", true},
		{"generic error", "something went wrong", false},
		{"empty message", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, isInvalidInputError(tc.errMsg))
		})
	}
}
