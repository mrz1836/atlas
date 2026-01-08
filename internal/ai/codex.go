package ai

import (
	"bytes"
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
	base BaseRunner // Embedded BaseRunner for timeout/retry handling
}

// NewCodexRunner creates a new CodexRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewCodexRunner(cfg *config.AIConfig, executor CommandExecutor) *CodexRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &CodexRunner{
		base: BaseRunner{
			Config:   cfg,
			Executor: executor,
			ErrType:  atlaserrors.ErrCodexInvocation,
		},
	}
}

// Run executes an AI request using the Codex CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Codex-specific command execution.
func (r *CodexRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
}

// execute performs a single AI request execution.
func (r *CodexRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
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
	return r.base.HandleExecutionError(ctx, err,
		func() (*domain.AIResult, bool) {
			return r.tryParseErrorResponse(err, stdout, stderr)
		},
		func(e error) error {
			return wrapCodexExecutionError(e, stderr)
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
