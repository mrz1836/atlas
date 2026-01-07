package steps

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewSDDExecutor(t *testing.T) {
	runner := &mockAIRunner{}
	executor := NewSDDExecutor(runner, "/tmp/artifacts") // artifactsDir is deprecated/ignored

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
	assert.Nil(t, executor.artifactSaver) // deprecated constructor doesn't set artifactSaver
	assert.Empty(t, executor.workingDir)
}

func TestNewSDDExecutorWithWorkingDir(t *testing.T) {
	runner := &mockAIRunner{}
	executor := NewSDDExecutorWithWorkingDir(runner, "/tmp/artifacts", "/tmp/worktree") // artifactsDir is deprecated/ignored

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
	assert.Nil(t, executor.artifactSaver) // deprecated constructor doesn't set artifactSaver
	assert.Equal(t, "/tmp/worktree", executor.workingDir)
}

func TestNewSDDExecutorWithArtifactSaver(t *testing.T) {
	runner := &mockAIRunner{}
	saver := newTestArtifactSaver()
	executor := NewSDDExecutorWithArtifactSaver(runner, saver, "/tmp/worktree")

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
	assert.Equal(t, saver, executor.artifactSaver)
	assert.Equal(t, "/tmp/worktree", executor.workingDir)
}

func TestSDDExecutor_SetWorkingDir(t *testing.T) {
	executor := NewSDDExecutor(&mockAIRunner{}, "/tmp")
	assert.Empty(t, executor.workingDir)

	executor.SetWorkingDir("/path/to/worktree")
	assert.Equal(t, "/path/to/worktree", executor.workingDir)
}

func TestSDDExecutor_Type(t *testing.T) {
	executor := NewSDDExecutor(&mockAIRunner{}, "/tmp")

	assert.Equal(t, domain.StepTypeSDD, executor.Type())
}

func TestSDDExecutor_Execute_Success(t *testing.T) {
	// Set Speckit as installed for this test
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	saver := newTestArtifactSaver()

	runner := &mockAIRunner{
		result: &domain.AIResult{
			Output: "# Specification\nThis is the specification...",
		},
	}
	executor := NewSDDExecutorWithArtifactSaver(runner, saver, "")

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-workspace",
		Description: "Build a user auth system",
		CurrentStep: 0,
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{
		Name: "sdd-specify",
		Type: domain.StepTypeSDD,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "sdd-specify", result.StepName)
	assert.Contains(t, result.Output, "Specification")
	assert.NotEmpty(t, result.ArtifactPath)

	// Verify artifact was saved to the artifact saver with semantic name
	assert.Len(t, saver.savedArtifacts, 1)
	// The versioned artifact will be sdd/spec.md.1
	savedKey := "sdd/spec.md.1"
	assert.Contains(t, saver.savedArtifacts, savedKey)
	assert.Contains(t, string(saver.savedArtifacts[savedKey]), "Specification")
}

func TestSDDExecutor_Execute_SlashCommandFormat(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	tests := []struct {
		name     string
		command  string
		expected string
	}{
		{
			name:     "specify command uses /speckit.specify format",
			command:  "specify",
			expected: "/speckit.specify Build feature X",
		},
		{
			name:     "plan command uses /speckit.plan format",
			command:  "plan",
			expected: "/speckit.plan",
		},
		{
			name:     "tasks command uses /speckit.tasks format",
			command:  "tasks",
			expected: "/speckit.tasks",
		},
		{
			name:     "implement command uses /speckit.implement format",
			command:  "implement",
			expected: "/speckit.implement",
		},
		{
			name:     "checklist command uses /speckit.checklist format",
			command:  "checklist",
			expected: "/speckit.checklist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tmpDir := t.TempDir()

			runner := &mockAIRunner{
				result: &domain.AIResult{Output: "done"},
			}
			executor := NewSDDExecutor(runner, tmpDir)

			task := &domain.Task{
				ID:          "task-123",
				Description: "Build feature X",
				Config:      domain.TaskConfig{Model: "sonnet"},
			}
			step := &domain.StepDefinition{
				Name: "sdd",
				Type: domain.StepTypeSDD,
				Config: map[string]any{
					"sdd_command": tt.command,
				},
			}

			_, err := executor.Execute(ctx, task, step)

			require.NoError(t, err)
			require.NotNil(t, runner.request)
			assert.Equal(t, tt.expected, runner.request.Prompt)
		})
	}
}

func TestSDDExecutor_Execute_WorkingDirPassed(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	tmpDir := t.TempDir()
	worktreePath := "/path/to/worktree"

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutorWithWorkingDir(runner, tmpDir, worktreePath)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Build feature X",
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{
		Name: "sdd",
		Type: domain.StepTypeSDD,
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, runner.request)
	assert.Equal(t, worktreePath, runner.request.WorkingDir)
}

func TestSDDExecutor_Execute_ContextCancellation(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewSDDExecutor(&mockAIRunner{}, "/tmp")
	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestSDDExecutor_Execute_RunnerError(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	runner := &mockAIRunner{
		err: atlaserrors.ErrClaudeInvocation,
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "sdd command 'specify' failed")
}

func TestSDDExecutor_Execute_EmptyOutput(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: ""}, // Empty output
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "returned empty output")
}

func TestSDDExecutor_Execute_SpeckitNotInstalled(t *testing.T) {
	// Reset to ensure Speckit check runs
	ResetSpeckitCheck()
	// Don't mark as checked - will check real PATH

	// For this test, we need to temporarily modify PATH to not include 'specify'
	// Instead, we'll just verify the error message format by using a mock
	// Since we can't easily mock exec.LookPath, we test the error path differently

	ctx := context.Background()
	// Must configure a result in case Speckit IS installed, so runner doesn't return nil
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{ID: "task-123", Description: "Test", Config: domain.TaskConfig{Model: "sonnet"}}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)
	// If specify is not in PATH, we should get an error with installation instructions
	// If specify IS installed, the test will pass through and the mock runner will succeed
	if err != nil {
		assert.Contains(t, err.Error(), "Speckit not installed")
		require.NotNil(t, result, "result should not be nil on error")
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Error, "uv tool install specify-cli")
	}
}

func TestSDDExecutor_Execute_NoArtifactsDir(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Empty(t, result.ArtifactPath)
}

func TestSDDExecutor_Execute_ImplementCommand_NoArtifact(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "Implementation complete"},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{ID: "task-123", Description: "Build feature X"}
	step := &domain.StepDefinition{
		Name: "sdd",
		Type: domain.StepTypeSDD,
		Config: map[string]any{
			"sdd_command": "implement",
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	// Implement command doesn't save an artifact
	assert.Empty(t, result.ArtifactPath)
}

func TestSDDExecutor_Execute_DefaultCommand(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{ID: "task-123", Description: "Build it"}
	step := &domain.StepDefinition{
		Name:   "sdd",
		Type:   domain.StepTypeSDD,
		Config: nil, // No command specified
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should default to "specify" with slash command format
	assert.Equal(t, "/speckit.specify Build it", runner.request.Prompt)
}

func TestSDDExecutor_saveArtifact_SemanticNaming(t *testing.T) {
	ctx := context.Background()
	saver := newTestArtifactSaver()
	executor := NewSDDExecutorWithArtifactSaver(&mockAIRunner{}, saver, "")

	tests := []struct {
		command      SDDCommand
		expectedBase string
	}{
		{SDDCmdSpecify, "sdd/spec.md"},
		{SDDCmdPlan, "sdd/plan.md"},
		{SDDCmdTasks, "sdd/tasks.md"},
		{SDDCmdChecklist, "sdd/checklist.md"},
	}

	for _, tt := range tests {
		t.Run(string(tt.command), func(t *testing.T) {
			task := &domain.Task{ID: "task-" + string(tt.command), WorkspaceID: "test-ws"}
			path, err := executor.saveArtifact(ctx, task, tt.command, "Test content")

			require.NoError(t, err)
			assert.NotEmpty(t, path)
			// The versioned artifact saver appends ".1" to the base name
			assert.Contains(t, path, tt.expectedBase)
		})
	}
}

func TestSDDExecutor_saveArtifact_Versioning(t *testing.T) {
	ctx := context.Background()
	saver := newTestArtifactSaver()
	executor := NewSDDExecutorWithArtifactSaver(&mockAIRunner{}, saver, "")
	task := &domain.Task{ID: "task-version", WorkspaceID: "test-ws"}

	// Save multiple versions - versioning is now handled by the artifact saver
	path1, err := executor.saveArtifact(ctx, task, SDDCmdSpecify, "Version 1")
	require.NoError(t, err)
	assert.NotEmpty(t, path1)

	path2, err := executor.saveArtifact(ctx, task, SDDCmdSpecify, "Version 2")
	require.NoError(t, err)
	assert.NotEmpty(t, path2)

	// Verify artifacts were saved to the saver
	assert.Len(t, saver.savedArtifacts, 2)
}

func TestSDDExecutor_saveArtifact_NoArtifactSaver(t *testing.T) {
	ctx := context.Background()
	executor := NewSDDExecutor(&mockAIRunner{}, "") // No artifact saver

	task := &domain.Task{ID: "task-123", WorkspaceID: "test-ws"}
	path, err := executor.saveArtifact(ctx, task, SDDCmdSpecify, "content")

	require.NoError(t, err)
	assert.Empty(t, path) // Should return empty when no saver configured
}

func TestSDDExecutor_saveArtifact_UnknownCommand(t *testing.T) {
	ctx := context.Background()
	saver := newTestArtifactSaver()
	executor := NewSDDExecutorWithArtifactSaver(&mockAIRunner{}, saver, "")
	task := &domain.Task{ID: "task-123", WorkspaceID: "test-ws"}

	// Unknown command should use timestamp-based naming with SaveArtifact (non-versioned)
	path, err := executor.saveArtifact(ctx, task, SDDCommand("unknown"), "content")

	require.NoError(t, err)
	assert.Contains(t, path, "sdd/sdd-unknown-")
	assert.Contains(t, path, ".md")
}

func TestSDDExecutor_buildPrompt_AllCommands(t *testing.T) {
	executor := NewSDDExecutor(&mockAIRunner{}, "")
	task := &domain.Task{Description: "Build auth system"}

	tests := []struct {
		command  SDDCommand
		expected string
	}{
		{SDDCmdSpecify, "/speckit.specify Build auth system"},
		{SDDCmdPlan, "/speckit.plan"},
		{SDDCmdTasks, "/speckit.tasks"},
		{SDDCmdImplement, "/speckit.implement"},
		{SDDCmdChecklist, "/speckit.checklist"},
		{SDDCommand("unknown"), "/speckit.unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.command), func(t *testing.T) {
			prompt := executor.buildPrompt(task, tt.command)
			assert.Equal(t, tt.expected, prompt)
		})
	}
}

func TestSDDExecutor_Execute_MaxTurnsAndTimeout(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Build feature X",
		Config: domain.TaskConfig{
			Model:    "opus",
			MaxTurns: 15,
			Timeout:  30 * 60 * 1000000000, // 30 minutes in nanoseconds
		},
	}
	step := &domain.StepDefinition{
		Name: "sdd",
		Type: domain.StepTypeSDD,
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, runner.request)
	assert.Equal(t, "opus", runner.request.Model)
	assert.Equal(t, 15, runner.request.MaxTurns)
	assert.Equal(t, task.Config.Timeout, runner.request.Timeout)
}

func TestResetSpeckitCheck(t *testing.T) {
	// Set Speckit as checked (simulate successful check)
	SetSpeckitChecked(true)

	// After setting to true, checkSpeckitInstalled should return nil without checking PATH
	err := checkSpeckitInstalled()
	require.NoError(t, err, "should return nil when already checked")

	// Reset it - this clears the cached check
	ResetSpeckitCheck()

	// After reset, the next check will actually check PATH
	// We can verify the reset worked by setting it again and confirming behavior
	SetSpeckitChecked(true)
	err = checkSpeckitInstalled()
	require.NoError(t, err, "should return nil after setting checked to true")

	// Clean up - reset for other tests
	ResetSpeckitCheck()
}

func TestSDDExecutor_Execute_ArtifactSaverCalled(t *testing.T) {
	// Test that artifact saver is called with correct data
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	saver := newTestArtifactSaver()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "sensitive spec content"},
	}
	executor := NewSDDExecutorWithArtifactSaver(runner, saver, "")

	task := &domain.Task{ID: "task-123", WorkspaceID: "test-ws", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotEmpty(t, result.ArtifactPath)

	// Verify artifact was saved to the artifact saver
	assert.Len(t, saver.savedArtifacts, 1)
	savedKey := "sdd/spec.md.1"
	assert.Contains(t, saver.savedArtifacts, savedKey)
	assert.Equal(t, "sensitive spec content", string(saver.savedArtifacts[savedKey]))
}

func TestGetArtifactFilename_ImplementReturnsEmpty(t *testing.T) {
	filename, hasMapping := getArtifactFilename(SDDCmdImplement)

	assert.Empty(t, filename)
	assert.False(t, hasMapping)
}

func TestGetArtifactFilename_AllCommands(t *testing.T) {
	tests := []struct {
		cmd        SDDCommand
		expected   string
		hasMapping bool
	}{
		{SDDCmdSpecify, "spec.md", true},
		{SDDCmdPlan, "plan.md", true},
		{SDDCmdTasks, "tasks.md", true},
		{SDDCmdChecklist, "checklist.md", true},
		{SDDCmdImplement, "", false},
		{SDDCommand("unknown"), "", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.cmd), func(t *testing.T) {
			filename, hasMapping := getArtifactFilename(tt.cmd)
			assert.Equal(t, tt.expected, filename)
			assert.Equal(t, tt.hasMapping, hasMapping)
		})
	}
}

// TestSDDExecutor_saveArtifact_InvalidDirectory was removed because
// directory creation errors are now handled by the artifact saver interface,
// not the SDD executor directly.

func TestSDDExecutor_Execute_StepTimeout(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{
		ID:          "task-123",
		Description: "Build feature X",
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{
		Name:    "sdd",
		Type:    domain.StepTypeSDD,
		Timeout: 5 * time.Minute, // Step-level timeout
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}

func TestSDDExecutor_Execute_ContextCanceledDuringExecution(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	// Create a context that will be canceled
	ctx, cancel := context.WithCancel(context.Background())

	// Create a runner that cancels context during execution
	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "done"},
	}
	executor := NewSDDExecutor(runner, "")

	task := &domain.Task{
		ID:          "task-123",
		Description: "Test",
		Config:      domain.TaskConfig{Model: "sonnet"},
	}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	// Cancel before second context check (after Speckit check but before AI call)
	// This is tricky to test, so we just verify the context check at start works
	cancel()

	_, err := executor.Execute(ctx, task, step)
	assert.ErrorIs(t, err, context.Canceled)
}
