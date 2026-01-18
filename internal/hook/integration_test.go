package hook_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/crypto/native"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/hook"
)

// Integration tests for the hook system.
// These tests verify end-to-end flows and interactions between components.

// writeHookDirect writes a hook directly to the filesystem without using Store.Save
// which would update the timestamp. This is useful for testing stale detection.
func writeHookDirect(t *testing.T, basePath, taskID string, h *domain.Hook) {
	t.Helper()
	hookPath := filepath.Join(basePath, taskID, constants.HookFileName)

	data, err := json.MarshalIndent(h, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(hookPath, data, 0o600)
	require.NoError(t, err)
}

// TestIntegration_HappyPathRecovery tests the full recovery flow:
// start → run 2 steps → crash → resume at step 3
func TestIntegration_HappyPathRecovery(t *testing.T) {
	t.Parallel()

	// Setup
	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints:     50,
		StaleThreshold:     5 * time.Minute,
		CheckpointInterval: 5 * time.Minute,
	}

	mdGen := hook.NewMarkdownGenerator()
	store := hook.NewFileStore(tempDir, hook.WithMarkdownGenerator(mdGen))
	transitioner := hook.NewTransitioner()
	checkpointer := hook.NewCheckpointer(cfg, store)
	recoveryDetector := hook.NewRecoveryDetector(cfg)

	taskID := "test-task-001"
	workspaceID := "test-workspace"

	// Step 1: Create hook on task start
	h, err := store.Create(ctx, taskID, workspaceID)
	require.NoError(t, err)
	require.NotNil(t, h)
	require.Equal(t, domain.HookStateInitializing, h.State)

	// Transition to step_pending (ready to start)
	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Step 2: Run step 1 (analyze)
	h.CurrentStep = &domain.StepContext{
		StepName:    "analyze",
		StepIndex:   0,
		StartedAt:   time.Now().UTC(),
		Attempt:     1,
		MaxAttempts: 3,
		WorkingOn:   "Analyzing codebase",
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Complete step 1
	err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerStepComplete, "Analysis complete")
	require.NoError(t, err)
	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "step_complete", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Step 3: Run step 2 (plan)
	h.CurrentStep = &domain.StepContext{
		StepName:    "plan",
		StepIndex:   1,
		StartedAt:   time.Now().UTC(),
		Attempt:     1,
		MaxAttempts: 3,
		WorkingOn:   "Creating implementation plan",
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Complete step 2
	err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerStepComplete, "Planning complete")
	require.NoError(t, err)
	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "step_complete", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Step 4: Start step 3 (implement)
	h.CurrentStep = &domain.StepContext{
		StepName:    "implement",
		StepIndex:   2,
		StartedAt:   time.Now().UTC(),
		Attempt:     1,
		MaxAttempts: 3,
		WorkingOn:   "Implementing feature",
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// *** SIMULATE CRASH ***
	// Make hook stale by backdating UpdatedAt
	// We need to modify the file directly since Save() updates the timestamp
	h.UpdatedAt = time.Now().UTC().Add(-10 * time.Minute)
	writeHookDirect(t, tempDir, taskID, h)

	// Step 5: Resume - detect crash and recover
	// Reload hook from store (simulating process restart)
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)

	// Check if recovery is needed
	needsRecovery := recoveryDetector.DetectRecoveryNeeded(ctx, h)
	require.True(t, needsRecovery, "Should detect recovery is needed")

	// Diagnose and recommend
	err = recoveryDetector.DiagnoseAndRecommend(ctx, h)
	require.NoError(t, err)
	require.NotNil(t, h.Recovery)
	// Recovery detector recommends retry_from_checkpoint when there's a recent checkpoint
	// (which there is from step 2), otherwise it would recommend manual for implement step
	require.Equal(t, "retry_from_checkpoint", h.Recovery.RecommendedAction, "should retry from recent checkpoint")
	require.Equal(t, domain.HookStateStepRunning, h.Recovery.LastKnownState)

	// Transition to recovering state
	err = transitioner.TransitionToRecovering(ctx, h, "crash_detected", nil)
	require.NoError(t, err)
	require.Equal(t, domain.HookStateRecovering, h.State)

	// Verify history has all the transitions
	require.GreaterOrEqual(t, len(h.History), 7)

	// Verify checkpoints from steps 1 and 2 are preserved
	require.Len(t, h.Checkpoints, 2)
	require.Equal(t, "Analysis complete", h.Checkpoints[0].Description)
	require.Equal(t, "Planning complete", h.Checkpoints[1].Description)

	// Step 6: Resume from step 3 (implement)
	// After manual review, user decides to continue
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "recovery_continue", nil)
	require.NoError(t, err)
	require.Equal(t, domain.HookStateStepRunning, h.State)
	require.Equal(t, "implement", h.CurrentStep.StepName)
	require.Equal(t, 2, h.CurrentStep.StepIndex, "Should resume at step 3 (index 2)")
}

// TestIntegration_CheckpointRecovery tests checkpoint-based recovery:
// start → checkpoint → crash → resume with checkpoint info
func TestIntegration_CheckpointRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints:     50,
		StaleThreshold:     5 * time.Minute,
		CheckpointInterval: 5 * time.Minute,
	}

	mdGen := hook.NewMarkdownGenerator()
	store := hook.NewFileStore(tempDir, hook.WithMarkdownGenerator(mdGen))
	transitioner := hook.NewTransitioner()
	checkpointer := hook.NewCheckpointer(cfg, store)
	recoveryDetector := hook.NewRecoveryDetector(cfg)

	taskID := "test-task-checkpoint"
	workspaceID := "test-workspace"

	// Create and initialize hook
	h, err := store.Create(ctx, taskID, workspaceID)
	require.NoError(t, err)

	// Progress through setup
	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
	require.NoError(t, err)

	// Start implement step with files touched
	h.CurrentStep = &domain.StepContext{
		StepName:     "implement",
		StepIndex:    2,
		StartedAt:    time.Now().UTC(),
		Attempt:      1,
		MaxAttempts:  3,
		WorkingOn:    "Adding new feature",
		FilesTouched: []string{"internal/feature/handler.go", "internal/feature/handler_test.go"},
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)

	// Create a checkpoint mid-step (simulating a git commit)
	err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerCommit, "Added handler skeleton")
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Verify checkpoint was created
	require.Len(t, h.Checkpoints, 1)
	checkpoint := h.Checkpoints[0]
	require.Equal(t, domain.CheckpointTriggerCommit, checkpoint.Trigger)
	require.Equal(t, "Added handler skeleton", checkpoint.Description)
	require.Equal(t, "implement", checkpoint.StepName)
	require.Equal(t, h.CurrentStep.CurrentCheckpointID, checkpoint.CheckpointID)

	// *** SIMULATE CRASH ***
	// Make hook stale but checkpoint is recent
	h.UpdatedAt = time.Now().UTC().Add(-6 * time.Minute)
	writeHookDirect(t, tempDir, taskID, h)

	// Reload and detect recovery needed
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)

	needsRecovery := recoveryDetector.DetectRecoveryNeeded(ctx, h)
	require.True(t, needsRecovery)

	// Diagnose - should recommend retry from checkpoint
	err = recoveryDetector.DiagnoseAndRecommend(ctx, h)
	require.NoError(t, err)
	require.NotNil(t, h.Recovery)
	require.Equal(t, "retry_from_checkpoint", h.Recovery.RecommendedAction)
	require.NotEmpty(t, h.Recovery.LastCheckpointID)
	require.Equal(t, checkpoint.CheckpointID, h.Recovery.LastCheckpointID)
}

// TestIntegration_ValidationReceiptChain tests receipt signing and verification:
// run validation → verify signature
func TestIntegration_ValidationReceiptChain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup key manager with ephemeral key
	keyPath := filepath.Join(tempDir, "test-master.key")
	km := native.NewKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)
	require.True(t, km.Exists())

	// Create receipt signer
	signer, err := hook.NewNativeReceiptSigner(km)
	require.NoError(t, err)

	// Create a validation receipt
	receipt := &domain.ValidationReceipt{
		ReceiptID:   "rcpt-test001",
		StepName:    "validate",
		Command:     "go test ./...",
		ExitCode:    0,
		StartedAt:   time.Now().UTC().Add(-30 * time.Second),
		CompletedAt: time.Now().UTC(),
		Duration:    "28.5s",
		StdoutHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		StderrHash:  "0000000000000000000000000000000000000000000000000000000000000000",
	}

	// Sign the receipt
	err = signer.SignReceipt(ctx, receipt, 0)
	require.NoError(t, err)
	require.NotEmpty(t, receipt.Signature)

	// Verify the signature
	err = signer.VerifyReceipt(ctx, receipt)
	require.NoError(t, err, "Valid signature should verify successfully")

	// Test tampering detection - modify receipt after signing
	tamperedReceipt := *receipt
	tamperedReceipt.ExitCode = 1 // Change exit code

	err = signer.VerifyReceipt(ctx, &tamperedReceipt)
	require.Error(t, err, "Tampered receipt should fail verification")

	// Test tampering detection - modify command
	tamperedReceipt2 := *receipt
	tamperedReceipt2.Command = "go test -race ./..."

	err = signer.VerifyReceipt(ctx, &tamperedReceipt2)
	require.Error(t, err, "Tampered command should fail verification")

	// Verify key path
	keyPathStr := signer.KeyPath(0)
	require.NotEmpty(t, keyPathStr)
	require.Equal(t, "native-ed25519-v1", keyPathStr)
}

// TestIntegration_StaleDetection tests stale hook detection:
// start → wait 6 min → check status
func TestIntegration_StaleDetection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints: 50,
		StaleThreshold: 5 * time.Minute,
	}

	store := hook.NewFileStore(tempDir)
	transitioner := hook.NewTransitioner()
	recoveryDetector := hook.NewRecoveryDetector(cfg)

	taskID := "test-task-stale"
	workspaceID := "test-workspace"

	// Create hook and start step
	h, err := store.Create(ctx, taskID, workspaceID)
	require.NoError(t, err)

	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
	require.NoError(t, err)

	h.CurrentStep = &domain.StepContext{
		StepName:    "implement",
		StepIndex:   2,
		StartedAt:   time.Now().UTC(),
		Attempt:     1,
		MaxAttempts: 3,
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Fresh hook should not be stale
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)
	require.False(t, recoveryDetector.DetectRecoveryNeeded(ctx, h), "Fresh hook should not need recovery")

	// Backdate to simulate 6 minutes passing
	h.UpdatedAt = time.Now().UTC().Add(-6 * time.Minute)
	writeHookDirect(t, tempDir, taskID, h)

	// Reload and check - should now be stale
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)
	require.True(t, recoveryDetector.DetectRecoveryNeeded(ctx, h), "Stale hook should need recovery")

	// Terminal state hooks should never be stale
	terminalStates := []domain.HookState{
		domain.HookStateCompleted,
		domain.HookStateFailed,
		domain.HookStateAbandoned,
	}

	for _, state := range terminalStates {
		termTaskID := taskID + "-" + string(state)
		h2, err := store.Create(ctx, termTaskID, workspaceID)
		require.NoError(t, err)

		// Manually set state (bypassing normal transitions for test)
		h2.State = state
		h2.UpdatedAt = time.Now().UTC().Add(-10 * time.Minute) // Very old
		writeHookDirect(t, tempDir, termTaskID, h2)

		h2, err = store.Get(ctx, termTaskID)
		require.NoError(t, err)
		require.False(t, recoveryDetector.DetectRecoveryNeeded(ctx, h2),
			"Terminal state %s should never be stale", state)
	}
}

// TestIntegration_HOOKMDAccuracy tests HOOK.md generation accuracy:
// various states → generate → verify content
func TestIntegration_HOOKMDAccuracy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints: 50,
		StaleThreshold: 5 * time.Minute,
	}

	mdGen := hook.NewMarkdownGenerator()
	store := hook.NewFileStore(tempDir, hook.WithMarkdownGenerator(mdGen))
	transitioner := hook.NewTransitioner()
	checkpointer := hook.NewCheckpointer(cfg, store)
	recoveryDetector := hook.NewRecoveryDetector(cfg)

	t.Run("initializing state", func(t *testing.T) {
		taskID := "md-test-init"
		h, err := store.Create(ctx, taskID, "test-workspace")
		require.NoError(t, err)

		// Generate markdown
		mdContent, err := mdGen.Generate(h)
		require.NoError(t, err)
		require.Contains(t, string(mdContent), "ATLAS Task Recovery Hook")
		require.Contains(t, string(mdContent), "`initializing`")
		require.Contains(t, string(mdContent), taskID)

		// Verify file was created
		mdPath := filepath.Join(tempDir, taskID, "HOOK.md")
		_, err = os.Stat(mdPath)
		require.NoError(t, err)
	})

	t.Run("step_running with current step", func(t *testing.T) {
		taskID := "md-test-running"
		h, err := store.Create(ctx, taskID, "test-workspace")
		require.NoError(t, err)

		err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
		require.NoError(t, err)

		h.CurrentStep = &domain.StepContext{
			StepName:     "implement",
			StepIndex:    2,
			StartedAt:    time.Now().UTC(),
			Attempt:      2,
			MaxAttempts:  3,
			WorkingOn:    "Adding error handling",
			FilesTouched: []string{"handler.go", "handler_test.go"},
			LastOutput:   "Added nil check for input parameter",
		}
		err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
		require.NoError(t, err)
		err = store.Save(ctx, h)
		require.NoError(t, err)

		mdContent, err := mdGen.Generate(h)
		require.NoError(t, err)
		require.Contains(t, string(mdContent), "`step_running`")
		require.Contains(t, string(mdContent), "`implement`")
		require.Contains(t, string(mdContent), "2/3") // Attempt
		require.Contains(t, string(mdContent), "Adding error handling")
		require.Contains(t, string(mdContent), "handler.go")
		require.Contains(t, string(mdContent), "Added nil check")
	})

	t.Run("with checkpoints", func(t *testing.T) {
		taskID := "md-test-checkpoints"
		h, err := store.Create(ctx, taskID, "test-workspace")
		require.NoError(t, err)

		err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
		require.NoError(t, err)

		h.CurrentStep = &domain.StepContext{
			StepName:  "implement",
			StepIndex: 1,
		}
		err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
		require.NoError(t, err)

		// Create multiple checkpoints
		err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerCommit, "Added feature A")
		require.NoError(t, err)
		err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerValidation, "Tests passing")
		require.NoError(t, err)
		err = store.Save(ctx, h)
		require.NoError(t, err)

		mdContent, err := mdGen.Generate(h)
		require.NoError(t, err)
		require.Contains(t, string(mdContent), "Checkpoint Timeline")
		require.Contains(t, string(mdContent), "Added feature A")
		require.Contains(t, string(mdContent), "Tests passing")
		require.Contains(t, string(mdContent), "git_commit")
		require.Contains(t, string(mdContent), "validation")
	})

	t.Run("with recovery context", func(t *testing.T) {
		taskID := "md-test-recovery"
		h, err := store.Create(ctx, taskID, "test-workspace")
		require.NoError(t, err)

		err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
		require.NoError(t, err)

		h.CurrentStep = &domain.StepContext{
			StepName:    "analyze",
			StepIndex:   0,
			StartedAt:   time.Now().UTC(),
			Attempt:     1,
			MaxAttempts: 3,
		}
		err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
		require.NoError(t, err)

		// Simulate crash
		h.UpdatedAt = time.Now().UTC().Add(-10 * time.Minute)

		// Diagnose
		err = recoveryDetector.DiagnoseAndRecommend(ctx, h)
		require.NoError(t, err)
		err = store.Save(ctx, h)
		require.NoError(t, err)

		mdContent, err := mdGen.Generate(h)
		require.NoError(t, err)
		require.Contains(t, string(mdContent), "What To Do Now")
		require.Contains(t, string(mdContent), "Retry Step") // Action label
		require.Contains(t, string(mdContent), "idempotent")
	})

	t.Run("with validation receipts", func(t *testing.T) {
		taskID := "md-test-receipts"
		h, err := store.Create(ctx, taskID, "test-workspace")
		require.NoError(t, err)

		h.Receipts = []domain.ValidationReceipt{
			{
				ReceiptID:   "rcpt-001",
				StepName:    "validate",
				Command:     "go test ./...",
				ExitCode:    0,
				Duration:    "12.5s",
				CompletedAt: time.Now().UTC(),
				Signature:   "abcd1234", // Has signature
			},
			{
				ReceiptID:   "rcpt-002",
				StepName:    "lint",
				Command:     "golangci-lint run",
				ExitCode:    0,
				Duration:    "8.2s",
				CompletedAt: time.Now().UTC(),
				// No signature
			},
		}
		err = store.Save(ctx, h)
		require.NoError(t, err)

		mdContent, err := mdGen.Generate(h)
		require.NoError(t, err)
		require.Contains(t, string(mdContent), "Completed Steps")
		require.Contains(t, string(mdContent), "validate") // Step name column
		require.Contains(t, string(mdContent), "go test")
		require.Contains(t, string(mdContent), "12.5s")
	})

	t.Run("completed state", func(t *testing.T) {
		taskID := "md-test-completed"
		h, err := store.Create(ctx, taskID, "test-workspace")
		require.NoError(t, err)

		err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
		require.NoError(t, err)
		err = transitioner.Transition(ctx, h, domain.HookStateCompleted, "task_complete", nil)
		require.NoError(t, err)
		err = store.Save(ctx, h)
		require.NoError(t, err)

		mdContent, err := mdGen.Generate(h)
		require.NoError(t, err)
		require.Contains(t, string(mdContent), "`completed`")
		require.Contains(t, string(mdContent), "✅") // Completed emoji
	})
}

// TestIntegration_ConcurrentCheckpoints tests thread-safety of interval checkpoints.
func TestIntegration_ConcurrentCheckpoints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints:     50,
		StaleThreshold:     5 * time.Minute,
		CheckpointInterval: 100 * time.Millisecond, // Fast interval for testing
	}

	mdGen := hook.NewMarkdownGenerator()
	store := hook.NewFileStore(tempDir, hook.WithMarkdownGenerator(mdGen))
	transitioner := hook.NewTransitioner()
	checkpointer := hook.NewCheckpointer(cfg, store)

	taskID := "test-concurrent"
	workspaceID := "test-workspace"

	// Create hook
	h, err := store.Create(ctx, taskID, workspaceID)
	require.NoError(t, err)

	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
	require.NoError(t, err)

	h.CurrentStep = &domain.StepContext{
		StepName:    "implement",
		StepIndex:   0,
		StartedAt:   time.Now().UTC(),
		Attempt:     1,
		MaxAttempts: 3,
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Start interval checkpointer
	intervalChkpt := hook.NewIntervalCheckpointer(checkpointer, taskID, store, 100*time.Millisecond, zerolog.Nop())
	intervalChkpt.Start(ctx)

	// Let it run for a bit
	time.Sleep(350 * time.Millisecond)

	// Stop the checkpointer
	intervalChkpt.Stop()

	// Verify checkpoints were created
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(h.Checkpoints), 2, "Should have created at least 2 interval checkpoints")

	// Verify all checkpoints are interval type
	for _, cp := range h.Checkpoints {
		require.Equal(t, domain.CheckpointTriggerInterval, cp.Trigger)
	}
}

// TestIntegration_CheckpointPruning tests that old checkpoints are pruned correctly.
func TestIntegration_CheckpointPruning(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints: 5, // Low limit for testing
		StaleThreshold: 5 * time.Minute,
	}

	store := hook.NewFileStore(tempDir)
	checkpointer := hook.NewCheckpointer(cfg, store)

	taskID := "test-pruning"

	h, err := store.Create(ctx, taskID, "test-workspace")
	require.NoError(t, err)

	h.CurrentStep = &domain.StepContext{
		StepName:  "test",
		StepIndex: 0,
	}
	h.State = domain.HookStateStepRunning

	// Create more checkpoints than the limit
	for i := 0; i < 8; i++ {
		err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerManual, "Checkpoint "+string(rune('A'+i)))
		require.NoError(t, err)
	}

	// Should only have max checkpoints
	require.Len(t, h.Checkpoints, 5)

	// Oldest should be removed - verify we have the newest ones
	require.Equal(t, "Checkpoint D", h.Checkpoints[0].Description)
	require.Equal(t, "Checkpoint E", h.Checkpoints[1].Description)
	require.Equal(t, "Checkpoint F", h.Checkpoints[2].Description)
	require.Equal(t, "Checkpoint G", h.Checkpoints[3].Description)
	require.Equal(t, "Checkpoint H", h.Checkpoints[4].Description)
}

// TestIntegration_FullRecoveryWorkflow tests the complete recovery workflow end-to-end.
func TestIntegration_FullRecoveryWorkflow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	cfg := &config.HookConfig{
		MaxCheckpoints:     50,
		StaleThreshold:     5 * time.Minute,
		CheckpointInterval: 5 * time.Minute,
	}

	mdGen := hook.NewMarkdownGenerator()
	store := hook.NewFileStore(tempDir, hook.WithMarkdownGenerator(mdGen))
	transitioner := hook.NewTransitioner()
	checkpointer := hook.NewCheckpointer(cfg, store)
	recoveryDetector := hook.NewRecoveryDetector(cfg)

	// Setup key manager for receipt signing
	keyPath := filepath.Join(tempDir, "master.key")
	km := native.NewKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	signer, err := hook.NewNativeReceiptSigner(km)
	require.NoError(t, err)

	taskID := "full-workflow"

	// Phase 1: Task Start
	h, err := store.Create(ctx, taskID, "test-workspace")
	require.NoError(t, err)

	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "setup_complete", nil)
	require.NoError(t, err)

	// Phase 2: Run analyze step
	h.CurrentStep = &domain.StepContext{
		StepName:    "analyze",
		StepIndex:   0,
		StartedAt:   time.Now().UTC(),
		Attempt:     1,
		MaxAttempts: 3,
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)

	// Validation
	err = transitioner.Transition(ctx, h, domain.HookStateStepValidating, "validation_start", nil)
	require.NoError(t, err)

	// Create validation receipt
	receipt := &domain.ValidationReceipt{
		ReceiptID:   "rcpt-analyze",
		StepName:    "analyze",
		Command:     "go vet ./...",
		ExitCode:    0,
		StartedAt:   time.Now().UTC().Add(-5 * time.Second),
		CompletedAt: time.Now().UTC(),
		Duration:    "5.0s",
		StdoutHash:  "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		StderrHash:  "0000000000000000000000000000000000000000000000000000000000000000",
	}
	err = signer.SignReceipt(ctx, receipt, 0)
	require.NoError(t, err)
	h.Receipts = append(h.Receipts, *receipt)

	err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerValidation, "Analysis validated")
	require.NoError(t, err)

	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "validation_passed", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Phase 3: Start implement step
	h.CurrentStep = &domain.StepContext{
		StepName:     "implement",
		StepIndex:    1,
		StartedAt:    time.Now().UTC(),
		Attempt:      1,
		MaxAttempts:  3,
		FilesTouched: []string{"feature.go"},
	}
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "step_start", nil)
	require.NoError(t, err)

	// Checkpoint on commit
	err = checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerCommit, "Initial implementation")
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Phase 4: Crash during implementation
	h.UpdatedAt = time.Now().UTC().Add(-8 * time.Minute)
	writeHookDirect(t, tempDir, taskID, h)

	// Phase 5: Recovery
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)

	needsRecovery := recoveryDetector.DetectRecoveryNeeded(ctx, h)
	require.True(t, needsRecovery)

	err = recoveryDetector.DiagnoseAndRecommend(ctx, h)
	require.NoError(t, err)

	err = transitioner.TransitionToRecovering(ctx, h, "crash_detected", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Verify HOOK.md was generated with recovery context
	mdPath := filepath.Join(tempDir, taskID, "HOOK.md")
	mdContent, err := os.ReadFile(mdPath) //nolint:gosec // G304: path is from t.TempDir(), safe in tests
	require.NoError(t, err)
	require.Contains(t, string(mdContent), "What To Do Now")

	// Verify receipt signature
	require.Len(t, h.Receipts, 1)
	err = signer.VerifyReceipt(ctx, &h.Receipts[0])
	require.NoError(t, err)

	// Phase 6: Resume
	err = transitioner.Transition(ctx, h, domain.HookStateStepRunning, "recovery_continue", nil)
	require.NoError(t, err)
	require.Equal(t, domain.HookStateStepRunning, h.State)
	require.Equal(t, "implement", h.CurrentStep.StepName)

	// Complete implementation and task
	err = transitioner.Transition(ctx, h, domain.HookStateStepPending, "step_complete", nil)
	require.NoError(t, err)
	err = transitioner.Transition(ctx, h, domain.HookStateCompleted, "task_complete", nil)
	require.NoError(t, err)
	err = store.Save(ctx, h)
	require.NoError(t, err)

	// Verify final state
	h, err = store.Get(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, domain.HookStateCompleted, h.State)
	require.GreaterOrEqual(t, len(h.History), 9) // Many transitions through the workflow
	require.Len(t, h.Checkpoints, 2)             // validation + commit
	require.Len(t, h.Receipts, 1)                // 1 validation receipt
}
