package ai

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// codexCLIInfo contains Codex-specific CLI metadata for error messages.
//
//nolint:gochecknoglobals // Constant-like structure
var codexCLIInfo = CLIInfo{
	Name:        "codex",
	InstallHint: "install with: npm install -g @openai/codex",
	ErrType:     atlaserrors.ErrCodexInvocation,
	EnvVar:      "OPENAI_API_KEY",
}

// CodexRunner implements Runner for OpenAI Codex CLI invocation.
// It builds command-line arguments and executes the codex CLI,
// parsing the JSON response into an AIResult.
type CodexRunner struct {
	config   *config.AIConfig
	executor CommandExecutor
}

// NewCodexRunner creates a new CodexRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewCodexRunner(cfg *config.AIConfig, executor CommandExecutor) *CodexRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &CodexRunner{
		config:   cfg,
		executor: executor,
	}
}

// Run executes an AI request using the Codex CLI.
// This method builds the command, executes it, and parses the JSON response.
func (r *CodexRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Check cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
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
func (r *CodexRunner) runWithRetry(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
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

	return nil, fmt.Errorf("%w: max retries exceeded: %s", atlaserrors.ErrCodexInvocation, lastErr.Error())
}

// execute performs a single AI request execution.
func (r *CodexRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
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
	resp, parseErr := parseCodexResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
}

// handleExecutionError processes errors from command execution.
func (r *CodexRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	// Check if context was canceled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Try to parse response even on error (may have valid JSON with error info)
	result, handled := r.tryParseErrorResponse(err, stdout, stderr)
	if handled {
		return result, nil
	}

	return nil, wrapCodexExecutionError(err, stderr)
}

// tryParseErrorResponse attempts to extract error information from a JSON response.
// Returns the result and true if the error was successfully parsed, otherwise nil and false.
func (r *CodexRunner) tryParseErrorResponse(execErr error, stdout, stderr []byte) (*domain.AIResult, bool) {
	if len(stdout) == 0 {
		return nil, false
	}

	resp, parseErr := parseCodexResponse(stdout)
	if parseErr != nil || resp.Success {
		return nil, false
	}

	result := resp.toAIResult(string(stderr))
	result.Error = fmt.Sprintf("%s: %s", execErr.Error(), string(stderr))
	return result, true
}

// buildCommand constructs the codex CLI command with appropriate flags.
func (r *CodexRunner) buildCommand(ctx context.Context, req *domain.AIRequest) *exec.Cmd {
	// Codex uses "codex exec" for non-interactive mode with --json for JSON output
	args := []string{
		"exec",   // Non-interactive execution mode
		"--json", // JSON output format
	}

	// Determine model: request > config
	model := req.Model
	if model == "" && r.config != nil {
		model = r.config.Model
	}

	// Resolve model alias to full model name
	if model != "" {
		model = domain.AgentCodex.ResolveModelAlias(model)
		args = append(args, "-m", model)
	}

	// Codex CLI may support additional flags in the future.
	// Add them here as they become available.

	cmd := exec.CommandContext(ctx, "codex", args...)

	// Set working directory if specified
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	return cmd
}

// wrapCodexExecutionError wraps an execution error with context.
func wrapCodexExecutionError(err error, stderr []byte) error {
	return WrapCLIExecutionError(codexCLIInfo, err, stderr)
}

// CodexExecutor provides a custom executor for Codex CLI.
// This can be used to capture and transform output.
type CodexExecutor struct{}

// Execute runs the codex command and captures output.
func (e *CodexExecutor) Execute(_ context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Compile-time check that CodexRunner implements Runner.
var _ Runner = (*CodexRunner)(nil)
