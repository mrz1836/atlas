// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/git"
)

// ArtifactSaver abstracts artifact persistence for validation results.
// This interface matches task.Store.SaveVersionedArtifact and
// validation.ArtifactSaver, allowing the validation step executor to
// save artifacts without direct dependency on the task package.
type ArtifactSaver interface {
	SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
}

// Notifier abstracts user notifications.
// This interface matches tui.Notifier, allowing the validation step
// executor to emit notifications without direct dependency on tui.
type Notifier interface {
	Bell()
}

// RetryHandler abstracts retry operations for validation.
// This interface matches validation.RetryHandler, allowing the validation
// step executor to perform AI-assisted retries without direct dependency
// on the validation package's concrete type.
type RetryHandler interface {
	CanRetry(attemptNum int) bool
	MaxAttempts() int
	IsEnabled() bool
}

// ExecutorDeps holds dependencies for creating executors.
// Use this to inject dependencies when creating the default registry.
type ExecutorDeps struct {
	// AIRunner is the AI execution interface for AI and SDD steps.
	AIRunner ai.Runner

	// WorkDir is the working directory for validation and git commands.
	WorkDir string

	// ArtifactsDir is where SDD artifacts are saved.
	ArtifactsDir string

	// ArtifactSaver is used to save validation result artifacts.
	// If nil, artifact saving is skipped.
	ArtifactSaver ArtifactSaver

	// Notifier is used for user notifications (e.g., terminal bell).
	// If nil, notifications are skipped.
	Notifier Notifier

	// RetryHandler is used for AI-assisted validation retry.
	// If nil, retry capability is not available.
	RetryHandler RetryHandler

	// Logger is used for structured logging.
	// If nil, a no-op logger is used.
	Logger zerolog.Logger

	// SmartCommitter is used for intelligent commit operations.
	// If nil, commit operations will fail with a configuration error.
	SmartCommitter git.SmartCommitService

	// Pusher is used for push operations.
	// If nil, push operations will fail with a configuration error.
	Pusher git.PushService

	// HubRunner is used for GitHub operations (PR creation).
	// If nil, GitHub operations will fail with a configuration error.
	HubRunner git.HubRunner

	// PRDescriptionGenerator generates PR descriptions.
	// If nil, PR operations will fail with a configuration error.
	PRDescriptionGenerator git.PRDescriptionGenerator

	// GitRunner is used for basic git operations.
	// If nil, some git operations may fail.
	GitRunner git.Runner
}

// NewDefaultRegistry creates a registry with all built-in executors.
// Pass nil for optional dependencies that aren't available yet.
func NewDefaultRegistry(deps ExecutorDeps) *ExecutorRegistry {
	r := NewExecutorRegistry()

	// Register AI executor (requires AIRunner)
	if deps.AIRunner != nil {
		r.Register(NewAIExecutor(deps.AIRunner))
	}

	// Register validation executor with optional artifact saving, notifications, and retry
	r.Register(NewValidationExecutorWithDeps(deps.WorkDir, deps.ArtifactSaver, deps.Notifier, deps.RetryHandler))

	// Register git executor with dependencies for commit, push, and PR creation
	gitExecutorOpts := []GitExecutorOption{
		WithGitLogger(deps.Logger),
		WithArtifactsDir(deps.ArtifactsDir),
	}
	if deps.SmartCommitter != nil {
		gitExecutorOpts = append(gitExecutorOpts, WithSmartCommitter(deps.SmartCommitter))
	}
	if deps.Pusher != nil {
		gitExecutorOpts = append(gitExecutorOpts, WithPusher(deps.Pusher))
	}
	if deps.HubRunner != nil {
		gitExecutorOpts = append(gitExecutorOpts, WithHubRunner(deps.HubRunner))
	}
	if deps.PRDescriptionGenerator != nil {
		gitExecutorOpts = append(gitExecutorOpts, WithPRDescriptionGenerator(deps.PRDescriptionGenerator))
	}
	if deps.GitRunner != nil {
		gitExecutorOpts = append(gitExecutorOpts, WithGitRunner(deps.GitRunner))
	}
	r.Register(NewGitExecutor(deps.WorkDir, gitExecutorOpts...))

	// Register human executor
	r.Register(NewHumanExecutor())

	// Register SDD executor (requires AIRunner)
	if deps.AIRunner != nil {
		r.Register(NewSDDExecutor(deps.AIRunner, deps.ArtifactsDir))
	}

	// Register CI executor (placeholder for Epic 6)
	r.Register(NewCIExecutor())

	// Register verify executor (requires AIRunner for AI verification)
	if deps.AIRunner != nil {
		garbageDetector := git.NewGarbageDetector(nil)
		r.Register(NewVerifyExecutor(deps.AIRunner, garbageDetector, deps.Logger))
	}

	return r
}

// NewMinimalRegistry creates a registry with only non-AI executors.
// This is useful for testing or when AI is not available.
func NewMinimalRegistry(workDir string) *ExecutorRegistry {
	r := NewExecutorRegistry()

	r.Register(NewValidationExecutor(workDir))
	r.Register(NewGitExecutor(workDir))
	r.Register(NewHumanExecutor())
	r.Register(NewCIExecutor())

	return r
}
