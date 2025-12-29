// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/tui"
)

// AddTestCommand adds the test command to the root command.
func AddTestCommand(root *cobra.Command) {
	root.AddCommand(newTestCmd())
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run tests",
		Long: `Run configured test commands on the current directory.

Uses 'magex test' by default if no test commands are configured.

Examples:
  atlas test
  atlas test --output json
  atlas test --verbose`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTest(cmd.Context(), cmd, os.Stdout)
		},
	}

	return cmd
}

func runTest(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := GetLogger()
	outputFormat := cmd.Flag("output").Value.String()
	verbose := cmd.Flag("verbose").Value.String() == "true"
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// Load config
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using defaults")
		cfg = config.DefaultConfig()
	}

	// Get test commands
	commands := cfg.Validation.Commands.Test
	if len(commands) == 0 {
		commands = []string{constants.DefaultTestCommand}
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	opts := UtilityOptions{
		Verbose:      verbose,
		OutputFormat: outputFormat,
		Writer:       w,
	}

	return runCommandsWithOutput(ctx, commands, workDir, "Test", out, opts, logger)
}
