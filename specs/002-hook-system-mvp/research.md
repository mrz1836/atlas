# Research: Hook System for Crash Recovery

**Feature**: 002-hook-system-mvp
**Date**: 2026-01-17
**Status**: Complete

## Research Tasks

This document resolves all technical unknowns identified during planning.

---

## 1. HD Key Derivation Library

### Decision
Use `github.com/bsv-blockchain/go-sdk` for HD (Hierarchical Deterministic) key derivation and ECDSA signing, **behind an interface** for future swappability.

### Rationale
- **Battle-tested implementation**: BSV SDK is production-grade with extensive usage
- **BIP32/BIP44 compliant**: Follows industry standards for key derivation paths
- **Pure Go**: No CGO dependencies, simplifies builds and cross-compilation
- **Active maintenance**: Regular updates and security patches
- **Referenced in original spec**: Design document explicitly specifies this library

### Interface Abstraction

The `go-sdk` dependency is encapsulated behind `ReceiptSigner` and `KeyManager` interfaces (see data-model.md). This allows:
- Swapping to a different crypto library without changing hook business logic
- Easier testing with mock implementations
- Future migration if `go-sdk` becomes unmaintained

```
hook/ package → ReceiptSigner interface → HDReceiptSigner (go-sdk)      [current]
                                        → BitcoinSigner (go-bitcoin)   [alternative]
                                        → MockSigner (tests)
```

### Alternatives Considered

| Library | Status |
|---------|--------|
| `github.com/BitcoinSchema/go-bitcoin` | **Viable alternative** - can swap to this via interface if needed |
| `btcsuite/btcutil/hdkeychain` | Rejected: Bitcoin-specific, less active maintenance |
| `tyler-smith/go-bip32` | Rejected: Minimal features, no ECDSA signing built-in |
| Standard library only | Rejected: Would require implementing BIP32 from scratch |

**Note**: `go-bitcoin` is the primary alternative if `go-sdk` needs to be replaced. The `ReceiptSigner` interface ensures only the implementation file changes - no modifications to hook business logic.

### Key Path Structure
```
m/44'/236'/0'/{task_index}/{receipt_index}
```
- `44'` = BIP44 purpose
- `236'` = ATLAS coin type (arbitrary choice from spec)
- `0'` = Receipt signing account
- `{task_index}` = Incremental task counter (non-hardened for performance)
- `{receipt_index}` = Receipt within task (non-hardened)

**Simplification from spec**: Use two-level derivation (master → task key) where all receipts in a task share the same derived key. This reduces complexity while maintaining security since tasks are short-lived.

---

## 2. Git Hook Integration Pattern

### Decision
Use wrapper script approach that chains to existing hooks.

### Rationale
- **Non-destructive**: Preserves user's existing git hooks
- **Discoverable**: Users can inspect the wrapper scripts
- **Follows constitution**: "Text is Truth" - hooks are readable shell scripts
- **Precedent**: go-pre-commit uses similar pattern

### Implementation Pattern
```bash
#!/bin/sh
# ATLAS auto-checkpoint hook - chains to original hook if exists

# Run ATLAS checkpoint
atlas checkpoint --trigger git_commit --auto "Commit: $(git log -1 --format=%s)" 2>/dev/null || true

# Chain to original hook if it exists
if [ -x ".git/hooks/post-commit.original" ]; then
    exec ".git/hooks/post-commit.original" "$@"
fi
```

### Hooks to Install

| Git Hook | Checkpoint Trigger | When |
|----------|-------------------|------|
| `post-commit` | `git_commit` | After every commit |
| `post-push` (custom) | `git_push` | After push completes |
| `post-checkout` | (none) | Reserved for future |

**Note**: `post-push` is not a standard git hook. We'll use `post-receive` for local tracking or rely on monitoring `git push` command completion from the CLI.

### Alternatives Considered

| Approach | Rejected Because |
|----------|------------------|
| Replace hooks entirely | Destructive to user's existing hooks |
| Core git hooks (global) | Too invasive, affects all repos |
| File watcher for `.git/refs` | Complex, race conditions, platform issues |

---

## 3. Atomic File Write Pattern

### Decision
Use temp file + rename pattern (already established in ATLAS).

### Rationale
- **Crash-safe**: Rename is atomic on POSIX systems
- **Consistent**: Matches existing task.Store and workspace.Store patterns
- **Proven**: Battle-tested pattern in ATLAS codebase

### Implementation Pattern (from existing code)
```go
func atomicWrite(path string, data []byte) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".tmp-*")
    if err != nil {
        return err
    }
    tmpPath := tmp.Name()
    defer os.Remove(tmpPath) // Clean up on failure

    if _, err := tmp.Write(data); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Sync(); err != nil { // Ensure data is on disk
        tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    return os.Rename(tmpPath, path)
}
```

### File Locking
Use existing `internal/flock` package with same timeout settings:
- Lock timeout: 5 seconds
- Retry interval: 50ms

---

## 4. Stale Hook Detection

### Decision
Use timestamp-based detection with 5-minute threshold.

### Rationale
- **Simple**: Just compare file modification time to current time
- **Configurable**: Threshold can be adjusted in config
- **Conservative**: 5 minutes handles slow operations without false positives
- **Matches spec**: Original design specified 5-minute threshold

### Detection Algorithm
```go
func isStaleHook(hook *Hook, cfg *config.HookConfig) bool {
    // Threshold from config (hooks.stale_threshold), default: 5m
    return time.Since(hook.UpdatedAt) > cfg.StaleThreshold &&
           !isTerminalState(hook.State)
}
```

### Edge Cases

| Scenario | Handling |
|----------|----------|
| Long AI step (>5 min) | Interval checkpoints update timestamp, preventing false stale detection |
| System clock skew | Compare to file mtime, not hook.UpdatedAt field |
| Hook in terminal state | Never considered stale |

---

## 5. Checkpoint Pruning Strategy

### Decision
Keep most recent 50 checkpoints, prune oldest when exceeded.

### Rationale
- **Bounded storage**: Prevents unbounded growth
- **Recent history**: Most recent checkpoints are most valuable for recovery
- **Simple**: FIFO pruning is deterministic and predictable
- **Configurable**: 50 is default, can be overridden

### Implementation
```go
func (h *Hook) pruneCheckpoints(cfg *config.HookConfig) {
    // Max from config (hooks.max_checkpoints), default: 50
    if len(h.Checkpoints) <= cfg.MaxCheckpoints {
        return
    }
    h.Checkpoints = h.Checkpoints[len(h.Checkpoints)-cfg.MaxCheckpoints:]
}
```

---

## 6. Recovery Action Determination

### Decision
Use decision tree based on state and checkpoint availability.

### Rationale
- **Deterministic**: Same inputs always produce same recommendation
- **Safe defaults**: When uncertain, recommend manual intervention
- **Auditable**: Decision reasons are recorded in recovery context

### Decision Tree
```
Was hook in terminal state?
  → Yes: No recovery needed
  → No: Continue...

Is there a recent checkpoint (<10 minutes old)?
  → Yes: Recommend retry from checkpoint
  → No: Continue...

Was the crash during validation?
  → Yes: Recommend retry validation (idempotent)
  → No: Continue...

Was the crash during AI step execution?
  → Yes, step is idempotent (analyze, plan): Recommend retry
  → Yes, step modifies files (implement): Recommend manual review
  → No: Continue...

Default: Recommend manual intervention
```

### Idempotent Steps
Steps that can safely be retried without side effects:
- `analyze` - Reading/analyzing code
- `plan` - Creating plans
- `validate` - Running validation (read-only checks)

Non-idempotent steps requiring caution:
- `implement` - Modifies files
- `commit` - Creates git commits
- `pr` - Creates pull requests

---

## 7. HOOK.md Template Structure

### Decision
Use structured Markdown with clear sections and explicit instructions.

### Rationale
- **AI-parseable**: Clear section headers that AI can navigate
- **Human-readable**: Markdown renders nicely, easy to skim
- **Action-oriented**: "What To Do Now" section is prominent
- **Audit-friendly**: History and receipts preserved

### Template Sections
1. **Header**: Task ID, template, timestamps
2. **Current State**: State machine position, step info
3. **What You Were Doing**: Last known activity, files touched
4. **What To Do Now**: Explicit resume/skip/manual instructions
5. **DO NOT**: Clear list of actions to avoid
6. **Completed Steps**: Table with validation receipts
7. **Checkpoint Timeline**: Chronological checkpoint list
8. **Troubleshooting**: Commands for manual intervention

---

## 8. State Machine Synchronization with Task State

### Decision
Hook state tracks step-level recovery context; task state remains authoritative for task lifecycle.

### Rationale
- **Single source of truth**: task.json remains canonical for task status
- **Complementary**: hook.json adds recovery detail without duplicating
- **Independent updates**: Hook can be updated without task file lock

### Relationship
```
task.json (Task)          hook.json (Hook)
─────────────────         ─────────────────
status: running      ←→   state: step_running
current_step: 3      ←→   current_step.step_index: 3
                          current_step.attempt: 2
                          current_step.files_touched: [...]
                          checkpoints: [...]
                          receipts: [...]
```

### Synchronization Points
- Hook state transitions when task state transitions
- Hook adds detail (attempt count, files, checkpoints) that task doesn't track
- On recovery, hook state informs whether to retry or skip

---

## 9. Interval Checkpoint Goroutine Management

### Decision
Use cancellable goroutine with ticker, managed by Engine.

### Rationale
- **Clean shutdown**: Context cancellation stops ticker gracefully
- **Single responsibility**: Checkpointer only creates checkpoints
- **Testable**: Can inject mock time or disable in tests

### Implementation Pattern
```go
type IntervalCheckpointer struct {
    hook   *Hook
    store  HookStore
    cfg    *config.HookConfig // Interval from cfg.CheckpointInterval (default: 5m)
    cancel context.CancelFunc
}

func (ic *IntervalCheckpointer) Start(ctx context.Context) {
    ctx, ic.cancel = context.WithCancel(ctx)
    go func() {
        // Interval from config (hooks.checkpoint_interval)
        ticker := time.NewTicker(ic.cfg.CheckpointInterval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if ic.hook.State == HookStateStepRunning {
                    ic.createCheckpoint()
                }
            }
        }
    }()
}

func (ic *IntervalCheckpointer) Stop() {
    if ic.cancel != nil {
        ic.cancel()
    }
}
```

---

## 10. Signature Message Format

### Decision
Use pipe-delimited string with sorted fields for deterministic signing.

### Rationale
- **Deterministic**: Same receipt always produces same message to sign
- **Human-readable**: Can inspect signed data easily
- **Compact**: No JSON parsing overhead

### Format
```
{receipt_id}|{command}|{exit_code}|{stdout_hash}|{stderr_hash}|{completed_at_unix}
```

### Example
```
rcpt-001|magex lint|0|a1b2c3d4e5f6...|def456789abc...|1737129933
```

---

## Summary of Key Decisions

| Area | Decision | Impact |
|------|----------|--------|
| Crypto library | go-sdk (BSV) | New dependency in go.mod |
| Git hooks | Wrapper approach | Non-destructive, chains existing hooks |
| Atomic writes | Temp+rename | Consistent with existing ATLAS patterns |
| Stale detection | `hooks.stale_threshold` (default: 5m) | Configurable in config.yaml |
| Checkpoint limit | `hooks.max_checkpoints` (default: 50), FIFO prune | Configurable in config.yaml |
| Checkpoint interval | `hooks.checkpoint_interval` (default: 5m) | Configurable in config.yaml |
| Key derivation path | `hooks.key_derivation.*` | Configurable in config.yaml |
| Retention periods | `hooks.retention.*` | Configurable per terminal state |
| Recovery actions | Decision tree | Deterministic, auditable |
| HOOK.md format | Structured Markdown | AI and human readable |
| State relationship | Hook complements task | No duplication, clear ownership |
| Interval checkpoints | Cancellable goroutine | Clean lifecycle management |
| Signature format | Pipe-delimited | Deterministic, inspectable |

> **Note**: All configurable values are stored in `~/.atlas/config.yaml` under the `hooks` key. See `data-model.md` for the full `HookConfig` schema.
