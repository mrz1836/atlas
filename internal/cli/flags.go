// Package cli provides the command-line interface for atlas.
package cli

import (
	stderrors "errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mrz1836/atlas/internal/errors"
)

// Exit codes for the CLI.
const (
	// ExitSuccess indicates successful execution.
	ExitSuccess = 0
	// ExitError indicates a general error.
	ExitError = 1
	// ExitInvalidInput indicates invalid user input.
	ExitInvalidInput = 2
)

// Output format constants.
const (
	// OutputText is the default human-readable output format.
	OutputText = "text"
	// OutputJSON is the machine-readable JSON output format.
	OutputJSON = "json"
)

// GlobalFlags holds flags available to all commands.
type GlobalFlags struct {
	// Output specifies the output format (text or json).
	Output string
	// Verbose enables debug-level logging.
	Verbose bool
	// Quiet suppresses non-essential output (warn level only).
	Quiet bool
}

// AddGlobalFlags adds global flags to a command.
// These flags are available to all subcommands via PersistentFlags.
func AddGlobalFlags(cmd *cobra.Command, flags *GlobalFlags) {
	cmd.PersistentFlags().StringVarP(&flags.Output, "output", "o", OutputText, "output format (text|json)")
	cmd.PersistentFlags().BoolVarP(&flags.Verbose, "verbose", "v", false, "enable verbose output")
	cmd.PersistentFlags().BoolVarP(&flags.Quiet, "quiet", "q", false, "suppress non-essential output")
	cmd.MarkFlagsMutuallyExclusive("verbose", "quiet")
}

// BindGlobalFlags binds global flags to Viper for configuration file and
// environment variable support. The ATLAS_ prefix is used for environment
// variables (e.g., ATLAS_OUTPUT, ATLAS_VERBOSE).
func BindGlobalFlags(v *viper.Viper, cmd *cobra.Command) error {
	// Use Root().PersistentFlags() to find flags defined on the root command,
	// even when called from a subcommand's PersistentPreRunE.
	rootFlags := cmd.Root().PersistentFlags()

	if err := v.BindPFlag("output", rootFlags.Lookup("output")); err != nil {
		return err
	}
	if err := v.BindPFlag("verbose", rootFlags.Lookup("verbose")); err != nil {
		return err
	}
	if err := v.BindPFlag("quiet", rootFlags.Lookup("quiet")); err != nil {
		return err
	}

	// Enable environment variable support with ATLAS_ prefix
	v.SetEnvPrefix("ATLAS")
	v.AutomaticEnv()

	return nil
}

// ValidOutputFormats returns the list of valid output format values.
func ValidOutputFormats() []string {
	return []string{OutputText, OutputJSON}
}

// IsValidOutputFormat checks if the given format is a valid output format.
func IsValidOutputFormat(format string) bool {
	for _, valid := range ValidOutputFormats() {
		if format == valid {
			return true
		}
	}
	return false
}

// ExitCodeForError returns the appropriate exit code for the given error.
// Returns ExitSuccess (0) for nil errors, ExitInvalidInput (2) for user input
// errors (invalid flags, bad arguments), and ExitError (1) for all other errors.
func ExitCodeForError(err error) int {
	if err == nil {
		return ExitSuccess
	}

	// Check for our custom exit code 2 error wrapper
	if errors.IsExitCode2Error(err) {
		return ExitInvalidInput
	}

	// Check for our custom invalid input error
	if stderrors.Is(err, errors.ErrInvalidOutputFormat) {
		return ExitInvalidInput
	}

	// Check for Cobra flag parsing errors (mutually exclusive flags, unknown flags, etc.)
	errMsg := err.Error()
	if isInvalidInputError(errMsg) {
		return ExitInvalidInput
	}

	return ExitError
}

// isInvalidInputError checks if an error message indicates invalid user input.
// This catches Cobra's built-in flag validation errors.
func isInvalidInputError(errMsg string) bool {
	invalidInputPatterns := []string{
		"unknown flag",
		"unknown shorthand flag",
		"flag needs an argument",
		"invalid argument",
		"if any flags in the group",
		"required flag",
		"unknown command",
	}

	for _, pattern := range invalidInputPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}
