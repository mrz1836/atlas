package hook

import (
	"context"
	"time"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

// RecoveryDetector identifies and diagnoses crash recovery scenarios.
type RecoveryDetector struct {
	cfg *config.HookConfig
}

// NewRecoveryDetector creates a new recovery detector.
func NewRecoveryDetector(cfg *config.HookConfig) *RecoveryDetector {
	return &RecoveryDetector{cfg: cfg}
}

// DetectRecoveryNeeded checks if a hook requires crash recovery.
// Returns true if hook is stale (no update within threshold) and not terminal.
func (rd *RecoveryDetector) DetectRecoveryNeeded(_ context.Context, hook *domain.Hook) bool {
	return rd.isStaleHook(hook)
}

// DiagnoseAndRecommend analyzes the crash and populates RecoveryContext.
// Sets recommended_action to: retry_step, retry_from_checkpoint, skip_step, or manual.
func (rd *RecoveryDetector) DiagnoseAndRecommend(_ context.Context, hook *domain.Hook) error {
	if hook.Recovery == nil {
		hook.Recovery = &domain.RecoveryContext{}
	}

	rc := hook.Recovery
	rc.DetectedAt = time.Now().UTC()
	rc.LastKnownState = hook.State
	rc.CrashType = rd.detectCrashType(hook)

	// Check if was validating
	if hook.State == domain.HookStateStepValidating {
		rc.WasValidating = true
	}

	// Find last checkpoint if any
	if len(hook.Checkpoints) > 0 {
		lastCP := hook.Checkpoints[len(hook.Checkpoints)-1]
		rc.LastCheckpointID = lastCP.CheckpointID
	}

	// Determine recovery recommendation
	action, reason := rd.determineRecoveryAction(hook)
	rc.RecommendedAction = action
	rc.Reason = reason

	// Extract partial output if available
	if hook.CurrentStep != nil && hook.CurrentStep.LastOutput != "" {
		rc.PartialOutput = hook.CurrentStep.LastOutput
	}

	return nil
}

// isStaleHook checks if the hook hasn't been updated within threshold.
func (rd *RecoveryDetector) isStaleHook(hook *domain.Hook) bool {
	// Terminal states are never considered stale
	if domain.IsTerminalState(hook.State) {
		return false
	}

	threshold := rd.cfg.StaleThreshold
	if threshold == 0 {
		threshold = 5 * time.Minute // Default
	}

	return time.Since(hook.UpdatedAt) > threshold
}

// detectCrashType analyzes the hook state to determine crash type.
func (rd *RecoveryDetector) detectCrashType(hook *domain.Hook) string {
	// Simple heuristics for crash type
	if hook.State == domain.HookStateStepValidating {
		return "validation_interrupted"
	}
	if hook.State == domain.HookStateStepRunning {
		return "step_interrupted"
	}
	return "unknown"
}

// determineRecoveryAction decides the best recovery action based on state and context.
func (rd *RecoveryDetector) determineRecoveryAction(hook *domain.Hook) (action, reason string) {
	// Check for recent checkpoint (within 10 minutes)
	if len(hook.Checkpoints) > 0 {
		lastCP := hook.Checkpoints[len(hook.Checkpoints)-1]
		if time.Since(lastCP.CreatedAt) < 10*time.Minute {
			return "retry_from_checkpoint", "Recent checkpoint available from " + lastCP.Description
		}
	}

	// Check if was validating (always safe to retry)
	if hook.State == domain.HookStateStepValidating {
		return "retry_step", "Validation is idempotent, safe to retry"
	}

	// Check if current step is idempotent
	if hook.CurrentStep != nil {
		if isIdempotentStep(hook.CurrentStep.StepName) {
			return "retry_step", "Step '" + hook.CurrentStep.StepName + "' is idempotent, safe to retry"
		}

		// Non-idempotent step (implement, commit, pr)
		return "manual", "Step '" + hook.CurrentStep.StepName + "' modifies state, manual review recommended"
	}

	// Default to manual intervention
	return "manual", "Unable to determine safe recovery action"
}

// isIdempotentStep returns true if the step can be safely retried.
// Idempotent steps: analyze, plan, validate (read-only operations)
// Non-idempotent steps: implement, commit, pr (modify state)
func isIdempotentStep(stepName string) bool {
	idempotentSteps := map[string]bool{
		"analyze":  true,
		"plan":     true,
		"validate": true,
		"review":   true,
		"test":     true,
		"lint":     true,
	}
	return idempotentSteps[stepName]
}

// GetRecoveryContext returns the recovery context from a hook.
// Creates one if it doesn't exist.
func GetRecoveryContext(hook *domain.Hook) *domain.RecoveryContext {
	if hook.Recovery == nil {
		return nil
	}
	return hook.Recovery
}
