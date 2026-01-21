package ai

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mrz1836/atlas/internal/config"
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
	base            BaseRunner       // Embedded BaseRunner for timeout/retry handling
	activityOptions *ActivityOptions // Activity streaming options
}

// CodexRunnerOption is a functional option for configuring CodexRunner.
type CodexRunnerOption func(*CodexRunner)

// WithCodexActivityCallback configures the activity callback for streaming activity events.
func WithCodexActivityCallback(opts ActivityOptions) CodexRunnerOption {
	return func(r *CodexRunner) {
		r.activityOptions = &opts
	}
}

// NewCodexRunner creates a new CodexRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewCodexRunner(cfg *config.AIConfig, executor CommandExecutor, opts ...CodexRunnerOption) *CodexRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	r := &CodexRunner{
		base: BaseRunner{
			Config:   cfg,
			Executor: executor,
			ErrType:  atlaserrors.ErrCodexInvocation,
		},
	}
	for _, opt := range opts {
		opt(r)
	}

	// If activity streaming is enabled, swap executor for StreamingExecutor
	if r.activityOptions != nil && r.activityOptions.Callback != nil {
		r.base.Executor = NewStreamingExecutor(*r.activityOptions)
	}

	return r
}

// Run executes an AI request using the Codex CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Codex-specific command execution.
func (r *CodexRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
}

// TerminateRunningProcess terminates any currently running AI subprocess.
// This implements the TerminatableRunner interface for cleanup on Ctrl+C.
func (r *CodexRunner) TerminateRunningProcess() error {
	return r.base.TerminateRunningProcess()
}

// execute performs a single AI request execution.
func (r *CodexRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
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

	// Parse the JSON response
	resp, parseErr := parseCodexResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
}

// handleExecutionError processes errors from command execution.
func (r *CodexRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	return r.base.HandleProviderExecutionError(ctx, codexCLIInfo, err, stderr,
		func() (*domain.AIResult, bool) {
			return r.tryParseErrorResponse(err, stdout, stderr)
		},
	)
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
	if model == "" && r.base.Config != nil {
		model = r.base.Config.Model
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

// Compile-time check that CodexRunner implements Runner.
var _ Runner = (*CodexRunner)(nil)

// Compile-time check that CodexRunner implements TerminatableRunner.
var _ TerminatableRunner = (*CodexRunner)(nil)
