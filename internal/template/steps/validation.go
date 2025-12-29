// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/validation"
)

// CommandRunner is an alias to validation.CommandRunner for backward compatibility.
// Deprecated: Use validation.CommandRunner directly. Will be removed in Epic 7+.
type CommandRunner = validation.CommandRunner

// DefaultCommandRunner is an alias to validation.DefaultCommandRunner for backward compatibility.
// Deprecated: Use validation.DefaultCommandRunner directly. Will be removed in Epic 7+.
type DefaultCommandRunner = validation.DefaultCommandRunner

// ValidationExecutor handles validation steps.
// It runs configured validation commands in order and captures their output.
type ValidationExecutor struct {
	workDir string
	runner  validation.CommandRunner
}

// NewValidationExecutor creates a new validation executor.
func NewValidationExecutor(workDir string) *ValidationExecutor {
	return &ValidationExecutor{
		workDir: workDir,
		runner:  &validation.DefaultCommandRunner{},
	}
}

// NewValidationExecutorWithRunner creates a validation executor with a custom command runner.
// This is primarily used for testing.
func NewValidationExecutorWithRunner(workDir string, runner validation.CommandRunner) *ValidationExecutor {
	return &ValidationExecutor{
		workDir: workDir,
		runner:  runner,
	}
}

// Execute runs validation commands.
// Commands are retrieved from task.Config.ValidationCommands.
// If no commands are configured, default commands are used.
// Execution stops on the first failure.
func (e *ValidationExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log := zerolog.Ctx(ctx)
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing validation step")

	startTime := time.Now()

	// Get validation commands from task config or use defaults
	commands := task.Config.ValidationCommands
	if len(commands) == 0 {
		commands = []string{"magex format:fix", "magex lint", "magex test"}
	}

	log.Debug().
		Strs("commands", commands).
		Str("work_dir", e.workDir).
		Msg("running validation commands")

	// Use validation.Executor to run commands with proper timeout handling
	executor := validation.NewExecutorWithRunner(validation.DefaultTimeout, e.runner)
	results, execErr := executor.Run(ctx, commands, e.workDir)

	// Build output from results
	var output bytes.Buffer
	for _, result := range results {
		writeResultOutput(&output, result, log)
	}

	elapsed := time.Since(startTime)

	// Handle execution error (validation failed or context canceled)
	if execErr != nil {
		log.Error().
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Err(execErr).
			Dur("duration_ms", elapsed).
			Msg("validation step failed")

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      "failed",
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  elapsed.Milliseconds(),
			Output:      output.String(),
			Error:       execErr.Error(),
		}, execErr
	}

	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Int("commands_run", len(commands)).
		Dur("duration_ms", elapsed).
		Msg("validation step completed")

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  elapsed.Milliseconds(),
		Output:      output.String(),
	}, nil
}

// Type returns the step type this executor handles.
func (e *ValidationExecutor) Type() domain.StepType {
	return domain.StepTypeValidation
}

// writeResultOutput formats a single validation result and writes it to the output buffer.
func writeResultOutput(output *bytes.Buffer, result validation.Result, log *zerolog.Logger) {
	if result.Success {
		fmt.Fprintf(output, "✓ %s\n", result.Command)
		if result.Stdout != "" {
			log.Debug().
				Str("command", result.Command).
				Str("output", result.Stdout).
				Msg("command output")
		}
		return
	}

	fmt.Fprintf(output, "✗ Command failed: %s (exit code: %d)\n", result.Command, result.ExitCode)
	if result.Stdout != "" {
		fmt.Fprintf(output, "stdout:\n%s\n", result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintf(output, "stderr:\n%s\n", result.Stderr)
	}
}
