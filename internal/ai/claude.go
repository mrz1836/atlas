package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	base BaseRunner // Embedded BaseRunner for timeout/retry handling
}

// NewClaudeCodeRunner creates a new ClaudeCodeRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewClaudeCodeRunner(cfg *config.AIConfig, executor CommandExecutor) *ClaudeCodeRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &ClaudeCodeRunner{
		base: BaseRunner{
			Config:   cfg,
			Executor: executor,
			ErrType:  atlaserrors.ErrClaudeInvocation,
		},
	}
}

// Run executes an AI request using the Claude Code CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Claude-specific command execution.
func (r *ClaudeCodeRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
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
	stdout, stderr, err := r.base.Executor.Execute(ctx, cmd)
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

// buildCommand constructs the claude CLI command with appropriate flags.
func (r *ClaudeCodeRunner) buildCommand(ctx context.Context, req *domain.AIRequest) *exec.Cmd {
	args := []string{
		"-p",                      // Print mode (non-interactive)
		"--output-format", "json", // JSON output format
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
