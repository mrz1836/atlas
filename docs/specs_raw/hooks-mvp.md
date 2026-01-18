# ATLAS Hook Files: Crash Recovery & Context Persistence

## Technical Specification v1.0

**Author:** Z
**Date:** January 2026
**Status:** Design Specification
**Target:** ATLAS AI Task Lifecycle Automation System

---

## Executive Summary

Hook Files provide durable, crash-resistant context persistence for AI-assisted development workflows. Inspired by Gas Town's "propulsion system," but simplified for ATLAS's single-agent, validation-heavy architecture.

**Core Principle:** The AI agent should be able to crash, restart, lose all memory, and still resume work exactly where it left off by reading a single file.

---

## Problem Statement

### Current Pain Points

1. **Context Loss on Crash**: When Claude Code crashes mid-task, the AI loses all working memory. The human must manually explain what was happening.

2. **Validation State Ambiguity**: If validation was running when a crash occurred, we don't know if it passed, failed, or never completed.

3. **Retry Confusion**: After a crash, the AI might repeat work that was already done, or skip work that wasn't actually completed.

4. **Progress Opacity**: The human can't easily see exactly what the AI was doing at the moment of failure.

### Success Criteria

- [ ] AI can resume any interrupted task within 30 seconds of restart
- [ ] Zero repeated work after crash recovery
- [ ] Zero skipped work after crash recovery
- [ ] Human can audit exactly what happened before crash
- [ ] Works with all existing ATLAS templates

---

## Design Philosophy

### Principles

1. **File as Source of Truth**: The hook file is canonical. If it says step X is in progress, step X is in progress—regardless of what the AI "remembers."

2. **Append-Only History**: Never delete history entries. Only append. This creates an audit trail.

3. **Explicit State Machine**: Every possible state is defined. No implicit states.

4. **Human-Readable First**: The hook file should be readable by a human in a text editor. JSON structured data is secondary.

5. **Idempotent Recovery**: Running recovery twice should produce the same result as running it once.

6. **Tamper-Evident Integrity**: Validation receipts are signed to prevent accidental corruption or AI hallucination (claiming work was done when it wasn't).

### Non-Goals

- Multi-agent coordination (ATLAS is single-agent)
- Real-time sync across machines (future: serverless API)
- Complex dependency graphs (use templates for that)

---

## Architecture

### File Location

```
~/.atlas/workspaces/<workspace>/tasks/<task-id>/
├── task.json           # Existing: task metadata and step history
├── task.log            # Existing: JSON-lines execution log
├── HOOK.md             # NEW: AI-readable recovery context
├── hook.json           # NEW: Machine-readable hook state
└── artifacts/
    └── ...             # Existing: step outputs
```

### Hook File Relationship

```
┌─────────────────────────────────────────────────────────────┐
│                        HOOK.md                               │
│  Human & AI readable recovery instructions                   │
│  - Current state summary                                     │
│  - What to do next                                          │
│  - What NOT to do (already completed)                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Generated from
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       hook.json                              │
│  Machine-readable state (source of truth)                    │
│  - Precise state machine position                           │
│  - Timestamps and durations                                 │
│  - Validation receipts                                      │
│  - Attempt history                                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Informs
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       task.json                              │
│  Existing task metadata (unchanged)                          │
│  - Template configuration                                   │
│  - Step definitions                                         │
│  - Final outcomes                                           │
└─────────────────────────────────────────────────────────────┘
```

---

## Data Structures

### hook.json Schema

```go
// Hook represents the durable state of an in-progress task
type Hook struct {
    // Metadata
    Version     string    `json:"version"`      // Schema version: "1.0"
    TaskID      string    `json:"task_id"`      // Parent task ID
    CreatedAt   time.Time `json:"created_at"`   // Hook creation time
    UpdatedAt   time.Time `json:"updated_at"`   // Last modification time

    // Current State
    State       HookState `json:"state"`        // Current state machine position
    CurrentStep *StepContext `json:"current_step,omitempty"` // Active step details

    // History (append-only)
    History     []HookEvent `json:"history"`    // All state transitions

    // Recovery Data
    Recovery    *RecoveryContext `json:"recovery,omitempty"` // Populated on crash detection

    // Validation Receipts
    Receipts    []ValidationReceipt `json:"receipts"` // Cryptographic proof of validation

    // Checkpoints (auto and manual)
    Checkpoints []StepCheckpoint `json:"checkpoints"` // All checkpoints for this task
}

// HookState represents the state machine position
type HookState string

const (
    HookStateInitializing   HookState = "initializing"    // Task setup in progress
    HookStateStepPending    HookState = "step_pending"    // Ready to start a step
    HookStateStepRunning    HookState = "step_running"    // AI executing a step
    HookStateStepValidating HookState = "step_validating" // Validation in progress
    HookStateAwaitingHuman  HookState = "awaiting_human"  // Blocked on human action
    HookStateRecovering     HookState = "recovering"      // Crash recovery in progress
    HookStateCompleted      HookState = "completed"       // Task finished successfully
    HookStateFailed         HookState = "failed"          // Task failed permanently
    HookStateAbandoned      HookState = "abandoned"       // Human abandoned task
)

// StepContext captures everything about the current step
type StepContext struct {
    StepName    string            `json:"step_name"`     // e.g., "analyze", "implement"
    StepIndex   int               `json:"step_index"`    // Position in template
    StartedAt   time.Time         `json:"started_at"`    // When step began
    Prompt      string            `json:"prompt"`        // The prompt sent to AI
    Attempt     int               `json:"attempt"`       // Current attempt number (1-based)
    MaxAttempts int               `json:"max_attempts"`  // Configured retry limit

    // AI Context (what the AI was working on)
    WorkingOn   string            `json:"working_on,omitempty"`   // Brief description
    FilesTouched []string         `json:"files_touched,omitempty"` // Files modified
    LastOutput  string            `json:"last_output,omitempty"`  // Last AI response (truncated)

    // Current checkpoint reference
    CurrentCheckpointID string    `json:"current_checkpoint_id,omitempty"`
}

// StepCheckpoint captures progress within a step (auto or manual)
type StepCheckpoint struct {
    CheckpointID string          `json:"checkpoint_id"` // Unique ID for this checkpoint
    CreatedAt    time.Time       `json:"created_at"`
    StepName     string          `json:"step_name"`     // Which step this belongs to
    StepIndex    int             `json:"step_index"`
    Description  string          `json:"description"`   // What was accomplished
    Trigger      CheckpointTrigger `json:"trigger"`     // What caused this checkpoint

    // Git state at checkpoint
    GitBranch    string          `json:"git_branch"`
    GitCommit    string          `json:"git_commit,omitempty"` // If committed
    GitDirty     bool            `json:"git_dirty"`            // Uncommitted changes?

    // Artifact references
    Artifacts    []string        `json:"artifacts,omitempty"`  // Files in artifacts/

    // For debugging/resilience
    FilesSnapshot []FileSnapshot `json:"files_snapshot,omitempty"` // Key file states
}

// CheckpointTrigger indicates what caused a checkpoint
type CheckpointTrigger string

const (
    CheckpointTriggerManual      CheckpointTrigger = "manual"       // atlas checkpoint "desc"
    CheckpointTriggerCommit      CheckpointTrigger = "git_commit"   // Auto on git commit
    CheckpointTriggerPush        CheckpointTrigger = "git_push"     // Auto on git push
    CheckpointTriggerPR          CheckpointTrigger = "pr_created"   // Auto on PR creation
    CheckpointTriggerValidation  CheckpointTrigger = "validation"   // Auto after validation pass
    CheckpointTriggerStepComplete CheckpointTrigger = "step_complete" // Auto on step completion
    CheckpointTriggerInterval    CheckpointTrigger = "interval"     // Auto periodic (5 min)
)

// FileSnapshot captures a file's state for debugging
type FileSnapshot struct {
    Path     string `json:"path"`
    Size     int64  `json:"size"`
    ModTime  string `json:"mod_time"`
    SHA256   string `json:"sha256"` // First 16 chars of hash
    Exists   bool   `json:"exists"`
}

// HookEvent represents a state transition (append-only history)
type HookEvent struct {
    Timestamp   time.Time         `json:"timestamp"`
    FromState   HookState         `json:"from_state"`
    ToState     HookState         `json:"to_state"`
    Trigger     string            `json:"trigger"`      // What caused transition
    StepName    string            `json:"step_name,omitempty"`
    Details     map[string]any    `json:"details,omitempty"`
}

// RecoveryContext is populated when crash recovery is needed
type RecoveryContext struct {
    DetectedAt      time.Time `json:"detected_at"`
    CrashType       string    `json:"crash_type"`       // "timeout", "signal", "unknown"
    LastKnownState  HookState `json:"last_known_state"`

    // Diagnosis
    WasValidating   bool      `json:"was_validating"`
    ValidationCmd   string    `json:"validation_cmd,omitempty"`
    PartialOutput   string    `json:"partial_output,omitempty"`

    // Recovery plan
    RecommendedAction string  `json:"recommended_action"` // "retry_step", "skip_step", "manual"
    Reason            string  `json:"reason"`

    // Last good checkpoint
    LastCheckpointID string   `json:"last_checkpoint_id,omitempty"`
}

// ValidationReceipt provides cryptographic proof that validation ran
// Signed with Ed25519 - protects against accidental corruption
type ValidationReceipt struct {
    ReceiptID   string    `json:"receipt_id"`    // Unique receipt ID
    StepName    string    `json:"step_name"`     // Which step this validates
    Command     string    `json:"command"`       // Exact command run
    ExitCode    int       `json:"exit_code"`     // Process exit code
    StartedAt   time.Time `json:"started_at"`
    CompletedAt time.Time `json:"completed_at"`
    Duration    string    `json:"duration"`      // Human-readable duration

    // Output hashes (integrity check)
    StdoutHash  string    `json:"stdout_hash"`   // SHA256 of stdout
    StderrHash  string    `json:"stderr_hash"`   // SHA256 of stderr

    // Cryptographic integrity
    Signature   string    `json:"signature"`     // Hex-encoded Ed25519 signature
}
```

### HOOK.md Template

The `HOOK.md` file is generated from `hook.json` and is what the AI actually reads. It's designed to be unambiguous and actionable.

```markdown
# ATLAS Task Recovery Hook

> ⚠️ **READ THIS FIRST** - This file contains your recovery context.
> Do NOT proceed without understanding the current state.

## Current State: `step_running`

**Task:** fix-null-pointer-config
**Template:** bugfix
**Started:** 2026-01-17T14:30:22Z
**Last Updated:** 2026-01-17T14:45:33Z (12 minutes ago)

---

## What You Were Doing

**Step:** `implement` (step 3 of 7)
**Attempt:** 2 of 3
**Working On:** Fixing nil pointer dereference in config/parser.go

### Files You Modified
- `config/parser.go` (modified, uncommitted)
- `config/parser_test.go` (modified, uncommitted)

### Last Checkpoint
> ID: `ckpt-a1b2c3d4`
> Time: 2026-01-17T14:42:15Z
> Trigger: `git_commit`
> Status: "Added nil check for config.Server field"

### Your Last Output (truncated)
```
I've added the nil check for the Server field. Now I need to add
similar checks for the Database and Cache fields. Let me continue...
```

---

## What To Do Now

### ✅ RESUME: Continue the `implement` step

1. Review the files you modified: `config/parser.go`, `config/parser_test.go`
2. Check git status to see uncommitted changes
3. Continue adding nil checks for Database and Cache fields
4. When complete, run `atlas validate` to verify your changes

### ❌ DO NOT:
- Start over from the beginning
- Re-analyze the issue (already done in step 1)
- Recreate the branch (already exists: `fix/fix-null-pointer-config`)

---

## Completed Steps (DO NOT REPEAT)

| Step | Status | Duration | Receipt |
|------|--------|----------|---------|
| 1. analyze | ✅ Completed | 45s | `rcpt-001` ✓ |
| 2. plan | ✅ Completed | 32s | `rcpt-002` ✓ |

---

## Checkpoints Timeline

| Time | Trigger | Description |
|------|---------|-------------|
| 14:35:00 | `step_complete` | Analysis complete |
| 14:38:22 | `step_complete` | Plan complete |
| 14:40:15 | `git_commit` | Initial implementation scaffold |
| 14:42:15 | `git_commit` | Added nil check for config.Server field |

---

## Validation Receipts

These receipts are cryptographically signed and prove validation actually ran:

### Receipt `rcpt-002` (plan step)
```
Command: magex lint
Exit Code: 0
Duration: 12.3s
Output Hash: sha256:a1b2c3d4e5f6...
Signature: VALID ✓ (verified with HD key m/44'/236'/0'/0/2)
```

---

## If Something Is Wrong

If this recovery context seems incorrect:

1. Run `atlas status` to see ATLAS's view of the task
2. Run `atlas recover` to enter manual recovery mode
3. Run `atlas abandon` to give up and start fresh

---

*Generated by ATLAS v1.1.35 at 2026-01-17T14:45:33Z*
```

---

## State Machine

### State Transitions

```
                                    ┌─────────────┐
                                    │ ABANDONED   │
                                    └─────────────┘
                                          ▲
                                          │ abandon
                                          │
┌─────────────┐    init      ┌─────────────────────┐
│             │─────────────▶│                     │
│ (new task)  │              │   INITIALIZING      │
│             │              │                     │
└─────────────┘              └─────────────────────┘
                                       │
                                       │ setup_complete
                                       ▼
                             ┌─────────────────────┐
                      ┌─────▶│                     │◀─────┐
                      │      │   STEP_PENDING      │      │
                      │      │                     │      │
                      │      └─────────────────────┘      │
                      │                │                  │
                      │                │ start_step       │
                      │                ▼                  │
                      │      ┌─────────────────────┐      │
                      │      │                     │      │
         step_complete│      │   STEP_RUNNING      │──────┘
         (more steps) │      │                     │  step_complete
                      │      └─────────────────────┘  (no more steps)
                      │                │                  │
                      │                │ step_output      │
                      │                ▼                  │
                      │      ┌─────────────────────┐      │
                      │      │                     │      │
                      │      │  STEP_VALIDATING    │      │
                      │      │                     │      │
                      │      └─────────────────────┘      │
                      │           │          │            │
                      │  validate_pass   validate_fail    │
                      │           │          │            │
                      │           │          ▼            │
                      │           │  ┌──────────────┐     │
                      │           │  │              │     │
                      │           │  │ AWAITING_    │     │
                      │           │  │ HUMAN        │     │
                      │           │  │              │     │
                      │           │  └──────────────┘     │
                      │           │         │             │
                      │           │    human_approve      │
                      │           │    human_reject       │
                      │           │         │             │
                      └───────────┴─────────┘             │
                                                         │
                                                         ▼
                                               ┌─────────────────┐
                                               │                 │
                                               │   COMPLETED     │
                                               │                 │
                                               └─────────────────┘

                    ════════════════════════════════════════════

                    CRASH RECOVERY (can occur from any state)

                    ════════════════════════════════════════════

                             Any State
                                 │
                                 │ crash_detected
                                 ▼
                       ┌─────────────────────┐
                       │                     │
                       │    RECOVERING       │
                       │                     │
                       └─────────────────────┘
                                 │
                    ┌────────────┼────────────┐
                    │            │            │
              retry_step    skip_step    manual_required
                    │            │            │
                    ▼            ▼            ▼
              STEP_PENDING  STEP_PENDING  AWAITING_HUMAN
              (same step)   (next step)
```

### Transition Table

| From State | Event | To State | Action |
|------------|-------|----------|--------|
| (none) | `init` | `initializing` | Create hook.json, HOOK.md |
| `initializing` | `setup_complete` | `step_pending` | Record workspace ready |
| `step_pending` | `start_step` | `step_running` | Update current_step |
| `step_running` | `step_output` | `step_validating` | Trigger validation |
| `step_running` | `checkpoint` | `step_running` | Save checkpoint |
| `step_validating` | `validate_pass` | `step_pending` or `completed` | Create receipt, advance |
| `step_validating` | `validate_fail` | `awaiting_human` | Create receipt, notify |
| `awaiting_human` | `human_approve` | `step_pending` | Advance to next step |
| `awaiting_human` | `human_reject` | `step_pending` | Retry current step |
| * | `crash_detected` | `recovering` | Populate recovery context |
| `recovering` | `retry_step` | `step_pending` | Reset to step start |
| `recovering` | `skip_step` | `step_pending` | Advance to next step |
| `recovering` | `manual_required` | `awaiting_human` | Notify human |
| * | `abandon` | `abandoned` | Mark task abandoned |

---

## Auto-Checkpoint System

ATLAS automatically creates checkpoints at key moments for resilience and debugging. No manual intervention required.

### Automatic Checkpoint Triggers

| Trigger | When | Rationale |
|---------|------|-----------|
| `git_commit` | After any `git commit` | Captures known-good state |
| `git_push` | After any `git push` | Remote backup confirmed |
| `pr_created` | After PR creation | Major milestone |
| `validation` | After validation passes | Verified working state |
| `step_complete` | After each step completes | Progress marker |
| `interval` | Every 5 minutes during `step_running` | Periodic safety net |

### Checkpoint Implementation

```go
// internal/hooks/checkpoint.go

// AutoCheckpointConfig controls automatic checkpoint behavior
type AutoCheckpointConfig struct {
    OnCommit       bool          `yaml:"on_commit"`        // Default: true
    OnPush         bool          `yaml:"on_push"`          // Default: true
    OnPR           bool          `yaml:"on_pr"`            // Default: true
    OnValidation   bool          `yaml:"on_validation"`    // Default: true
    OnStepComplete bool          `yaml:"on_step_complete"` // Default: true
    Interval       time.Duration `yaml:"interval"`         // Default: 5m, 0 = disabled
}

// DefaultAutoCheckpointConfig returns sensible defaults
func DefaultAutoCheckpointConfig() AutoCheckpointConfig {
    return AutoCheckpointConfig{
        OnCommit:       true,
        OnPush:         true,
        OnPR:           true,
        OnValidation:   true,
        OnStepComplete: true,
        Interval:       5 * time.Minute,
    }
}

// GitHookIntegration sets up git hooks for auto-checkpointing
func (h *Hook) SetupGitHooks(repoPath string) error {
    // post-commit hook
    postCommit := `#!/bin/sh
atlas checkpoint --trigger git_commit --auto "Commit: $(git log -1 --format=%s)"
`
    // post-push hook (called by git after successful push)
    // Note: This is a custom hook, may need git config

    // Write hooks to .git/hooks/
    // ...
}

// CreateAutoCheckpoint creates a checkpoint with automatic metadata
func (h *Hook) CreateAutoCheckpoint(trigger CheckpointTrigger, description string) error {
    checkpoint := &StepCheckpoint{
        CheckpointID: generateCheckpointID(),
        CreatedAt:    time.Now(),
        StepName:     h.CurrentStep.StepName,
        StepIndex:    h.CurrentStep.StepIndex,
        Description:  description,
        Trigger:      trigger,
        GitBranch:    h.getCurrentBranch(),
        GitCommit:    h.getHeadCommit(),
        GitDirty:     h.hasUncommittedChanges(),
    }

    // Capture file snapshots for key files (debugging)
    if h.CurrentStep.FilesTouched != nil {
        checkpoint.FilesSnapshot = h.snapshotFiles(h.CurrentStep.FilesTouched)
    }

    h.Checkpoints = append(h.Checkpoints, *checkpoint)
    h.CurrentStep.CurrentCheckpointID = checkpoint.CheckpointID

    h.appendEvent(HookEvent{
        Trigger: "checkpoint",
        Details: map[string]any{
            "checkpoint_id": checkpoint.CheckpointID,
            "trigger":       string(trigger),
        },
    })

    return h.save()
}

// snapshotFiles captures state of files for debugging
func (h *Hook) snapshotFiles(paths []string) []FileSnapshot {
    snapshots := make([]FileSnapshot, 0, len(paths))
    for _, path := range paths {
        info, err := os.Stat(path)
        snapshot := FileSnapshot{Path: path}

        if err != nil {
            snapshot.Exists = false
        } else {
            snapshot.Exists = true
            snapshot.Size = info.Size()
            snapshot.ModTime = info.ModTime().Format(time.RFC3339)

            // Quick hash (first 16 chars of SHA256)
            if hash, err := hashFile(path); err == nil {
                snapshot.SHA256 = hash[:16]
            }
        }
        snapshots = append(snapshots, snapshot)
    }
    return snapshots
}

// IntervalCheckpointer runs periodic checkpoints during long steps
type IntervalCheckpointer struct {
    hook     *Hook
    interval time.Duration
    stopCh   chan struct{}
}

func (ic *IntervalCheckpointer) Start() {
    go func() {
        ticker := time.NewTicker(ic.interval)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                if ic.hook.State == HookStateStepRunning {
                    ic.hook.CreateAutoCheckpoint(
                        CheckpointTriggerInterval,
                        fmt.Sprintf("Periodic checkpoint at %s", time.Now().Format("15:04:05")),
                    )
                }
            case <-ic.stopCh:
                return
            }
        }
    }()
}
```

---

## Native Cryptographic Signing

Validation receipts are signed using native Ed25519 keys. This provides:

- **Unforgeable signatures**: AI cannot produce valid signatures
- **Simple implementation**: Uses standard Go `crypto/ed25519` library
- **Zero dependencies**: No external blockchain SDKs required
- **Performance**: High-speed signing and verification

### Threat Model

The signing mechanism provides **tamper-evidence**, not authorization.

-   **Protects Against**:
    -   Accidental corruption (disk errors, partial writes).
    -   AI Hallucination (AI claiming it ran a command it simulated).
    -   Process crashes mid-write.
-   **Does NOT Protect Against**:
    -   Malicious agents with shell access (if the AI can read the key, it can sign).
    -   Root users/Admins.

### Key Management

- **Master Key**: Stored in `~/.atlas/keys/master.key`
- **Permissions**: `0600` (read/write only by owner) to prevent casual snooping.
- **Format**: Raw 32-byte Ed25519 private key (encoded as hex).

### Implementation

```go
// internal/crypto/native/signer.go

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
)

// Signer uses standard Ed25519 for signing
type Signer struct {
	privKey ed25519.PrivateKey
}


const (
    // BIP44 path components
    ATLASPurpose  = 44
    ATLASCoinType = 236 // "AT" in ASCII = 65+84 = 149, but 236 looks cooler

    // Account indices
    AccountReceipts = 0
)

// KeyManager handles HD key derivation for ATLAS
type KeyManager struct {
    masterKey *bip32.ExtendedKey
    keyPath   string
}

// NewKeyManager creates or loads the master key
func NewKeyManager() (*KeyManager, error) {
    keyDir := filepath.Join(os.Getenv("HOME"), ".atlas", "keys")
    masterPath := filepath.Join(keyDir, "master.key")

    var masterKey *bip32.ExtendedKey

    if _, err := os.Stat(masterPath); os.IsNotExist(err) {
        // Generate new master key
        seed, err := bip32.GenerateSeed(32) // 256 bits
        if err != nil {
            return nil, fmt.Errorf("generating seed: %w", err)
        }

        masterKey, err = bip32.NewMaster(seed)
        if err != nil {
            return nil, fmt.Errorf("creating master key: %w", err)
        }

        // Save encrypted (TODO: add passphrase encryption)
        if err := os.MkdirAll(keyDir, 0700); err != nil {
            return nil, err
        }
        if err := os.WriteFile(masterPath, []byte(masterKey.String()), 0600); err != nil {
            return nil, err
        }
    } else {
        // Load existing master key
        data, err := os.ReadFile(masterPath)
        if err != nil {
            return nil, err
        }
        masterKey, err = bip32.NewKeyFromString(string(data))
        if err != nil {
            return nil, fmt.Errorf("parsing master key: %w", err)
        }
    }

    return &KeyManager{masterKey: masterKey}, nil
}

// DeriveReceiptKey derives a key for signing a specific receipt
func (km *KeyManager) DeriveReceiptKey(taskIndex, receiptIndex uint32) (*ec.PrivateKey, string, error) {
    // Path: m/44'/236'/0'/taskIndex/receiptIndex
    path := fmt.Sprintf("m/%d'/%d'/%d'/%d/%d",
        ATLASPurpose, ATLASCoinType, AccountReceipts, taskIndex, receiptIndex)

    // Derive the key
    derived, err := km.masterKey.DeriveChildFromPath(path)
    if err != nil {
        return nil, "", fmt.Errorf("deriving key at %s: %w", path, err)
    }

    privKey, err := derived.ECPrivKey()
    if err != nil {
        return nil, "", fmt.Errorf("extracting private key: %w", err)
    }

    return privKey, path, nil
}

// SignReceipt signs a validation receipt
func (km *KeyManager) SignReceipt(receipt *ValidationReceipt, taskIndex, receiptIndex uint32) error {
    privKey, keyPath, err := km.DeriveReceiptKey(taskIndex, receiptIndex)
    if err != nil {
        return err
    }

    // Create message to sign
    message := fmt.Sprintf("%s|%s|%d|%s|%s|%d",
        receipt.ReceiptID,
        receipt.Command,
        receipt.ExitCode,
        receipt.StdoutHash,
        receipt.StderrHash,
        receipt.CompletedAt.Unix(),
    )

    // Sign using BSV ECDSA
    msgHash := sha256.Sum256([]byte(message))
    signature, err := privKey.Sign(msgHash[:])
    if err != nil {
        return fmt.Errorf("signing: %w", err)
    }

    receipt.KeyPath = keyPath
    receipt.Signature = hex.EncodeToString(signature.Serialize())

    return nil
}

// VerifyReceipt verifies a receipt signature
func (km *KeyManager) VerifyReceipt(receipt *ValidationReceipt) (bool, error) {
    // Re-derive the public key from path
    derived, err := km.masterKey.DeriveChildFromPath(receipt.KeyPath)
    if err != nil {
        return false, err
    }

    pubKey, err := derived.ECPubKey()
    if err != nil {
        return false, err
    }

    // Recreate message
    message := fmt.Sprintf("%s|%s|%d|%s|%s|%d",
        receipt.ReceiptID,
        receipt.Command,
        receipt.ExitCode,
        receipt.StdoutHash,
        receipt.StderrHash,
        receipt.CompletedAt.Unix(),
    )

    // Verify signature
    msgHash := sha256.Sum256([]byte(message))
    sigBytes, err := hex.DecodeString(receipt.Signature)
    if err != nil {
        return false, err
    }

    sig, err := ec.SignatureFromDER(sigBytes)
    if err != nil {
        return false, err
    }

    return sig.Verify(msgHash[:], pubKey), nil
}
```

### Key Storage

```
~/.atlas/
├── keys/
│   ├── master.key      # HD master key (600 permissions)
│   └── master.key.bak  # Backup (created on first use)
└── ...
```

**Security Notes:**
- Master key file has 0600 permissions (owner read/write only)
- Consider adding passphrase encryption for production
- Backup the master.key file - loss means receipts can't be verified

---

## Hook File Cleanup

Hook files are lightweight and provide valuable audit history. The cleanup policy balances disk space with debugging utility.

### Cleanup Policy

| Condition | Action | Rationale |
|-----------|--------|-----------|
| Task completed successfully | Keep for 30 days | Audit trail |
| Task abandoned | Keep for 7 days | May want to resume |
| Task failed | Keep for 14 days | Debugging |
| Workspace destroyed | Delete immediately | User explicitly cleaned up |
| Manual `atlas cleanup` | Delete per flags | User-controlled |

### Implementation

```go
// internal/hooks/cleanup.go

type CleanupPolicy struct {
    CompletedRetention  time.Duration `yaml:"completed_retention"`  // Default: 30 days
    AbandonedRetention  time.Duration `yaml:"abandoned_retention"`  // Default: 7 days
    FailedRetention     time.Duration `yaml:"failed_retention"`     // Default: 14 days
}

func DefaultCleanupPolicy() CleanupPolicy {
    return CleanupPolicy{
        CompletedRetention:  30 * 24 * time.Hour,
        AbandonedRetention:  7 * 24 * time.Hour,
        FailedRetention:     14 * 24 * time.Hour,
    }
}

// CleanupHooks removes old hook files based on policy
func CleanupHooks(workspacesDir string, policy CleanupPolicy) (int, error) {
    cleaned := 0

    // Walk all workspaces
    err := filepath.Walk(workspacesDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() || info.Name() != "hook.json" {
            return nil
        }

        hook, err := LoadHook(path)
        if err != nil {
            return nil // Skip corrupted hooks
        }

        var retention time.Duration
        switch hook.State {
        case HookStateCompleted:
            retention = policy.CompletedRetention
        case HookStateAbandoned:
            retention = policy.AbandonedRetention
        case HookStateFailed:
            retention = policy.FailedRetention
        default:
            return nil // Don't clean active hooks
        }

        if time.Since(hook.UpdatedAt) > retention {
            taskDir := filepath.Dir(path)
            if err := os.Remove(filepath.Join(taskDir, "hook.json")); err == nil {
                os.Remove(filepath.Join(taskDir, "HOOK.md"))
                cleaned++
            }
        }

        return nil
    })

    return cleaned, err
}
```

### CLI Integration

```bash
# Clean up old hooks based on default policy
atlas cleanup --hooks

# Clean up with custom retention
atlas cleanup --hooks --retention 7d

# Dry run - show what would be cleaned
atlas cleanup --hooks --dry-run

# Clean everything (workspace destroy does this)
atlas workspace destroy my-workspace  # Deletes hooks too
```

---

## AI Agent Integration

The AI reads `HOOK.md` via a lightweight instruction in CLAUDE.md. This keeps the prompt small while ensuring recovery works.

### CLAUDE.md Addition

Add this section to your project's CLAUDE.md:

```markdown
## ATLAS Task Recovery

**Before starting any ATLAS task work**, check for an active hook:

```bash
# Check if there's a recovery hook
ls ~/.atlas/workspaces/*/tasks/*/HOOK.md 2>/dev/null | head -1
```

If a HOOK.md file exists:
1. Read the ENTIRE file with `cat <path>`
2. Follow the "What To Do Now" section EXACTLY
3. Do NOT restart from the beginning
4. Do NOT repeat completed steps (check the table)

The hook file is the source of truth for task state. Your memory may be stale or empty after a crash - the hook file is not.
```

### Alternative: Synopsis Injection

For even lighter integration, ATLAS can inject a one-line synopsis into the AI prompt:

```go
// internal/hooks/synopsis.go

// GenerateSynopsis creates a one-line recovery hint
func (h *Hook) GenerateSynopsis() string {
    if h.State == HookStateCompleted || h.State == HookStateAbandoned {
        return "" // No active task
    }

    return fmt.Sprintf(
        "[ATLAS RECOVERY] Task '%s' in progress. State: %s, Step: %s (%d/%d). "+
        "READ ~/.atlas/workspaces/%s/tasks/%s/HOOK.md BEFORE PROCEEDING.",
        h.TaskID,
        h.State,
        h.CurrentStep.StepName,
        h.CurrentStep.StepIndex+1,
        h.getTotalSteps(),
        h.getWorkspaceName(),
        h.TaskID,
    )
}
```

This synopsis can be prepended to the AI's context when ATLAS detects an active hook.

---

## Integration Points

### 1. ATLAS CLI Integration

```go
// cmd/atlas/start.go
func (c *StartCommand) Execute(ctx context.Context) error {
    // ... existing setup ...

    // Create hook after workspace setup
    hook, err := hooks.Create(ctx, task.ID, task.Template)
    if err != nil {
        return fmt.Errorf("creating hook: %w", err)
    }

    // Set up auto-checkpointing
    hook.SetupGitHooks(workspace.RepoPath)

    // Start interval checkpointer
    checkpointer := hooks.NewIntervalCheckpointer(hook, 5*time.Minute)
    checkpointer.Start()
    defer checkpointer.Stop()

    // Transition: init -> initializing -> step_pending
    if err := hook.TransitionTo(hooks.StateStepPending); err != nil {
        return fmt.Errorf("hook transition: %w", err)
    }

    // ... continue with AI execution ...
}
```

### 2. Validation Receipt Generation

```go
// internal/validation/receipt.go

func GenerateReceipt(cmd string, result *ExecResult, taskIndex, receiptIndex uint32) (*ValidationReceipt, error) {
    receipt := &ValidationReceipt{
        ReceiptID:   generateReceiptID(),
        Command:     cmd,
        ExitCode:    result.ExitCode,
        StartedAt:   result.StartedAt,
        CompletedAt: result.CompletedAt,
        Duration:    result.Duration.String(),
        StdoutHash:  sha256Hash(result.Stdout),
        StderrHash:  sha256Hash(result.Stderr),
    }

    // Sign with HD-derived key
    keyManager, err := crypto.NewKeyManager()
    if err != nil {
        return nil, fmt.Errorf("loading key manager: %w", err)
    }

    if err := keyManager.SignReceipt(receipt, taskIndex, receiptIndex); err != nil {
        return nil, fmt.Errorf("signing receipt: %w", err)
    }

    return receipt, nil
}
```

### 3. Recovery Detection

```go
// internal/hooks/recovery.go
func DetectAndRecover(ctx context.Context, workspaceDir string) (*Hook, error) {
    hookPath := filepath.Join(workspaceDir, "hook.json")

    hook, err := LoadHook(hookPath)
    if err != nil {
        return nil, nil // No hook = no recovery needed
    }

    // Check if hook indicates incomplete work
    if hook.State == HookStateCompleted || hook.State == HookStateAbandoned {
        return nil, nil // Task finished, no recovery
    }

    // Check for stale state (no update in N minutes)
    staleDuration := 5 * time.Minute
    if time.Since(hook.UpdatedAt) < staleDuration {
        return hook, nil // Still fresh, might be running
    }

    // Crash detected - populate recovery context
    hook.Recovery = &RecoveryContext{
        DetectedAt:       time.Now(),
        CrashType:        detectCrashType(hook),
        LastKnownState:   hook.State,
        LastCheckpointID: findLastCheckpoint(hook),
    }

    hook.Recovery.RecommendedAction = determineRecoveryAction(hook)
    hook.Recovery.Reason = explainRecoveryReason(hook)

    hook.transitionTo(HookStateRecovering)
    hook.regenerateHookMD()

    return hook, nil
}

func determineRecoveryAction(h *Hook) string {
    // If we have a recent checkpoint, always retry from there
    if h.Recovery.LastCheckpointID != "" {
        return "retry_step"
    }

    switch h.State {
    case HookStateStepRunning:
        if isIdempotentStep(h.CurrentStep.StepName) {
            return "retry_step"
        }
        return "manual" // Human needs to verify

    case HookStateStepValidating:
        return "retry_validation"

    case HookStateAwaitingHuman:
        return "await_human"

    default:
        return "retry_step"
    }
}
```

---

## CLI Commands

### New Commands

```bash
# Show hook status
atlas hook status
# Output: Current state, step, checkpoint info

# Create manual checkpoint
atlas checkpoint "description of progress"
# Creates checkpoint in current step

# Create checkpoint with specific trigger (internal use)
atlas checkpoint --trigger git_commit --auto "Commit: message"

# Force hook regeneration (if HOOK.md is corrupted)
atlas hook regenerate

# Verify receipt signature
atlas hook verify-receipt <receipt-id>
# Output: VALID or INVALID with details

# Export hook history for debugging
atlas hook export --format json > hook-debug.json

# List all checkpoints
atlas hook checkpoints
# Output: Table of all checkpoints with triggers

# Clean up old hooks
atlas cleanup --hooks
atlas cleanup --hooks --retention 7d --dry-run
```

### Modified Commands

```bash
# atlas start - now creates hook
atlas start "task description" --template bugfix
# Creates hook.json and HOOK.md, sets up auto-checkpointing

# atlas resume - reads hook first
atlas resume
# Checks for HOOK.md, follows recovery instructions

# atlas status - shows hook state
atlas status
# Now includes: Hook State: step_running (implement, attempt 2)
# Shows: Last Checkpoint: ckpt-a1b2c3d4 (2 min ago)

# atlas abandon - updates hook
atlas abandon
# Transitions hook to abandoned state
```

---

## Usage Examples

### Example 1: Normal Task Flow with Auto-Checkpoints

```bash
# Human starts task
atlas start "fix null pointer in config" --template bugfix

# ATLAS creates hook.json, HOOK.md, sets up git hooks
# State: initializing -> step_pending

# AI performs analysis, completes step
# Auto-checkpoint created (trigger: step_complete)

# AI commits initial implementation
# Auto-checkpoint created (trigger: git_commit)

# AI continues work, 5 minutes pass
# Auto-checkpoint created (trigger: interval)

# AI pushes to remote
# Auto-checkpoint created (trigger: git_push)

# AI creates PR
# Auto-checkpoint created (trigger: pr_created)
# Validation runs, receipt signed with HD key
# State: completed
```

### Example 2: Crash Recovery with Checkpoints

```bash
# AI is implementing a fix
# State: step_running (step: implement, attempt: 1)
# Has 3 checkpoints: step_complete, git_commit, interval

# ** CRASH ** (Claude Code disconnects)

# Human restarts Claude Code
# ATLAS detects stale hook (no update in 5+ minutes)
# Finds last checkpoint: ckpt-xyz (interval, 3 min ago)
# State: step_running -> recovering

# HOOK.md is regenerated:
# "Resume from checkpoint ckpt-xyz. You were adding nil checks..."

# AI reads HOOK.md, continues from checkpoint
# State: recovering -> step_running
```

### Example 3: Receipt Verification

```bash
# After task completion, verify all receipts
atlas hook verify-receipt rcpt-001

# Output:
# Receipt: rcpt-001
# Step: analyze
# Command: magex lint
# Exit Code: 0
# Key Path: m/44'/236'/0'/0/0
# Signature: VALID ✓
#
# This receipt was signed by your ATLAS installation.
# The validation command ran and completed successfully.
```

---

## Testing Strategy

### Unit Tests

```go
func TestHookStateTransitions(t *testing.T) {
    tests := []struct {
        name      string
        fromState HookState
        event     string
        toState   HookState
        wantErr   bool
    }{
        {"init to initializing", "", "init", HookStateInitializing, false},
        {"initializing to step_pending", HookStateInitializing, "setup_complete", HookStateStepPending, false},
        {"invalid transition", HookStateCompleted, "start_step", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            hook := &Hook{State: tt.fromState}
            err := hook.handleEvent(tt.event)
            if (err != nil) != tt.wantErr {
                t.Errorf("handleEvent() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && hook.State != tt.toState {
                t.Errorf("State = %v, want %v", hook.State, tt.toState)
            }
        })
    }
}

func TestAutoCheckpoints(t *testing.T) {
    hook := createTestHook(t)
    hook.CurrentStep = &StepContext{StepName: "implement", StepIndex: 2}

    // Test each trigger type
    triggers := []CheckpointTrigger{
        CheckpointTriggerCommit,
        CheckpointTriggerPush,
        CheckpointTriggerValidation,
    }

    for _, trigger := range triggers {
        err := hook.CreateAutoCheckpoint(trigger, "Test checkpoint")
        require.NoError(t, err)
    }

    assert.Len(t, hook.Checkpoints, 3)
    assert.Equal(t, CheckpointTriggerCommit, hook.Checkpoints[0].Trigger)
}

func TestReceiptSigning(t *testing.T) {
    km, err := crypto.NewKeyManager()
    require.NoError(t, err)

    receipt := &ValidationReceipt{
        ReceiptID:   "test-001",
        Command:     "go test",
        ExitCode:    0,
        StdoutHash:  "abc123",
        StderrHash:  "def456",
        CompletedAt: time.Now(),
    }

    // Sign
    err = km.SignReceipt(receipt, 0, 0)
    require.NoError(t, err)
    assert.NotEmpty(t, receipt.Signature)
    assert.NotEmpty(t, receipt.KeyPath)

    // Verify
    valid, err := km.VerifyReceipt(receipt)
    require.NoError(t, err)
    assert.True(t, valid)

    // Tamper and verify fails
    receipt.ExitCode = 1
    valid, err = km.VerifyReceipt(receipt)
    require.NoError(t, err)
    assert.False(t, valid)
}
```

### Integration Tests

```go
func TestCrashRecoveryWithCheckpoint(t *testing.T) {
    workspace := setupTestWorkspace(t)
    hook := createTestHook(t, workspace)

    // Advance to step_running with checkpoint
    hook.transitionTo(HookStateStepRunning)
    hook.CurrentStep = &StepContext{StepName: "implement", Attempt: 1}
    hook.CreateAutoCheckpoint(CheckpointTriggerCommit, "Initial commit")
    hook.save()

    // Simulate crash
    hook.UpdatedAt = time.Now().Add(-10 * time.Minute)
    hook.save()

    // Test recovery
    recovered, err := DetectAndRecover(context.Background(), workspace)
    require.NoError(t, err)
    require.NotNil(t, recovered)

    assert.Equal(t, HookStateRecovering, recovered.State)
    assert.Equal(t, "retry_step", recovered.Recovery.RecommendedAction)
    assert.NotEmpty(t, recovered.Recovery.LastCheckpointID)
}
```

---

## Migration Plan

### Phase 1: Core Implementation
- [ ] Implement `Hook` struct and state machine
- [ ] Implement `hook.json` persistence
- [ ] Implement `HOOK.md` generation
- [ ] Add hook creation to `atlas start`
- [ ] Implement manual checkpoint command

### Phase 2: Auto-Checkpoints
- [ ] Implement auto-checkpoint triggers
- [ ] Set up git hook integration
- [ ] Implement interval checkpointer
- [ ] Add file snapshot capability

### Phase 3: Cryptographic Signing
- [ ] Integrate go-sdk for HD keys
- [ ] Implement KeyManager
- [ ] Implement receipt signing
- [ ] Implement receipt verification
- [ ] Add `atlas hook verify-receipt` command

### Phase 4: Recovery & Cleanup
- [ ] Implement crash detection
- [ ] Implement recovery recommendations
- [ ] Add cleanup policy and commands
- [ ] Update CLAUDE.md instructions

### Phase 5: Testing & Documentation
- [ ] Unit tests for state machine, checkpoints, and signing
- [ ] Integration tests for crash recovery scenarios
- [ ] End-to-end tests for full task lifecycle with hooks
- [ ] Update `quick-start.md` with Hook Files documentation:
  - [ ] Add "Hook Files" section under CLI Commands Reference
  - [ ] Document `atlas hook` subcommands (status, regenerate, verify-receipt, checkpoints, export)
  - [ ] Document `atlas checkpoint` command
  - [ ] Add hook states to Task States section
  - [ ] Update File Structure section with hook.json and HOOK.md
  - [ ] Add Hook Files troubleshooting entries
  - [ ] Document auto-checkpoint triggers and configuration
  - [ ] Add HD key setup to Installation/Prerequisites if needed
- [ ] Create standalone `docs/hook-files.md` deep-dive reference
- [ ] Update README with hook files feature summary

---

## Appendix: Dependencies

### Required Go Packages

```go
// go.mod additions
require (
    github.com/bsv-blockchain/go-sdk v1.x.x  // HD keys, ECDSA signing
)
```

### File Permissions

| File | Permissions | Rationale |
|------|-------------|-----------|
| `~/.atlas/keys/` | 0700 | Directory for sensitive keys |
| `~/.atlas/keys/master.key` | 0600 | HD master key (owner only) |
| `hook.json` | 0644 | Task state (readable) |
| `HOOK.md` | 0644 | Recovery instructions (readable) |

---

*End of Specification*
