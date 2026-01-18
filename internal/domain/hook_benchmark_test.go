package domain

import (
	"testing"
	"time"
)

// BenchmarkHook_DeepCopy benchmarks the manual deep copy implementation.
func BenchmarkHook_DeepCopy(b *testing.B) {
	hook := createBenchmarkHook()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = hook.DeepCopy()
	}
}

// BenchmarkHook_DeepCopy_WithHistory benchmarks deep copy with substantial history.
func BenchmarkHook_DeepCopy_WithHistory(b *testing.B) {
	hook := createBenchmarkHook()

	// Add 50 history events with details
	hook.History = make([]HookEvent, 50)
	for i := range hook.History {
		hook.History[i] = HookEvent{
			Timestamp: time.Now(),
			FromState: HookStateStepPending,
			ToState:   HookStateStepRunning,
			Trigger:   "test_trigger",
			StepName:  "step_name",
			Details: map[string]any{
				"key1":   "value1",
				"key2":   42,
				"nested": map[string]any{"inner": "value"},
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = hook.DeepCopy()
	}
}

// BenchmarkHook_DeepCopy_WithCheckpoints benchmarks deep copy with checkpoints.
func BenchmarkHook_DeepCopy_WithCheckpoints(b *testing.B) {
	hook := createBenchmarkHook()

	// Add 50 checkpoints with file snapshots
	hook.Checkpoints = make([]StepCheckpoint, 50)
	for i := range hook.Checkpoints {
		hook.Checkpoints[i] = StepCheckpoint{
			CheckpointID: "ckpt-12345678",
			CreatedAt:    time.Now(),
			StepName:     "implement",
			StepIndex:    i,
			Description:  "Periodic checkpoint during implementation",
			Trigger:      CheckpointTriggerInterval,
			GitBranch:    "feature/test",
			GitCommit:    "abc1234",
			GitDirty:     true,
			Artifacts:    []string{"file1.go", "file2.go", "file3.go"},
			FilesSnapshot: []FileSnapshot{
				{Path: "/path/to/file1.go", Size: 1234, ModTime: "2024-01-01T00:00:00Z", SHA256: "abc123", Exists: true},
				{Path: "/path/to/file2.go", Size: 5678, ModTime: "2024-01-01T00:00:00Z", SHA256: "def456", Exists: true},
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = hook.DeepCopy()
	}
}

// BenchmarkHook_DeepCopy_Full benchmarks deep copy with all fields populated.
func BenchmarkHook_DeepCopy_Full(b *testing.B) {
	hook := createBenchmarkHook()

	// Add history
	hook.History = make([]HookEvent, 20)
	for i := range hook.History {
		hook.History[i] = HookEvent{
			Timestamp: time.Now(),
			FromState: HookStateStepPending,
			ToState:   HookStateStepRunning,
			Trigger:   "trigger",
			Details:   map[string]any{"key": "value"},
		}
	}

	// Add checkpoints
	hook.Checkpoints = make([]StepCheckpoint, 10)
	for i := range hook.Checkpoints {
		hook.Checkpoints[i] = StepCheckpoint{
			CheckpointID:  "ckpt-12345678",
			CreatedAt:     time.Now(),
			StepName:      "step",
			FilesSnapshot: []FileSnapshot{{Path: "/file", Exists: true}},
		}
	}

	// Add receipts
	hook.Receipts = make([]ValidationReceipt, 5)
	for i := range hook.Receipts {
		hook.Receipts[i] = ValidationReceipt{
			ReceiptID:   "rcpt-12345678",
			StepName:    "validate",
			Command:     "go test ./...",
			ExitCode:    0,
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			Duration:    "1.5s",
		}
	}

	// Add recovery context
	hook.Recovery = &RecoveryContext{
		DetectedAt:        time.Now(),
		CrashType:         "timeout",
		LastKnownState:    HookStateStepRunning,
		RecommendedAction: "retry_step",
		Reason:            "step timed out",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = hook.DeepCopy()
	}
}

// createBenchmarkHook creates a typical hook for benchmarking.
func createBenchmarkHook() *Hook {
	return &Hook{
		Version:       "1.0",
		TaskID:        "task-12345678",
		WorkspaceID:   "workspace-abc",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		State:         HookStateStepRunning,
		SchemaVersion: "1.0",
		CurrentStep: &StepContext{
			StepName:            "implement",
			StepIndex:           2,
			StartedAt:           time.Now(),
			Attempt:             1,
			MaxAttempts:         3,
			WorkingOn:           "Adding feature X",
			FilesTouched:        []string{"file1.go", "file2.go", "file3.go"},
			LastOutput:          "Processing step...",
			CurrentCheckpointID: "ckpt-latest",
		},
		History:     make([]HookEvent, 0),
		Checkpoints: make([]StepCheckpoint, 0),
		Receipts:    make([]ValidationReceipt, 0),
	}
}
