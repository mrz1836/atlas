package ai

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// geminiCLIInfo contains Gemini-specific CLI metadata for error messages.
//
//nolint:gochecknoglobals // Constant-like structure
var geminiCLIInfo = CLIInfo{
	Name:        "gemini",
	InstallHint: "install with: npm install -g @google/gemini-cli",
	ErrType:     atlaserrors.ErrGeminiInvocation,
	EnvVar:      "GEMINI_API_KEY",
}

// GeminiRunner implements Runner for Gemini CLI invocation.
// It builds command-line arguments and executes the gemini CLI,
// parsing the JSON response into an AIResult.
type GeminiRunner struct {
	base            BaseRunner       // Embedded BaseRunner for timeout/retry handling
	logger          zerolog.Logger   // Logger for debug output
	activityOptions *ActivityOptions // Activity streaming options
}

// GeminiRunnerOption is a functional option for configuring GeminiRunner.
type GeminiRunnerOption func(*GeminiRunner)

// WithGeminiLogger sets the logger for the GeminiRunner.
func WithGeminiLogger(logger zerolog.Logger) GeminiRunnerOption {
	return func(r *GeminiRunner) {
		r.logger = logger
	}
}

// WithGeminiActivityCallback configures the activity callback for streaming activity events.
func WithGeminiActivityCallback(opts ActivityOptions) GeminiRunnerOption {
	return func(r *GeminiRunner) {
		r.activityOptions = &opts
	}
}

// NewGeminiRunner creates a new GeminiRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewGeminiRunner(cfg *config.AIConfig, executor CommandExecutor, opts ...GeminiRunnerOption) *GeminiRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	r := &GeminiRunner{
		base: BaseRunner{
			Config:   cfg,
			Executor: executor,
			ErrType:  atlaserrors.ErrGeminiInvocation,
			Logger:   zerolog.Nop(), // Will be updated if WithGeminiLogger is used
		},
		logger: zerolog.Nop(), // Default to no-op logger
	}
	for _, opt := range opts {
		opt(r)
	}

	// Sync BaseRunner logger with GeminiRunner logger
	r.base.Logger = r.logger

	// If activity streaming is enabled, swap executor for StreamingExecutor
	if r.activityOptions != nil && r.activityOptions.Callback != nil {
		r.base.Executor = NewStreamingExecutor(*r.activityOptions, WithStreamProvider(StreamProviderGemini))
	}

	return r
}

// Run executes an AI request using the Gemini CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Gemini-specific command execution.
func (r *GeminiRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
}

// TerminateRunningProcess terminates any currently running AI subprocess.
// This implements the TerminatableRunner interface for cleanup on Ctrl+C.
func (r *GeminiRunner) TerminateRunningProcess() error {
	return r.base.TerminateRunningProcess()
}

// execute performs a single AI request execution.
func (r *GeminiRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Pre-flight check: verify working directory exists
	if err := r.base.ValidateWorkingDir(req.WorkingDir); err != nil {
		return nil, err
	}

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

// isStreamingEnabled returns true if activity streaming is enabled.
func (r *GeminiRunner) isStreamingEnabled() bool {
	return r.activityOptions != nil && r.activityOptions.Callback != nil
}

// parseResponse parses the Gemini CLI response.
// When streaming is enabled, it prefers the result from StreamingExecutor.
// Otherwise, it parses stdout as a single JSON object.
func (r *GeminiRunner) parseResponse(stdout []byte) (*GeminiResponse, error) {
	// If streaming is enabled, try to get result from StreamingExecutor first
	if r.isStreamingEnabled() {
		if streamExec, ok := r.base.Executor.(*StreamingExecutor); ok {
			if result := streamExec.LastGeminiResult(); result != nil {
				return r.streamResultToGeminiResponse(result), nil
			}
		}
	}

	// Fall back to parsing stdout as JSON (for non-streaming or if no result in stream)
	return parseGeminiResponse(stdout)
}

// streamResultToGeminiResponse converts a GeminiStreamResult to a GeminiResponse.
func (r *GeminiRunner) streamResultToGeminiResponse(result *GeminiStreamResult) *GeminiResponse {
	return &GeminiResponse{
		Success:    result.Success,
		Response:   result.ResponseText,
		SessionID:  result.SessionID,
		DurationMs: result.DurationMs,
		// NumTurns and TotalCostUSD are not available in stream-json result
	}
}

// handleExecutionError processes errors from command execution.
func (r *GeminiRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	return r.base.HandleProviderExecutionError(ctx, geminiCLIInfo, err, stderr,
		func() (*domain.AIResult, bool) {
			return r.tryParseErrorResponse(err, stdout, stderr)
		},
	)
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
	// Use stream-json format when activity streaming is enabled for real-time tool events
	outputFormat := "json"
	if r.isStreamingEnabled() {
		outputFormat = "stream-json"
	}

	args := []string{
		"--output-format", outputFormat,
	}

	// For verification (plan mode), use --sandbox --yolo
	// --sandbox restricts to read-only/safe operations
	// --yolo auto-approves tool use for non-interactive execution
	// For implementation, use --yolo for non-interactive execution (auto-approve allowed actions)
	if req.PermissionMode == "plan" {
		args = append(args, "--sandbox", "--yolo")
		r.logger.Debug().
			Str("permission_mode", req.PermissionMode).
			Msg("gemini running in sandbox mode with --yolo (read-only verification)")
	} else {
		args = append(args, "--yolo")
	}

	// Determine model: request > config
	model := req.Model
	if model == "" && r.base.Config != nil {
		model = r.base.Config.Model
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

// Compile-time check that GeminiRunner implements Runner.
var _ Runner = (*GeminiRunner)(nil)

// Compile-time check that GeminiRunner implements TerminatableRunner.
var _ TerminatableRunner = (*GeminiRunner)(nil)
