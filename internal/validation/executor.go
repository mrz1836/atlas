package validation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	return e.RunWithPhase(ctx, commands, workDir, "")
}

// RunWithPhase executes commands sequentially with phase context for logging.
// The phase parameter identifies which validation phase is running (pre-commit, format, lint, test).
// Returns all collected results and an error if any command failed.
func (e *Executor) RunWithPhase(ctx context.Context, commands []string, workDir, phase string) ([]Result, error) {
	results := make([]Result, 0, len(commands))
	total := len(commands)

	for i, cmd := range commands {
		// Check for context cancellation between commands
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := e.runSingleWithPhase(ctx, cmd, workDir, phase, i+1, total)
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
	return e.runSingleWithPhase(ctx, command, workDir, "", 0, 0)
}

// runSingleWithPhase executes a single command with phase context for logging.
func (e *Executor) runSingleWithPhase(ctx context.Context, command, workDir, phase string, cmdNum, totalCmds int) (*Result, error) {
	log := zerolog.Ctx(ctx)

	// Pre-flight check: verify workDir exists
	if result, err := e.validateWorkDir(command, workDir, log); err != nil {
		return result, err
	}

	startTime := time.Now()
	e.logCommandStart(log, command, workDir, phase, cmdNum, totalCmds)

	// Execute command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	stdout, stderr, exitCode, runErr := e.executeCommand(cmdCtx, command, workDir)

	completedAt := time.Now()
	duration := completedAt.Sub(startTime)

	result := e.buildResult(command, stdout, stderr, exitCode, startTime, completedAt, duration)

	return e.handleCommandOutcome(ctx, cmdCtx, result, command, exitCode, duration, runErr, log)
}

// validateWorkDir checks if the work directory exists before running a command.
func (e *Executor) validateWorkDir(command, workDir string, log *zerolog.Logger) (*Result, error) {
	if workDir == "" {
		return nil, nil //nolint:nilnil // No validation needed when workDir is empty
	}

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		log.Error().
			Str("work_dir", workDir).
			Str("command", command).
			Msg("CRITICAL: work directory missing before validation command")
		return &Result{
			Command: command,
			Success: false,
			Error:   fmt.Sprintf("work directory missing: %s", workDir),
		}, fmt.Errorf("work directory missing: %s: %w", workDir, atlaserrors.ErrWorktreeNotFound)
	}

	return nil, nil //nolint:nilnil // Validation passed, no result or error needed
}

// logCommandStart logs the start of a validation command with phase context.
func (e *Executor) logCommandStart(log *zerolog.Logger, command, workDir, phase string, cmdNum, totalCmds int) {
	logEvent := log.Info().
		Str("command", command).
		Str("work_dir", workDir)

	if phase != "" {
		logEvent = logEvent.Str("phase", phase)
	}
	if cmdNum > 0 && totalCmds > 0 {
		logEvent = logEvent.Int("command_num", cmdNum).Int("total_commands", totalCmds)
	}

	logEvent.Msg("executing validation command")
}

// executeCommand runs the command and returns raw output.
func (e *Executor) executeCommand(ctx context.Context, command, workDir string) (stdout, stderr string, exitCode int, runErr error) {
	if e.liveOutput != nil {
		if liveRunner, ok := e.runner.(LiveOutputRunner); ok {
			return liveRunner.RunWithLiveOutput(ctx, workDir, command, e.liveOutput)
		}
	}

	return e.runner.Run(ctx, workDir, command)
}

// buildResult constructs a Result struct from command execution data.
func (e *Executor) buildResult(command, stdout, stderr string, exitCode int, startTime, completedAt time.Time, duration time.Duration) *Result {
	return &Result{
		Command:     command,
		ExitCode:    exitCode,
		Stdout:      stdout,
		Stderr:      stderr,
		DurationMs:  duration.Milliseconds(),
		StartedAt:   startTime,
		CompletedAt: completedAt,
	}
}

// handleCommandOutcome processes the result and determines success/failure.
func (e *Executor) handleCommandOutcome(ctx, cmdCtx context.Context, result *Result, command string, exitCode int, duration time.Duration, runErr error, log *zerolog.Logger) (*Result, error) {
	// Check for timeout
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		result.Success = false
		result.Error = "command timed out"

		log.Error().
			Str("command", command).
			Dur("duration_ms", duration).
			Str("stdout", result.Stdout).
			Str("stderr", result.Stderr).
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
			Str("stderr", result.Stderr).
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
