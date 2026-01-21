// Package ai provides AI execution capabilities for ATLAS.
//
// This package defines the AIRunner interface for executing AI operations
// and provides the ClaudeCodeRunner implementation for invoking Claude Code CLI.
//
// IMPORTANT: This package may import internal/constants, internal/errors,
// internal/config, and internal/domain. It MUST NOT import internal/task,
// internal/workspace, or internal/cli.
package ai

import (
	"context"

	"github.com/mrz1836/atlas/internal/domain"
)

// Runner defines the interface for AI execution (exported as ai.Runner).
// Implementations handle the actual invocation of AI systems (like Claude Code CLI)
// and return structured results.
//
// Architecture docs refer to this as "AIRunner" but Go conventions prefer
// non-stuttering names, so it's exported as ai.Runner.
//
// Context should be used to control timeouts and cancellation.
// The implementation should check ctx.Done() for long-running operations.
type Runner interface {
	// Run executes an AI request and returns the result.
	// The context controls timeout and cancellation.
	// Returns an error wrapped with errors.ErrClaudeInvocation on failure.
	Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}

// TerminatableRunner is implemented by runners that can terminate running processes.
// This is used to clean up AI subprocesses during Ctrl+C interruption to prevent
// orphaned processes from lingering after Atlas exits.
type TerminatableRunner interface {
	// TerminateRunningProcess terminates any currently running AI subprocess.
	// Returns nil if no process is running or if termination succeeds.
	TerminateRunningProcess() error
}
