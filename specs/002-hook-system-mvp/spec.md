# Feature Specification: Hook System for Crash Recovery & Context Persistence

**Feature Branch**: `002-hook-system-mvp`
**Created**: 2026-01-17
**Status**: Draft
**Input**: User description: "Hook Files provide durable, crash-resistant context persistence for AI-assisted development workflows"

## Problem Statement

When an AI agent (such as Claude Code) crashes mid-task, it loses all working memory. This creates several pain points:

1. **Context Loss**: The human must manually explain what was happening
2. **Validation State Ambiguity**: Unknown whether validation passed, failed, or never completed
3. **Retry Confusion**: AI might repeat completed work or skip incomplete work
4. **Progress Opacity**: Human can't easily see exactly what the AI was doing at failure time

**Core Principle**: The AI agent should be able to crash, restart, lose all memory, and still resume work exactly where it left off by reading a single file. Validation receipts provide tamper-evidence against accidental corruption.
## User Scenarios & Testing *(mandatory)*

### User Story 1 - Resume After Crash (Priority: P1)

As a developer using AI-assisted task automation, I want the system to automatically save my task progress so that when Claude Code crashes, I can resume exactly where I left off without losing work or repeating completed steps.

**Why this priority**: This is the core value proposition. Without crash recovery, all other features are moot.

**Independent Test**: Can be fully tested by starting a multi-step task, simulating a crash mid-step, and verifying the AI resumes from the correct position with accurate context.

**Acceptance Scenarios**:

1. **Given** a task is running in "implement" step (step 3 of 7), **When** Claude Code crashes and restarts, **Then** the system provides a recovery file indicating the exact step, attempt number, and what was being worked on
2. **Given** a crash occurred during step execution, **When** the AI reads the recovery file, **Then** it can continue from the last known state without re-executing completed steps
3. **Given** multiple steps have been completed before crash, **When** recovery occurs, **Then** all completed steps are clearly marked as "do not repeat" with their validation receipts

---

### User Story 2 - Automatic Progress Checkpoints (Priority: P1)

As a developer, I want the system to automatically create checkpoints at key moments (git commits, validation passes, step completions) so that I have reliable recovery points throughout the task lifecycle.

**Why this priority**: Checkpoints are essential for meaningful recovery. Without them, recovery would only work from step boundaries.

**Independent Test**: Can be fully tested by running a task, making commits, and verifying checkpoints are created with appropriate metadata.

**Acceptance Scenarios**:

1. **Given** the AI commits code during a task, **When** the commit completes, **Then** a checkpoint is automatically created with the commit reference and description
2. **Given** validation passes for a step, **When** the validation completes, **Then** a checkpoint is created linking to the validation receipt
3. **Given** 5 minutes pass during a long-running step, **When** the interval elapses, **Then** a periodic checkpoint is created capturing current state
4. **Given** multiple checkpoints exist, **When** viewing task status, **Then** all checkpoints are visible in chronological order with their triggers

---

### User Story 3 - Human-Readable Recovery Context (Priority: P1)

As a developer or AI agent, I want a human-readable recovery file that clearly explains the current state and exactly what to do next, so that recovery is unambiguous and requires no guesswork.

**Why this priority**: The recovery file is the interface between crashes and resumption. It must be crystal clear.

**Independent Test**: Can be fully tested by generating a recovery file and having both a human and AI correctly interpret the next actions.

**Acceptance Scenarios**:

1. **Given** a task is in progress, **When** viewing the recovery file, **Then** it shows current state, step, attempt number, and what was being worked on
2. **Given** recovery is needed, **When** reading the recovery file, **Then** it explicitly states what to do (resume step X) and what NOT to do (don't repeat steps Y, Z)
3. **Given** steps have been completed, **When** viewing the recovery file, **Then** completed steps are shown with their validation receipts for audit

---

### User Story 4 - Validation Receipt Integrity (Priority: P2)

As a developer, I want cryptographically signed validation receipts so that I can trust that validation commands actually ran and weren't fabricated.

**Why this priority**: Trust in validation results is important but secondary to basic recovery functionality.

**Independent Test**: Can be fully tested by generating receipts, verifying signatures, and confirming tampered receipts fail verification.

**Acceptance Scenarios**:

1. **Given** validation runs successfully, **When** the receipt is created, **Then** it includes command, exit code, duration, and output hashes
2. **Given** a receipt exists, **When** verifying its signature, **Then** the system confirms or denies authenticity
3. **Given** a receipt has been tampered with, **When** verification runs, **Then** the system reports the receipt as invalid

---

### User Story 5 - Task State Visibility (Priority: P2)

As a developer, I want to see the current task state, step progress, and checkpoint history at any time so that I have full visibility into what's happening.

**Why this priority**: Visibility supports debugging and oversight but isn't required for core recovery functionality.

**Independent Test**: Can be fully tested by running status commands during various task states and verifying accurate information.

**Acceptance Scenarios**:

1. **Given** a task is running, **When** checking status, **Then** the current state, step, and attempt number are displayed
2. **Given** checkpoints exist, **When** listing checkpoints, **Then** all checkpoints are shown with their triggers and timestamps
3. **Given** validation receipts exist, **When** viewing receipts, **Then** receipt details and signature status are displayed

---

### User Story 6 - Manual Checkpoint Creation (Priority: P3)

As a developer, I want to manually create checkpoints with descriptions so that I can mark important progress points during complex work.

**Why this priority**: Nice-to-have control, but automatic checkpoints cover most needs.

**Independent Test**: Can be fully tested by creating a manual checkpoint and verifying it appears in checkpoint history.

**Acceptance Scenarios**:

1. **Given** a task is in step_running state, **When** creating a manual checkpoint with description "halfway done with refactor", **Then** the checkpoint is saved with that description
2. **Given** manual checkpoints exist, **When** crash recovery occurs, **Then** manual checkpoints are available as recovery points

---

### User Story 7 - Hook File Cleanup (Priority: P3)

As a developer, I want old hook files to be automatically cleaned up based on retention policies so that disk space is managed appropriately.

**Why this priority**: Housekeeping feature that can be deferred.

**Independent Test**: Can be fully tested by creating old hook files and running cleanup, verifying correct files are removed.

**Acceptance Scenarios**:

1. **Given** a completed task's hook is 31 days old, **When** cleanup runs, **Then** the hook files are removed
2. **Given** an abandoned task's hook is 8 days old, **When** cleanup runs, **Then** the hook files are removed
3. **Given** cleanup is run with --dry-run, **When** viewing output, **Then** files that would be deleted are listed without actual deletion

---

### Edge Cases

- What happens when hook.md becomes corrupted or stale? System regenerates from source of truth (hook.json)
- How does system handle crash during checkpoint creation? Checkpoint operations are atomic or recoverable
- What happens when recovery file says "retry step" but files have changed since crash? Recovery context includes file state warnings via snapshots
- How does system handle missing master key for receipt verification? System warns but does not block recovery operations
- What happens when task is abandoned but hook file persists? Cleanup policy handles based on configurable retention period

## Requirements *(mandatory)*

### Functional Requirements

**State Management**

- **FR-001**: System MUST maintain a durable JSON state file (hook.json) that survives process crashes
- **FR-002**: System MUST generate a human-readable Markdown recovery file (hook.md) from the JSON state file
- **FR-003**: System MUST implement an explicit state machine with defined states: initializing, step_pending, step_running, step_validating, awaiting_human, recovering, completed, failed, abandoned
- **FR-004**: System MUST record all state transitions in an append-only history log
- **FR-005**: System MUST support idempotent recovery (running recovery twice produces same result as once)

**Checkpoint System**

- **FR-006**: System MUST automatically create checkpoints on git commit, git push, PR creation, validation pass, and step completion
- **FR-006a**: System MUST install git hooks using a wrapper approach that chains to existing hooks (non-destructive)
- **FR-007**: System MUST support interval-based checkpoints during long-running steps (interval configured via `hooks.checkpoint_interval`, default: 5 minutes)
- **FR-008**: System MUST capture version control state (branch, commit, dirty status) at each checkpoint
- **FR-009**: System MUST support manual checkpoint creation with user-provided descriptions
- **FR-010**: System MUST capture file snapshots (path, size, modification time, hash) for tracked files at checkpoints
- **FR-010a**: System MUST enforce a maximum checkpoint count per task (configured via `hooks.max_checkpoints`, default: 50), pruning oldest when exceeded

**Recovery Context**

- **FR-011**: System MUST detect stale hooks (no update within threshold configured via `hooks.stale_threshold`, default: 5 minutes) as potential crashes
- **FR-012**: System MUST populate recovery context with crash type, last known state, and recommended action
- **FR-013**: System MUST generate clear "What To Do Now" instructions in recovery file
- **FR-014**: System MUST clearly mark completed steps as "DO NOT REPEAT" in recovery file
- **FR-015**: System MUST track files modified during current step for recovery context

**Validation Receipts**

- **FR-016**: System MUST create validation receipts with command, exit code, timestamps, and duration
- **FR-017**: System MUST hash validation output for integrity verification
- **FR-018**: System MUST sign receipts using native Ed25519 keys (stored in `~/.atlas/keys/master.key`)
- **FR-019**: System MUST support receipt signature verification
- **FR-020**: System MUST store signing key securely with restricted file permissions
- **FR-020a**: Cryptographic operations MUST be abstracted behind `ReceiptSigner` and `KeyManager` interfaces to allow swapping implementations without modifying hook business logic

**Cleanup & Maintenance**

- **FR-021**: System MUST implement retention-based cleanup policies per terminal state (configured via `hooks.retention.completed`, `hooks.retention.failed`, `hooks.retention.abandoned`)
- **FR-022**: System MUST support dry-run mode for cleanup operations
- **FR-023**: System MUST support hook file regeneration from state file

### Key Entities

- **Hook**: The primary state container - tracks task ID, current state, current step context, history of transitions, checkpoints, and validation receipts
- **HookState**: Enumeration of all valid state machine positions (initializing, step_pending, step_running, step_validating, awaiting_human, recovering, completed, failed, abandoned)
- **StepContext**: Details about the currently executing step - name, index, attempt number, files touched, working description
- **StepCheckpoint**: A snapshot of progress - ID, timestamp, step reference, trigger type, version control state, file snapshots
- **HookEvent**: An append-only history entry recording state transitions with timestamps and details
- **RecoveryContext**: Crash diagnosis and recovery recommendations - crash type, last state, recommended action, last checkpoint reference
- **ValidationReceipt**: Cryptographic proof of validation execution - command, results, output hashes, signature

### Testing Requirements

All hook system functionality MUST have comprehensive unit tests. This is non-negotiable for a crash recovery system where correctness is critical.

- **TR-001**: Each component (`store`, `state`, `checkpoint`, `recovery`, `markdown`, `signing`) MUST have a corresponding `*_test.go` file
- **TR-002**: State machine transitions MUST have 100% test coverage for all valid and invalid transition paths
- **TR-003**: Cryptographic signing and verification MUST have tests for signing, verification, and tamper detection
- **TR-004**: Recovery logic MUST have tests for each crash type and recovery recommendation
- **TR-005**: All tests MUST pass with race detection enabled (`-race` flag)
- **TR-006**: Minimum 80% line coverage for `internal/hook/` package
- **TR-007**: Integration tests MUST verify end-to-end crash recovery scenarios
- **TR-008**: Test fixtures MUST be provided for common hook states (initializing, running, completed, stale, corrupted)
- **TR-009**: Test keys MUST be generated at runtime using `t.TempDir()` - never commit cryptographic keys to the repository
- **TR-010**: All tests MUST pass `go-pre-commit run --all-files` including gitleaks; use `// gitleaks:allow` inline comments only when unavoidable (e.g., test vector hashes)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Developer can resume any interrupted task within 30 seconds of restart
- **SC-002**: Zero repeated work after crash recovery (all completed steps preserved)
- **SC-003**: Zero skipped work after crash recovery (incomplete work correctly identified)
- **SC-004**: Developer can audit exactly what happened before crash via recovery file and history
- **SC-005**: All validation receipts are cryptographically verifiable
- **SC-006**: Automatic checkpoints created within 1 second of triggering events
- **SC-007**: Recovery file is understandable by both humans and AI agents without ambiguity
- **SC-008**: System works with all existing task templates without modification

## Assumptions

- ATLAS operates as a single-agent system (no multi-agent coordination needed)
- Tasks follow a template-based step structure with defined validation commands
- Version control (git) is available in the workspace for commit/push checkpoint triggers
- The cryptographic key storage location is accessible and secure on the local filesystem
- AI agents will read the recovery file as instructed in project configuration
- Stale hook detection threshold (configured via `hooks.stale_threshold`, default: 5 minutes) is appropriate for identifying crashes

## Dependencies

- Existing ATLAS task management system (task metadata and execution logs)
- Existing ATLAS template system for step definitions
- Standard Go `crypto/ed25519` for cryptographic signing
- Version control hooks capability for automatic checkpoint triggers

## Out of Scope

- Multi-agent coordination
- Real-time sync across machines
- Complex dependency graphs (handled by templates)
- Passphrase encryption for master key (future enhancement)

## Clarifications

### Session 2026-01-17

- Q: Cryptographic signing algorithm? → A: Use go-sdk with HD keys
- Q: HD key derivation path structure? → A: Two-level: master → task (each task gets derived key, receipts share task key); path configured via `hooks.key_derivation` in config
- Q: Hook storage format? → A: JSON state (hook.json) + generated Markdown recovery (hook.md)
- Q: Maximum checkpoint retention count? → A: Configured via `hooks.max_checkpoints` (default: 50), oldest pruned when exceeded
- Q: Git hooks integration approach? → A: Wrapper approach (chain to existing hooks, non-destructive)
- Q: Where are tunable settings stored? → A: All configurable values (timeouts, intervals, key paths, retention) stored in `~/.atlas/config.yaml` under `hooks` key; see data-model.md for full schema
