package validation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// DefaultTimeout is the default timeout for validation commands.
const DefaultTimeout = 5 * time.Minute

// Executor runs validation commands.
type Executor struct {
	runner     CommandRunner
	timeout    time.Duration
	liveOutput io.Writer // Optional: if set, streams command output in real-time
}

// NewExecutor creates a validation executor with default command runner.
func NewExecutor(timeout time.Duration) *Executor {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Executor{
		runner:  &DefaultCommandRunner{},
		timeout: timeout,
	}
}

// NewExecutorWithRunner creates an executor with custom runner (for testing).
func NewExecutorWithRunner(timeout time.Duration, runner CommandRunner) *Executor {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Executor{
		runner:  runner,
		timeout: timeout,
	}
}

// SetLiveOutput configures the executor to stream command output in real-time.
// When set, stdout and stderr are written to w as they are produced.
func (e *Executor) SetLiveOutput(w io.Writer) {
	e.liveOutput = w
}

// Run executes commands sequentially, stopping on first failure.
// Returns all collected results and an error if any command failed.
func (e *Executor) Run(ctx context.Context, commands []string, workDir string) ([]Result, error) {
	results := make([]Result, 0, len(commands))

	for _, cmd := range commands {
		// Check for context cancellation between commands
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := e.RunSingle(ctx, cmd, workDir)
		if result != nil {
			results = append(results, *result)
		}

		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// RunSingle executes a single command with timeout handling.
func (e *Executor) RunSingle(ctx context.Context, command, workDir string) (*Result, error) {
	log := zerolog.Ctx(ctx)
	startTime := time.Now()

	log.Info().
		Str("command", command).
		Str("work_dir", workDir).
		Msg("executing validation command")

	// Create timeout context for this specific command
	cmdCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Execute command with timeout context
	var stdout, stderr string
	var exitCode int
	var runErr error

	// Use live output runner if available and liveOutput is configured
	if e.liveOutput != nil {
		if liveRunner, ok := e.runner.(LiveOutputRunner); ok {
			stdout, stderr, exitCode, runErr = liveRunner.RunWithLiveOutput(cmdCtx, workDir, command, e.liveOutput)
		} else {
			stdout, stderr, exitCode, runErr = e.runner.Run(cmdCtx, workDir, command)
		}
	} else {
		stdout, stderr, exitCode, runErr = e.runner.Run(cmdCtx, workDir, command)
	}

	completedAt := time.Now()
	duration := completedAt.Sub(startTime)

	result := &Result{
		Command:     command,
		ExitCode:    exitCode,
		Stdout:      stdout,
		Stderr:      stderr,
		DurationMs:  duration.Milliseconds(),
		StartedAt:   startTime,
		CompletedAt: completedAt,
	}

	// Check for timeout
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		result.Success = false
		result.Error = "command timed out"

		log.Error().
			Str("command", command).
			Dur("duration_ms", duration).
			Str("stdout", stdout).
			Str("stderr", stderr).
			Msg("validation command timed out")

		return result, atlaserrors.ErrCommandTimeout
	}

	// Check for context cancellation (from parent context)
	if ctx.Err() != nil {
		result.Success = false
		result.Error = "context canceled"
		return result, ctx.Err()
	}

	// Check for command failure
	if runErr != nil || exitCode != 0 {
		result.Success = false
		if runErr != nil {
			result.Error = runErr.Error()
		} else {
			result.Error = fmt.Sprintf("exit code %d", exitCode)
		}

		log.Error().
			Str("command", command).
			Int("exit_code", exitCode).
			Dur("duration_ms", duration).
			Str("stderr", stderr).
			Msg("validation command failed")

		return result, fmt.Errorf("%w: %s", atlaserrors.ErrValidationFailed, command)
	}

	// Success
	result.Success = true

	log.Info().
		Str("command", command).
		Int("exit_code", exitCode).
		Dur("duration_ms", duration).
		Msg("validation command completed")

	return result, nil
}
