package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

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
type DefaultExecutor struct{}

// Execute runs the command and captures its output.
func (e *DefaultExecutor) Execute(_ context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// ClaudeCodeRunner implements AIRunner for Claude Code CLI invocation.
// It builds command-line arguments and executes the claude CLI,
// parsing the JSON response into an AIResult.
type ClaudeCodeRunner struct {
	config   *config.AIConfig
	executor CommandExecutor
}

// NewClaudeCodeRunner creates a new ClaudeCodeRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewClaudeCodeRunner(cfg *config.AIConfig, executor CommandExecutor) *ClaudeCodeRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &ClaudeCodeRunner{
		config:   cfg,
		executor: executor,
	}
}

// Run executes an AI request using the Claude Code CLI.
// This method builds the command, executes it, and parses the JSON response.
func (r *ClaudeCodeRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Determine timeout: request > config > default
	timeout := req.Timeout
	if timeout == 0 && r.config != nil {
		timeout = r.config.Timeout
	}
	if timeout == 0 {
		timeout = constants.DefaultAITimeout
	}

	// Create child context with timeout
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retry logic
	return r.runWithRetry(runCtx, req)
}

// runWithRetry executes the AI request with exponential backoff retry logic.
// Only transient errors are retried; non-retryable errors return immediately.
func (r *ClaudeCodeRunner) runWithRetry(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	var lastErr error
	backoff := constants.InitialBackoff

	for attempt := 1; attempt <= constants.MaxRetryAttempts; attempt++ {
		result, err := r.execute(ctx, req)
		if err == nil {
			return result, nil
		}

		// Don't retry non-retryable errors
		if !isRetryable(err) {
			return nil, err
		}

		lastErr = err
		if attempt < constants.MaxRetryAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeSleep(backoff):
				backoff *= 2 // Exponential backoff
			}
		}
	}

	return nil, fmt.Errorf("%w: max retries exceeded: %s", atlaserrors.ErrClaudeInvocation, lastErr.Error())
}

// execute performs a single AI request execution.
func (r *ClaudeCodeRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Pre-flight check: verify working directory exists before attempting to run command
	// This prevents wasteful retry attempts when the worktree has been deleted
	if req.WorkingDir != "" {
		if _, err := os.Stat(req.WorkingDir); os.IsNotExist(err) {
			return nil, fmt.Errorf("working directory missing: %s: %w",
				req.WorkingDir, atlaserrors.ErrWorktreeNotFound)
		}
	}

	// Build the command
	cmd := r.buildCommand(ctx, req)

	// Pass prompt via stdin for large prompts
	cmd.Stdin = strings.NewReader(req.Prompt)

	// Execute the command
	stdout, stderr, err := r.executor.Execute(ctx, cmd)
	if err != nil {
		return r.handleExecutionError(ctx, err, stdout, stderr)
	}

	// Parse the JSON response
	resp, parseErr := parseClaudeResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
}

// handleExecutionError processes errors from command execution.
func (r *ClaudeCodeRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	// Check if context was canceled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Try to parse response even on error (may have valid JSON with error info)
	result, handled := r.tryParseErrorResponse(err, stdout, stderr)
	if handled {
		return result, nil
	}

	return nil, wrapExecutionError(err, stderr)
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

// buildCommand constructs the claude CLI command with appropriate flags.
func (r *ClaudeCodeRunner) buildCommand(ctx context.Context, req *domain.AIRequest) *exec.Cmd {
	args := []string{
		"-p",                      // Print mode (non-interactive)
		"--output-format", "json", // JSON output format
	}

	// Determine model: request > config
	model := req.Model
	if model == "" && r.config != nil {
		model = r.config.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Budget limiting: request > config (0 = unlimited)
	budgetUSD := req.MaxBudgetUSD
	if budgetUSD == 0 && r.config != nil {
		budgetUSD = r.config.MaxBudgetUSD
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

// wrapExecutionError wraps an execution error with context.
func wrapExecutionError(err error, stderr []byte) error {
	stderrStr := strings.TrimSpace(string(stderr))

	// Check for specific error conditions
	if strings.Contains(stderrStr, "command not found") ||
		strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("%w: claude CLI not found - please install claude code", atlaserrors.ErrClaudeInvocation)
	}

	if strings.Contains(stderrStr, "api key") || strings.Contains(stderrStr, "API key") ||
		strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "ANTHROPIC_API_KEY") {
		return fmt.Errorf("%w: API key error: %s", atlaserrors.ErrClaudeInvocation, stderrStr)
	}

	if stderrStr != "" {
		return fmt.Errorf("%w: %s", atlaserrors.ErrClaudeInvocation, stderrStr)
	}

	return fmt.Errorf("%w: %s", atlaserrors.ErrClaudeInvocation, err.Error())
}

// Compile-time check that ClaudeCodeRunner implements Runner.
var _ Runner = (*ClaudeCodeRunner)(nil)
