// Package hook provides crash recovery and context persistence for ATLAS tasks.
package hook

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

// DefaultCheckpointInterval is the default interval for periodic checkpoints.
const DefaultCheckpointInterval = 5 * time.Minute

// ErrNoCurrentStepContext is returned when attempting to complete a step without a current step context.
var ErrNoCurrentStepContext = errors.New("no current step context")

// ErrStepMismatch is returned when attempting to complete a step that doesn't match the current step.
var ErrStepMismatch = errors.New("step name mismatch")

// Manager implements task.HookManager and manages hook lifecycle.
// It uses a FileStore for persistence and MarkdownGenerator for HOOK.md.
type Manager struct {
	store            *FileStore
	cfg              *config.HookConfig
	signer           ReceiptSigner // Optional signer for validation receipts
	checkpointsMu    sync.Mutex
	intervalCheckers map[string]*IntervalCheckpointer // keyed by task ID
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithReceiptSigner sets the receipt signer for validation receipts.
// If not set, receipts will be created without signatures.
func WithReceiptSigner(signer ReceiptSigner) ManagerOption {
	return func(m *Manager) {
		m.signer = signer
	}
}

// NewManager creates a new Manager with the given store and config.
func NewManager(store *FileStore, cfg *config.HookConfig, opts ...ManagerOption) *Manager {
	m := &Manager{
		store:            store,
		cfg:              cfg,
		intervalCheckers: make(map[string]*IntervalCheckpointer),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// CreateHook initializes a hook for a new task.
func (m *Manager) CreateHook(ctx context.Context, task *domain.Task) error {
	// Create hook via store (which handles initialization)
	_, err := m.store.Create(ctx, task.ID, task.WorkspaceID)
	return err
}

// TransitionStep updates the hook when entering a step.
func (m *Manager) TransitionStep(ctx context.Context, task *domain.Task, stepName string, stepIndex int) error {
	return m.store.Update(ctx, task.ID, func(h *domain.Hook) error {
		// Validate transition before applying
		if err := domain.ValidateTransition(h.State, domain.HookStateStepRunning); err != nil {
			return fmt.Errorf("transition to step_running: %w", err)
		}

		now := time.Now().UTC()
		oldState := h.State

		// Update state to step_running
		h.State = domain.HookStateStepRunning
		h.UpdatedAt = now

		// Update current step context
		h.CurrentStep = &domain.StepContext{
			StepName:    stepName,
			StepIndex:   stepIndex,
			StartedAt:   now,
			Attempt:     1,
			MaxAttempts: 3, // Default, could be made configurable
		}

		// Record transition
		h.History = append(h.History, domain.HookEvent{
			Timestamp: now,
			FromState: oldState,
			ToState:   domain.HookStateStepRunning,
			Trigger:   "step_started",
			StepName:  stepName,
		})

		return nil
	})
}

// CompleteStep updates the hook when a step completes successfully.
// filesChanged contains the list of files modified during the step.
func (m *Manager) CompleteStep(ctx context.Context, task *domain.Task, stepName string, filesChanged []string) error {
	return m.store.Update(ctx, task.ID, func(h *domain.Hook) error {
		// Guard: CurrentStep must be set before completing
		if h.CurrentStep == nil {
			return fmt.Errorf("cannot complete step %q: %w (state: %s)", stepName, ErrNoCurrentStepContext, h.State)
		}

		// Guard: stepName must match current step to prevent completing wrong step
		if h.CurrentStep.StepName != stepName {
			return fmt.Errorf("%w: cannot complete step %q: current step is %q", ErrStepMismatch, stepName, h.CurrentStep.StepName)
		}

		// Validate transition before applying
		if err := domain.ValidateTransition(h.State, domain.HookStateStepPending); err != nil {
			return fmt.Errorf("transition to step_pending: %w", err)
		}

		now := time.Now().UTC()
		oldState := h.State

		// Update state to step_pending (ready for next step)
		h.State = domain.HookStateStepPending
		h.UpdatedAt = now

		// Track files touched during this step
		if len(filesChanged) > 0 {
			h.CurrentStep.FilesTouched = filesChanged
		}

		// Create checkpoint for step completion
		checkpoint := domain.StepCheckpoint{
			CheckpointID: GenerateCheckpointID(),
			CreatedAt:    now,
			StepName:     stepName,
			StepIndex:    h.CurrentStep.StepIndex,
			Description:  "Step completed: " + stepName,
			Trigger:      domain.CheckpointTriggerStepComplete,
		}

		// Capture file snapshots BEFORE appending (struct is copied on append)
		if len(filesChanged) > 0 {
			checkpoint.FilesSnapshot = snapshotFiles(filesChanged)
		} else if len(h.CurrentStep.FilesTouched) > 0 {
			// If explicit filesChanged not provided, fall back to accumulated files touched
			checkpoint.FilesSnapshot = snapshotFiles(h.CurrentStep.FilesTouched)
		}

		// Add git state if available from task metadata
		if task.Metadata != nil {
			if branch, ok := task.Metadata["branch"].(string); ok {
				checkpoint.GitBranch = branch
			}
		}

		// Append the fully-populated checkpoint to the slice
		h.Checkpoints = append(h.Checkpoints, checkpoint)

		// Prune checkpoints if over limit (uses shared constant for consistency)
		PruneCheckpoints(h, 0) // 0 means use DefaultMaxCheckpoints

		// Record transition
		h.History = append(h.History, domain.HookEvent{
			Timestamp: now,
			FromState: oldState,
			ToState:   domain.HookStateStepPending,
			Trigger:   "step_completed",
			StepName:  stepName,
		})

		// Update current step checkpoint reference
		h.CurrentStep.CurrentCheckpointID = checkpoint.CheckpointID

		return nil
	})
}

// FailStep updates the hook when a step fails.
func (m *Manager) FailStep(ctx context.Context, task *domain.Task, stepName string, stepErr error) error {
	return m.store.Update(ctx, task.ID, func(h *domain.Hook) error {
		// Validate transition before applying
		if err := domain.ValidateTransition(h.State, domain.HookStateAwaitingHuman); err != nil {
			return fmt.Errorf("transition to awaiting_human: %w", err)
		}

		now := time.Now().UTC()
		oldState := h.State

		// Update state to awaiting_human (needs manual intervention)
		h.State = domain.HookStateAwaitingHuman
		h.UpdatedAt = now

		// Record transition with error details
		h.History = append(h.History, domain.HookEvent{
			Timestamp: now,
			FromState: oldState,
			ToState:   domain.HookStateAwaitingHuman,
			Trigger:   "step_failed",
			StepName:  stepName,
			Details: map[string]any{
				"error": stepErr.Error(),
			},
		})
		return nil
	})
}

// CompleteTask finalizes the hook when the task completes.
func (m *Manager) CompleteTask(ctx context.Context, task *domain.Task) error {
	// Always stop the interval checkpointer when the task is done
	defer func() {
		_ = m.StopIntervalCheckpointing(ctx, task)
	}()

	return m.store.Update(ctx, task.ID, func(h *domain.Hook) error {
		// Validate transition before applying
		if err := domain.ValidateTransition(h.State, domain.HookStateCompleted); err != nil {
			return fmt.Errorf("transition to completed: %w", err)
		}

		now := time.Now().UTC()
		oldState := h.State

		// Update state to completed
		h.State = domain.HookStateCompleted
		h.UpdatedAt = now
		h.CurrentStep = nil

		// Record transition
		h.History = append(h.History, domain.HookEvent{
			Timestamp: now,
			FromState: oldState,
			ToState:   domain.HookStateCompleted,
			Trigger:   "task_completed",
		})
		return nil
	})
}

// FailTask updates the hook when the task fails.
func (m *Manager) FailTask(ctx context.Context, task *domain.Task, taskErr error) error {
	// Always stop the interval checkpointer when the task is done
	defer func() {
		_ = m.StopIntervalCheckpointing(ctx, task)
	}()

	return m.store.Update(ctx, task.ID, func(h *domain.Hook) error {
		// Validate transition before applying
		if err := domain.ValidateTransition(h.State, domain.HookStateFailed); err != nil {
			return fmt.Errorf("transition to failed: %w", err)
		}

		now := time.Now().UTC()
		oldState := h.State

		// Update state to failed
		h.State = domain.HookStateFailed
		h.UpdatedAt = now

		// Record transition
		h.History = append(h.History, domain.HookEvent{
			Timestamp: now,
			FromState: oldState,
			ToState:   domain.HookStateFailed,
			Trigger:   "task_failed",
			Details: map[string]any{
				"error": taskErr.Error(),
			},
		})
		return nil
	})
}

// StartIntervalCheckpointing starts periodic checkpoint creation for long-running steps.
// Checkpoints are created at DefaultCheckpointInterval only when the hook is in step_running state.
func (m *Manager) StartIntervalCheckpointing(ctx context.Context, task *domain.Task) error {
	m.checkpointsMu.Lock()
	defer m.checkpointsMu.Unlock()

	// Stop any existing interval checkpointer for this task
	if existing, ok := m.intervalCheckers[task.ID]; ok {
		existing.Stop()
		delete(m.intervalCheckers, task.ID)
	}

	// Verify the hook exists before starting interval checkpointing
	if _, err := m.store.Get(ctx, task.ID); err != nil {
		return err
	}

	// Create checkpointer and interval checkpointer
	// Pass taskID instead of hook pointer to avoid data races
	checkpointer := NewCheckpointer(m.cfg, m.store)
	interval := DefaultCheckpointInterval
	if m.cfg != nil && m.cfg.CheckpointInterval > 0 {
		interval = m.cfg.CheckpointInterval
	}
	ic := NewIntervalCheckpointer(checkpointer, task.ID, m.store, interval)

	// Start interval checkpointing
	ic.Start(ctx)

	// Store for later cleanup
	m.intervalCheckers[task.ID] = ic

	return nil
}

// StopIntervalCheckpointing stops the periodic checkpoint creation for a task.
func (m *Manager) StopIntervalCheckpointing(_ context.Context, task *domain.Task) error {
	m.checkpointsMu.Lock()
	defer m.checkpointsMu.Unlock()

	if ic, ok := m.intervalCheckers[task.ID]; ok {
		ic.Stop()
		delete(m.intervalCheckers, task.ID)
	}

	return nil
}

// CreateValidationReceipt creates and stores a signed receipt for a passed validation.
// If signing fails (e.g., no master key), the receipt is still created but without a signature.
// The taskIndex for key derivation is computed from the number of existing receipts.
func (m *Manager) CreateValidationReceipt(ctx context.Context, task *domain.Task, stepName string, result *domain.StepResult) error {
	return m.store.Update(ctx, task.ID, func(h *domain.Hook) error {
		// Extract command info from result metadata
		command := ""
		exitCode := 0
		if result.Metadata != nil {
			if cmd, ok := result.Metadata["command"].(string); ok {
				command = cmd
			}
			if code, ok := result.Metadata["exit_code"].(int); ok {
				exitCode = code
			}
		}

		// Calculate output hashes
		stdoutHash := hashOutput(result.Output)
		stderrHash := "" // Validation output typically goes to combined output

		// Calculate duration
		duration := time.Duration(result.DurationMs) * time.Millisecond

		// Create receipt
		receipt := domain.ValidationReceipt{
			ReceiptID:   "rcpt-" + GenerateCheckpointID()[5:], // reuse UUID generation
			StepName:    stepName,
			Command:     command,
			ExitCode:    exitCode,
			StartedAt:   result.StartedAt,
			CompletedAt: result.CompletedAt,
			Duration:    formatReceiptDuration(duration),
			StdoutHash:  stdoutHash,
			StderrHash:  stderrHash,
		}

		// Sign the receipt if a signer is available.
		// Signing provides cryptographic proof that validation actually ran.
		// If signing fails, we still save the receipt (integrity is nice-to-have, not blocking).
		m.signReceiptIfAvailable(ctx, &receipt, len(h.Receipts))

		// Add receipt to hook
		h.Receipts = append(h.Receipts, receipt)
		h.UpdatedAt = time.Now().UTC()

		return nil
	})
}

// signReceiptIfAvailable signs the receipt if a signer is available.
// Signing is best-effort; failures are logged but don't block receipt creation.
func (m *Manager) signReceiptIfAvailable(ctx context.Context, receipt *domain.ValidationReceipt, receiptCount int) {
	if m.signer == nil {
		return
	}

	// Use the receipt index as taskIndex (though ignored by native signer)
	if receiptCount > math.MaxUint32 {
		receiptCount = math.MaxUint32
	}
	//nolint:gosec // G115: Bounds check above ensures safe conversion
	taskIndex := uint32(receiptCount)

	signErr := m.signer.SignReceipt(ctx, receipt, taskIndex)
	if signErr != nil {
		// Log but don't fail - unsigned receipts are still valuable for debugging
		// In production, this would be: log.Warn("failed to sign receipt", "error", signErr)
		_ = signErr // Intentionally ignoring - signing is optional per spec threat model
		return
	}

	// If signing succeeded, verify we have the key path
	if receipt.KeyPath == "" {
		receipt.KeyPath = m.signer.KeyPath(taskIndex)
	}
}

// hashOutput computes a SHA256 hash of output content.
func hashOutput(output string) string {
	if output == "" {
		return ""
	}
	// Use crypto/sha256 for hashing
	hash := sha256.Sum256([]byte(output))
	return fmt.Sprintf("%x", hash[:8]) // First 8 bytes hex
}

// formatReceiptDuration formats a duration as a human-readable string for receipts.
func formatReceiptDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
