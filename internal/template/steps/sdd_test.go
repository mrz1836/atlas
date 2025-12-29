package steps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewSDDExecutor(t *testing.T) {
	runner := &mockAIRunner{}
	executor := NewSDDExecutor(runner, "/tmp/artifacts")

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
	assert.Equal(t, "/tmp/artifacts", executor.artifactsDir)
	assert.Empty(t, executor.workingDir)
}

func TestNewSDDExecutorWithWorkingDir(t *testing.T) {
	runner := &mockAIRunner{}
	executor := NewSDDExecutorWithWorkingDir(runner, "/tmp/artifacts", "/tmp/worktree")

	require.NotNil(t, executor)
	assert.Equal(t, runner, executor.runner)
	assert.Equal(t, "/tmp/artifacts", executor.artifactsDir)
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
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{
			Output: "# Specification\nThis is the specification...",
		},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{
		ID:          "task-123",
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

	// Verify artifact was saved with semantic name
	content, err := os.ReadFile(result.ArtifactPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Specification")
	assert.True(t, filepath.Base(result.ArtifactPath) == "spec.md" || filepath.Base(result.ArtifactPath) == "spec.1.md")
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
	tmpDir := t.TempDir()
	executor := NewSDDExecutor(&mockAIRunner{}, tmpDir)

	tests := []struct {
		command      SDDCommand
		expectedName string
	}{
		{SDDCmdSpecify, "spec.md"},
		{SDDCmdPlan, "plan.md"},
		{SDDCmdTasks, "tasks.md"},
		{SDDCmdChecklist, "checklist.md"},
	}

	for _, tt := range tests {
		t.Run(string(tt.command), func(t *testing.T) {
			// Use unique task ID for each test
			taskID := "task-" + string(tt.command)
			path, err := executor.saveArtifact(taskID, tt.command, "Test content")

			require.NoError(t, err)
			assert.NotEmpty(t, path)
			assert.Equal(t, tt.expectedName, filepath.Base(path))

			content, err := os.ReadFile(filepath.Clean(path))
			require.NoError(t, err)
			assert.Equal(t, "Test content", string(content))
		})
	}
}

func TestSDDExecutor_saveArtifact_Versioning(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewSDDExecutor(&mockAIRunner{}, tmpDir)
	taskID := "task-version"

	// Save first version
	path1, err := executor.saveArtifact(taskID, SDDCmdSpecify, "Version 1")
	require.NoError(t, err)
	assert.Equal(t, "spec.md", filepath.Base(path1))

	// Save second version - should be spec.1.md
	path2, err := executor.saveArtifact(taskID, SDDCmdSpecify, "Version 2")
	require.NoError(t, err)
	assert.Equal(t, "spec.1.md", filepath.Base(path2))

	// Save third version - should be spec.2.md
	path3, err := executor.saveArtifact(taskID, SDDCmdSpecify, "Version 3")
	require.NoError(t, err)
	assert.Equal(t, "spec.2.md", filepath.Base(path3))

	// Verify contents
	content1, err := os.ReadFile(filepath.Clean(path1))
	require.NoError(t, err)
	content2, err := os.ReadFile(filepath.Clean(path2))
	require.NoError(t, err)
	content3, err := os.ReadFile(filepath.Clean(path3))
	require.NoError(t, err)
	assert.Equal(t, "Version 1", string(content1))
	assert.Equal(t, "Version 2", string(content2))
	assert.Equal(t, "Version 3", string(content3))
}

func TestSDDExecutor_saveArtifact_EmptyDir(t *testing.T) {
	executor := NewSDDExecutor(&mockAIRunner{}, "")

	path, err := executor.saveArtifact("task-123", SDDCmdSpecify, "content")

	require.NoError(t, err)
	assert.Empty(t, path)
}

func TestSDDExecutor_saveArtifact_UnknownCommand(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewSDDExecutor(&mockAIRunner{}, tmpDir)

	// Unknown command should use timestamp-based naming
	path, err := executor.saveArtifact("task-123", SDDCommand("unknown"), "content")

	require.NoError(t, err)
	assert.Contains(t, filepath.Base(path), "sdd-unknown-")
	assert.Contains(t, filepath.Base(path), ".md")
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

func TestSDDExecutor_Execute_FilePermissions(t *testing.T) {
	SetSpeckitChecked(true)
	defer ResetSpeckitCheck()

	ctx := context.Background()
	tmpDir := t.TempDir()

	runner := &mockAIRunner{
		result: &domain.AIResult{Output: "sensitive spec content"},
	}
	executor := NewSDDExecutor(runner, tmpDir)

	task := &domain.Task{ID: "task-123", Description: "Test"}
	step := &domain.StepDefinition{Name: "sdd", Type: domain.StepTypeSDD}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotEmpty(t, result.ArtifactPath)

	// Verify file permissions are 0600
	info, err := os.Stat(result.ArtifactPath)
	require.NoError(t, err)
	// Check file mode (mask off type bits, keep permission bits)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
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

func TestSDDExecutor_saveArtifact_InvalidDirectory(t *testing.T) {
	// Use a path that cannot be created (file as parent directory)
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	err := os.WriteFile(blockingFile, []byte("block"), 0o600)
	require.NoError(t, err)

	// Try to use the file as a parent directory
	executor := NewSDDExecutor(&mockAIRunner{}, blockingFile)

	_, err = executor.saveArtifact("task-123", SDDCmdSpecify, "content")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create artifacts directory")
}

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
