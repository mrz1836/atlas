# Data Model: Hook System

**Feature**: 002-hook-system-mvp
**Date**: 2026-01-17
**Location**: `internal/domain/hook.go`

## Overview

The Hook system adds crash recovery and context persistence to ATLAS tasks. This document defines the domain entities that will be added to `internal/domain/` following the existing package conventions.

## Import Rules

Per existing `internal/domain/` conventions:
- **CAN import**: `internal/constants`, `internal/errors`, standard library
- **MUST NOT import**: any other internal packages

## Entities

### Hook (Primary Entity)

The main state container for crash recovery context. One Hook per Task.

```go
// Hook represents the durable state of an in-progress task for crash recovery.
// It is the source of truth for recovery context and is stored alongside task.json.
//
// File location: ~/.atlas/workspaces/<name>/tasks/<id>/hook.json
type Hook struct {
    // Metadata
    Version     string    `json:"version"`       // Schema version: "1.0"
    TaskID      string    `json:"task_id"`       // Parent task ID (matches task.json)
    WorkspaceID string    `json:"workspace_id"`  // Parent workspace ID
    CreatedAt   time.Time `json:"created_at"`    // Hook creation time
    UpdatedAt   time.Time `json:"updated_at"`    // Last modification time

    // Current State
    State       HookState    `json:"state"`                    // State machine position
    CurrentStep *StepContext `json:"current_step,omitempty"`   // Active step details

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
```

**Validation Rules**:
- `TaskID` must match existing task in same directory
- `State` must be valid HookState enum value
- `History` is append-only (never truncate)
- `Checkpoints` max length: configurable via `hooks.max_checkpoints` (default: 50, oldest pruned when exceeded)

**State Transitions**: See HookState enum below.

**Test Scenarios** (see `hook_test.go`):
- Create hook with valid/invalid TaskID
- Serialize/deserialize round-trip preserves all fields
- Schema version mismatch handling
- Empty vs populated optional fields

---

### HookState (Enumeration)

All valid state machine positions for a Hook.

```go
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
```

**Terminal States**: `completed`, `failed`, `abandoned`

**Test Scenarios** (see `state_test.go`):
- All valid state transitions succeed and record event
- All invalid transitions return error
- Terminal states reject all outgoing transitions
- String conversion round-trip for all states

---

### StepContext

Details about the currently executing step, enabling precise recovery.

```go
// StepContext captures everything needed to resume the current step.
type StepContext struct {
    StepName    string    `json:"step_name"`      // e.g., "analyze", "implement"
    StepIndex   int       `json:"step_index"`     // Zero-based position in template
    StartedAt   time.Time `json:"started_at"`     // When step began
    Attempt     int       `json:"attempt"`        // Current attempt number (1-based)
    MaxAttempts int       `json:"max_attempts"`   // Configured retry limit

    // AI Context (what the AI was working on)
    WorkingOn    string   `json:"working_on,omitempty"`     // Brief description
    FilesTouched []string `json:"files_touched,omitempty"`  // Files modified this step
    LastOutput   string   `json:"last_output,omitempty"`    // Last AI response (truncated to 500 chars)

    // Current checkpoint reference
    CurrentCheckpointID string `json:"current_checkpoint_id,omitempty"`
}
```

**Validation Rules**:
- `Attempt` must be >= 1
- `StepIndex` must be valid index in task's steps array
- `LastOutput` truncated to 500 characters max

**Test Scenarios**:
- Attempt counter increments correctly on retry
- LastOutput truncation at exactly 500 chars
- FilesTouched deduplication
- Empty working description handling

---

### StepCheckpoint

A snapshot of progress at a specific point, enabling recovery to known-good states.

```go
// StepCheckpoint captures progress within a step for recovery.
type StepCheckpoint struct {
    CheckpointID string           `json:"checkpoint_id"` // Unique ID: ckpt-{uuid8}
    CreatedAt    time.Time        `json:"created_at"`
    StepName     string           `json:"step_name"`     // Which step this belongs to
    StepIndex    int              `json:"step_index"`
    Description  string           `json:"description"`   // What was accomplished
    Trigger      CheckpointTrigger `json:"trigger"`      // What caused this checkpoint

    // Version control state at checkpoint
    GitBranch string `json:"git_branch"`
    GitCommit string `json:"git_commit,omitempty"` // SHA if committed
    GitDirty  bool   `json:"git_dirty"`            // Uncommitted changes?

    // Artifact references
    Artifacts []string `json:"artifacts,omitempty"` // Paths in artifacts/

    // File state for debugging
    FilesSnapshot []FileSnapshot `json:"files_snapshot,omitempty"`
}
```

**Validation Rules**:
- `CheckpointID` format: `ckpt-{8 hex chars}`
- `Trigger` must be valid CheckpointTrigger enum value

**Test Scenarios** (see `checkpoint_test.go`):
- Checkpoint ID generation uniqueness
- Each trigger type creates checkpoint correctly
- Git state capture (branch, commit, dirty)
- File snapshot creation and hash verification
- Pruning removes oldest when max exceeded
- Interval checkpointer starts/stops cleanly
- Concurrent checkpoint creation is safe

---

### CheckpointTrigger (Enumeration)

What caused a checkpoint to be created.

```go
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
```

---

### FileSnapshot

Captures a file's state for debugging and recovery verification.

```go
// FileSnapshot captures a file's state at checkpoint time.
type FileSnapshot struct {
    Path    string `json:"path"`
    Size    int64  `json:"size"`
    ModTime string `json:"mod_time"` // RFC3339 format
    SHA256  string `json:"sha256"`   // First 16 chars of hash
    Exists  bool   `json:"exists"`
}
```

---

### HookEvent

An append-only history entry for audit trail.

```go
// HookEvent records a state transition in the hook history.
type HookEvent struct {
    Timestamp time.Time      `json:"timestamp"`
    FromState HookState      `json:"from_state"`
    ToState   HookState      `json:"to_state"`
    Trigger   string         `json:"trigger"`             // What caused transition
    StepName  string         `json:"step_name,omitempty"` // If step-related
    Details   map[string]any `json:"details,omitempty"`   // Additional context
}
```

---

### RecoveryContext

Populated when crash recovery is detected and needed.

```go
// RecoveryContext is populated when crash recovery is needed.
type RecoveryContext struct {
    DetectedAt     time.Time `json:"detected_at"`
    CrashType      string    `json:"crash_type"`        // "timeout", "signal", "unknown"
    LastKnownState HookState `json:"last_known_state"`

    // Diagnosis
    WasValidating bool   `json:"was_validating"`
    ValidationCmd string `json:"validation_cmd,omitempty"`
    PartialOutput string `json:"partial_output,omitempty"` // Truncated to 500 chars

    // Recovery recommendation
    RecommendedAction string `json:"recommended_action"` // "retry_step", "skip_step", "manual"
    Reason            string `json:"reason"`

    // Last good checkpoint reference
    LastCheckpointID string `json:"last_checkpoint_id,omitempty"`
}
```

**RecommendedAction Values**:
- `retry_step`: Safe to retry the current step from beginning
- `retry_from_checkpoint`: Resume from last checkpoint
- `skip_step`: Step was completing, safe to advance
- `manual`: Human intervention required

**Test Scenarios** (see `recovery_test.go`):
- Stale detection with various timestamps and states
- Each crash type produces correct recommendation
- Idempotent steps (analyze, plan) → `retry_step`
- Non-idempotent steps (implement) → `manual`
- Recent checkpoint available → `retry_from_checkpoint`
- Recovery context fully populated on detection
- Terminal states never trigger recovery

---

### ValidationReceipt

Cryptographic proof that validation actually ran.

```go
// ValidationReceipt provides cryptographic proof that validation ran.
// Signed with HD-derived keys, impossible for AI to forge.
type ValidationReceipt struct {
    ReceiptID   string    `json:"receipt_id"`    // Unique: rcpt-{uuid8}
    StepName    string    `json:"step_name"`     // Which step this validates
    Command     string    `json:"command"`       // Exact command run
    ExitCode    int       `json:"exit_code"`     // Process exit code
    StartedAt   time.Time `json:"started_at"`
    CompletedAt time.Time `json:"completed_at"`
    Duration    string    `json:"duration"`      // Human-readable: "12.3s"

    // Output integrity
    StdoutHash string `json:"stdout_hash"` // SHA256 of stdout
    StderrHash string `json:"stderr_hash"` // SHA256 of stderr

    // Cryptographic signature
    KeyPath   string `json:"key_path"`   // HD derivation path used
    Signature string `json:"signature"`  // Hex-encoded ECDSA signature
}
```

**Signature Message Format**:
```
{receipt_id}|{command}|{exit_code}|{stdout_hash}|{stderr_hash}|{completed_at_unix}
```

**Test Scenarios** (see `signing_test.go`):
- Receipt creation with all fields populated
- Output hashing produces consistent SHA256
- Signature message format is deterministic
- HD key derivation follows configured path
- Valid receipts verify successfully
- Tampered receipts (any field changed) fail verification
- Missing master key returns clear error
- Key file permissions enforced (0600)
- Receipt ID uniqueness within task

**Test Key Handling** (gitleaks compliance):
- Test keys MUST be generated at runtime using `t.TempDir()` - never commit keys
- Fixtures with signatures are generated during test setup, not committed
- Use `// gitleaks:allow` inline comments only for unavoidable test vectors (e.g., expected hash values)

---

## Interfaces

Cryptographic operations are abstracted behind interfaces to allow swapping implementations (e.g., replacing `go-sdk` with another library in the future).

### ReceiptSigner Interface

```go
// ReceiptSigner handles cryptographic signing of validation receipts.
// Implementations can use different key derivation schemes (HD, simple, etc.).
//
// Current implementation: HDReceiptSigner (uses github.com/bsv-blockchain/go-sdk)
// This interface allows swapping to a different crypto library without changing hook logic.
type ReceiptSigner interface {
    // Sign signs a validation receipt and populates its Signature and KeyPath fields.
    // The taskIndex is used for key derivation (e.g., HD path component).
    Sign(ctx context.Context, receipt *ValidationReceipt, taskIndex uint32) error

    // Verify checks that a receipt's signature is valid.
    // Returns nil if valid, error if invalid or verification fails.
    Verify(ctx context.Context, receipt *ValidationReceipt) error

    // KeyPath returns the derivation path used for a given task index.
    // Format depends on implementation (e.g., "m/44'/236'/0'/5/0" for HD).
    KeyPath(taskIndex uint32) string
}
```

### KeyManager Interface

```go
// KeyManager handles master key lifecycle (generation, loading, storage).
// Abstracted to allow different storage backends or key types.
//
// Current implementation: FileKeyManager (stores in ~/.atlas/keys/master.key)
type KeyManager interface {
    // Load retrieves the master key, generating one if it doesn't exist.
    // Returns error if key cannot be loaded or generated.
    Load(ctx context.Context) error

    // Exists checks if a master key is already configured.
    Exists() bool

    // NewSigner creates a ReceiptSigner using the loaded master key.
    // Must call Load() first.
    NewSigner() (ReceiptSigner, error)
}
```

### Implementation Notes

```go
// HDReceiptSigner implements ReceiptSigner using BIP32/BIP44 HD key derivation.
// Uses github.com/bsv-blockchain/go-sdk for cryptographic operations.
//
// Alternative: github.com/BitcoinSchema/go-bitcoin (create BitcoinSigner)
//
// To swap implementations:
// 1. Create new type implementing ReceiptSigner interface (e.g., BitcoinSigner)
// 2. Update KeyManager.NewSigner() to return new implementation
// 3. No changes needed to hook/, cli/, or domain/ packages
type HDReceiptSigner struct {
    masterKey *bip32.ExtendedKey  // go-sdk type (internal detail)
    cfg       *config.KeyDerivationConfig
}

// FileKeyManager implements KeyManager with filesystem storage.
type FileKeyManager struct {
    keyPath string  // ~/.atlas/keys/master.key
    key     []byte  // Loaded key bytes
}
```

**Swappability guarantee**: The `hook/` package depends only on the `ReceiptSigner` and `KeyManager` interfaces, never on `go-sdk` types directly. This allows replacing the crypto implementation without modifying hook business logic.

---

## Relationships

```
┌─────────────────┐
│      Task       │ (existing in domain/task.go)
│    task.json    │
└────────┬────────┘
         │ 1:1 (same directory)
         ▼
┌─────────────────┐
│      Hook       │
│    hook.json    │
└────────┬────────┘
         │
    ┌────┼────┬────────────┐
    │    │    │            │
    ▼    ▼    ▼            ▼
┌──────┐ ┌────────┐ ┌──────────┐ ┌─────────────────┐
│Event │ │Context │ │Checkpoint│ │ValidationReceipt│
│ (n)  │ │  (1)   │ │  (≤50)   │ │       (n)       │
└──────┘ └────────┘ └──────────┘ └─────────────────┘
                          │
                          ▼
                    ┌───────────┐
                    │FileSnapshot│
                    │    (n)     │
                    └───────────┘
```

---

## State Machine Transitions

```go
// ValidHookTransitions defines allowed state transitions.
var ValidHookTransitions = map[HookState][]HookState{
    "":                       {HookStateInitializing},
    HookStateInitializing:    {HookStateStepPending},
    HookStateStepPending:     {HookStateStepRunning, HookStateCompleted, HookStateAbandoned},
    HookStateStepRunning:     {HookStateStepValidating, HookStateStepPending, HookStateAwaitingHuman, HookStateAbandoned},
    HookStateStepValidating:  {HookStateStepPending, HookStateAwaitingHuman},
    HookStateAwaitingHuman:   {HookStateStepPending, HookStateStepRunning, HookStateAbandoned},
    HookStateRecovering:      {HookStateStepPending, HookStateStepRunning, HookStateAwaitingHuman},
}

// From any non-terminal state:
// - crash_detected → HookStateRecovering
// - abandon → HookStateAbandoned
```

**Terminal States** (no outgoing transitions):
- `HookStateCompleted`
- `HookStateFailed`
- `HookStateAbandoned`

---

## JSON Schema Example

```json
{
  "version": "1.0",
  "task_id": "task-20260117-143022",
  "workspace_id": "fix-null-pointer",
  "created_at": "2026-01-17T14:30:22Z",
  "updated_at": "2026-01-17T14:45:33Z",
  "state": "step_running",
  "current_step": {
    "step_name": "implement",
    "step_index": 2,
    "started_at": "2026-01-17T14:40:00Z",
    "attempt": 2,
    "max_attempts": 3,
    "working_on": "Adding nil checks for config fields",
    "files_touched": ["config/parser.go", "config/parser_test.go"],
    "last_output": "I've added the nil check for Server. Now adding for Database...",
    "current_checkpoint_id": "ckpt-a1b2c3d4"
  },
  "history": [
    {
      "timestamp": "2026-01-17T14:30:22Z",
      "from_state": "",
      "to_state": "initializing",
      "trigger": "task_start"
    },
    {
      "timestamp": "2026-01-17T14:30:25Z",
      "from_state": "initializing",
      "to_state": "step_pending",
      "trigger": "setup_complete"
    }
  ],
  "checkpoints": [
    {
      "checkpoint_id": "ckpt-a1b2c3d4",
      "created_at": "2026-01-17T14:42:15Z",
      "step_name": "implement",
      "step_index": 2,
      "description": "Added nil check for config.Server field",
      "trigger": "git_commit",
      "git_branch": "fix/fix-null-pointer",
      "git_commit": "abc123def456",
      "git_dirty": false,
      "files_snapshot": [
        {
          "path": "config/parser.go",
          "size": 4523,
          "mod_time": "2026-01-17T14:42:10Z",
          "sha256": "e3b0c44298fc1c14",
          "exists": true
        }
      ]
    }
  ],
  "receipts": [
    {
      "receipt_id": "rcpt-00000001",
      "step_name": "analyze",
      "command": "magex lint",
      "exit_code": 0,
      "started_at": "2026-01-17T14:35:00Z",
      "completed_at": "2026-01-17T14:35:12Z",
      "duration": "12.3s",
      "stdout_hash": "a1b2c3d4e5f6...",
      "stderr_hash": "0000000000000000",
      "key_path": "m/44'/236'/0'/0/0",
      "signature": "3045022100..."
    }
  ],
  "schema_version": "1.0"
}
```

---

## Constants

Add to `internal/constants/hook.go`:

```go
// Hook-related constants (fixed, non-configurable values)
const (
    // HookFileName is the JSON state file name.
    HookFileName = "hook.json"

    // HookMarkdownFileName is the human-readable recovery file name.
    HookMarkdownFileName = "HOOK.md"

    // HookSchemaVersion is the current hook.json schema version.
    HookSchemaVersion = "1.0"

    // MaxLastOutputLength is the maximum length of LastOutput field.
    MaxLastOutputLength = 500
)
```

---

## Configuration

All tunable settings are stored in the ATLAS config file (`~/.atlas/config.yaml`) under the `hooks` key. This centralizes configuration and allows users to customize behavior without code changes.

Add to `internal/config/config.go`:

```go
// HookConfig contains all configurable settings for the hook system.
type HookConfig struct {
    // Checkpoints
    MaxCheckpoints     int           `yaml:"max_checkpoints" default:"50"`      // Max checkpoints per task (oldest pruned)
    CheckpointInterval time.Duration `yaml:"checkpoint_interval" default:"5m"`  // Interval for periodic checkpoints

    // Crash Detection
    StaleThreshold time.Duration `yaml:"stale_threshold" default:"5m"` // Time after which hook is considered stale

    // Cryptographic Signing (HD Keys)
    KeyDerivation KeyDerivationConfig `yaml:"key_derivation"`

    // Cleanup Retention (per terminal state)
    Retention RetentionConfig `yaml:"retention"`
}

// KeyDerivationConfig contains HD key derivation settings.
type KeyDerivationConfig struct {
    // BIP44 path components: m/purpose'/coin_type'/account'/{task_index}/{receipt_index}
    Purpose  uint32 `yaml:"purpose" default:"44"`   // BIP44 purpose
    CoinType uint32 `yaml:"coin_type" default:"236"` // ATLAS coin type (arbitrary)
    Account  uint32 `yaml:"account" default:"0"`    // Receipt signing account
}

// RetentionConfig specifies how long to keep hook files per terminal state.
type RetentionConfig struct {
    Completed time.Duration `yaml:"completed" default:"720h"` // 30 days for completed tasks
    Failed    time.Duration `yaml:"failed" default:"168h"`    // 7 days for failed tasks
    Abandoned time.Duration `yaml:"abandoned" default:"168h"` // 7 days for abandoned tasks
}
```

### Config YAML Example

```yaml
# ~/.atlas/config.yaml
hooks:
  # Checkpoint settings
  max_checkpoints: 50
  checkpoint_interval: 5m

  # Crash detection
  stale_threshold: 5m

  # HD key derivation path: m/{purpose}'/{coin_type}'/{account}'/{task}/{receipt}
  key_derivation:
    purpose: 44
    coin_type: 236
    account: 0

  # Cleanup retention periods
  retention:
    completed: 720h  # 30 days
    failed: 168h     # 7 days
    abandoned: 168h  # 7 days
```

### Config vs Constants

| Value | Location | Rationale |
|-------|----------|-----------|
| File names (`hook.json`, `HOOK.md`) | Constant | Fixed convention, changing would break compatibility |
| Schema version (`1.0`) | Constant | Tied to code version, not user preference |
| Max output length (500) | Constant | Internal implementation detail |
| Max checkpoints | **Config** | User may want more/fewer based on task complexity |
| Checkpoint interval | **Config** | User may adjust for slower/faster systems |
| Stale threshold | **Config** | User may have different tolerance for crash detection |
| Key derivation path | **Config** | Advanced users may need different paths |
| Retention periods | **Config** | User controls disk space vs history tradeoff |
