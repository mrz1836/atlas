package ai

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// GeminiRunner implements Runner for Gemini CLI invocation.
// It builds command-line arguments and executes the gemini CLI,
// parsing the JSON response into an AIResult.
type GeminiRunner struct {
	config   *config.AIConfig
	executor CommandExecutor
	logger   zerolog.Logger
}

// NewGeminiRunner creates a new GeminiRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewGeminiRunner(cfg *config.AIConfig, executor CommandExecutor) *GeminiRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &GeminiRunner{
		config:   cfg,
		executor: executor,
		logger:   zerolog.Nop(), // Default to no-op logger
	}
}

// NewGeminiRunnerWithLogger creates a new GeminiRunner with logging support.
func NewGeminiRunnerWithLogger(cfg *config.AIConfig, executor CommandExecutor, logger zerolog.Logger) *GeminiRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &GeminiRunner{
		config:   cfg,
		executor: executor,
		logger:   logger,
	}
}

// Run executes an AI request using the Gemini CLI.
// This method builds the command, executes it, and parses the JSON response.
func (r *GeminiRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
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
func (r *GeminiRunner) runWithRetry(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
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
			case <-r.sleepChan(backoff):
				backoff *= 2 // Exponential backoff
			}
		}
	}

	return nil, fmt.Errorf("%w: max retries exceeded: %s", atlaserrors.ErrGeminiInvocation, lastErr.Error())
}

// sleepChan returns a channel that receives after the duration.
// This is a method to allow overriding in tests.
func (r *GeminiRunner) sleepChan(d interface{ Nanoseconds() int64 }) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		<-timeSleep(d)
		close(ch)
	}()
	return ch
}

// execute performs a single AI request execution.
func (r *GeminiRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Build the command (prompt is passed as positional argument)
	cmd := r.buildCommand(ctx, req)

	// Log the CLI command being executed (debug level for verbose mode)
	r.logger.Debug().
		Str("cli", "gemini").
		Strs("args", cmd.Args[1:]).
		Str("working_dir", cmd.Dir).
		Int("prompt_length", len(req.Prompt)).
		Msg("executing gemini CLI")

	// Execute the command
	stdout, stderr, err := r.executor.Execute(ctx, cmd)
	if err != nil {
		return r.handleExecutionError(ctx, err, stdout, stderr)
	}

	// Parse the JSON response
	resp, parseErr := parseGeminiResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
}

// handleExecutionError processes errors from command execution.
func (r *GeminiRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	// Check if context was canceled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Try to parse response even on error (may have valid JSON with error info)
	result, handled := r.tryParseErrorResponse(err, stdout, stderr)
	if handled {
		return result, nil
	}

	return nil, wrapGeminiExecutionError(err, stderr)
}

// tryParseErrorResponse attempts to extract error information from a JSON response.
// Returns the result and true if the error was successfully parsed, otherwise nil and false.
func (r *GeminiRunner) tryParseErrorResponse(execErr error, stdout, stderr []byte) (*domain.AIResult, bool) {
	if len(stdout) == 0 {
		return nil, false
	}

	resp, parseErr := parseGeminiResponse(stdout)
	if parseErr != nil || resp.Success {
		return nil, false
	}

	result := resp.toAIResult(string(stderr))

	// Preserve parsed error if available, enhance with execution context
	if result.Error != "" {
		result.Error = fmt.Sprintf("%s (exit: %s)", result.Error, execErr.Error())
	} else {
		// Fall back to execution error + stderr if no parsed error
		stderrStr := strings.TrimSpace(string(stderr))
		if stderrStr != "" {
			result.Error = fmt.Sprintf("%s: %s", execErr.Error(), stderrStr)
		} else {
			result.Error = execErr.Error()
		}
	}
	return result, true
}

// buildCommand constructs the gemini CLI command with appropriate flags.
func (r *GeminiRunner) buildCommand(ctx context.Context, req *domain.AIRequest) *exec.Cmd {
	args := []string{
		"--output-format", "json", // JSON output format
	}

	// Always use --yolo for non-interactive execution (auto-approve allowed actions)
	args = append(args, "--yolo")

	// Add --sandbox for read-only mode (restricts WHAT can be done)
	if req.PermissionMode == "plan" {
		args = append(args, "--sandbox")
	}

	// Determine model: request > config
	model := req.Model
	if model == "" && r.config != nil {
		model = r.config.Model
	}

	// Resolve model alias to full model name
	if model != "" {
		model = domain.AgentGemini.ResolveModelAlias(model)
		args = append(args, "-m", model)
	}

	// Gemini CLI may not support all the same flags as Claude CLI.
	// Add additional flags here as Gemini CLI supports them.

	// Add prompt as positional argument (required for one-shot mode)
	// The -p/--prompt flag is deprecated in favor of positional arguments
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, "gemini", args...)

	// Set working directory if specified
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	return cmd
}

// wrapGeminiExecutionError wraps an execution error with context.
func wrapGeminiExecutionError(err error, stderr []byte) error {
	stderrStr := strings.TrimSpace(string(stderr))

	// Check for specific error conditions
	if strings.Contains(stderrStr, "command not found") ||
		strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("%w: gemini CLI not found - install with: npm install -g @google/gemini-cli", atlaserrors.ErrGeminiInvocation)
	}

	if strings.Contains(stderrStr, "api key") || strings.Contains(stderrStr, "API key") ||
		strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "GEMINI_API_KEY") {
		return fmt.Errorf("%w: API key error: %s", atlaserrors.ErrGeminiInvocation, stderrStr)
	}

	if stderrStr != "" {
		return fmt.Errorf("%w: %s", atlaserrors.ErrGeminiInvocation, stderrStr)
	}

	return fmt.Errorf("%w: %s", atlaserrors.ErrGeminiInvocation, err.Error())
}

// GeminiExecutor provides a custom executor for Gemini CLI.
// This can be used to capture and transform output.
type GeminiExecutor struct{}

// Execute runs the gemini command and captures output.
func (e *GeminiExecutor) Execute(_ context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Compile-time check that GeminiRunner implements Runner.
var _ Runner = (*GeminiRunner)(nil)
