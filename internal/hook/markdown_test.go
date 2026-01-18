package hook

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestMarkdownGenerator_Generate(t *testing.T) {
	gen := NewMarkdownGenerator()

	t.Run("fresh task - initializing state", func(t *testing.T) {
		hook := &domain.Hook{
			Version:       "1.0",
			TaskID:        "task-20260117-143022",
			WorkspaceID:   "fix-null-pointer",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			State:         domain.HookStateInitializing,
			History:       []domain.HookEvent{},
			Checkpoints:   []domain.StepCheckpoint{},
			Receipts:      []domain.ValidationReceipt{},
			SchemaVersion: "1.0",
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "# ATLAS Task Recovery Hook")
		assert.Contains(t, content, "task-20260117-143022")
		assert.Contains(t, content, "fix-null-pointer")
		assert.Contains(t, content, "`initializing`")
		// Should not have recovery section
		assert.NotContains(t, content, "What To Do Now")
	})

	t.Run("mid-step crash - with current step", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:      "task-123",
			WorkspaceID: "ws-456",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			State:       domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:     "implement",
				StepIndex:    2,
				Attempt:      1,
				MaxAttempts:  3,
				WorkingOn:    "Adding nil checks",
				FilesTouched: []string{"config/parser.go", "config/parser_test.go"},
				LastOutput:   "I've added the nil check for Server...",
			},
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "Current Step")
		assert.Contains(t, content, "`implement`")
		assert.Contains(t, content, "Attempt | 1/3")
		assert.Contains(t, content, "Files Touched")
		assert.Contains(t, content, "config/parser.go")
		assert.Contains(t, content, "Last Output")
	})

	t.Run("with recovery context", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:      "task-123",
			WorkspaceID: "ws-456",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			State:       domain.HookStateRecovering,
			CurrentStep: &domain.StepContext{
				StepName:     "implement",
				FilesTouched: []string{"file.go"},
			},
			Recovery: &domain.RecoveryContext{
				RecommendedAction: "retry_step",
				Reason:            "Step is idempotent, safe to retry",
				CrashType:         "step_interrupted",
				LastKnownState:    domain.HookStateStepRunning,
			},
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "What To Do Now")
		assert.Contains(t, content, "Retry Step")
		assert.Contains(t, content, "idempotent")
	})

	t.Run("with manual recovery recommendation", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:    "task-123",
			State:     domain.HookStateRecovering,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			CurrentStep: &domain.StepContext{
				StepName:     "implement",
				FilesTouched: []string{"file.go"},
			},
			Recovery: &domain.RecoveryContext{
				RecommendedAction: "manual",
				Reason:            "Step modifies state",
			},
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "Manual Intervention")
		assert.Contains(t, content, "git status")
	})

	t.Run("with checkpoints", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:    "task-123",
			State:     domain.HookStateStepRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Checkpoints: []domain.StepCheckpoint{
				{
					CheckpointID: "ckpt-001",
					CreatedAt:    time.Now().Add(-10 * time.Minute),
					Trigger:      domain.CheckpointTriggerCommit,
					StepName:     "implement",
					Description:  "Added nil check",
					GitBranch:    "fix/null-pointer",
					GitCommit:    "abc123",
				},
				{
					CheckpointID: "ckpt-002",
					CreatedAt:    time.Now().Add(-5 * time.Minute),
					Trigger:      domain.CheckpointTriggerManual,
					StepName:     "implement",
					Description:  "Before refactor",
				},
			},
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "Checkpoint Timeline")
		assert.Contains(t, content, "ckpt-001")
		assert.Contains(t, content, "ckpt-002")
		assert.Contains(t, content, "Added nil check")
		assert.Contains(t, content, "git_commit")
		assert.Contains(t, content, "manual")
	})

	t.Run("with validation receipts", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:    "task-123",
			State:     domain.HookStateStepRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Receipts: []domain.ValidationReceipt{
				{
					ReceiptID: "rcpt-001",
					StepName:  "analyze",
					Command:   "magex lint",
					ExitCode:  0,
					Duration:  "12.3s",
					Signature: "abc123", // Has signature = valid
				},
				{
					ReceiptID: "rcpt-002",
					StepName:  "plan",
					Command:   "magex test",
					ExitCode:  0,
					Duration:  "45.6s",
					Signature: "def456",
				},
			},
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "Completed Steps")
		assert.Contains(t, content, "Validation Receipts")
		assert.Contains(t, content, "analyze")
		assert.Contains(t, content, "magex lint")
		assert.Contains(t, content, "12.3s")
	})

	t.Run("with history", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:    "task-123",
			State:     domain.HookStateStepRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			History: []domain.HookEvent{
				{
					Timestamp: time.Now().Add(-30 * time.Minute),
					FromState: "",
					ToState:   domain.HookStateInitializing,
					Trigger:   "task_start",
				},
				{
					Timestamp: time.Now().Add(-25 * time.Minute),
					FromState: domain.HookStateInitializing,
					ToState:   domain.HookStateStepPending,
					Trigger:   "setup_complete",
				},
			},
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "State History")
		assert.Contains(t, content, "task_start")
		assert.Contains(t, content, "setup_complete")
	})

	t.Run("DO NOT section always present", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:    "task-123",
			State:     domain.HookStateStepRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "DO NOT")
		assert.Contains(t, content, "start the task from the beginning")
		assert.Contains(t, content, "repeat steps")
	})

	t.Run("troubleshooting section present", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:    "task-123",
			State:     domain.HookStateStepRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		md, err := gen.Generate(hook)
		require.NoError(t, err)
		content := string(md)

		assert.Contains(t, content, "Troubleshooting")
		assert.Contains(t, content, "git status")
		assert.Contains(t, content, "atlas hook regenerate")
		assert.Contains(t, content, "atlas hook export")
	})
}

func TestMarkdownGenerator_Regeneration(t *testing.T) {
	gen := NewMarkdownGenerator()

	t.Run("regeneration produces same content", func(t *testing.T) {
		hook := &domain.Hook{
			TaskID:      "task-123",
			WorkspaceID: "ws-456",
			CreatedAt:   time.Date(2026, 1, 17, 14, 30, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 1, 17, 14, 45, 0, 0, time.UTC),
			State:       domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 2,
				Attempt:   1,
			},
		}

		md1, err := gen.Generate(hook)
		require.NoError(t, err)

		md2, err := gen.Generate(hook)
		require.NoError(t, err)

		assert.Equal(t, string(md1), string(md2))
	})
}

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "just now"},
		{1 * time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{1 * time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{72 * time.Hour, "3 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatRelativeTime(time.Now().Add(-tt.duration))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"ab", 5, "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFilesTouchedList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		result := FormatFilesTouchedList(nil, 5)
		assert.Equal(t, "(none)", result)
	})

	t.Run("few files", func(t *testing.T) {
		files := []string{"a.go", "b.go", "c.go"}
		result := FormatFilesTouchedList(files, 5)
		assert.Equal(t, "a.go, b.go, c.go", result)
	})

	t.Run("many files", func(t *testing.T) {
		files := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"}
		result := FormatFilesTouchedList(files, 3)
		assert.Equal(t, "a.go, b.go, c.go (+3 more)", result)
	})
}

func TestGetStateEmoji(t *testing.T) {
	tests := []struct {
		state    domain.HookState
		expected string
	}{
		{domain.HookStateInitializing, "🔄"},
		{domain.HookStateStepPending, "⏳"},
		{domain.HookStateStepRunning, "▶️"},
		{domain.HookStateStepValidating, "🔍"},
		{domain.HookStateAwaitingHuman, "👤"},
		{domain.HookStateRecovering, "🔧"},
		{domain.HookStateCompleted, "✅"},
		{domain.HookStateFailed, "❌"},
		{domain.HookStateAbandoned, "🚫"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := getStateEmoji(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateHookMarkdown(t *testing.T) {
	hook := &domain.Hook{
		TaskID:    "task-123",
		State:     domain.HookStateStepRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	md, err := GenerateHookMarkdown(hook)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(md), "# ATLAS Task Recovery Hook"))
}
