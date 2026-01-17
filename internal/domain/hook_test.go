package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookState_Values(t *testing.T) {
	// Verify all 9 states are defined
	states := []HookState{
		HookStateInitializing,
		HookStateStepPending,
		HookStateStepRunning,
		HookStateStepValidating,
		HookStateAwaitingHuman,
		HookStateRecovering,
		HookStateCompleted,
		HookStateFailed,
		HookStateAbandoned,
	}

	assert.Len(t, states, 9, "expected 9 hook states")

	// Verify string values
	expectedValues := map[HookState]string{
		HookStateInitializing:   "initializing",
		HookStateStepPending:    "step_pending",
		HookStateStepRunning:    "step_running",
		HookStateStepValidating: "step_validating",
		HookStateAwaitingHuman:  "awaiting_human",
		HookStateRecovering:     "recovering",
		HookStateCompleted:      "completed",
		HookStateFailed:         "failed",
		HookStateAbandoned:      "abandoned",
	}

	for state, expected := range expectedValues {
		assert.Equal(t, expected, string(state), "state %s has unexpected string value", state)
	}
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    HookState
		terminal bool
	}{
		{HookStateInitializing, false},
		{HookStateStepPending, false},
		{HookStateStepRunning, false},
		{HookStateStepValidating, false},
		{HookStateAwaitingHuman, false},
		{HookStateRecovering, false},
		{HookStateCompleted, true},
		{HookStateFailed, true},
		{HookStateAbandoned, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.terminal, IsTerminalState(tt.state))
		})
	}
}

func TestValidHookTransitions(t *testing.T) {
	validTransitions := GetValidTransitions()

	// Verify initial state can only transition to initializing
	initialTransitions, ok := validTransitions[""]
	require.True(t, ok, "empty state should have transitions")
	assert.Equal(t, []HookState{HookStateInitializing}, initialTransitions)

	// Verify terminal states have no outgoing transitions
	terminalStates := []HookState{HookStateCompleted, HookStateFailed, HookStateAbandoned}
	for _, state := range terminalStates {
		transitions, exists := validTransitions[state]
		assert.False(t, exists || len(transitions) > 0, "terminal state %s should have no transitions", state)
	}

	// Verify step_running can transition to expected states
	runningTransitions := validTransitions[HookStateStepRunning]
	assert.Contains(t, runningTransitions, HookStateStepValidating)
	assert.Contains(t, runningTransitions, HookStateStepPending)
	assert.Contains(t, runningTransitions, HookStateAwaitingHuman)
}

func TestCheckpointTrigger_Values(t *testing.T) {
	triggers := []CheckpointTrigger{
		CheckpointTriggerManual,
		CheckpointTriggerCommit,
		CheckpointTriggerPush,
		CheckpointTriggerPR,
		CheckpointTriggerValidation,
		CheckpointTriggerStepComplete,
		CheckpointTriggerInterval,
	}

	assert.Len(t, triggers, 7, "expected 7 checkpoint triggers")

	expectedValues := map[CheckpointTrigger]string{
		CheckpointTriggerManual:       "manual",
		CheckpointTriggerCommit:       "git_commit",
		CheckpointTriggerPush:         "git_push",
		CheckpointTriggerPR:           "pr_created",
		CheckpointTriggerValidation:   "validation",
		CheckpointTriggerStepComplete: "step_complete",
		CheckpointTriggerInterval:     "interval",
	}

	for trigger, expected := range expectedValues {
		assert.Equal(t, expected, string(trigger))
	}
}

func TestHook_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := &Hook{
		Version:     "1.0",
		TaskID:      "task-20260117-143022",
		WorkspaceID: "fix-null-pointer",
		CreatedAt:   now,
		UpdatedAt:   now.Add(time.Minute),
		State:       HookStateStepRunning,
		CurrentStep: &StepContext{
			StepName:    "implement",
			StepIndex:   2,
			StartedAt:   now,
			Attempt:     1,
			MaxAttempts: 3,
			WorkingOn:   "Adding nil checks",
			FilesTouched: []string{
				"config/parser.go",
				"config/parser_test.go",
			},
			LastOutput: "Working on nil checks...",
		},
		History: []HookEvent{
			{
				Timestamp: now,
				FromState: "",
				ToState:   HookStateInitializing,
				Trigger:   "task_start",
			},
		},
		Checkpoints: []StepCheckpoint{
			{
				CheckpointID: "ckpt-a1b2c3d4",
				CreatedAt:    now,
				StepName:     "implement",
				StepIndex:    2,
				Description:  "Added nil check",
				Trigger:      CheckpointTriggerCommit,
				GitBranch:    "fix/fix-null-pointer",
				GitCommit:    "abc123",
				GitDirty:     false,
			},
		},
		Receipts: []ValidationReceipt{
			{
				ReceiptID:   "rcpt-00000001",
				StepName:    "analyze",
				Command:     "magex lint",
				ExitCode:    0,
				StartedAt:   now,
				CompletedAt: now.Add(12 * time.Second),
				Duration:    "12.3s",
				StdoutHash:  "a1b2c3d4",
				StderrHash:  "00000000",
				Signature:   "3045022100",
			},
		},
		SchemaVersion: "1.0",
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var restored Hook
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, original.Version, restored.Version)
	assert.Equal(t, original.TaskID, restored.TaskID)
	assert.Equal(t, original.WorkspaceID, restored.WorkspaceID)
	assert.Equal(t, original.CreatedAt, restored.CreatedAt)
	assert.Equal(t, original.UpdatedAt, restored.UpdatedAt)
	assert.Equal(t, original.State, restored.State)
	assert.Equal(t, original.SchemaVersion, restored.SchemaVersion)

	// Verify CurrentStep
	require.NotNil(t, restored.CurrentStep)
	assert.Equal(t, original.CurrentStep.StepName, restored.CurrentStep.StepName)
	assert.Equal(t, original.CurrentStep.Attempt, restored.CurrentStep.Attempt)
	assert.Equal(t, original.CurrentStep.FilesTouched, restored.CurrentStep.FilesTouched)

	// Verify History
	require.Len(t, restored.History, 1)
	assert.Equal(t, original.History[0].Trigger, restored.History[0].Trigger)

	// Verify Checkpoints
	require.Len(t, restored.Checkpoints, 1)
	assert.Equal(t, original.Checkpoints[0].CheckpointID, restored.Checkpoints[0].CheckpointID)
	assert.Equal(t, original.Checkpoints[0].Trigger, restored.Checkpoints[0].Trigger)

	// Verify Receipts
	require.Len(t, restored.Receipts, 1)
	assert.Equal(t, original.Receipts[0].ReceiptID, restored.Receipts[0].ReceiptID)
	assert.Equal(t, original.Receipts[0].Signature, restored.Receipts[0].Signature)
}

func TestStepContext_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := StepContext{
		StepName:            "implement",
		StepIndex:           2,
		StartedAt:           now,
		Attempt:             2,
		MaxAttempts:         3,
		WorkingOn:           "Adding nil checks",
		FilesTouched:        []string{"file1.go", "file2.go"},
		LastOutput:          "Last output here",
		CurrentCheckpointID: "ckpt-12345678",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored StepContext
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original, restored)
}

func TestFileSnapshot_JSONRoundTrip(t *testing.T) {
	original := FileSnapshot{
		Path:    "config/parser.go",
		Size:    4523,
		ModTime: "2026-01-17T14:42:10Z",
		SHA256:  "e3b0c44298fc1c14",
		Exists:  true,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored FileSnapshot
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original, restored)
}

func TestRecoveryContext_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := RecoveryContext{
		DetectedAt:        now,
		CrashType:         "timeout",
		LastKnownState:    HookStateStepRunning,
		WasValidating:     true,
		ValidationCmd:     "magex lint",
		PartialOutput:     "Linting...",
		RecommendedAction: "retry_step",
		Reason:            "Step is idempotent, safe to retry",
		LastCheckpointID:  "ckpt-a1b2c3d4",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored RecoveryContext
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original, restored)
}

func TestValidationReceipt_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := ValidationReceipt{
		ReceiptID:   "rcpt-00000001",
		StepName:    "analyze",
		Command:     "magex lint",
		ExitCode:    0,
		StartedAt:   now,
		CompletedAt: now.Add(12 * time.Second),
		Duration:    "12.3s",
		StdoutHash:  "a1b2c3d4e5f6a7b8",
		StderrHash:  "0000000000000000",
		Signature:   "3045022100abcdef",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored ValidationReceipt
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original, restored)
}

func TestHookEvent_WithDetails(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := HookEvent{
		Timestamp: now,
		FromState: HookStateStepPending,
		ToState:   HookStateStepRunning,
		Trigger:   "step_started",
		StepName:  "implement",
		Details: map[string]any{
			"attempt":   1,
			"file_path": "config/parser.go",
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored HookEvent
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original.Timestamp, restored.Timestamp)
	assert.Equal(t, original.FromState, restored.FromState)
	assert.Equal(t, original.ToState, restored.ToState)
	assert.Equal(t, original.Trigger, restored.Trigger)
	assert.Equal(t, original.StepName, restored.StepName)

	// Details are restored as map[string]any with float64 for numbers
	assert.InDelta(t, 1.0, restored.Details["attempt"], 0.001)
	assert.Equal(t, "config/parser.go", restored.Details["file_path"])
}

func TestHook_EmptyOptionalFields(t *testing.T) {
	// Test that optional fields serialize correctly when empty/nil
	now := time.Now().UTC().Truncate(time.Second)
	minimal := &Hook{
		Version:       "1.0",
		TaskID:        "task-123",
		WorkspaceID:   "ws-456",
		CreatedAt:     now,
		UpdatedAt:     now,
		State:         HookStateInitializing,
		History:       []HookEvent{},
		Checkpoints:   []StepCheckpoint{},
		Receipts:      []ValidationReceipt{},
		SchemaVersion: "1.0",
	}

	data, err := json.Marshal(minimal)
	require.NoError(t, err)

	// Verify omitempty fields are not present
	var m map[string]any
	err = json.Unmarshal(data, &m)
	require.NoError(t, err)

	// current_step should be omitted (nil)
	_, hasCurrentStep := m["current_step"]
	assert.False(t, hasCurrentStep, "current_step should be omitted when nil")

	// recovery should be omitted (nil)
	_, hasRecovery := m["recovery"]
	assert.False(t, hasRecovery, "recovery should be omitted when nil")
}
