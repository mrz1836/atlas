// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
	"github.com/spf13/cobra"
)

// ValidateOptions allows dependency injection for testing the validate command.
type ValidateOptions struct {
	// Runner is an optional validation runner. If nil, a real one will be created.
	Runner ValidationRunner
	// WorkDir is an optional working directory. If empty, os.Getwd() is used.
	// This allows tests to avoid race conditions with directory changes.
	WorkDir string
}

// ValidationRunner interface allows mocking the validation pipeline for tests.
type ValidationRunner interface {
	SetProgressCallback(cb validation.ProgressCallback)
	Run(ctx context.Context, workDir string) (*validation.PipelineResult, error)
}

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
  atlas validate --verbose
  atlas validate --quiet`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runValidate(cmd.Context(), cmd, os.Stdout)
		},
	}

	cmd.Flags().BoolP("quiet", "q", false, "Show only final pass/fail result")

	return cmd
}

func runValidate(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	return runValidateWithOptions(ctx, cmd, w, nil)
}

// runValidateWithOptions is the testable version that accepts optional dependencies.
//
//nolint:gocognit // TODO: Refactor to reduce complexity - extract progress callback setup and result handling
func runValidateWithOptions(ctx context.Context, cmd *cobra.Command, w io.Writer, opts *ValidateOptions) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := Logger()
	// Get flag values with defensive null checks for testing
	// (flags may not be properly inherited when runValidateWithOptions is called directly)
	outputFormat := getStringFlagValue(cmd, "output")
	verbose := getBoolFlagValue(cmd, "verbose")
	quiet := getBoolFlagValue(cmd, "quiet")
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// Load config
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using defaults")
		cfg = config.DefaultConfig()
	}

	// Get current working directory
	var workDir string
	if opts != nil && opts.WorkDir != "" {
		workDir = opts.WorkDir
	} else {
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Create spinner for progress indication (only for TTY output)
	// The spinner wraps the writer with mutex protection for thread safety
	spinner := tui.NewTerminalSpinner(w)

	// Use injected runner if provided (for testing), otherwise create real runner
	var runner ValidationRunner
	if opts != nil && opts.Runner != nil {
		runner = opts.Runner
	} else {
		// Create executor and runner
		executor := validation.NewExecutor(cfg.Validation.Timeout)

		// Enable live output streaming in verbose mode
		// Use the spinner's writer to ensure thread-safe access to the output
		if verbose {
			executor.SetLiveOutput(spinner.Writer())
		}

		runnerConfig := &validation.RunnerConfig{
			FormatCommands:    cfg.Validation.Commands.Format,
			LintCommands:      cfg.Validation.Commands.Lint,
			TestCommands:      cfg.Validation.Commands.Test,
			PreCommitCommands: cfg.Validation.Commands.PreCommit,
		}
		runner = validation.NewRunner(executor, runnerConfig)
	}

	// Set up progress callback for TUI output
	runner.SetProgressCallback(func(step, status string, info *validation.ProgressInfo) {
		// Skip all progress output in quiet mode
		if quiet {
			return
		}

		// For JSON output, skip visual feedback
		if outputFormat == OutputJSON {
			return
		}

		stepInfo := ""
		if info != nil && info.TotalSteps > 0 {
			stepInfo = fmt.Sprintf("[%d/%d] ", info.CurrentStep, info.TotalSteps)
		}

		switch constants.ValidationProgressStatus(status) {
		case constants.ValidationProgressStarting:
			// Use spinner for starting status
			spinner.Start(ctx, fmt.Sprintf("%sRunning %s...", stepInfo, step))
		case constants.ValidationProgressCompleted:
			// Stop spinner and show success
			duration := ""
			if info != nil && info.DurationMs > 0 {
				duration = fmt.Sprintf(" (%s)", tui.FormatDuration(info.DurationMs))
			}
			spinner.StopWithSuccess(fmt.Sprintf("%s passed%s", capitalizeStep(step), duration))
		case constants.ValidationProgressFailed:
			// Stop spinner - error will be reported later with details
			if verbose {
				spinner.StopWithError(fmt.Sprintf("%s failed", capitalizeStep(step)))
			} else {
				spinner.Stop()
			}
		case constants.ValidationProgressSkipped:
			spinner.StopWithWarning(fmt.Sprintf("%s skipped (tool not installed)", capitalizeStep(step)))
		}
	})

	// Run the validation pipeline
	result, err := runner.Run(ctx, workDir)

	// Ensure spinner is stopped on exit
	spinner.Stop()

	// Handle JSON output
	if outputFormat == OutputJSON {
		return out.JSON(pipelineResultToResponse(result))
	}

	// Handle error
	if err != nil {
		return handlePipelineFailure(out, result)
	}

	out.Success("All validations passed!")
	return nil
}

// capitalizeStep formats step names for display.
func capitalizeStep(step string) string {
	switch step {
	case "format":
		return "Format"
	case "lint":
		return "Lint"
	case "test":
		return "Test"
	case "pre-commit":
		return "Pre-commit"
	default:
		return step
	}
}

// pipelineResultToResponse converts PipelineResult to ValidationResponse for JSON output.
func pipelineResultToResponse(result *validation.PipelineResult) ValidationResponse {
	if result == nil {
		return ValidationResponse{Success: false}
	}

	allResults := result.AllResults()
	cliResults := make([]CommandResult, 0, len(allResults))
	for _, r := range allResults {
		cliResults = append(cliResults, CommandResult{
			Command:    r.Command,
			Success:    r.Success,
			ExitCode:   r.ExitCode,
			Output:     r.Stdout,
			Error:      r.Error,
			DurationMs: r.DurationMs,
		})
	}

	return ValidationResponse{
		Success:      result.Success,
		Results:      cliResults,
		SkippedSteps: result.SkippedSteps,
		SkipReasons:  result.SkipReasons,
	}
}

// handlePipelineFailure handles validation pipeline failure output.
func handlePipelineFailure(out tui.Output, result *validation.PipelineResult) error {
	if result == nil {
		return errors.ErrValidationFailed
	}

	// Find the failed result and display error details
	allResults := result.AllResults()
	for _, r := range allResults {
		if !r.Success {
			out.Error(tui.WrapWithSuggestion(fmt.Errorf("%w: %s (exit code: %d)", errors.ErrValidationFailed, r.Command, r.ExitCode)))
			if r.Error != "" {
				out.Info(r.Error)
			}
			if r.Stderr != "" && r.Stderr != r.Error {
				out.Info(r.Stderr)
			}
			break
		}
	}

	return errors.ErrValidationFailed
}

// getStringFlagValue safely retrieves a string flag value, returning empty string if flag doesn't exist.
func getStringFlagValue(cmd *cobra.Command, name string) string {
	if f := cmd.Flag(name); f != nil {
		return f.Value.String()
	}
	return ""
}

// getBoolFlagValue safely retrieves a bool flag value, returning false if flag doesn't exist.
func getBoolFlagValue(cmd *cobra.Command, name string) bool {
	if f := cmd.Flag(name); f != nil {
		return f.Value.String() == "true"
	}
	return false
}
