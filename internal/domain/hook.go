// Package domain contains core domain types for ATLAS.
// This file defines types for the hook system (crash recovery & context persistence).
//
// Import rules per existing conventions:
// - CAN import: internal/constants, internal/errors, standard library
// - MUST NOT import: any other internal packages
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// HookState represents the state machine position for crash recovery.
type HookState string

const (
	// HookStateInitializing indicates task setup is in progress.
	HookStateInitializing HookState = "initializing"

	// HookStateStepPending indicates ready to start the next step.
	HookStateStepPending HookState = "step_pending"

	// HookStateStepRunning indicates AI is executing a step.
	HookStateStepRunning HookState = "step_running"

	// HookStateStepValidating indicates validation is in progress.
	HookStateStepValidating HookState = "step_validating"

	// HookStateAwaitingHuman indicates blocked on human action.
	HookStateAwaitingHuman HookState = "awaiting_human"

	// HookStateRecovering indicates crash recovery is in progress.
	HookStateRecovering HookState = "recovering"

	// HookStateCompleted indicates task finished successfully.
	HookStateCompleted HookState = "completed"

	// HookStateFailed indicates task failed permanently.
	HookStateFailed HookState = "failed"

	// HookStateAbandoned indicates human abandoned the task.
	HookStateAbandoned HookState = "abandoned"
)

// GetValidTransitions returns the allowed state transitions.
// Empty string represents the initial state (no previous state).
func GetValidTransitions() map[HookState][]HookState {
	return map[HookState][]HookState{
		"":                      {HookStateInitializing},
		HookStateInitializing:   {HookStateStepPending, HookStateFailed},
		HookStateStepPending:    {HookStateStepRunning, HookStateCompleted, HookStateAbandoned},
		HookStateStepRunning:    {HookStateStepValidating, HookStateStepPending, HookStateAwaitingHuman, HookStateFailed, HookStateAbandoned},
		HookStateStepValidating: {HookStateStepPending, HookStateAwaitingHuman, HookStateFailed},
		HookStateAwaitingHuman:  {HookStateStepPending, HookStateStepRunning, HookStateAbandoned},
		HookStateRecovering:     {HookStateStepPending, HookStateStepRunning, HookStateAwaitingHuman, HookStateFailed},
	}
}

// IsTerminalState returns true if the state is a terminal state (no outgoing transitions).
func IsTerminalState(state HookState) bool {
	return state == HookStateCompleted || state == HookStateFailed || state == HookStateAbandoned
}

var (
	// ErrTerminalStateTransition is returned when attempting to transition from a terminal state.
	ErrTerminalStateTransition = errors.New("cannot transition from terminal state")

	// ErrInvalidStateTransition is returned when attempting an invalid state transition.
	ErrInvalidStateTransition = errors.New("invalid state transition")
)

// ValidateTransition checks if a state transition is allowed.
// Returns nil if valid, error describing why transition is invalid otherwise.
func ValidateTransition(from, to HookState) error {
	// Terminal states cannot transition
	if IsTerminalState(from) {
		return fmt.Errorf("%w from %q to %q", ErrTerminalStateTransition, from, to)
	}

	validTargets := GetValidTransitions()[from]
	for _, valid := range validTargets {
		if valid == to {
			return nil
		}
	}

	return fmt.Errorf("%w from %q to %q", ErrInvalidStateTransition, from, to)
}

// CheckpointTrigger indicates what caused a checkpoint.
type CheckpointTrigger string

const (
	// CheckpointTriggerManual is created via `atlas checkpoint "desc"`.
	CheckpointTriggerManual CheckpointTrigger = "manual"

	// CheckpointTriggerCommit is auto-created after git commit.
	CheckpointTriggerCommit CheckpointTrigger = "git_commit"

	// CheckpointTriggerPush is auto-created after git push.
	CheckpointTriggerPush CheckpointTrigger = "git_push"

	// CheckpointTriggerPR is auto-created after PR creation.
	CheckpointTriggerPR CheckpointTrigger = "pr_created"

	// CheckpointTriggerValidation is auto-created after validation passes.
	CheckpointTriggerValidation CheckpointTrigger = "validation"

	// CheckpointTriggerStepComplete is auto-created when step completes.
	CheckpointTriggerStepComplete CheckpointTrigger = "step_complete"

	// CheckpointTriggerInterval is auto-created periodically (default: 5 min).
	CheckpointTriggerInterval CheckpointTrigger = "interval"
)

// FileSnapshot captures a file's state at checkpoint time.
type FileSnapshot struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"` // RFC3339 format
	SHA256  string `json:"sha256"`   // First 16 chars of hash
	Exists  bool   `json:"exists"`
}

// StepCheckpoint captures progress within a step for recovery.
type StepCheckpoint struct {
	CheckpointID string            `json:"checkpoint_id"` // Unique ID: ckpt-{uuid8}
	CreatedAt    time.Time         `json:"created_at"`
	StepName     string            `json:"step_name"` // Which step this belongs to
	StepIndex    int               `json:"step_index"`
	Description  string            `json:"description"` // What was accomplished
	Trigger      CheckpointTrigger `json:"trigger"`     // What caused this checkpoint

	// Version control state at checkpoint
	GitBranch string `json:"git_branch"`
	GitCommit string `json:"git_commit,omitempty"` // SHA if committed
	GitDirty  bool   `json:"git_dirty"`            // Uncommitted changes?

	// Artifact references
	Artifacts []string `json:"artifacts,omitempty"` // Paths in artifacts/

	// File state for debugging
	FilesSnapshot []FileSnapshot `json:"files_snapshot,omitempty"`
}

// StepContext captures everything needed to resume the current step.
type StepContext struct {
	StepName    string    `json:"step_name"`    // e.g., "analyze", "implement"
	StepIndex   int       `json:"step_index"`   // Zero-based position in template
	StartedAt   time.Time `json:"started_at"`   // When step began
	Attempt     int       `json:"attempt"`      // Current attempt number (1-based)
	MaxAttempts int       `json:"max_attempts"` // Configured retry limit

	// AI Context (what the AI was working on)
	WorkingOn    string   `json:"working_on,omitempty"`    // Brief description
	FilesTouched []string `json:"files_touched,omitempty"` // Files modified this step
	LastOutput   string   `json:"last_output,omitempty"`   // Last AI response (truncated to 500 chars)

	// Current checkpoint reference
	CurrentCheckpointID string `json:"current_checkpoint_id,omitempty"`
}

// HookEvent records a state transition in the hook history.
type HookEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	FromState HookState      `json:"from_state"`
	ToState   HookState      `json:"to_state"`
	Trigger   string         `json:"trigger"`             // What caused transition
	StepName  string         `json:"step_name,omitempty"` // If step-related
	Details   map[string]any `json:"details,omitempty"`   // Additional context
}

// RecoveryContext is populated when crash recovery is needed.
type RecoveryContext struct {
	DetectedAt     time.Time `json:"detected_at"`
	CrashType      string    `json:"crash_type"` // "timeout", "signal", "unknown"
	LastKnownState HookState `json:"last_known_state"`

	// Diagnosis
	WasValidating bool   `json:"was_validating"`
	ValidationCmd string `json:"validation_cmd,omitempty"`
	PartialOutput string `json:"partial_output,omitempty"` // Truncated to 500 chars

	// Recovery recommendation
	RecommendedAction string `json:"recommended_action"` // "retry_step", "skip_step", "manual", "retry_from_checkpoint"
	Reason            string `json:"reason"`

	// Last good checkpoint reference
	LastCheckpointID string `json:"last_checkpoint_id,omitempty"`
}

// ValidationReceipt provides cryptographic proof that validation ran.
// Signed with HD-derived keys, impossible for AI to forge.
type ValidationReceipt struct {
	ReceiptID   string    `json:"receipt_id"` // Unique: rcpt-{uuid8}
	StepName    string    `json:"step_name"`  // Which step this validates
	Command     string    `json:"command"`    // Exact command run
	ExitCode    int       `json:"exit_code"`  // Process exit code
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Duration    string    `json:"duration"` // Human-readable: "12.3s"

	// Output integrity
	StdoutHash string `json:"stdout_hash"` // SHA256 of stdout
	StderrHash string `json:"stderr_hash"` // SHA256 of stderr

	// Cryptographic signature
	// This signature protects against accidental corruption and AI hallucination.
	Signature string `json:"signature"` // Hex-encoded Ed25519 signature

	// KeyPath identifies which key was used for signing (e.g., "native-ed25519-v1")
	KeyPath string `json:"key_path,omitempty"`
}

// Hook represents the durable state of an in-progress task for crash recovery.
// It is the source of truth for recovery context and is stored alongside task.json.
//
// File location: ~/.atlas/workspaces/<name>/tasks/<id>/hook.json
type Hook struct {
	// Metadata
	Version     string    `json:"version"`      // Schema version: "1.0"
	TaskID      string    `json:"task_id"`      // Parent task ID (matches task.json)
	WorkspaceID string    `json:"workspace_id"` // Parent workspace ID
	CreatedAt   time.Time `json:"created_at"`   // Hook creation time
	UpdatedAt   time.Time `json:"updated_at"`   // Last modification time

	// Current State
	State       HookState    `json:"state"`                  // State machine position
	CurrentStep *StepContext `json:"current_step,omitempty"` // Active step details

	// History (append-only audit trail)
	History []HookEvent `json:"history"` // All state transitions

	// Recovery Data (populated on crash detection)
	Recovery *RecoveryContext `json:"recovery,omitempty"`

	// Checkpoints (auto and manual, max 50)
	Checkpoints []StepCheckpoint `json:"checkpoints"`

	// Validation Receipts (cryptographic proof)
	Receipts []ValidationReceipt `json:"receipts"`

	// Schema version for forward compatibility
	SchemaVersion string `json:"schema_version"` // "1.0"
}

// DeepCopy creates a deep copy of the hook for safe read-only access.
// This is useful when you need to inspect hook state without risking
// accidental modifications that could lead to race conditions.
//
// The copy is created via JSON round-trip which handles all nested
// structures correctly. Returns nil if the hook is nil or if
// marshaling/unmarshaling fails.
func (h *Hook) DeepCopy() *Hook {
	if h == nil {
		return nil
	}

	// Use JSON round-trip for simplicity (acceptable for read-only copies)
	data, err := json.Marshal(h)
	if err != nil {
		return nil
	}

	var copyHook Hook
	if err := json.Unmarshal(data, &copyHook); err != nil {
		return nil
	}

	return &copyHook
}
