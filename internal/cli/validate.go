// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
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

	// Create executor and runner
	executor := validation.NewExecutor(cfg.Validation.Timeout)
	runnerConfig := &validation.RunnerConfig{
		FormatCommands:    cfg.Validation.Commands.Format,
		LintCommands:      cfg.Validation.Commands.Lint,
		TestCommands:      cfg.Validation.Commands.Test,
		PreCommitCommands: cfg.Validation.Commands.PreCommit,
	}
	runner := validation.NewRunner(executor, runnerConfig)

	// Set up progress callback for TUI output
	runner.SetProgressCallback(func(step, status string) {
		switch status {
		case "starting":
			out.Info(fmt.Sprintf("Running %s...", step))
		case "completed":
			out.Success(fmt.Sprintf("%s passed", capitalizeStep(step)))
		case "failed":
			// Error will be reported later with details
			if verbose {
				out.Info(fmt.Sprintf("%s failed", capitalizeStep(step)))
			}
		case "skipped":
			out.Warning(fmt.Sprintf("%s skipped (tool not installed)", capitalizeStep(step)))
		}
	})

	// Run the validation pipeline
	result, err := runner.Run(ctx, workDir)

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
			out.Error(fmt.Errorf("%w: %s (exit code: %d)", errors.ErrValidationFailed, r.Command, r.ExitCode))
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
