// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
)

// AddValidateCommand adds the validate command to the root command.
func AddValidateCommand(root *cobra.Command) {
	root.AddCommand(newValidateCmd())
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run the full validation suite (format, lint, test, pre-commit)",
		Long: `Run the complete validation pipeline configured for the project.

The validation suite runs in this order:
  1. Format - Code formatting (sequential)
  2. Lint + Test - Run in parallel
  3. Pre-commit - Pre-commit hooks (sequential)

Examples:
  atlas validate
  atlas validate --output json
  atlas validate --verbose`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runValidate(cmd.Context(), cmd, os.Stdout)
		},
	}

	return cmd
}

func runValidate(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
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
	opts := UtilityOptions{
		Verbose:      verbose,
		OutputFormat: outputFormat,
		Writer:       w,
	}

	// Load config
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using defaults")
		cfg = config.DefaultConfig()
	}

	// Get current working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	runner := &validation.DefaultCommandRunner{}
	results := make([]CommandResult, 0)

	// 1. Run format commands first (sequential)
	formatResults, err := runSequentialCommands(ctx, cfg.Validation.Commands.Format, constants.DefaultFormatCommand, workDir, runner, logger, out, opts, "format")
	if err != nil {
		results = append(results, formatResults...)
		return handleValidationFailure(out, outputFormat, results)
	}
	results = append(results, formatResults...)
	out.Success("Format passed")

	// 2. Run lint and test in parallel
	lintTestResults, err := runParallelLintAndTest(ctx, cfg, workDir, runner, logger, out, opts)
	if err != nil {
		results = append(results, lintTestResults...)
		return handleValidationFailure(out, outputFormat, results)
	}
	results = append(results, lintTestResults...)
	out.Success("Lint passed")
	out.Success("Test passed")

	// 3. Run pre-commit commands (sequential)
	preCommitCmds := cfg.Validation.Commands.PreCommit
	if len(preCommitCmds) == 0 {
		preCommitCmds = []string{constants.DefaultPreCommitCommand}
	}
	preCommitResults, err := runSequentialCommands(ctx, preCommitCmds, "", workDir, runner, logger, out, opts, "pre-commit")
	if err != nil {
		results = append(results, preCommitResults...)
		return handleValidationFailure(out, outputFormat, results)
	}
	results = append(results, preCommitResults...)
	out.Success("Pre-commit passed")

	// All passed
	if outputFormat == OutputJSON {
		return out.JSON(ValidationResponse{
			Success: true,
			Results: results,
		})
	}

	out.Success("All validations passed!")
	return nil
}

// runSequentialCommands executes commands sequentially, returning results and error.
func runSequentialCommands(
	ctx context.Context,
	commands []string,
	defaultCmd string,
	workDir string,
	runner validation.CommandRunner,
	logger zerolog.Logger,
	out tui.Output,
	opts UtilityOptions,
	label string,
) ([]CommandResult, error) {
	cmds := commands
	if len(cmds) == 0 && defaultCmd != "" {
		cmds = []string{defaultCmd}
	}

	if len(cmds) == 0 {
		return nil, nil
	}

	out.Info(fmt.Sprintf("Running %s...", label))
	results := make([]CommandResult, 0, len(cmds))

	for _, cmdStr := range cmds {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// Show command being executed in verbose mode
		if opts.Verbose {
			out.Info(fmt.Sprintf("⏳ Running: %s", cmdStr))
		}

		result := runSingleCommand(ctx, runner, workDir, cmdStr, logger)
		results = append(results, result)
		showVerboseOutput(opts, result)

		if !result.Success {
			return results, fmt.Errorf("%w: %s command %s", errors.ErrValidationFailed, label, cmdStr)
		}
	}

	return results, nil
}

// runParallelLintAndTest runs lint and test commands in parallel.
func runParallelLintAndTest(
	ctx context.Context,
	cfg *config.Config,
	workDir string,
	runner validation.CommandRunner,
	logger zerolog.Logger,
	out tui.Output,
	opts UtilityOptions,
) ([]CommandResult, error) {
	lintCmds := cfg.Validation.Commands.Lint
	if len(lintCmds) == 0 {
		lintCmds = []string{constants.DefaultLintCommand}
	}

	testCmds := cfg.Validation.Commands.Test
	if len(testCmds) == 0 {
		testCmds = []string{constants.DefaultTestCommand}
	}

	g, gCtx := errgroup.WithContext(ctx)
	var lintResults, testResults []CommandResult
	var lintMu, testMu sync.Mutex

	g.Go(func() error {
		return runParallelCommands(gCtx, lintCmds, "lint", workDir, runner, logger, out, opts, &lintResults, &lintMu)
	})

	g.Go(func() error {
		return runParallelCommands(gCtx, testCmds, "test", workDir, runner, logger, out, opts, &testResults, &testMu)
	})

	allResults := make([]CommandResult, 0)
	if err := g.Wait(); err != nil {
		allResults = append(allResults, lintResults...)
		allResults = append(allResults, testResults...)
		return allResults, err
	}

	allResults = append(allResults, lintResults...)
	allResults = append(allResults, testResults...)
	return allResults, nil
}

// runParallelCommands runs commands for a category in a goroutine-safe manner.
func runParallelCommands(
	ctx context.Context,
	cmds []string,
	category string,
	workDir string,
	runner validation.CommandRunner,
	logger zerolog.Logger,
	out tui.Output,
	opts UtilityOptions,
	results *[]CommandResult,
	mu *sync.Mutex,
) error {
	out.Info(fmt.Sprintf("Running %s...", category))
	for _, cmdStr := range cmds {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if opts.Verbose {
			out.Info(fmt.Sprintf("⏳ Running: %s", cmdStr))
		}

		result := runSingleCommand(ctx, runner, workDir, cmdStr, logger)
		mu.Lock()
		*results = append(*results, result)
		mu.Unlock()

		showVerboseOutput(opts, result)

		if !result.Success {
			return fmt.Errorf("%w: %s command %s", errors.ErrValidationFailed, category, cmdStr)
		}
	}
	return nil
}
