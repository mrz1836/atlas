// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/spf13/cobra"
)

// AddFormatCommand adds the format command to the root command.
func AddFormatCommand(root *cobra.Command) {
	root.AddCommand(newFormatCmd())
}

func newFormatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "format",
		Short: "Run code formatters",
		Long: `Run configured code formatters on the current directory.

Uses 'magex format:fix' by default if no formatters are configured.

Examples:
  atlas format
  atlas format --output json
  atlas format --verbose`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFormat(cmd.Context(), cmd, os.Stdout)
		},
	}

	return cmd
}

func runFormat(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := Logger()
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

	// Get format commands
	commands := cfg.Validation.Commands.Format
	if len(commands) == 0 {
		commands = []string{constants.DefaultFormatCommand}
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

	return runCommandsWithOutput(ctx, commands, workDir, "Format", out, opts, logger)
}
