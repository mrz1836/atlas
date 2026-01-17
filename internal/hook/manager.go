// Package hook provides crash recovery and context persistence for ATLAS tasks.
package hook

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mrz1836/atlas/internal/domain"
)

// generateCheckpointID generates a unique checkpoint ID.
// Format: ckpt-{uuid8} (e.g., ckpt-a1b2c3d4)
func generateCheckpointID() string {
	return "ckpt-" + uuid.New().String()[:8]
}

// Manager implements task.HookManager and manages hook lifecycle.
// It uses a FileStore for persistence and MarkdownGenerator for HOOK.md.
type Manager struct {
	store *FileStore
}

// NewManager creates a new Manager with the given store.
func NewManager(store *FileStore) *Manager {
	return &Manager{
		store: store,
	}
}

// CreateHook initializes a hook for a new task.
func (m *Manager) CreateHook(ctx context.Context, task *domain.Task) error {
	// Create hook via store (which handles initialization)
	_, err := m.store.Create(ctx, task.ID, task.WorkspaceID)
	return err
}

// TransitionStep updates the hook when entering a step.
func (m *Manager) TransitionStep(ctx context.Context, task *domain.Task, stepName string, stepIndex int) error {
	h, err := m.store.Get(ctx, task.ID)
	if err != nil {
		return err
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

	return m.store.Save(ctx, h)
}

// CompleteStep updates the hook when a step completes successfully.
func (m *Manager) CompleteStep(ctx context.Context, task *domain.Task, stepName string) error {
	h, err := m.store.Get(ctx, task.ID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	oldState := h.State

	// Update state to step_pending (ready for next step)
	h.State = domain.HookStateStepPending
	h.UpdatedAt = now

	// Create checkpoint for step completion
	checkpoint := domain.StepCheckpoint{
		CheckpointID: generateCheckpointID(),
		CreatedAt:    now,
		StepName:     stepName,
		StepIndex:    h.CurrentStep.StepIndex,
		Description:  "Step completed: " + stepName,
		Trigger:      domain.CheckpointTriggerStepComplete,
	}

	// Add git state if available from task metadata
	if task.Metadata != nil {
		if branch, ok := task.Metadata["branch"].(string); ok {
			checkpoint.GitBranch = branch
		}
	}

	h.Checkpoints = append(h.Checkpoints, checkpoint)

	// Prune checkpoints if over limit (keep most recent 50)
	if len(h.Checkpoints) > 50 {
		h.Checkpoints = h.Checkpoints[len(h.Checkpoints)-50:]
	}

	// Record transition
	h.History = append(h.History, domain.HookEvent{
		Timestamp: now,
		FromState: oldState,
		ToState:   domain.HookStateStepPending,
		Trigger:   "step_completed",
		StepName:  stepName,
	})

	// Update current step checkpoint reference
	if h.CurrentStep != nil {
		h.CurrentStep.CurrentCheckpointID = checkpoint.CheckpointID
	}

	return m.store.Save(ctx, h)
}

// FailStep updates the hook when a step fails.
func (m *Manager) FailStep(ctx context.Context, task *domain.Task, stepName string, stepErr error) error {
	h, err := m.store.Get(ctx, task.ID)
	if err != nil {
		return err
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

	return m.store.Save(ctx, h)
}

// CompleteTask finalizes the hook when the task completes.
func (m *Manager) CompleteTask(ctx context.Context, task *domain.Task) error {
	h, err := m.store.Get(ctx, task.ID)
	if err != nil {
		return err
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

	return m.store.Save(ctx, h)
}

// FailTask updates the hook when the task fails.
func (m *Manager) FailTask(ctx context.Context, task *domain.Task, taskErr error) error {
	h, err := m.store.Get(ctx, task.ID)
	if err != nil {
		return err
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

	return m.store.Save(ctx, h)
}
