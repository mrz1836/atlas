// Package git provides Git operations for ATLAS.
// This file implements the HubRunner for GitHub operations via gh CLI.
//
// File organization:
//   - github.go: Core infrastructure (this file) - interfaces, struct, constructor
//   - github_pr.go: PR creation and status operations
//   - github_ci.go: CI monitoring and check watching
//   - github_modify.go: PR modifications (draft, merge, review, comment)
//   - github_utils.go: Shared utility functions
//   - github_errors.go: Error types and classification
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// errContinuePolling is a sentinel error used internally to signal that polling should continue.
var errContinuePolling = errors.New("continue polling")

// PRCreator handles pull request creation.
type PRCreator interface {
	// CreatePR creates a pull request and returns the result.
	CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error)
}

// PRStatusReader provides read-only PR status operations.
type PRStatusReader interface {
	// GetPRStatus gets the current status of a PR.
	GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)

	// WatchPRChecks monitors CI checks until completion or timeout.
	WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error)
}

// PRModifier handles PR state modifications.
type PRModifier interface {
	// ConvertToDraft converts an open PR to draft status.
	ConvertToDraft(ctx context.Context, prNumber int) error

	// MergePR merges a pull request using the specified merge method.
	// mergeMethod: "squash", "merge", or "rebase"
	// adminBypass: if true, attempts merge with admin privileges (bypasses branch protection)
	// deleteBranch: if true, deletes the source branch after successful merge
	MergePR(ctx context.Context, prNumber int, mergeMethod string, adminBypass, deleteBranch bool) error
}

// PRReviewer handles PR review operations.
type PRReviewer interface {
	// AddPRReview adds a review to a pull request.
	// event: "APPROVE", "REQUEST_CHANGES", or "COMMENT"
	AddPRReview(ctx context.Context, prNumber int, body, event string) error

	// AddPRComment adds a comment to a pull request.
	AddPRComment(ctx context.Context, prNumber int, body string) error
}

// HubRunner combines all GitHub operations.
// Deprecated: Prefer using specific interfaces (PRCreator, PRStatusReader, etc.)
// for better interface segregation. This composite interface is retained
// for backward compatibility.
type HubRunner interface {
	PRCreator
	PRStatusReader
	PRModifier
	PRReviewer
}

// Compile-time interface check.
var _ HubRunner = (*CLIGitHubRunner)(nil)

// CLIGitHubRunner implements HubRunner using the gh CLI.
type CLIGitHubRunner struct {
	workDir string
	logger  zerolog.Logger
	config  RetryConfig
	cmdExec CommandExecutor
}

// CommandExecutor executes shell commands. Used for testing.
type CommandExecutor interface {
	// Execute runs a command and returns its combined output.
	Execute(ctx context.Context, workDir, name string, args ...string) ([]byte, error)
}

// CLIGitHubRunnerOption configures a CLIGitHubRunner.
type CLIGitHubRunnerOption func(*CLIGitHubRunner)

// NewCLIGitHubRunner creates a CLIGitHubRunner with the given options.
func NewCLIGitHubRunner(workDir string, opts ...CLIGitHubRunnerOption) *CLIGitHubRunner {
	r := &CLIGitHubRunner{
		workDir: workDir,
		logger:  zerolog.Nop(),
		config:  DefaultRetryConfig(),
		cmdExec: &defaultCommandExecutor{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithGHLogger sets the logger for GitHub operations.
func WithGHLogger(logger zerolog.Logger) CLIGitHubRunnerOption {
	return func(r *CLIGitHubRunner) {
		r.logger = logger
	}
}

// WithGHRetryConfig sets custom retry configuration.
func WithGHRetryConfig(config RetryConfig) CLIGitHubRunnerOption {
	return func(r *CLIGitHubRunner) {
		r.config = config
	}
}

// WithGHCommandExecutor sets a custom command executor (for testing).
func WithGHCommandExecutor(exec CommandExecutor) CLIGitHubRunnerOption {
	return func(r *CLIGitHubRunner) {
		r.cmdExec = exec
	}
}

// defaultCommandExecutor is the default implementation using exec.Command.
// This struct and runGHCommand have 0% unit test coverage by design.
// Unit tests mock the CommandExecutor interface to avoid external dependencies.
// Integration tests (with //go:build integration tag) should cover these paths.
type defaultCommandExecutor struct{}

// Execute runs a command using the standard exec package.
func (e *defaultCommandExecutor) Execute(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
	return runGHCommand(ctx, workDir, name, args...)
}

// runGHCommand executes a gh CLI command and returns its output as bytes.
func runGHCommand(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- args are validated
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Include stderr in error for debugging
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s failed [%s]: %w", name, strings.TrimSpace(stderr.String()), atlaserrors.ErrGitHubOperation)
		}
		return nil, fmt.Errorf("%s failed: %w", name, atlaserrors.ErrGitHubOperation)
	}

	return stdout.Bytes(), nil
}

// emitBellIfEnabled emits a terminal bell if enabled and not already emitted.
func (r *CLIGitHubRunner) emitBellIfEnabled(enabled bool, emitted *bool) {
	if enabled && !*emitted {
		_, _ = os.Stdout.Write([]byte("\a")) // BEL character
		*emitted = true
	}
}
