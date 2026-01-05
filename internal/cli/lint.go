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

// AddLintCommand adds the lint command to the root command.
func AddLintCommand(root *cobra.Command) {
	root.AddCommand(newLintCmd())
}

func newLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run code linters",
		Long: `Run configured code linters on the current directory.

Uses 'magex lint' by default if no linters are configured.

Examples:
  atlas lint
  atlas lint --output json
  atlas lint --verbose`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLint(cmd.Context(), cmd, os.Stdout)
		},
	}

	return cmd
}

func runLint(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
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

	// Get lint commands
	commands := cfg.Validation.Commands.Lint
	if len(commands) == 0 {
		commands = []string{constants.DefaultLintCommand}
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

	return runCommandsWithOutput(ctx, commands, workDir, "Lint", out, opts, logger)
}
