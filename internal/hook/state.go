package hook

import (
	"context"
	"fmt"
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// ErrInvalidTransition is returned when a state transition is not allowed.
var ErrInvalidTransition = fmt.Errorf("invalid state transition")

// ErrTerminalState is returned when trying to transition from a terminal state.
var ErrTerminalState = fmt.Errorf("cannot transition from terminal state")

// Transitioner manages state transitions for hooks.
type Transitioner struct{}

// NewTransitioner creates a new state transitioner.
func NewTransitioner() *Transitioner {
	return &Transitioner{}
}

// Transition moves the hook to a new state.
// Records the transition in history and updates timestamps.
// Returns error if transition is invalid.
func (t *Transitioner) Transition(_ context.Context, hook *domain.Hook, to domain.HookState, trigger string, details map[string]any) error {
	from := hook.State

	// Check if current state is terminal
	if domain.IsTerminalState(from) {
		return fmt.Errorf("%w: %s is terminal", ErrTerminalState, from)
	}

	// Validate transition
	if !t.IsValidTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}

	// Record the transition
	event := domain.HookEvent{
		Timestamp: time.Now().UTC(),
		FromState: from,
		ToState:   to,
		Trigger:   trigger,
		Details:   details,
	}

	// Add step name if current step exists
	if hook.CurrentStep != nil {
		event.StepName = hook.CurrentStep.StepName
	}

	hook.History = append(hook.History, event)
	hook.State = to
	hook.UpdatedAt = time.Now().UTC()

	return nil
}

// IsValidTransition checks if a state transition is allowed.
func (t *Transitioner) IsValidTransition(from, to domain.HookState) bool {
	validTransitions := domain.GetValidTransitions()
	validTargets, ok := validTransitions[from]
	if !ok {
		return false
	}

	for _, validTarget := range validTargets {
		if validTarget == to {
			return true
		}
	}

	return false
}

// IsTerminalState returns true for completed, failed, abandoned.
func (t *Transitioner) IsTerminalState(state domain.HookState) bool {
	return domain.IsTerminalState(state)
}

// TransitionToRecovering transitions any non-terminal hook to the recovering state.
// This is a special transition allowed from any non-terminal state.
func (t *Transitioner) TransitionToRecovering(_ context.Context, hook *domain.Hook, trigger string, details map[string]any) error {
	from := hook.State

	// Check if current state is terminal
	if domain.IsTerminalState(from) {
		return fmt.Errorf("%w: %s is terminal", ErrTerminalState, from)
	}

	// Record the transition
	event := domain.HookEvent{
		Timestamp: time.Now().UTC(),
		FromState: from,
		ToState:   domain.HookStateRecovering,
		Trigger:   trigger,
		Details:   details,
	}

	if hook.CurrentStep != nil {
		event.StepName = hook.CurrentStep.StepName
	}

	hook.History = append(hook.History, event)
	hook.State = domain.HookStateRecovering
	hook.UpdatedAt = time.Now().UTC()

	return nil
}

// TransitionToAbandoned transitions any non-terminal hook to the abandoned state.
// This is a special transition allowed from any non-terminal state.
func (t *Transitioner) TransitionToAbandoned(_ context.Context, hook *domain.Hook, trigger string, details map[string]any) error {
	from := hook.State

	// Check if current state is terminal
	if domain.IsTerminalState(from) {
		return fmt.Errorf("%w: %s is terminal", ErrTerminalState, from)
	}

	// Record the transition
	event := domain.HookEvent{
		Timestamp: time.Now().UTC(),
		FromState: from,
		ToState:   domain.HookStateAbandoned,
		Trigger:   trigger,
		Details:   details,
	}

	if hook.CurrentStep != nil {
		event.StepName = hook.CurrentStep.StepName
	}

	hook.History = append(hook.History, event)
	hook.State = domain.HookStateAbandoned
	hook.UpdatedAt = time.Now().UTC()

	return nil
}

// AppendEvent adds an event to the hook's history without changing state.
// Use this for recording events that don't involve state transitions.
func (t *Transitioner) AppendEvent(hook *domain.Hook, trigger string, details map[string]any) {
	event := domain.HookEvent{
		Timestamp: time.Now().UTC(),
		FromState: hook.State,
		ToState:   hook.State, // Same state
		Trigger:   trigger,
		Details:   details,
	}

	if hook.CurrentStep != nil {
		event.StepName = hook.CurrentStep.StepName
	}

	hook.History = append(hook.History, event)
	hook.UpdatedAt = time.Now().UTC()
}
