// Package contracts defines the CLI command contracts for the Hook system.
// These commands will be implemented in internal/cli/.
//
// This file is a design artifact documenting the CLI interface.
package contracts

// CLI Commands for Hook System
//
// New commands:
//   - atlas hook status
//   - atlas hook checkpoints
//   - atlas hook verify-receipt <receipt-id>
//   - atlas hook regenerate
//   - atlas hook export
//   - atlas checkpoint "description"
//
// Modified commands:
//   - atlas start (creates hook)
//   - atlas resume (reads hook for recovery)
//   - atlas cleanup --hooks

// HookStatusCommand displays the current hook state.
//
// Usage: atlas hook status [--json|--yaml]
//
// Output (text):
//   Hook State: step_running
//   Task: task-20260117-143022 (fix-null-pointer)
//   Step: implement (3/7), Attempt 2/3
//   Last Updated: 2 minutes ago
//   Last Checkpoint: ckpt-a1b2c3d4 (git_commit, 5 min ago)
//   Receipts: 2 (all valid)
//
// Output (json):
//   { "state": "step_running", "task_id": "...", ... }
//
// Exit codes:
//   0: Success
//   1: No active hook found
//   2: Hook in error state
type HookStatusCommand struct{}

// HookCheckpointsCommand lists all checkpoints for the current task.
//
// Usage: atlas hook checkpoints [--json|--yaml]
//
// Output (text):
//   Checkpoints for task-20260117-143022:
//
//   | Time       | Trigger       | Description                      |
//   |------------|---------------|----------------------------------|
//   | 14:42:15   | git_commit    | Added nil check for Server field |
//   | 14:38:22   | step_complete | Plan complete                    |
//   | 14:35:00   | step_complete | Analysis complete                |
//
// Exit codes:
//   0: Success (may have 0 checkpoints)
//   1: No active hook found
type HookCheckpointsCommand struct{}

// HookVerifyReceiptCommand verifies a validation receipt signature.
//
// Usage: atlas hook verify-receipt <receipt-id>
//
// Output (text):
//   Receipt: rcpt-00000001
//   Step: analyze
//   Command: magex lint
//   Exit Code: 0
//   Duration: 12.3s
//   Key Path: m/44'/236'/0'/0/0
//   Signature: VALID ✓
//
// Exit codes:
//   0: Signature valid
//   1: Receipt not found
//   2: Signature invalid
//   3: Key manager error (missing master key)
type HookVerifyReceiptCommand struct{}

// HookRegenerateCommand regenerates HOOK.md from hook.json.
//
// Usage: atlas hook regenerate
//
// Use case: When HOOK.md is corrupted or manually edited incorrectly.
//
// Output: "Regenerated HOOK.md from hook.json"
//
// Exit codes:
//   0: Success
//   1: No active hook found
//   2: Failed to regenerate
type HookRegenerateCommand struct{}

// HookExportCommand exports hook history for debugging.
//
// Usage: atlas hook export [--format json|yaml]
//
// Output: Full hook.json content to stdout
//
// Exit codes:
//   0: Success
//   1: No active hook found
type HookExportCommand struct{}

// CheckpointCommand creates a manual checkpoint.
//
// Usage: atlas checkpoint "description"
//
// Output: "Created checkpoint ckpt-a1b2c3d4: description"
//
// Prerequisites:
//   - Active task in step_running state
//
// Exit codes:
//   0: Success
//   1: No active task
//   2: Task not in checkpointable state
type CheckpointCommand struct{}

// CleanupHooksFlag adds --hooks support to atlas cleanup.
//
// Usage: atlas cleanup --hooks [--retention 30d] [--dry-run]
//
// Behavior:
//   - Removes hook files from tasks in terminal states
//   - Respects retention policy (from config, see data-model.md):
//     - completed: 30 days (720h)
//     - failed: 7 days (168h)
//     - abandoned: 7 days (168h)
//
// Output:
//   Cleaned up 5 hook files (dry-run would show: "Would clean up 5 hook files")
//
// Exit codes:
//   0: Success
//   1: Error during cleanup
type CleanupHooksFlag struct{}

// StartCommandModification documents changes to atlas start.
//
// Modified behavior:
//   1. After task creation, create hook.json in task directory
//   2. Transition hook: "" -> initializing -> step_pending
//   3. Set up git hook wrappers in repository (if not already installed)
//   4. Start interval checkpointer goroutine
//
// New flags: none (hook creation is automatic)
type StartCommandModification struct{}

// ResumeCommandModification documents changes to atlas resume.
//
// Modified behavior:
//   1. Before resuming, check for HOOK.md in task directory
//   2. If hook exists and state is "recovering", follow recovery recommendations
//   3. If hook is stale (>5 min since update), trigger recovery detection
//   4. Display recovery context to user before proceeding
//
// New output section:
//   ⚠️  Recovery Mode
//   Last state: step_running (implement, attempt 2)
//   Last checkpoint: ckpt-a1b2c3d4 (5 min ago)
//   Recommendation: Resume from checkpoint
//
//   Continue? [Y/n]
type ResumeCommandModification struct{}
