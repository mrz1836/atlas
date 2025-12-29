// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// CommandRunner defines the interface for executing shell commands.
// This allows for testing by injecting mock implementations.
type CommandRunner interface {
	// Run executes a shell command and returns its output.
	Run(ctx context.Context, workDir, command string) (stdout, stderr string, exitCode int, err error)
}

// DefaultCommandRunner implements CommandRunner using os/exec.
type DefaultCommandRunner struct{}

// Run executes a shell command using sh -c.
func (r *DefaultCommandRunner) Run(ctx context.Context, workDir, command string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return stdout, stderr, exitCode, err
}

// ValidationExecutor handles validation steps.
// It runs configured validation commands in order and captures their output.
type ValidationExecutor struct {
	workDir string
	runner  CommandRunner
}

// NewValidationExecutor creates a new validation executor.
func NewValidationExecutor(workDir string) *ValidationExecutor {
	return &ValidationExecutor{
		workDir: workDir,
		runner:  &DefaultCommandRunner{},
	}
}

// NewValidationExecutorWithRunner creates a validation executor with a custom command runner.
// This is primarily used for testing.
func NewValidationExecutorWithRunner(workDir string, runner CommandRunner) *ValidationExecutor {
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
	var output bytes.Buffer

	// Get validation commands from task config or use defaults
	commands := task.Config.ValidationCommands
	if len(commands) == 0 {
		commands = []string{"magex format:fix", "magex lint", "magex test"}
	}

	log.Debug().
		Strs("commands", commands).
		Str("work_dir", e.workDir).
		Msg("running validation commands")

	// Execute each command in order
	for i, cmdStr := range commands {
		// Check for cancellation between commands
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.Debug().
			Int("index", i).
			Str("command", cmdStr).
			Msg("running validation command")

		stdout, stderr, exitCode, err := e.runner.Run(ctx, e.workDir, cmdStr)

		if err != nil || exitCode != 0 {
			output.WriteString(fmt.Sprintf("✗ Command failed: %s (exit code: %d)\n", cmdStr, exitCode))
			if stdout != "" {
				output.WriteString(fmt.Sprintf("stdout:\n%s\n", stdout))
			}
			if stderr != "" {
				output.WriteString(fmt.Sprintf("stderr:\n%s\n", stderr))
			}

			elapsed := time.Since(startTime)
			log.Error().
				Str("task_id", task.ID).
				Str("step_name", step.Name).
				Str("command", cmdStr).
				Int("exit_code", exitCode).
				Dur("duration_ms", elapsed).
				Msg("validation command failed")

			return &domain.StepResult{
				StepIndex:   task.CurrentStep,
				StepName:    step.Name,
				Status:      "failed",
				StartedAt:   startTime,
				CompletedAt: time.Now(),
				DurationMs:  elapsed.Milliseconds(),
				Output:      output.String(),
				Error:       fmt.Sprintf("validation command failed: %s", cmdStr),
			}, fmt.Errorf("%w: %s", atlaserrors.ErrValidationFailed, cmdStr)
		}

		output.WriteString(fmt.Sprintf("✓ %s\n", cmdStr))
		if stdout != "" {
			log.Debug().
				Str("command", cmdStr).
				Str("output", stdout).
				Msg("command output")
		}
	}

	elapsed := time.Since(startTime)
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
