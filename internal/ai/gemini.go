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
	base   BaseRunner     // Embedded BaseRunner for timeout/retry handling
	logger zerolog.Logger // Logger for debug output
}

// GeminiRunnerOption is a functional option for configuring GeminiRunner.
type GeminiRunnerOption func(*GeminiRunner)

// WithGeminiLogger sets the logger for the GeminiRunner.
func WithGeminiLogger(logger zerolog.Logger) GeminiRunnerOption {
	return func(r *GeminiRunner) {
		r.logger = logger
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
		},
		logger: zerolog.Nop(), // Default to no-op logger
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// NewGeminiRunnerWithLogger creates a new GeminiRunner with logging support.
// Deprecated: Use NewGeminiRunner with WithGeminiLogger option instead.
func NewGeminiRunnerWithLogger(cfg *config.AIConfig, executor CommandExecutor, logger zerolog.Logger) *GeminiRunner {
	return NewGeminiRunner(cfg, executor, WithGeminiLogger(logger))
}

// Run executes an AI request using the Gemini CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Gemini-specific command execution.
func (r *GeminiRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
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

	// Parse the JSON response
	resp, parseErr := parseGeminiResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
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
