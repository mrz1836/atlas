// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/git"
)

// ArtifactSaver abstracts artifact persistence for step executors.
// This interface matches task.Store methods, allowing step executors to
// save artifacts without direct dependency on the task package.
type ArtifactSaver interface {
	// SaveArtifact saves an artifact file for the task.
	// The filename can include subdirectories (e.g., "ci_wait/ci-result.json").
	SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error

	// SaveVersionedArtifact saves an artifact with version suffix (e.g., validation.1.json).
	// Returns the actual filename used.
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

	// ArtifactSaver is used to save step artifacts (validation, CI, git, SDD).
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

	// CIFailureHandler is used for handling CI failures.
	// If nil, CI failures return simple error without interactive options.
	CIFailureHandler CIFailureHandlerInterface

	// BaseBranch is the default base branch for PR creation.
	// Falls back to "main" if not specified.
	BaseBranch string

	// CIConfig contains CI polling and timeout configuration from project config.
	// If nil, CI executor will use default constant values.
	CIConfig *config.CIConfig

	// Validation command configuration from project config.
	// These commands override the defaults when running validation during task execution.
	FormatCommands    []string
	LintCommands      []string
	TestCommands      []string
	PreCommitCommands []string
}

// NewDefaultRegistry creates a registry with all built-in executors.
// Pass nil for optional dependencies that aren't available yet.
func NewDefaultRegistry(deps ExecutorDeps) *ExecutorRegistry {
	r := NewExecutorRegistry()

	// Register AI executor (requires AIRunner)
	if deps.AIRunner != nil {
		r.Register(NewAIExecutorWithWorkingDir(deps.AIRunner, deps.WorkDir, deps.ArtifactSaver, deps.Logger))
	}

	// Register validation executor with optional artifact saving, notifications, retry, and commands
	r.Register(NewValidationExecutorFull(deps.WorkDir, deps.ArtifactSaver, deps.Notifier, deps.RetryHandler, ValidationCommands{
		Format:    deps.FormatCommands,
		Lint:      deps.LintCommands,
		Test:      deps.TestCommands,
		PreCommit: deps.PreCommitCommands,
	}))

	// Register git executor with dependencies for commit, push, and PR creation
	gitExecutorOpts := []GitExecutorOption{
		WithGitLogger(deps.Logger),
	}
	if deps.ArtifactSaver != nil {
		gitExecutorOpts = append(gitExecutorOpts, WithGitArtifactSaver(deps.ArtifactSaver))
	}
	if deps.BaseBranch != "" {
		gitExecutorOpts = append(gitExecutorOpts, WithBaseBranch(deps.BaseBranch))
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
		r.Register(NewSDDExecutorWithArtifactSaver(deps.AIRunner, deps.ArtifactSaver, deps.WorkDir, deps.Logger))
	}

	// Register CI executor with HubRunner and CIFailureHandler dependencies
	ciExecutorOpts := []CIExecutorOption{
		WithCILogger(deps.Logger),
	}
	if deps.ArtifactSaver != nil {
		ciExecutorOpts = append(ciExecutorOpts, WithCIArtifactSaver(deps.ArtifactSaver))
	}
	if deps.HubRunner != nil {
		ciExecutorOpts = append(ciExecutorOpts, WithCIHubRunner(deps.HubRunner))
	}
	if deps.CIFailureHandler != nil {
		ciExecutorOpts = append(ciExecutorOpts, WithCIFailureHandlerInterface(deps.CIFailureHandler))
	}
	if deps.CIConfig != nil {
		ciExecutorOpts = append(ciExecutorOpts, WithCIConfig(deps.CIConfig))
	}
	r.Register(NewCIExecutor(ciExecutorOpts...))

	// Register verify executor (requires AIRunner for AI verification)
	if deps.AIRunner != nil {
		garbageDetector := git.NewGarbageDetector(nil)
		r.Register(NewVerifyExecutorWithWorkingDir(deps.AIRunner, garbageDetector, deps.ArtifactSaver, deps.Logger, deps.WorkDir))
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

// getIntFromAny extracts an int from various numeric types stored in interface{}.
// This handles JSON unmarshaling which may produce int, int64, or float64.
// Returns 0, false if the value is nil, not a number, or <= 0.
func getIntFromAny(val any) (int, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, v > 0
	case int64:
		return int(v), v > 0
	case float64:
		return int(v), v > 0
	default:
		return 0, false
	}
}
