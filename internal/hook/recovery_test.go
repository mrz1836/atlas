package hook

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestRecoveryDetector_DetectRecoveryNeeded(t *testing.T) {
	cfg := &config.HookConfig{
		StaleThreshold: 5 * time.Minute,
	}
	detector := NewRecoveryDetector(cfg)
	ctx := context.Background()

	t.Run("stale hook detection", func(t *testing.T) {
		hook := &domain.Hook{
			State:     domain.HookStateStepRunning,
			UpdatedAt: time.Now().Add(-10 * time.Minute), // 10 minutes old
		}

		assert.True(t, detector.DetectRecoveryNeeded(ctx, hook))
	})

	t.Run("fresh hook not stale", func(t *testing.T) {
		hook := &domain.Hook{
			State:     domain.HookStateStepRunning,
			UpdatedAt: time.Now().Add(-1 * time.Minute), // 1 minute old
		}

		assert.False(t, detector.DetectRecoveryNeeded(ctx, hook))
	})

	t.Run("terminal state excluded", func(t *testing.T) {
		terminalStates := []domain.HookState{
			domain.HookStateCompleted,
			domain.HookStateFailed,
			domain.HookStateAbandoned,
		}

		for _, state := range terminalStates {
			t.Run(string(state), func(t *testing.T) {
				hook := &domain.Hook{
					State:     state,
					UpdatedAt: time.Now().Add(-10 * time.Minute), // Old but terminal
				}

				assert.False(t, detector.DetectRecoveryNeeded(ctx, hook))
			})
		}
	})
}

func TestRecoveryDetector_DiagnoseAndRecommend(t *testing.T) {
	cfg := &config.HookConfig{
		StaleThreshold: 5 * time.Minute,
	}
	detector := NewRecoveryDetector(cfg)
	ctx := context.Background()

	t.Run("retry_step for idempotent analyze", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "analyze",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "retry_step", hook.Recovery.RecommendedAction)
		assert.Contains(t, hook.Recovery.Reason, "idempotent")
	})

	t.Run("retry_step for idempotent plan", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "plan",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "retry_step", hook.Recovery.RecommendedAction)
	})

	t.Run("retry_step for validation", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepValidating,
			CurrentStep: &domain.StepContext{
				StepName: "validate",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "retry_step", hook.Recovery.RecommendedAction)
		assert.True(t, hook.Recovery.WasValidating)
	})

	t.Run("manual for implement step", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "implement",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "manual", hook.Recovery.RecommendedAction)
		assert.Contains(t, hook.Recovery.Reason, "modifies state")
	})

	t.Run("manual for commit step", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "commit",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "manual", hook.Recovery.RecommendedAction)
	})

	t.Run("retry_from_checkpoint when recent checkpoint exists", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "implement",
			},
			Checkpoints: []domain.StepCheckpoint{
				{
					CheckpointID: "ckpt-12345678",
					CreatedAt:    time.Now().Add(-5 * time.Minute), // Recent checkpoint
					Description:  "Added nil check",
				},
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "retry_from_checkpoint", hook.Recovery.RecommendedAction)
		assert.Equal(t, "ckpt-12345678", hook.Recovery.LastCheckpointID)
		assert.Contains(t, hook.Recovery.Reason, "checkpoint available")
	})

	t.Run("does not use old checkpoint", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "implement",
			},
			Checkpoints: []domain.StepCheckpoint{
				{
					CheckpointID: "ckpt-old",
					CreatedAt:    time.Now().Add(-30 * time.Minute), // Old checkpoint
					Description:  "Old checkpoint",
				},
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		require.NotNil(t, hook.Recovery)
		assert.Equal(t, "manual", hook.Recovery.RecommendedAction)
		// Still records last checkpoint ID
		assert.Equal(t, "ckpt-old", hook.Recovery.LastCheckpointID)
	})

	t.Run("captures crash type", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepValidating,
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		assert.Equal(t, "validation_interrupted", hook.Recovery.CrashType)
	})

	t.Run("captures partial output", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:   "analyze",
				LastOutput: "Analyzing code structure...",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		assert.Equal(t, "Analyzing code structure...", hook.Recovery.PartialOutput)
	})

	t.Run("records last known state", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "analyze",
			},
		}

		err := detector.DiagnoseAndRecommend(ctx, hook)
		require.NoError(t, err)

		assert.Equal(t, domain.HookStateStepRunning, hook.Recovery.LastKnownState)
	})
}

func TestIsIdempotentStep(t *testing.T) {
	tests := []struct {
		stepName   string
		idempotent bool
	}{
		{"analyze", true},
		{"plan", true},
		{"validate", true},
		{"review", true},
		{"test", true},
		{"lint", true},
		{"implement", false},
		{"commit", false},
		{"pr", false},
		{"push", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.stepName, func(t *testing.T) {
			assert.Equal(t, tt.idempotent, isIdempotentStep(tt.stepName))
		})
	}
}

func TestGetRecoveryContext(t *testing.T) {
	t.Run("returns nil when no recovery", func(t *testing.T) {
		hook := &domain.Hook{}
		assert.Nil(t, GetRecoveryContext(hook))
	})

	t.Run("returns recovery context when present", func(t *testing.T) {
		hook := &domain.Hook{
			Recovery: &domain.RecoveryContext{
				RecommendedAction: "retry_step",
			},
		}

		rc := GetRecoveryContext(hook)
		require.NotNil(t, rc)
		assert.Equal(t, "retry_step", rc.RecommendedAction)
	})
}
