// Package contracts defines the internal API contracts for the Hook system.
// These interfaces will be implemented in internal/hook/.
//
// This file is a design artifact - the actual interfaces will live in internal/hook/store.go.
package contracts

import (
	"context"
	"time"
)

// HookStore defines the persistence interface for Hook state.
// Implementation: internal/hook/store.go (FileStore)
//
// Design notes:
// - Follows patterns from task.Store and workspace.Store
// - Uses atomic file writes (temp + rename)
// - File locking with 5-second timeout
// - Creates HOOK.md alongside hook.json on every save
type HookStore interface {
	// Create initializes a new hook for a task.
	// Returns error if hook already exists.
	Create(ctx context.Context, taskID, workspaceID string) (*Hook, error)

	// Get retrieves a hook by task ID.
	// Returns nil, nil if hook does not exist.
	Get(ctx context.Context, taskID string) (*Hook, error)

	// Save persists the hook state atomically.
	// Also regenerates HOOK.md from the updated state.
	Save(ctx context.Context, hook *Hook) error

	// Delete removes the hook files (hook.json and HOOK.md).
	Delete(ctx context.Context, taskID string) error

	// Exists checks if a hook exists for the given task.
	Exists(ctx context.Context, taskID string) (bool, error)

	// ListStale returns all hooks that haven't been updated within threshold.
	// Used by cleanup and crash detection.
	ListStale(ctx context.Context, threshold time.Duration) ([]*Hook, error)
}

// Hook is a placeholder for domain.Hook in this design document.
// See data-model.md for the full definition.
type Hook struct{}

// HookTransitioner manages state transitions for hooks.
// Implementation: internal/hook/state.go
type HookTransitioner interface {
	// Transition moves the hook to a new state.
	// Records the transition in history and updates timestamps.
	// Returns error if transition is invalid.
	Transition(ctx context.Context, hook *Hook, to HookState, trigger string, details map[string]any) error

	// IsValidTransition checks if a state transition is allowed.
	IsValidTransition(from, to HookState) bool

	// IsTerminalState returns true for completed, failed, abandoned.
	IsTerminalState(state HookState) bool
}

// HookState is a placeholder for domain.HookState.
type HookState string

// Checkpointer manages checkpoint creation and retrieval.
// Implementation: internal/hook/checkpoint.go
type Checkpointer interface {
	// CreateCheckpoint creates a new checkpoint with the given trigger and description.
	// Automatically captures git state and file snapshots.
	// Prunes oldest checkpoints if limit (50) exceeded.
	CreateCheckpoint(ctx context.Context, hook *Hook, trigger CheckpointTrigger, description string) error

	// GetLatestCheckpoint returns the most recent checkpoint, or nil if none.
	GetLatestCheckpoint(hook *Hook) *StepCheckpoint

	// GetCheckpointByID returns a specific checkpoint, or nil if not found.
	GetCheckpointByID(hook *Hook, checkpointID string) *StepCheckpoint
}

// CheckpointTrigger is a placeholder for domain.CheckpointTrigger.
type CheckpointTrigger string

// StepCheckpoint is a placeholder for domain.StepCheckpoint.
type StepCheckpoint struct{}

// IntervalCheckpointer manages periodic checkpoints during long-running steps.
// Implementation: internal/hook/checkpoint.go
type IntervalCheckpointer interface {
	// Start begins periodic checkpoint creation.
	// Checkpoints are created at the configured interval (default: 5 min)
	// only when the hook is in step_running state.
	Start(ctx context.Context)

	// Stop cancels the interval checkpointer.
	Stop()
}

// RecoveryDetector identifies and diagnoses crash recovery scenarios.
// Implementation: internal/hook/recovery.go
type RecoveryDetector interface {
	// DetectRecoveryNeeded checks if a hook requires crash recovery.
	// Returns true if hook is stale (no update within threshold) and not terminal.
	DetectRecoveryNeeded(ctx context.Context, hook *Hook, threshold time.Duration) bool

	// DiagnoseAndRecommend analyzes the crash and populates RecoveryContext.
	// Sets recommended_action to: retry_step, retry_from_checkpoint, skip_step, or manual.
	DiagnoseAndRecommend(ctx context.Context, hook *Hook) error
}

// MarkdownGenerator creates human-readable HOOK.md from hook.json.
// Implementation: internal/hook/markdown.go
type MarkdownGenerator interface {
	// Generate creates the HOOK.md content from hook state.
	Generate(hook *Hook) ([]byte, error)
}

// ReceiptSigner handles cryptographic signing of validation receipts.
// Implementation: internal/hook/signer_hd.go
//
// See data-model.md for the authoritative interface definition.
type ReceiptSigner interface {
	// Sign signs a validation receipt and populates its Signature and KeyPath fields.
	// The taskIndex is used for key derivation (e.g., HD path component).
	// All receipts within a task share the same derived key.
	Sign(ctx context.Context, receipt *ValidationReceipt, taskIndex uint32) error

	// Verify checks that a receipt's signature is valid.
	// Returns nil if valid, error if invalid or verification fails.
	Verify(ctx context.Context, receipt *ValidationReceipt) error

	// KeyPath returns the derivation path used for a given task index.
	// Format depends on implementation (e.g., "m/44'/236'/0'/5/0" for HD).
	KeyPath(taskIndex uint32) string
}

// ValidationReceipt is a placeholder for domain.ValidationReceipt.
type ValidationReceipt struct{}

// KeyManager handles master key lifecycle (generation, loading, storage).
// Implementation: internal/crypto/hd/signer.go
//
// See data-model.md for the authoritative interface definition.
type KeyManager interface {
	// Load retrieves the master key, generating one if it doesn't exist.
	// Creates ~/.atlas/keys/master.key with 0600 permissions if not exists.
	// Returns error if key cannot be loaded or generated.
	Load(ctx context.Context) error

	// Exists checks if a master key is already configured.
	Exists() bool

	// NewSigner creates a ReceiptSigner using the loaded master key.
	// Must call Load() first.
	NewSigner() (ReceiptSigner, error)
}

// GitHookInstaller manages git hook wrappers for auto-checkpoints.
// Implementation: internal/git/hooks.go
type GitHookInstaller interface {
	// Install installs ATLAS checkpoint wrappers in the repository.
	// Uses wrapper approach that chains to existing hooks.
	Install(ctx context.Context, repoPath string) error

	// Uninstall removes ATLAS checkpoint wrappers.
	// Restores original hooks from .original backups.
	Uninstall(ctx context.Context, repoPath string) error

	// IsInstalled checks if ATLAS hooks are installed.
	IsInstalled(repoPath string) (bool, error)
}
