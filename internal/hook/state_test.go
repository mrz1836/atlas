package hook

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestTransitioner_Transition(t *testing.T) {
	trans := NewTransitioner()
	ctx := context.Background()

	t.Run("valid transition", func(t *testing.T) {
		hook := &domain.Hook{
			State:     domain.HookStateStepPending,
			UpdatedAt: time.Now().Add(-1 * time.Minute),
			History:   []domain.HookEvent{},
		}

		err := trans.Transition(ctx, hook, domain.HookStateStepRunning, "step_started", nil)
		require.NoError(t, err)

		assert.Equal(t, domain.HookStateStepRunning, hook.State)
		assert.Len(t, hook.History, 1)
		assert.Equal(t, domain.HookStateStepPending, hook.History[0].FromState)
		assert.Equal(t, domain.HookStateStepRunning, hook.History[0].ToState)
		assert.Equal(t, "step_started", hook.History[0].Trigger)
	})

	t.Run("invalid transition", func(t *testing.T) {
		hook := &domain.Hook{
			State:   domain.HookStateStepPending,
			History: []domain.HookEvent{},
		}

		// step_pending -> step_validating is not valid
		err := trans.Transition(ctx, hook, domain.HookStateStepValidating, "invalid", nil)
		require.ErrorIs(t, err, ErrInvalidTransition)

		// State should not change
		assert.Equal(t, domain.HookStateStepPending, hook.State)
		assert.Empty(t, hook.History)
	})

	t.Run("terminal state rejection", func(t *testing.T) {
		terminalStates := []domain.HookState{
			domain.HookStateCompleted,
			domain.HookStateFailed,
			domain.HookStateAbandoned,
		}

		for _, state := range terminalStates {
			t.Run(string(state), func(t *testing.T) {
				hook := &domain.Hook{
					State:   state,
					History: []domain.HookEvent{},
				}

				err := trans.Transition(ctx, hook, domain.HookStateStepPending, "attempt", nil)
				assert.ErrorIs(t, err, ErrTerminalState)
			})
		}
	})

	t.Run("with details", func(t *testing.T) {
		hook := &domain.Hook{
			State:   domain.HookStateStepPending,
			History: []domain.HookEvent{},
		}

		details := map[string]any{
			"step_index": 2,
			"file_path":  "config/parser.go",
		}

		err := trans.Transition(ctx, hook, domain.HookStateStepRunning, "step_started", details)
		require.NoError(t, err)

		assert.Equal(t, details, hook.History[0].Details)
	})

	t.Run("records step name", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepPending,
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 2,
			},
			History: []domain.HookEvent{},
		}

		err := trans.Transition(ctx, hook, domain.HookStateStepRunning, "step_started", nil)
		require.NoError(t, err)

		assert.Equal(t, "implement", hook.History[0].StepName)
	})
}

func TestTransitioner_IsValidTransition(t *testing.T) {
	trans := NewTransitioner()

	tests := []struct {
		from  domain.HookState
		to    domain.HookState
		valid bool
	}{
		// Initial transitions
		{"", domain.HookStateInitializing, true},
		{"", domain.HookStateStepRunning, false},

		// Initializing transitions
		{domain.HookStateInitializing, domain.HookStateStepPending, true},
		{domain.HookStateInitializing, domain.HookStateFailed, true},
		{domain.HookStateInitializing, domain.HookStateStepRunning, false},

		// StepPending transitions
		{domain.HookStateStepPending, domain.HookStateStepRunning, true},
		{domain.HookStateStepPending, domain.HookStateCompleted, true},
		{domain.HookStateStepPending, domain.HookStateAbandoned, true},
		{domain.HookStateStepPending, domain.HookStateStepValidating, false},

		// StepRunning transitions
		{domain.HookStateStepRunning, domain.HookStateStepValidating, true},
		{domain.HookStateStepRunning, domain.HookStateStepPending, true},
		{domain.HookStateStepRunning, domain.HookStateAwaitingHuman, true},
		{domain.HookStateStepRunning, domain.HookStateFailed, true},
		{domain.HookStateStepRunning, domain.HookStateAbandoned, true},
		{domain.HookStateStepRunning, domain.HookStateCompleted, false},

		// StepValidating transitions
		{domain.HookStateStepValidating, domain.HookStateStepPending, true},
		{domain.HookStateStepValidating, domain.HookStateAwaitingHuman, true},
		{domain.HookStateStepValidating, domain.HookStateFailed, true},
		{domain.HookStateStepValidating, domain.HookStateStepRunning, false},

		// AwaitingHuman transitions
		{domain.HookStateAwaitingHuman, domain.HookStateStepPending, true},
		{domain.HookStateAwaitingHuman, domain.HookStateStepRunning, true},
		{domain.HookStateAwaitingHuman, domain.HookStateAbandoned, true},
		{domain.HookStateAwaitingHuman, domain.HookStateCompleted, false},

		// Recovering transitions
		{domain.HookStateRecovering, domain.HookStateStepPending, true},
		{domain.HookStateRecovering, domain.HookStateStepRunning, true},
		{domain.HookStateRecovering, domain.HookStateAwaitingHuman, true},
		{domain.HookStateRecovering, domain.HookStateFailed, true},
		{domain.HookStateRecovering, domain.HookStateCompleted, false},

		// Terminal states have no valid outgoing transitions
		{domain.HookStateCompleted, domain.HookStateStepPending, false},
		{domain.HookStateFailed, domain.HookStateRecovering, false},
		{domain.HookStateAbandoned, domain.HookStateStepRunning, false},
	}

	for _, tt := range tests {
		name := string(tt.from) + " -> " + string(tt.to)
		if tt.from == "" {
			name = "(initial) -> " + string(tt.to)
		}

		t.Run(name, func(t *testing.T) {
			result := trans.IsValidTransition(tt.from, tt.to)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestTransitioner_IsTerminalState(t *testing.T) {
	trans := NewTransitioner()

	tests := []struct {
		state    domain.HookState
		terminal bool
	}{
		{domain.HookStateInitializing, false},
		{domain.HookStateStepPending, false},
		{domain.HookStateStepRunning, false},
		{domain.HookStateStepValidating, false},
		{domain.HookStateAwaitingHuman, false},
		{domain.HookStateRecovering, false},
		{domain.HookStateCompleted, true},
		{domain.HookStateFailed, true},
		{domain.HookStateAbandoned, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.terminal, trans.IsTerminalState(tt.state))
		})
	}
}

func TestTransitioner_TransitionToRecovering(t *testing.T) {
	trans := NewTransitioner()
	ctx := context.Background()

	t.Run("from any non-terminal state", func(t *testing.T) {
		nonTerminalStates := []domain.HookState{
			domain.HookStateInitializing,
			domain.HookStateStepPending,
			domain.HookStateStepRunning,
			domain.HookStateStepValidating,
			domain.HookStateAwaitingHuman,
		}

		for _, state := range nonTerminalStates {
			t.Run(string(state), func(t *testing.T) {
				hook := &domain.Hook{
					State:   state,
					History: []domain.HookEvent{},
				}

				err := trans.TransitionToRecovering(ctx, hook, "crash_detected", nil)
				require.NoError(t, err)

				assert.Equal(t, domain.HookStateRecovering, hook.State)
				assert.Len(t, hook.History, 1)
				assert.Equal(t, state, hook.History[0].FromState)
				assert.Equal(t, domain.HookStateRecovering, hook.History[0].ToState)
			})
		}
	})

	t.Run("rejects terminal states", func(t *testing.T) {
		hook := &domain.Hook{
			State:   domain.HookStateCompleted,
			History: []domain.HookEvent{},
		}

		err := trans.TransitionToRecovering(ctx, hook, "crash_detected", nil)
		assert.ErrorIs(t, err, ErrTerminalState)
	})
}

func TestTransitioner_TransitionToAbandoned(t *testing.T) {
	trans := NewTransitioner()
	ctx := context.Background()

	t.Run("from any non-terminal state", func(t *testing.T) {
		nonTerminalStates := []domain.HookState{
			domain.HookStateInitializing,
			domain.HookStateStepPending,
			domain.HookStateStepRunning,
			domain.HookStateStepValidating,
			domain.HookStateAwaitingHuman,
			domain.HookStateRecovering,
		}

		for _, state := range nonTerminalStates {
			t.Run(string(state), func(t *testing.T) {
				hook := &domain.Hook{
					State:   state,
					History: []domain.HookEvent{},
				}

				err := trans.TransitionToAbandoned(ctx, hook, "user_abandoned", nil)
				require.NoError(t, err)

				assert.Equal(t, domain.HookStateAbandoned, hook.State)
				assert.Len(t, hook.History, 1)
			})
		}
	})

	t.Run("rejects terminal states", func(t *testing.T) {
		hook := &domain.Hook{
			State:   domain.HookStateFailed,
			History: []domain.HookEvent{},
		}

		err := trans.TransitionToAbandoned(ctx, hook, "user_abandoned", nil)
		assert.ErrorIs(t, err, ErrTerminalState)
	})
}

func TestTransitioner_AppendEvent(t *testing.T) {
	trans := NewTransitioner()

	hook := &domain.Hook{
		State:     domain.HookStateStepRunning,
		UpdatedAt: time.Now().Add(-1 * time.Minute),
		CurrentStep: &domain.StepContext{
			StepName: "implement",
		},
		History: []domain.HookEvent{},
	}

	details := map[string]any{
		"checkpoint_id": "ckpt-12345678",
	}

	trans.AppendEvent(hook, "checkpoint_created", details)

	// State should not change
	assert.Equal(t, domain.HookStateStepRunning, hook.State)

	// Event should be recorded
	require.Len(t, hook.History, 1)
	assert.Equal(t, "checkpoint_created", hook.History[0].Trigger)
	assert.Equal(t, domain.HookStateStepRunning, hook.History[0].FromState)
	assert.Equal(t, domain.HookStateStepRunning, hook.History[0].ToState)
	assert.Equal(t, "implement", hook.History[0].StepName)
	assert.Equal(t, details, hook.History[0].Details)
}

func TestTransitioner_HistoryAppend(t *testing.T) {
	trans := NewTransitioner()
	ctx := context.Background()

	hook := &domain.Hook{
		State:   domain.HookStateInitializing,
		History: []domain.HookEvent{},
	}

	// Make multiple transitions
	err := trans.Transition(ctx, hook, domain.HookStateStepPending, "setup_complete", nil)
	require.NoError(t, err)

	err = trans.Transition(ctx, hook, domain.HookStateStepRunning, "step_started", nil)
	require.NoError(t, err)

	err = trans.Transition(ctx, hook, domain.HookStateStepValidating, "validation_started", nil)
	require.NoError(t, err)

	err = trans.Transition(ctx, hook, domain.HookStateStepPending, "validation_passed", nil)
	require.NoError(t, err)

	// Verify history is append-only and complete
	require.Len(t, hook.History, 4)

	expectedTransitions := []struct {
		from    domain.HookState
		to      domain.HookState
		trigger string
	}{
		{domain.HookStateInitializing, domain.HookStateStepPending, "setup_complete"},
		{domain.HookStateStepPending, domain.HookStateStepRunning, "step_started"},
		{domain.HookStateStepRunning, domain.HookStateStepValidating, "validation_started"},
		{domain.HookStateStepValidating, domain.HookStateStepPending, "validation_passed"},
	}

	for i, expected := range expectedTransitions {
		assert.Equal(t, expected.from, hook.History[i].FromState)
		assert.Equal(t, expected.to, hook.History[i].ToState)
		assert.Equal(t, expected.trigger, hook.History[i].Trigger)
	}
}
