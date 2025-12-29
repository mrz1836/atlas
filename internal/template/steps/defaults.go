// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"

	"github.com/mrz1836/atlas/internal/ai"
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
}

// NewDefaultRegistry creates a registry with all built-in executors.
// Pass nil for optional dependencies that aren't available yet.
func NewDefaultRegistry(deps ExecutorDeps) *ExecutorRegistry {
	r := NewExecutorRegistry()

	// Register AI executor (requires AIRunner)
	if deps.AIRunner != nil {
		r.Register(NewAIExecutor(deps.AIRunner))
	}

	// Register validation executor with optional artifact saving and notifications
	r.Register(NewValidationExecutorWithDeps(deps.WorkDir, deps.ArtifactSaver, deps.Notifier))

	// Register git executor (placeholder for Epic 6)
	r.Register(NewGitExecutor(deps.WorkDir))

	// Register human executor
	r.Register(NewHumanExecutor())

	// Register SDD executor (requires AIRunner)
	if deps.AIRunner != nil {
		r.Register(NewSDDExecutor(deps.AIRunner, deps.ArtifactsDir))
	}

	// Register CI executor (placeholder for Epic 6)
	r.Register(NewCIExecutor())

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
