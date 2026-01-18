# Hook System Quick Start

**Feature**: 002-hook-system-mvp
**Status**: Design Phase

> This document will be integrated into `docs/internal/quick-start.md` after implementation.

## Overview

The Hook System provides crash-resistant context persistence for ATLAS tasks. When Claude Code crashes mid-task, you can resume exactly where you left off without losing work.

## How It Works

1. **Automatic**: When you start a task, ATLAS creates a hook file that tracks progress
2. **Checkpoints**: Progress is saved automatically on git commits, validation passes, and periodically
3. **Recovery**: If a crash occurs, the hook file tells you (and the AI) exactly what to do next

## Key Concepts

### Hook Files

Two files are created in your task directory:

| File | Purpose |
|------|---------|
| `hook.json` | Machine-readable state (source of truth) |
| `HOOK.md` | Human-readable recovery guide |

Location: `~/.atlas/workspaces/<name>/tasks/<id>/`

### Hook States

| State | Meaning |
|-------|---------|
| `initializing` | Task setup in progress |
| `step_pending` | Ready to start next step |
| `step_running` | AI executing a step |
| `step_validating` | Validation in progress |
| `awaiting_human` | Needs human action |
| `recovering` | Crash recovery in progress |
| `completed` | Task finished successfully |
| `failed` | Task failed permanently |
| `abandoned` | User abandoned task |

### Checkpoints

Checkpoints are automatically created when:
- You commit code (`git commit`)
- You push to remote (`git push`)
- A PR is created
- Validation passes
- A step completes
- Every 5 minutes during long steps

## CLI Commands

### View Hook Status

```bash
# See current hook state
atlas hook status

# Output:
# Hook State: step_running
# Task: task-20260117-143022 (fix-null-pointer)
# Step: implement (3/7), Attempt 2/3
# Last Updated: 2 minutes ago
# Last Checkpoint: ckpt-a1b2c3d4 (git_commit, 5 min ago)
```

### List Checkpoints

```bash
# See all checkpoints for current task
atlas hook checkpoints

# Output:
# | Time     | Trigger       | Description                      |
# |----------|---------------|----------------------------------|
# | 14:42:15 | git_commit    | Added nil check for Server field |
# | 14:38:22 | step_complete | Plan complete                    |
```

### Create Manual Checkpoint

```bash
# Save current progress with description
atlas checkpoint "halfway through refactor"

# Output:
# Created checkpoint ckpt-e5f6g7h8: halfway through refactor
```

### Verify Validation Receipt

```bash
# Verify a receipt's cryptographic signature
atlas hook verify-receipt rcpt-00000001

# Output:
# Receipt: rcpt-00000001
# Step: analyze
# Command: magex lint
# Exit Code: 0
# Signature: VALID ✓
```

### Regenerate Recovery File

```bash
# If HOOK.md gets corrupted, regenerate from hook.json
atlas hook regenerate
```

### Export Hook for Debugging

```bash
# Export full hook state as JSON
atlas hook export > hook-debug.json
```

### Clean Up Old Hooks

```bash
# Remove hooks from completed/abandoned tasks
atlas cleanup --hooks

# Dry run to see what would be cleaned
atlas cleanup --hooks --dry-run

# Custom retention (default: 30 days for completed)
atlas cleanup --hooks --retention 7d
```

## Recovery Workflow

### Scenario: Claude Code Crashes Mid-Task

1. **Restart Claude Code** - The AI loses all memory

2. **AI reads HOOK.md** - The recovery file tells it:
   - What state the task was in
   - What step was running
   - What files were modified
   - What to do next
   - What NOT to do (completed steps)

3. **Resume** - The AI continues from exactly where it left off

### Example HOOK.md

```markdown
# ATLAS Task Recovery Hook

> ⚠️ **READ THIS FIRST** - This file contains your recovery context.

## Current State: `step_running`

**Task:** fix-null-pointer-config
**Step:** `implement` (step 3 of 7)
**Attempt:** 2 of 3

---

## What To Do Now

### ✅ RESUME: Continue the `implement` step

1. Review the files you modified: `config/parser.go`
2. Check git status to see uncommitted changes
3. Continue adding nil checks

### ❌ DO NOT:
- Start over from the beginning
- Re-analyze the issue (already done)
- Recreate the branch (exists: `fix/fix-null-pointer`)

---

## Completed Steps (DO NOT REPEAT)

| Step | Status | Receipt |
|------|--------|---------|
| 1. analyze | ✅ Completed | `rcpt-001` ✓ |
| 2. plan | ✅ Completed | `rcpt-002` ✓ |
```

## Validation Receipts

Validation receipts are cryptographically signed proofs that validation actually ran. This prevents scenarios where the AI claims validation passed without actually running it.

### What's in a Receipt

- Command that was run
- Exit code
- Duration
- Output hashes (SHA256)
- Cryptographic signature

### Key Storage

The signing key is stored at `~/.atlas/keys/master.key` with restricted permissions (0600).

**Important**: Back up this file. If lost, you cannot verify existing receipts.

## Configuration

Hook behavior can be configured in `.atlas/config.yaml`:

```yaml
hooks:
  # Stale detection threshold (default: 5m)
  stale_threshold: 5m

  # Interval checkpoint frequency (default: 5m, 0 to disable)
  checkpoint_interval: 5m

  # Maximum checkpoints per task (default: 50)
  max_checkpoints: 50

  # Auto-checkpoint triggers
  auto_checkpoint:
    on_commit: true
    on_push: true
    on_pr: true
    on_validation: true
    on_step_complete: true
```

## Cleanup Policy

Hook files are automatically cleaned up based on task state:

| Task State | Retention |
|------------|-----------|
| Completed | 30 days |
| Abandoned | 7 days |
| Failed | 14 days |

Run `atlas cleanup --hooks` to apply the policy manually.

## Troubleshooting

### "No active hook found"

The task doesn't have a hook file. This can happen for:
- Tasks created before hook system was implemented
- Tasks where hook was manually deleted

**Solution**: Resume the task normally; a new hook will be created.

### "Hook is stale"

The hook hasn't been updated in 5+ minutes, indicating a possible crash.

**Solution**: Run `atlas resume` to enter recovery mode.

### "Signature invalid"

A validation receipt's signature doesn't verify.

**Causes**:
- Receipt was tampered with
- Master key was regenerated since receipt was created

**Solution**: Re-run validation to create a new receipt.

### "Key manager error"

The master key file is missing or corrupted.

**Solution**:
- If backup exists, restore `~/.atlas/keys/master.key`
- If no backup, a new key will be generated (old receipts unverifiable)

## Integration with AI Agents

Add this to your project's `CLAUDE.md`:

```markdown
## ATLAS Task Recovery

**Before starting any ATLAS task work**, check for a recovery hook:

```bash
ls ~/.atlas/workspaces/*/tasks/*/HOOK.md 2>/dev/null | head -1
```

If a HOOK.md file exists:
1. Read the ENTIRE file
2. Follow the "What To Do Now" section EXACTLY
3. Do NOT restart from the beginning
4. Do NOT repeat completed steps
```

## Architecture Notes

- Hook state is independent of task state (complementary, not duplicating)
- `hook.json` is the source of truth; `HOOK.md` is generated from it
- File operations use atomic writes (temp + rename pattern)
- State transitions are recorded in append-only history
