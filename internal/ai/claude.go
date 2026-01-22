package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// claudeCLIInfo contains Claude-specific CLI metadata for error messages.
//
//nolint:gochecknoglobals // Constant-like structure
var claudeCLIInfo = CLIInfo{
	Name:        "claude",
	InstallHint: "please install claude code",
	ErrType:     atlaserrors.ErrClaudeInvocation,
	EnvVar:      "ANTHROPIC_API_KEY",
}

// CommandExecutor abstracts command execution for testing.
// The production implementation uses exec.Cmd to run subprocesses,
// while tests can provide a mock implementation.
//
// The ctx parameter is included for interface consistency and future flexibility,
// even though the current implementation embeds context via exec.CommandContext().
// Mock implementations may use ctx to simulate cancellation behavior.
type CommandExecutor interface {
	// Execute runs the command and returns stdout, stderr, and any error.
	// The context is passed for mock implementations that need cancellation awareness.
	Execute(ctx context.Context, cmd *exec.Cmd) (stdout, stderr []byte, err error)
}

// DefaultExecutor is the production implementation of CommandExecutor.
// It runs commands using the operating system's process execution.
// Unlike StreamingExecutor, it captures output synchronously but still
// tracks the running process for termination support.
type DefaultExecutor struct {
	runningProcess *os.Process // Track the running process for termination
	processMu      sync.Mutex  // Protect process access
}

// Execute runs the command and captures its output.
// Uses Start() + Wait() pattern to allow process tracking for termination.
func (e *DefaultExecutor) Execute(_ context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	// Track the running process for termination
	e.processMu.Lock()
	e.runningProcess = cmd.Process
	e.processMu.Unlock()

	// Wait for command completion
	err := cmd.Wait()

	// Clear the process reference after completion
	e.processMu.Lock()
	e.runningProcess = nil
	e.processMu.Unlock()

	return stdout.Bytes(), stderr.Bytes(), err
}

// TerminateProcess terminates the running process gracefully with SIGTERM.
// If the process doesn't exit within a short timeout, SIGKILL is sent.
// This method blocks until the SIGKILL is sent (if needed) to prevent
// orphaned processes when the parent exits before the kill fires.
// Returns nil if no process is running or if termination succeeds.
func (e *DefaultExecutor) TerminateProcess() error {
	e.processMu.Lock()
	proc := e.runningProcess
	e.processMu.Unlock()

	if proc == nil {
		return nil // No process to terminate
	}

	// First try SIGTERM for graceful shutdown
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Process may have already exited
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}

	// Wait 2 seconds for graceful exit, then send SIGKILL.
	// We poll to check if process has exited rather than calling Wait()
	// because Execute() also calls cmd.Wait() and Wait() can only be called once.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// Check if process is still referenced (cleared after cmd.Wait() completes)
		e.processMu.Lock()
		currentProc := e.runningProcess
		e.processMu.Unlock()

		if currentProc == nil || currentProc.Pid != proc.Pid {
			// Process has exited and Execute() has completed
			return nil
		}

		// Also try to check if process is still alive by sending signal 0
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process no longer exists
			return err
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Timeout: force kill synchronously
	e.processMu.Lock()
	currentProc := e.runningProcess
	e.processMu.Unlock()

	// Only kill if same process is still running
	if currentProc != nil && currentProc.Pid == proc.Pid {
		_ = proc.Kill() // Best effort, ignore errors
	}

	return nil
}

// ClaudeCodeRunner implements AIRunner for Claude Code CLI invocation.
// It builds command-line arguments and executes the claude CLI,
// parsing the JSON response into an AIResult.
type ClaudeCodeRunner struct {
	base            BaseRunner // Embedded BaseRunner for timeout/retry handling
	logger          zerolog.Logger
	activityOptions *ActivityOptions
}

// ClaudeRunnerOption is a functional option for configuring ClaudeCodeRunner.
type ClaudeRunnerOption func(*ClaudeCodeRunner)

// WithClaudeLogger sets the logger for the ClaudeCodeRunner.
func WithClaudeLogger(logger zerolog.Logger) ClaudeRunnerOption {
	return func(r *ClaudeCodeRunner) {
		r.logger = logger
	}
}

// WithClaudeActivityCallback configures the activity callback for streaming activity events.
func WithClaudeActivityCallback(opts ActivityOptions) ClaudeRunnerOption {
	return func(r *ClaudeCodeRunner) {
		r.activityOptions = &opts
	}
}

// NewClaudeCodeRunner creates a new ClaudeCodeRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewClaudeCodeRunner(cfg *config.AIConfig, executor CommandExecutor, opts ...ClaudeRunnerOption) *ClaudeCodeRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	r := &ClaudeCodeRunner{
		base: BaseRunner{
			Config:   cfg,
			Executor: executor,
			ErrType:  atlaserrors.ErrClaudeInvocation,
			Logger:   zerolog.Nop(), // Will be updated if WithClaudeLogger is used
		},
		logger: zerolog.Nop(), // Default to no-op logger
	}
	for _, opt := range opts {
		opt(r)
	}

	// Sync BaseRunner logger with ClaudeCodeRunner logger
	r.base.Logger = r.logger

	// If activity streaming is enabled, swap executor for StreamingExecutor
	if r.activityOptions != nil && r.activityOptions.Callback != nil {
		r.base.Executor = NewStreamingExecutor(*r.activityOptions)
	}

	return r
}

// Run executes an AI request using the Claude Code CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Claude-specific command execution.
func (r *ClaudeCodeRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
}

// TerminateRunningProcess terminates any currently running AI subprocess.
// This implements the TerminatableRunner interface for cleanup on Ctrl+C.
func (r *ClaudeCodeRunner) TerminateRunningProcess() error {
	return r.base.TerminateRunningProcess()
}

// execute performs a single AI request execution.
func (r *ClaudeCodeRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Pre-flight check: verify working directory exists
	if err := r.base.ValidateWorkingDir(req.WorkingDir); err != nil {
		return nil, err
	}

	// Build the command
	cmd := r.buildCommand(ctx, req)

	// Pass prompt via stdin for large prompts
	cmd.Stdin = strings.NewReader(req.Prompt)

	// Execute the command
	stdout, stderr, err := r.base.Executor.Execute(ctx, cmd)
	if err != nil {
		return r.handleExecutionError(ctx, err, stdout, stderr)
	}

	// Parse the response - prefer streaming result if available
	resp, parseErr := r.parseResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
}

// parseResponse parses the Claude CLI response.
// When streaming is enabled, it prefers the result from StreamingExecutor.
// Otherwise, it parses stdout as a single JSON object.
func (r *ClaudeCodeRunner) parseResponse(stdout []byte) (*ClaudeResponse, error) {
	// If streaming is enabled, try to get result from StreamingExecutor first
	if r.isStreamingEnabled() {
		if streamExec, ok := r.base.Executor.(*StreamingExecutor); ok {
			if result := streamExec.LastClaudeResult(); result != nil {
				return result, nil
			}
		}
	}

	// Fall back to parsing stdout as JSON (for non-streaming or if no result in stream)
	return parseClaudeResponse(stdout)
}

// handleExecutionError processes errors from command execution.
func (r *ClaudeCodeRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	return r.base.HandleProviderExecutionError(ctx, claudeCLIInfo, err, stderr,
		func() (*domain.AIResult, bool) {
			return r.tryParseErrorResponse(err, stdout, stderr)
		},
	)
}

// tryParseErrorResponse attempts to extract error information from a JSON response.
// Returns the result and true if the error was successfully parsed, otherwise nil and false.
func (r *ClaudeCodeRunner) tryParseErrorResponse(execErr error, stdout, stderr []byte) (*domain.AIResult, bool) {
	if len(stdout) == 0 {
		return nil, false
	}

	resp, parseErr := parseClaudeResponse(stdout)
	if parseErr != nil || !resp.IsError {
		return nil, false
	}

	result := resp.toAIResult(string(stderr))
	result.Error = fmt.Sprintf("%s: %s", execErr.Error(), string(stderr))
	return result, true
}

// isStreamingEnabled returns true if activity streaming is enabled.
func (r *ClaudeCodeRunner) isStreamingEnabled() bool {
	return r.activityOptions != nil && r.activityOptions.Callback != nil
}

// buildCommand constructs the claude CLI command with appropriate flags.
func (r *ClaudeCodeRunner) buildCommand(ctx context.Context, req *domain.AIRequest) *exec.Cmd {
	// Use stream-json format when activity streaming is enabled for real-time tool events
	outputFormat := "json"
	useStreaming := r.isStreamingEnabled()
	if useStreaming {
		outputFormat = "stream-json"
	}

	args := []string{
		"-p", // Print mode (non-interactive)
		"--output-format", outputFormat,
	}

	// stream-json requires --verbose flag
	if useStreaming {
		args = append(args, "--verbose")
	}

	// Determine model: request > config
	model := req.Model
	if model == "" && r.base.Config != nil {
		model = r.base.Config.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Budget limiting: request > config (0 = unlimited)
	budgetUSD := req.MaxBudgetUSD
	if budgetUSD == 0 && r.base.Config != nil {
		budgetUSD = r.base.Config.MaxBudgetUSD
	}
	if budgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", budgetUSD))
	}

	// Add permission mode if specified
	if req.PermissionMode != "" {
		args = append(args, "--permission-mode", req.PermissionMode)
	}

	// Add system prompt if specified
	if req.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", req.SystemPrompt)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)

	// Set working directory if specified
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	return cmd
}

// Compile-time check that ClaudeCodeRunner implements Runner.
var _ Runner = (*ClaudeCodeRunner)(nil)

// Compile-time check that ClaudeCodeRunner implements TerminatableRunner.
var _ TerminatableRunner = (*ClaudeCodeRunner)(nil)
