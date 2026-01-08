package steps

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/validation"
)

// mockToolChecker implements validation.ToolChecker for testing.
type mockToolChecker struct {
	installed bool
	version   string
	err       error
}

// IsGoPreCommitInstalled implements validation.ToolChecker.
func (m *mockToolChecker) IsGoPreCommitInstalled(_ context.Context) (bool, string, error) {
	return m.installed, m.version, m.err
}

// Ensure mockToolChecker implements ToolChecker.
var _ validation.ToolChecker = (*mockToolChecker)(nil)

// mockCommandRunner implements CommandRunner for testing.
// It is thread-safe to support parallel pipeline execution.
type mockCommandRunner struct {
	mu      sync.Mutex
	results map[string]mockCommandResult // keyed by command prefix
	calls   []string
}

type mockCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func newMockCommandRunner() *mockCommandRunner {
	return &mockCommandRunner{
		results: make(map[string]mockCommandResult),
		calls:   make([]string, 0),
	}
}

// SetResult sets the result for a specific command (matches by prefix).
func (m *mockCommandRunner) SetResult(cmdPrefix string, result mockCommandResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[cmdPrefix] = result
}

// SetDefaultSuccess sets all commands to succeed by default.
func (m *mockCommandRunner) SetDefaultSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[""] = mockCommandResult{exitCode: 0}
}

func (m *mockCommandRunner) Run(_ context.Context, _, command string) (stdout, stderr string, exitCode int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, command)

	// Look for exact match first
	if result, ok := m.results[command]; ok {
		return result.stdout, result.stderr, result.exitCode, result.err
	}

	// Fallback to default
	if result, ok := m.results[""]; ok {
		return result.stdout, result.stderr, result.exitCode, result.err
	}

	return "", "", 0, nil
}

func (m *mockCommandRunner) GetCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

func TestNewValidationExecutor(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	require.NotNil(t, executor)
	assert.Equal(t, "/tmp/work", executor.workDir)
	assert.NotNil(t, executor.runner)
}

func TestValidationExecutor_Type(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	assert.Equal(t, domain.StepTypeValidation, executor.Type())
}

func TestValidationExecutor_Execute_AllSuccess(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	toolChecker := &mockToolChecker{installed: true, version: "1.0.0"}

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithAll(tmpDir, runner, toolChecker, nil, nil, nil)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{}, // Uses default pipeline
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "validate", result.StepName)
	assert.Contains(t, result.Output, "All validations passed")
	// Default pipeline has 4 commands: format, lint, test, pre-commit
	calls := runner.GetCalls()
	assert.GreaterOrEqual(t, len(calls), 4)
}

func TestValidationExecutor_Execute_FailsOnError(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	// Make lint fail
	runner.SetResult("magex lint", mockCommandResult{
		stdout:   "lint output",
		stderr:   "lint error",
		exitCode: 1,
		err:      atlaserrors.ErrCommandFailed,
	})

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithRunner(tmpDir, runner)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Output, "✗")
}

func TestValidationExecutor_Execute_DefaultCommands(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	toolChecker := &mockToolChecker{installed: true, version: "1.0.0"}

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithAll(tmpDir, runner, toolChecker, nil, nil, nil)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{}, // No validation commands - uses defaults
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)

	// Verify default commands were run
	calls := runner.GetCalls()
	hasFormat := false
	hasLint := false
	hasTest := false
	hasPreCommit := false
	for _, call := range calls {
		if call == "magex format:fix" {
			hasFormat = true
		}
		if call == "magex lint" {
			hasLint = true
		}
		if call == "magex test" {
			hasTest = true
		}
		if call == "go-pre-commit run --all-files" {
			hasPreCommit = true
		}
	}
	assert.True(t, hasFormat, "should run format command")
	assert.True(t, hasLint, "should run lint command")
	assert.True(t, hasTest, "should run test command")
	assert.True(t, hasPreCommit, "should run pre-commit command")
}

func TestValidationExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewValidationExecutor("/tmp/work")
	task := &domain.Task{ID: "task-123", WorkspaceID: "ws-123"}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestValidationExecutor_Execute_CapturesOutput(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithRunner(tmpDir, runner)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Captures formatted output from FormatResult
	assert.Contains(t, result.Output, "All validations passed")
}

func TestValidationExecutor_Execute_EmptyCommands(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	toolChecker := &mockToolChecker{installed: true, version: "1.0.0"}

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithAll(tmpDir, runner, toolChecker, nil, nil, nil)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		Config:      domain.TaskConfig{ValidationCommands: []string{}}, // Empty slice
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	// Should use defaults when empty - the parallel pipeline runs 4 commands
	calls := runner.GetCalls()
	assert.GreaterOrEqual(t, len(calls), 4)
}

func TestValidationExecutor_Execute_Timing(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithRunner(tmpDir, runner)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.True(t, result.CompletedAt.After(result.StartedAt) || result.CompletedAt.Equal(result.StartedAt))
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestValidationExecutor_Execute_WithArtifactSaver(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()

	// Create mock artifact saver
	var savedData []byte
	var savedBaseName string
	mockSaver := &validationMockArtifactSaver{
		saveFn: func(_ context.Context, _, _, baseName string, data []byte) (string, error) {
			savedData = data
			savedBaseName = baseName
			return "validation.1.json", nil
		},
	}

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithAll(tmpDir, runner, nil, mockSaver, nil, nil)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "validation.json", savedBaseName)
	assert.NotEmpty(t, savedData)
}

func TestValidationExecutor_Execute_WithNotifier(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	// Make lint fail
	runner.SetResult("magex lint", mockCommandResult{
		exitCode: 1,
	})

	// Create mock notifier
	mockNotifier := &mockStepsNotifier{}
	// Need artifact saver for the handler to be created
	mockSaver := &validationMockArtifactSaver{}

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithAll(tmpDir, runner, nil, mockSaver, mockNotifier, nil)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.True(t, mockNotifier.bellCalled, "bell should be called on failure")
}

// validationMockArtifactSaver for testing validation step artifact saving.
type validationMockArtifactSaver struct {
	saveFn func(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
}

func (m *validationMockArtifactSaver) SaveArtifact(_ context.Context, _, _, _ string, _ []byte) error {
	return nil
}

func (m *validationMockArtifactSaver) SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error) {
	if m.saveFn != nil {
		return m.saveFn(ctx, workspaceName, taskID, baseName, data)
	}
	return "validation.1.json", nil
}

// mockStepsNotifier for testing.
type mockStepsNotifier struct {
	bellCalled bool
}

func (m *mockStepsNotifier) Bell() {
	m.bellCalled = true
}

// mockRetryHandler for testing.
type mockRetryHandler struct {
	enabled     bool
	maxAttempts int
}

func (m *mockRetryHandler) CanRetry(attemptNum int) bool {
	return m.enabled && attemptNum <= m.maxAttempts
}

func (m *mockRetryHandler) MaxAttempts() int {
	return m.maxAttempts
}

func (m *mockRetryHandler) IsEnabled() bool {
	return m.enabled
}

func TestValidationExecutor_CanRetry_WithHandler(t *testing.T) {
	retryHandler := &mockRetryHandler{enabled: true, maxAttempts: 3}
	executor := NewValidationExecutorWithAll("/tmp/work", nil, nil, nil, nil, retryHandler)

	assert.True(t, executor.CanRetry(1))
	assert.True(t, executor.CanRetry(2))
	assert.True(t, executor.CanRetry(3))
	assert.False(t, executor.CanRetry(4))
}

func TestValidationExecutor_CanRetry_WithoutHandler(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	assert.False(t, executor.CanRetry(1))
}

func TestValidationExecutor_RetryEnabled_WithHandler(t *testing.T) {
	retryHandler := &mockRetryHandler{enabled: true, maxAttempts: 3}
	executor := NewValidationExecutorWithAll("/tmp/work", nil, nil, nil, nil, retryHandler)

	assert.True(t, executor.RetryEnabled())
}

func TestValidationExecutor_RetryEnabled_Disabled(t *testing.T) {
	retryHandler := &mockRetryHandler{enabled: false, maxAttempts: 3}
	executor := NewValidationExecutorWithAll("/tmp/work", nil, nil, nil, nil, retryHandler)

	assert.False(t, executor.RetryEnabled())
}

func TestValidationExecutor_RetryEnabled_WithoutHandler(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	assert.False(t, executor.RetryEnabled())
}

func TestValidationExecutor_MaxRetryAttempts_WithHandler(t *testing.T) {
	retryHandler := &mockRetryHandler{enabled: true, maxAttempts: 5}
	executor := NewValidationExecutorWithAll("/tmp/work", nil, nil, nil, nil, retryHandler)

	assert.Equal(t, 5, executor.MaxRetryAttempts())
}

func TestValidationExecutor_MaxRetryAttempts_WithoutHandler(t *testing.T) {
	executor := NewValidationExecutor("/tmp/work")

	assert.Equal(t, 0, executor.MaxRetryAttempts())
}

// TestBuildValidationChecks tests the PipelineResult.BuildChecksAsMap method via the step executor.
// Full unit tests for BuildChecksAsMap are in internal/validation/result_test.go
func TestBuildValidationChecks(t *testing.T) {
	t.Run("all passing", func(t *testing.T) {
		result := &validation.PipelineResult{
			Success: true,
			FormatResults: []validation.Result{
				{Command: "magex format:fix", Success: true},
			},
			LintResults: []validation.Result{
				{Command: "magex lint", Success: true},
			},
			TestResults: []validation.Result{
				{Command: "magex test", Success: true},
			},
			PreCommitResults: []validation.Result{
				{Command: "go-pre-commit", Success: true},
			},
		}

		checks := result.BuildChecksAsMap()

		require.Len(t, checks, 4)
		assert.Equal(t, "Format", checks[0]["name"])
		assert.True(t, checks[0]["passed"].(bool))
		assert.Equal(t, "Lint", checks[1]["name"])
		assert.True(t, checks[1]["passed"].(bool))
		assert.Equal(t, "Test", checks[2]["name"])
		assert.True(t, checks[2]["passed"].(bool))
		assert.Equal(t, "Pre-commit", checks[3]["name"])
		assert.True(t, checks[3]["passed"].(bool))
	})

	t.Run("lint fails", func(t *testing.T) {
		result := &validation.PipelineResult{
			Success:        false,
			FailedStepName: "lint",
			FormatResults: []validation.Result{
				{Command: "magex format:fix", Success: true},
			},
			LintResults: []validation.Result{
				{Command: "magex lint", Success: false, ExitCode: 1},
			},
			TestResults: []validation.Result{
				{Command: "magex test", Success: true},
			},
		}

		checks := result.BuildChecksAsMap()

		require.Len(t, checks, 4)
		assert.True(t, checks[0]["passed"].(bool))  // Format passed
		assert.False(t, checks[1]["passed"].(bool)) // Lint failed
		assert.True(t, checks[2]["passed"].(bool))  // Test passed
		assert.True(t, checks[3]["passed"].(bool))  // Pre-commit passed (no results = passed)
	})

	t.Run("pre-commit skipped", func(t *testing.T) {
		result := &validation.PipelineResult{
			Success: true,
			FormatResults: []validation.Result{
				{Command: "magex format:fix", Success: true},
			},
			LintResults: []validation.Result{
				{Command: "magex lint", Success: true},
			},
			TestResults: []validation.Result{
				{Command: "magex test", Success: true},
			},
			SkippedSteps: []string{"pre-commit"},
			SkipReasons:  map[string]string{"pre-commit": "go-pre-commit not installed"},
		}

		checks := result.BuildChecksAsMap()

		require.Len(t, checks, 4)
		assert.True(t, checks[0]["passed"].(bool))
		assert.True(t, checks[1]["passed"].(bool))
		assert.True(t, checks[2]["passed"].(bool))
		assert.True(t, checks[3]["passed"].(bool))
		assert.True(t, checks[3]["skipped"].(bool)) // Pre-commit is marked as skipped
	})

	t.Run("multiple lint failures", func(t *testing.T) {
		result := &validation.PipelineResult{
			Success:        false,
			FailedStepName: "lint",
			FormatResults: []validation.Result{
				{Command: "gofmt", Success: true},
			},
			LintResults: []validation.Result{
				{Command: "golangci-lint", Success: false},
				{Command: "go vet", Success: true},
			},
		}

		checks := result.BuildChecksAsMap()

		assert.True(t, checks[0]["passed"].(bool))  // Format passed
		assert.False(t, checks[1]["passed"].(bool)) // Lint failed (one failure)
	})

	t.Run("empty results treated as passed", func(t *testing.T) {
		result := &validation.PipelineResult{
			Success:       true,
			FormatResults: []validation.Result{}, // Empty
			LintResults:   nil,                   // Nil
			TestResults:   []validation.Result{}, // Empty
		}

		checks := result.BuildChecksAsMap()

		require.Len(t, checks, 4)
		assert.True(t, checks[0]["passed"].(bool)) // Format passed (empty)
		assert.True(t, checks[1]["passed"].(bool)) // Lint passed (nil)
		assert.True(t, checks[2]["passed"].(bool)) // Test passed (empty)
		assert.True(t, checks[3]["passed"].(bool)) // Pre-commit passed (nil)
	})
}

// Tests for hasFailedResult are in internal/validation/result_test.go

// TestValidationExecutor_Execute_IncludesMetadata tests that validation checks are stored in metadata.
func TestValidationExecutor_Execute_IncludesMetadata(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	toolChecker := &mockToolChecker{installed: true, version: "1.0.0"}

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithAll(tmpDir, runner, toolChecker, nil, nil, nil)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)

	// Verify metadata contains validation_checks
	require.NotNil(t, result.Metadata)
	checksData, ok := result.Metadata["validation_checks"]
	require.True(t, ok, "Metadata should contain validation_checks")

	checks, ok := checksData.([]map[string]any)
	require.True(t, ok, "validation_checks should be []map[string]any")
	require.Len(t, checks, 4)

	// Verify check names
	assert.Equal(t, "Format", checks[0]["name"])
	assert.Equal(t, "Lint", checks[1]["name"])
	assert.Equal(t, "Test", checks[2]["name"])
	assert.Equal(t, "Pre-commit", checks[3]["name"])

	// Verify all passed
	for _, check := range checks {
		assert.True(t, check["passed"].(bool), "Check %s should pass", check["name"])
	}
}

// TestValidationExecutor_Execute_MetadataOnFailure tests that metadata is included on failure.
func TestValidationExecutor_Execute_MetadataOnFailure(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	// Make lint fail
	runner.SetResult("magex lint", mockCommandResult{
		exitCode: 1,
	})

	// Create temp directory for test
	tmpDir := t.TempDir()
	executor := NewValidationExecutorWithRunner(tmpDir, runner)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)

	// Verify metadata contains validation_checks even on failure
	require.NotNil(t, result.Metadata)
	checksData, ok := result.Metadata["validation_checks"]
	require.True(t, ok, "Metadata should contain validation_checks on failure")

	checks, ok := checksData.([]map[string]any)
	require.True(t, ok, "validation_checks should be []map[string]any")

	// Find lint check and verify it failed
	var lintCheck map[string]any
	for _, check := range checks {
		if check["name"] == "Lint" {
			lintCheck = check
			break
		}
	}
	require.NotNil(t, lintCheck, "should have Lint check")
	assert.False(t, lintCheck["passed"].(bool), "Lint should be marked as failed")
}

func TestNewValidationExecutorFull(t *testing.T) {
	commands := ValidationCommands{
		Format:    []string{"custom-format"},
		Lint:      []string{"custom-lint"},
		Test:      []string{"custom-test"},
		PreCommit: []string{"custom-precommit"},
	}
	executor := NewValidationExecutorFull("/tmp/work", nil, nil, nil, commands)

	require.NotNil(t, executor)
	assert.Equal(t, "/tmp/work", executor.workDir)
	assert.Equal(t, []string{"custom-format"}, executor.formatCommands)
	assert.Equal(t, []string{"custom-lint"}, executor.lintCommands)
	assert.Equal(t, []string{"custom-test"}, executor.testCommands)
	assert.Equal(t, []string{"custom-precommit"}, executor.preCommitCommands)
}

func TestValidationExecutor_Execute_UsesCustomCommands(t *testing.T) {
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	toolChecker := &mockToolChecker{installed: true, version: "1.0.0"}

	// Create executor with custom commands from config
	commands := ValidationCommands{
		Format:    []string{"my-formatter --fix"},
		Lint:      []string{"my-linter ./..."},
		Test:      []string{"my-test:race"},
		PreCommit: []string{"my-precommit run"},
	}

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Create executor with custom commands
	executor := &ValidationExecutor{
		workDir:           tmpDir,
		runner:            runner,
		toolChecker:       toolChecker,
		formatCommands:    commands.Format,
		lintCommands:      commands.Lint,
		testCommands:      commands.Test,
		preCommitCommands: commands.PreCommit,
	}

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)

	// Verify custom commands were run instead of defaults
	calls := runner.GetCalls()
	hasCustomFormat := false
	hasCustomLint := false
	hasCustomTest := false
	hasCustomPreCommit := false
	hasDefaultFormat := false
	hasDefaultLint := false
	hasDefaultTest := false
	hasDefaultPreCommit := false

	for _, call := range calls {
		switch call {
		case "my-formatter --fix":
			hasCustomFormat = true
		case "my-linter ./...":
			hasCustomLint = true
		case "my-test:race":
			hasCustomTest = true
		case "my-precommit run":
			hasCustomPreCommit = true
		case "magex format:fix":
			hasDefaultFormat = true
		case "magex lint":
			hasDefaultLint = true
		case "magex test":
			hasDefaultTest = true
		case "go-pre-commit run --all-files":
			hasDefaultPreCommit = true
		}
	}

	// Should use custom commands
	assert.True(t, hasCustomFormat, "should run custom format command")
	assert.True(t, hasCustomLint, "should run custom lint command")
	assert.True(t, hasCustomTest, "should run custom test command")
	assert.True(t, hasCustomPreCommit, "should run custom pre-commit command")

	// Should NOT use default commands
	assert.False(t, hasDefaultFormat, "should NOT run default format command")
	assert.False(t, hasDefaultLint, "should NOT run default lint command")
	assert.False(t, hasDefaultTest, "should NOT run default test command")
	assert.False(t, hasDefaultPreCommit, "should NOT run default pre-commit command")
}

func TestValidationExecutor_Execute_UsesTestRaceFromConfig(t *testing.T) {
	// This test specifically verifies the bug fix where "magex test:race"
	// from config was being ignored in favor of the default "magex test"
	ctx := context.Background()
	runner := newMockCommandRunner()
	runner.SetDefaultSuccess()
	toolChecker := &mockToolChecker{installed: true, version: "1.0.0"}

	// Simulate config with "magex test:race" - this was the reported bug
	commands := ValidationCommands{
		Format:    []string{"magex format:fix"},
		Lint:      []string{"magex lint"},
		Test:      []string{"magex test:race"}, // The key config value that was being ignored
		PreCommit: []string{"go-pre-commit run --all-files"},
	}

	// Create temp directory for test
	tmpDir := t.TempDir()

	executor := &ValidationExecutor{
		workDir:           tmpDir,
		runner:            runner,
		toolChecker:       toolChecker,
		formatCommands:    commands.Format,
		lintCommands:      commands.Lint,
		testCommands:      commands.Test,
		preCommitCommands: commands.PreCommit,
	}

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "ws-123",
		CurrentStep: 0,
		Config:      domain.TaskConfig{},
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)

	// Verify that "magex test:race" was called, not "magex test"
	calls := runner.GetCalls()
	hasTestRace := false
	hasTestDefault := false

	for _, call := range calls {
		if call == "magex test:race" {
			hasTestRace = true
		}
		if call == "magex test" {
			hasTestDefault = true
		}
	}

	assert.True(t, hasTestRace, "should run 'magex test:race' from config")
	assert.False(t, hasTestDefault, "should NOT run default 'magex test'")
}

func TestValidationExecutor_BuildRunnerConfig(t *testing.T) {
	t.Run("uses executor commands when task has none", func(t *testing.T) {
		executor := &ValidationExecutor{
			workDir:           "/tmp/work",
			formatCommands:    []string{"custom-format"},
			lintCommands:      []string{"custom-lint"},
			testCommands:      []string{"custom-test"},
			preCommitCommands: []string{"custom-precommit"},
		}

		task := &domain.Task{
			Config: domain.TaskConfig{},
		}

		config := executor.buildRunnerConfig(task)

		assert.Equal(t, []string{"custom-format"}, config.FormatCommands)
		assert.Equal(t, []string{"custom-lint"}, config.LintCommands)
		assert.Equal(t, []string{"custom-test"}, config.TestCommands)
		assert.Equal(t, []string{"custom-precommit"}, config.PreCommitCommands)
	})

	t.Run("task validation commands are ignored in favor of config separation", func(t *testing.T) {
		executor := &ValidationExecutor{
			workDir:           "/tmp/work",
			formatCommands:    []string{"custom-format"},
			lintCommands:      []string{"custom-lint"},
			testCommands:      []string{"custom-test"},
			preCommitCommands: []string{"custom-precommit"},
		}

		task := &domain.Task{
			Config: domain.TaskConfig{
				ValidationCommands: []string{"task-specific-lint"}, // Legacy field (ignored)
			},
		}

		config := executor.buildRunnerConfig(task)

		// All commands should use executor's config commands; task.ValidationCommands is ignored
		assert.Equal(t, []string{"custom-format"}, config.FormatCommands)
		assert.Equal(t, []string{"custom-lint"}, config.LintCommands)
		assert.Equal(t, []string{"custom-test"}, config.TestCommands)
		assert.Equal(t, []string{"custom-precommit"}, config.PreCommitCommands)
	})

	t.Run("empty executor commands let defaults apply", func(t *testing.T) {
		executor := &ValidationExecutor{
			workDir: "/tmp/work",
			// No custom commands - empty slices
		}

		task := &domain.Task{
			Config: domain.TaskConfig{},
		}

		config := executor.buildRunnerConfig(task)

		// Empty slices are passed through; the validation.Runner applies defaults
		assert.Empty(t, config.FormatCommands)
		assert.Empty(t, config.LintCommands)
		assert.Empty(t, config.TestCommands)
		assert.Empty(t, config.PreCommitCommands)
	})
}

func TestValidationExecutor_Execute_DetectOnlyMode(t *testing.T) {
	t.Run("detect_only returns success even when validation fails", func(t *testing.T) {
		ctx := context.Background()
		runner := newMockCommandRunner()
		// Set lint to fail
		runner.SetResult("magex lint", mockCommandResult{
			stdout:   "",
			stderr:   "lint error: unused variable",
			exitCode: 1,
			err:      atlaserrors.ErrValidationFailed,
		})
		// Other commands succeed
		runner.SetResult("magex format:fix", mockCommandResult{exitCode: 0})
		runner.SetResult("magex test:race", mockCommandResult{exitCode: 0})

		toolChecker := &mockToolChecker{installed: false} // No pre-commit

		tmpDir := t.TempDir()

		executor := &ValidationExecutor{
			workDir:        tmpDir,
			runner:         runner,
			toolChecker:    toolChecker,
			formatCommands: []string{"magex format:fix"},
			lintCommands:   []string{"magex lint"},
			testCommands:   []string{"magex test:race"},
		}

		task := &domain.Task{
			ID:          "task-detect-only",
			WorkspaceID: "ws-detect-only",
			CurrentStep: 0,
		}

		// Step with detect_only: true
		step := &domain.StepDefinition{
			Name: "detect",
			Type: domain.StepTypeValidation,
			Config: map[string]any{
				"detect_only": true,
			},
		}

		result, err := executor.Execute(ctx, task, step)

		// Should return success (no error) even though validation failed
		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)

		// Check metadata contains validation_failed flag
		validationFailed, ok := result.Metadata["validation_failed"].(bool)
		require.True(t, ok, "metadata should contain validation_failed")
		assert.True(t, validationFailed, "validation_failed should be true")

		// Check detect_only flag is in metadata
		detectOnly, ok := result.Metadata["detect_only"].(bool)
		require.True(t, ok, "metadata should contain detect_only")
		assert.True(t, detectOnly, "detect_only should be true")

		// Check pipeline_result is in metadata
		_, ok = result.Metadata["pipeline_result"].(*validation.PipelineResult)
		assert.True(t, ok, "metadata should contain pipeline_result")
	})

	t.Run("detect_only with all passing returns validation_failed=false", func(t *testing.T) {
		ctx := context.Background()
		runner := newMockCommandRunner()
		runner.SetDefaultSuccess()

		toolChecker := &mockToolChecker{installed: false}
		tmpDir := t.TempDir()

		executor := &ValidationExecutor{
			workDir:        tmpDir,
			runner:         runner,
			toolChecker:    toolChecker,
			formatCommands: []string{"magex format:fix"},
			lintCommands:   []string{"magex lint"},
			testCommands:   []string{"magex test:race"},
		}

		task := &domain.Task{
			ID:          "task-detect-only-pass",
			WorkspaceID: "ws-detect-only-pass",
			CurrentStep: 0,
		}

		step := &domain.StepDefinition{
			Name: "detect",
			Type: domain.StepTypeValidation,
			Config: map[string]any{
				"detect_only": true,
			},
		}

		result, err := executor.Execute(ctx, task, step)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)

		// Should have validation_failed=false since all passed
		validationFailed, ok := result.Metadata["validation_failed"].(bool)
		require.True(t, ok)
		assert.False(t, validationFailed, "validation_failed should be false when all pass")
	})

	t.Run("normal mode (detect_only=false) fails on validation error", func(t *testing.T) {
		ctx := context.Background()
		runner := newMockCommandRunner()
		runner.SetResult("magex lint", mockCommandResult{
			stderr:   "lint error",
			exitCode: 1,
			err:      atlaserrors.ErrValidationFailed,
		})
		runner.SetResult("magex format:fix", mockCommandResult{exitCode: 0})

		toolChecker := &mockToolChecker{installed: false}
		tmpDir := t.TempDir()

		executor := &ValidationExecutor{
			workDir:        tmpDir,
			runner:         runner,
			toolChecker:    toolChecker,
			formatCommands: []string{"magex format:fix"},
			lintCommands:   []string{"magex lint"},
		}

		task := &domain.Task{
			ID:          "task-normal-mode",
			WorkspaceID: "ws-normal-mode",
			CurrentStep: 0,
		}

		// Step without detect_only (normal mode)
		step := &domain.StepDefinition{
			Name:   "validate",
			Type:   domain.StepTypeValidation,
			Config: map[string]any{},
		}

		result, err := executor.Execute(ctx, task, step)

		// Should return error in normal mode
		require.Error(t, err)
		assert.Equal(t, "failed", result.Status)
	})
}
