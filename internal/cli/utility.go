// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
)

// CommandResult holds the result of a single command execution.
type CommandResult struct {
	Command    string `json:"command"`
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ValidationResponse is the JSON response for validation commands.
type ValidationResponse struct {
	Success      bool              `json:"success"`
	Results      []CommandResult   `json:"results"`
	SkippedSteps []string          `json:"skipped_steps,omitempty"`
	SkipReasons  map[string]string `json:"skip_reasons,omitempty"`
}

// UtilityOptions holds options for utility command execution.
type UtilityOptions struct {
	Verbose      bool
	OutputFormat string
	Writer       io.Writer
}

// getDefaultCommands returns the default command for a given category.
func getDefaultCommands(category string) []string {
	switch category {
	case "format":
		return []string{constants.DefaultFormatCommand}
	case "lint":
		return []string{constants.DefaultLintCommand}
	case "test":
		return []string{constants.DefaultTestCommand}
	case "pre-commit":
		return []string{constants.DefaultPreCommitCommand}
	default:
		return nil
	}
}

// showVerboseOutput displays command output in verbose mode.
func showVerboseOutput(opts UtilityOptions, result CommandResult) {
	if !opts.Verbose {
		return
	}
	if result.Output != "" {
		_, _ = fmt.Fprintln(opts.Writer, result.Output)
	}
	if result.Error != "" {
		_, _ = fmt.Fprintln(opts.Writer, result.Error)
	}
}

// runSingleCommand executes a single command and returns the result.
func runSingleCommand(ctx context.Context, runner validation.CommandRunner, workDir, cmdStr string, logger zerolog.Logger) CommandResult {
	start := time.Now()

	stdout, stderr, exitCode, err := runner.Run(ctx, workDir, cmdStr)

	result := CommandResult{
		Command:    cmdStr,
		Success:    err == nil && exitCode == 0,
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}

	if stdout != "" {
		result.Output = stdout
	}
	if err != nil || exitCode != 0 {
		if stderr != "" {
			result.Error = stderr
		} else if err != nil {
			result.Error = err.Error()
		}
	}

	logger.Debug().
		Str("command", cmdStr).
		Bool("success", result.Success).
		Int("exit_code", exitCode).
		Int64("duration_ms", result.DurationMs).
		Msg("command executed")

	return result
}

// runCommandsWithOutput executes commands sequentially and handles output.
func runCommandsWithOutput(
	ctx context.Context,
	commands []string,
	workDir string,
	category string,
	out tui.Output,
	opts UtilityOptions,
	logger zerolog.Logger,
) error {
	runner := &validation.DefaultCommandRunner{}
	results := make([]CommandResult, 0, len(commands))

	for _, cmdStr := range commands {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Show command being executed in verbose mode (TTY only - JSON gets final response)
		if opts.Verbose && opts.OutputFormat != OutputJSON {
			out.Info(fmt.Sprintf("⏳ Running: %s", cmdStr))
		}

		result := runSingleCommand(ctx, runner, workDir, cmdStr, logger)
		results = append(results, result)

		// In verbose mode, show command output
		showVerboseOutput(opts, result)

		if !result.Success {
			if opts.OutputFormat == OutputJSON {
				return out.JSON(ValidationResponse{
					Success: false,
					Results: results,
				})
			}
			out.Error(tui.WrapWithSuggestion(fmt.Errorf("%w: %s in %s", errors.ErrCommandFailed, cmdStr, category)))
			return fmt.Errorf("%w: %s in %s", errors.ErrCommandFailed, cmdStr, category)
		}

		// Only show per-command success for TTY output (JSON gets final response only)
		// TTYOutput.Success() adds the ✓ prefix automatically, so don't include icon here
		if opts.OutputFormat != OutputJSON {
			out.Success(cmdStr)
		}
	}

	if opts.OutputFormat == OutputJSON {
		return out.JSON(ValidationResponse{
			Success: true,
			Results: results,
		})
	}

	out.Success(fmt.Sprintf("%s completed successfully", category))
	return nil
}

// handleValidationFailure handles validation failure output.
func handleValidationFailure(out tui.Output, outputFormat string, results []CommandResult) error {
	if outputFormat == OutputJSON {
		return out.JSON(ValidationResponse{
			Success: false,
			Results: results,
		})
	}

	// Find the failed command
	for _, r := range results {
		if !r.Success {
			out.Error(tui.WrapWithSuggestion(fmt.Errorf("%w: %s (exit code: %d)", errors.ErrValidationFailed, r.Command, r.ExitCode)))
			if r.Error != "" {
				out.Info(r.Error)
			}
			break
		}
	}

	return errors.ErrValidationFailed
}

// encodeJSONIndented encodes a value as indented JSON to the writer.
// This is a shared helper for JSON error output functions across commands.
func encodeJSONIndented(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// Default values for smart commit and PR description configuration.
const (
	DefaultSmartCommitTimeout       = 30 * time.Second
	DefaultSmartCommitMaxRetries    = 2
	DefaultSmartCommitBackoffFactor = 1.5
)

// ResolvedGitConfig holds resolved agent/model settings for git operations.
// This consolidates the fallback logic for SmartCommit and PRDescription settings.
type ResolvedGitConfig struct {
	CommitAgent         string
	CommitModel         string
	PRDescAgent         string
	PRDescModel         string
	CommitTimeout       time.Duration
	CommitMaxRetries    int
	CommitBackoffFactor float64
}

// ResolveGitConfig resolves SmartCommit and PRDescription settings with fallback to global AI config.
// This eliminates duplicated resolution logic across commands.
func ResolveGitConfig(cfg *config.Config) ResolvedGitConfig {
	// Resolve commit agent/model with fallback to global AI config
	commitAgent := cfg.SmartCommit.Agent
	if commitAgent == "" {
		commitAgent = cfg.AI.Agent
	}
	commitModel := cfg.SmartCommit.Model
	if commitModel == "" {
		commitModel = cfg.AI.Model
	}

	// Resolve PR description agent/model with fallback to global AI config
	prDescAgent := cfg.PRDescription.Agent
	if prDescAgent == "" {
		prDescAgent = cfg.AI.Agent
	}
	prDescModel := cfg.PRDescription.Model
	if prDescModel == "" {
		prDescModel = cfg.AI.Model
	}

	// Resolve smart commit timeout/retry settings with defaults
	commitTimeout := cfg.SmartCommit.Timeout
	if commitTimeout == 0 {
		commitTimeout = DefaultSmartCommitTimeout
	}
	commitMaxRetries := cfg.SmartCommit.MaxRetries
	if commitMaxRetries == 0 {
		commitMaxRetries = DefaultSmartCommitMaxRetries
	}
	commitBackoffFactor := cfg.SmartCommit.RetryBackoffFactor
	if commitBackoffFactor == 0 {
		commitBackoffFactor = DefaultSmartCommitBackoffFactor
	}

	return ResolvedGitConfig{
		CommitAgent:         commitAgent,
		CommitModel:         commitModel,
		PRDescAgent:         prDescAgent,
		PRDescModel:         prDescModel,
		CommitTimeout:       commitTimeout,
		CommitMaxRetries:    commitMaxRetries,
		CommitBackoffFactor: commitBackoffFactor,
	}
}
