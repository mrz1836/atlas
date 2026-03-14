// Package domain contains core domain types for ATLAS.
// This file defines types for the hook system (crash recovery & context persistence).
//
// Import rules per existing conventions:
// - CAN import: internal/constants, internal/errors, standard library
// - MUST NOT import: any other internal packages
package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrNilHook is returned when attempting to perform operations on a nil hook.
var ErrNilHook = errors.New("hook is nil")

// DefaultMaxHistoryEvents is the maximum number of history events to retain.
// Older events are pruned to prevent unbounded memory growth.
const DefaultMaxHistoryEvents = 200

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
// Returns ErrNilHook if the hook is nil.
func (h *Hook) DeepCopy() (*Hook, error) {
	if h == nil {
		return nil, ErrNilHook
	}

	copyHook := &Hook{
		Version:       h.Version,
		TaskID:        h.TaskID,
		WorkspaceID:   h.WorkspaceID,
		CreatedAt:     h.CreatedAt,
		UpdatedAt:     h.UpdatedAt,
		State:         h.State,
		SchemaVersion: h.SchemaVersion,
	}

	// Deep copy CurrentStep
	if h.CurrentStep != nil {
		copyHook.CurrentStep = h.CurrentStep.deepCopy()
	}

	// Deep copy History
	if len(h.History) > 0 {
		copyHook.History = make([]HookEvent, len(h.History))
		for i, event := range h.History {
			copyHook.History[i] = event.deepCopy()
		}
	}

	// Deep copy Recovery
	if h.Recovery != nil {
		copyHook.Recovery = h.Recovery.deepCopy()
	}

	// Deep copy Checkpoints
	if len(h.Checkpoints) > 0 {
		copyHook.Checkpoints = make([]StepCheckpoint, len(h.Checkpoints))
		for i, cp := range h.Checkpoints {
			copyHook.Checkpoints[i] = cp.deepCopy()
		}
	}

	// Deep copy Receipts (no nested pointers/slices, shallow copy is sufficient)
	if len(h.Receipts) > 0 {
		copyHook.Receipts = make([]ValidationReceipt, len(h.Receipts))
		copy(copyHook.Receipts, h.Receipts)
	}

	return copyHook, nil
}

// deepCopy creates a deep copy of StepContext.
func (s *StepContext) deepCopy() *StepContext {
	c := &StepContext{
		StepName:            s.StepName,
		StepIndex:           s.StepIndex,
		StartedAt:           s.StartedAt,
		Attempt:             s.Attempt,
		MaxAttempts:         s.MaxAttempts,
		WorkingOn:           s.WorkingOn,
		LastOutput:          s.LastOutput,
		CurrentCheckpointID: s.CurrentCheckpointID,
	}
	if len(s.FilesTouched) > 0 {
		c.FilesTouched = make([]string, len(s.FilesTouched))
		copy(c.FilesTouched, s.FilesTouched)
	}
	return c
}

// deepCopy creates a deep copy of HookEvent.
func (e HookEvent) deepCopy() HookEvent {
	c := HookEvent{
		Timestamp: e.Timestamp,
		FromState: e.FromState,
		ToState:   e.ToState,
		Trigger:   e.Trigger,
		StepName:  e.StepName,
	}
	if len(e.Details) > 0 {
		c.Details = deepCopyMap(e.Details)
	}
	return c
}

// deepCopy creates a deep copy of RecoveryContext.
func (r *RecoveryContext) deepCopy() *RecoveryContext {
	return &RecoveryContext{
		DetectedAt:        r.DetectedAt,
		CrashType:         r.CrashType,
		LastKnownState:    r.LastKnownState,
		WasValidating:     r.WasValidating,
		ValidationCmd:     r.ValidationCmd,
		PartialOutput:     r.PartialOutput,
		RecommendedAction: r.RecommendedAction,
		Reason:            r.Reason,
		LastCheckpointID:  r.LastCheckpointID,
	}
}

// deepCopy creates a deep copy of StepCheckpoint.
func (c StepCheckpoint) deepCopy() StepCheckpoint {
	cp := StepCheckpoint{
		CheckpointID: c.CheckpointID,
		CreatedAt:    c.CreatedAt,
		StepName:     c.StepName,
		StepIndex:    c.StepIndex,
		Description:  c.Description,
		Trigger:      c.Trigger,
		GitBranch:    c.GitBranch,
		GitCommit:    c.GitCommit,
		GitDirty:     c.GitDirty,
	}
	if len(c.Artifacts) > 0 {
		cp.Artifacts = make([]string, len(c.Artifacts))
		copy(cp.Artifacts, c.Artifacts)
	}
	if len(c.FilesSnapshot) > 0 {
		cp.FilesSnapshot = make([]FileSnapshot, len(c.FilesSnapshot))
		copy(cp.FilesSnapshot, c.FilesSnapshot)
	}
	return cp
}

// deepCopyMap creates a deep copy of map[string]any.
// Handles nested maps and slices recursively.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = deepCopyValue(v)
	}
	return c
}

// deepCopyValue recursively copies values for map deep copy.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		c := make([]any, len(val))
		for i, item := range val {
			c[i] = deepCopyValue(item)
		}
		return c
	default:
		// Primitives (string, int, float, bool, nil) are safe to copy directly
		return v
	}
}
