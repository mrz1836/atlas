package ai

import (
	"context"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// antigravityCLIInfo contains Antigravity-specific CLI metadata for error messages.
//
// Antigravity authenticates via Google sign-in (OAuth) rather than an API key,
// so EnvVar is intentionally empty.
//
//nolint:gochecknoglobals // Constant-like structure
var antigravityCLIInfo = CLIInfo{
	Name:          "agy",
	InstallHint:   "install with: curl -fsSL https://antigravity.google/cli/install.sh | bash (then run 'agy' once to sign in)",
	ErrType:       atlaserrors.ErrAntigravityInvocation,
	EnvVar:        "", // OAuth-based; no API key env var
	StatusPageURL: "https://status.cloud.google.com",
}

// AntigravityRunner implements Runner for the Antigravity CLI (agy).
// The agy CLI shares Claude Code's command-line surface (print mode, --model,
// prompt via stdin) but emits its own JSON schema, which is parsed with
// AntigravityResponse.
//
// Live activity streaming is not implemented: agy's stream-json event schema is
// undocumented, so the runner always uses single-object --output-format json.
type AntigravityRunner struct {
	base   BaseRunner     // Embedded BaseRunner for timeout/retry handling
	logger zerolog.Logger // Logger for debug output
}

// AntigravityRunnerOption is a functional option for configuring AntigravityRunner.
type AntigravityRunnerOption func(*AntigravityRunner)

// WithAntigravityLogger sets the logger for the AntigravityRunner.
func WithAntigravityLogger(logger zerolog.Logger) AntigravityRunnerOption {
	return func(r *AntigravityRunner) {
		r.logger = logger
	}
}

// NewAntigravityRunner creates a new AntigravityRunner with the given configuration.
// If executor is nil, a DefaultExecutor is used for production subprocess execution.
func NewAntigravityRunner(cfg *config.AIConfig, executor CommandExecutor, opts ...AntigravityRunnerOption) *AntigravityRunner {
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	r := &AntigravityRunner{
		base: BaseRunner{
			Config:        cfg,
			Executor:      executor,
			ErrType:       atlaserrors.ErrAntigravityInvocation,
			Logger:        zerolog.Nop(), // Will be updated if WithAntigravityLogger is used
			ProviderName:  antigravityCLIInfo.Name,
			StatusPageURL: antigravityCLIInfo.StatusPageURL,
		},
		logger: zerolog.Nop(), // Default to no-op logger
	}
	for _, opt := range opts {
		opt(r)
	}

	// Sync BaseRunner logger with AntigravityRunner logger
	r.base.Logger = r.logger

	return r
}

// Run executes an AI request using the Antigravity CLI.
// This method delegates to BaseRunner for timeout and retry handling,
// providing the execute function for Antigravity-specific command execution.
func (r *AntigravityRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	return r.base.RunWithTimeout(ctx, req, r.execute)
}

// TerminateRunningProcess terminates any currently running AI subprocess.
// This implements the TerminatableRunner interface for cleanup on Ctrl+C.
func (r *AntigravityRunner) TerminateRunningProcess() error {
	return r.base.TerminateRunningProcess()
}

// execute performs a single AI request execution.
func (r *AntigravityRunner) execute(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	// Pre-flight check: verify working directory exists
	if err := r.base.ValidateWorkingDir(req.WorkingDir); err != nil {
		return nil, err
	}

	// Build the command
	cmd := r.buildCommand(ctx, req)

	// Pass prompt via stdin (matches Claude Code print-mode behavior)
	cmd.Stdin = strings.NewReader(req.Prompt)

	r.logger.Debug().
		Str("cli", "agy").
		Strs("args", cmd.Args[1:]).
		Str("working_dir", cmd.Dir).
		Int("prompt_length", len(req.Prompt)).
		Msg("executing antigravity CLI")

	// Execute the command
	stdout, stderr, err := r.base.Executor.Execute(ctx, cmd)
	if err != nil {
		return r.handleExecutionError(ctx, err, stdout, stderr)
	}

	// Parse the response
	resp, parseErr := parseAntigravityResponse(stdout)
	if parseErr != nil {
		return nil, parseErr
	}

	return resp.toAIResult(string(stderr)), nil
}

// handleExecutionError processes errors from command execution.
func (r *AntigravityRunner) handleExecutionError(ctx context.Context, err error, stdout, stderr []byte) (*domain.AIResult, error) {
	return r.base.HandleProviderExecutionError(
		ctx, antigravityCLIInfo, err, stderr,
		func() (*domain.AIResult, bool) {
			return r.tryParseErrorResponse(err, stdout, stderr)
		},
	)
}

// tryParseErrorResponse attempts to extract error information from a JSON response.
// Returns the result and true if the error was successfully parsed, otherwise nil and false.
func (r *AntigravityRunner) tryParseErrorResponse(execErr error, stdout, stderr []byte) (*domain.AIResult, bool) {
	if len(stdout) == 0 {
		return nil, false
	}

	resp, parseErr := parseAntigravityResponse(stdout)
	if parseErr != nil || resp.isSuccess() {
		return nil, false
	}

	result := resp.toAIResult(string(stderr))
	if result.Error == "" {
		result.Error = execErr.Error()
	}
	return result, true
}

// buildCommand constructs the agy CLI command with appropriate flags.
func (r *AntigravityRunner) buildCommand(ctx context.Context, req *domain.AIRequest) *exec.Cmd {
	args := []string{
		"-p", // Print mode (non-interactive)
		"--output-format", "json",
		// Non-interactive print mode must not expand slash commands or skills:
		// otherwise agy shells out (e.g. to `agy --help`) and the run stalls on a
		// permission prompt that has no interactive user to answer, failing the run.
		"--disable-slash-commands",
	}

	// Permission handling. agy uses --mode (plan|accept-edits) rather than
	// Claude's --permission-mode. Verification runs in plan mode (read-only: no
	// edits); implementation uses the default edit mode. Either way we auto-approve
	// tool requests (analogous to gemini's --yolo) so the non-interactive run does
	// not block waiting for approval. In plan mode this only auto-approves
	// read-only operations, because plan mode still forbids edits.
	if req.PermissionMode == "plan" {
		args = append(args, "--mode", "plan")
		r.logger.Debug().
			Str("permission_mode", req.PermissionMode).
			Msg("agy running in plan mode (read-only verification)")
	}
	args = append(args, "--dangerously-skip-permissions")

	// Determine model: request > config
	model := req.Model
	if model == "" && r.base.Config != nil {
		model = r.base.Config.Model
	}

	// Resolve short aliases (e.g. "pro", "flash") to full agy model IDs.
	if model != "" {
		model = domain.AgentAntigravity.ResolveModelAlias(model)
		args = append(args, "--model", model)
	}

	cmd := exec.CommandContext(ctx, "agy", args...)

	// Set working directory if specified
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	return cmd
}

// Compile-time check that AntigravityRunner implements Runner.
var _ Runner = (*AntigravityRunner)(nil)

// Compile-time check that AntigravityRunner implements TerminatableRunner.
var _ TerminatableRunner = (*AntigravityRunner)(nil)
