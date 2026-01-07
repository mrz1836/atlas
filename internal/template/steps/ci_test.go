package steps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// errTestNetwork is a test error for network failures.
var errTestNetwork = atlaserrors.ErrGitHubOperation

// ciMockHubRunner is a mock implementation of git.HubRunner for CI testing.
type ciMockHubRunner struct {
	watchResult *git.CIWatchResult
	watchErr    error
	callCount   int
	watchFn     func(context.Context, git.CIWatchOptions) (*git.CIWatchResult, error)
}

func (m *ciMockHubRunner) CreatePR(_ context.Context, _ git.PRCreateOptions) (*git.PRResult, error) {
	return &git.PRResult{}, nil
}

func (m *ciMockHubRunner) GetPRStatus(_ context.Context, _ int) (*git.PRStatus, error) {
	return &git.PRStatus{}, nil
}

func (m *ciMockHubRunner) WatchPRChecks(ctx context.Context, opts git.CIWatchOptions) (*git.CIWatchResult, error) {
	m.callCount++
	if m.watchFn != nil {
		return m.watchFn(ctx, opts)
	}
	return m.watchResult, m.watchErr
}

func (m *ciMockHubRunner) ConvertToDraft(_ context.Context, _ int) error {
	return nil
}

func (m *ciMockHubRunner) MergePR(_ context.Context, _ int, _ string, _, _ bool) error {
	return nil
}

func (m *ciMockHubRunner) AddPRReview(_ context.Context, _ int, _, _ string) error {
	return nil
}

func (m *ciMockHubRunner) AddPRComment(_ context.Context, _ int, _ string) error {
	return nil
}

// mockCIFailureHandler implements CIFailureHandlerInterface for testing.
type mockCIFailureHandler struct {
	hasHandler bool
}

func (m *mockCIFailureHandler) HasHandler() bool {
	return m.hasHandler
}

func TestNewCIExecutor(t *testing.T) {
	executor := NewCIExecutor()

	require.NotNil(t, executor)
	assert.Nil(t, executor.hubRunner)
	assert.Nil(t, executor.ciFailureHandler)
}

func TestNewCIExecutor_WithOptions(t *testing.T) {
	mockRunner := &ciMockHubRunner{}
	mockHandler := &mockCIFailureHandler{hasHandler: true}
	logger := zerolog.New(os.Stderr)

	executor := NewCIExecutor(
		WithCIHubRunner(mockRunner),
		WithCIFailureHandlerInterface(mockHandler),
		WithCILogger(logger),
	)

	require.NotNil(t, executor)
	assert.Equal(t, mockRunner, executor.hubRunner)
	assert.Equal(t, mockHandler, executor.ciFailureHandler)
}

func TestCIExecutor_Type(t *testing.T) {
	executor := NewCIExecutor()

	assert.Equal(t, domain.StepTypeCI, executor.Type())
}

func TestCIExecutor_Execute_Success(t *testing.T) {
	// Setup mock
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: 5 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / lint", Bucket: "pass", State: "SUCCESS"},
				{Name: "CI / test", Bucket: "pass", State: "SUCCESS"},
			},
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-test-success",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name:    "ci-wait",
		Type:    domain.StepTypeCI,
		Timeout: 30 * time.Minute,
		Config: map[string]any{
			"poll_interval": 10 * time.Millisecond,
		},
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "ci-wait", result.StepName)
	assert.Contains(t, result.Output, "CI passed")
	assert.Contains(t, result.Output, "2 checks")
	assert.Equal(t, 1, mockRunner.callCount)
}

func TestCIExecutor_Execute_Failure_NoHandler(t *testing.T) {
	// Setup mock with failure
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusFailure,
			ElapsedTime: 3 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / lint", Bucket: "pass", State: "SUCCESS"},
				{Name: "CI / test", Bucket: "fail", State: "FAILURE", URL: "https://github.com/test/logs"},
			},
			Error: atlaserrors.ErrCIFailed,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-test-failure",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name:    "ci-wait",
		Type:    domain.StepTypeCI,
		Timeout: 30 * time.Minute,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrCIFailed)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Output, "CI checks failed")
	assert.Contains(t, result.Output, "CI / test")
	assert.Equal(t, "ci checks failed", result.Error)
}

func TestCIExecutor_Execute_Failure_WithHandler(t *testing.T) {
	// Setup mock with failure
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusFailure,
			ElapsedTime: 3 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / test", Bucket: "fail", State: "FAILURE"},
			},
			Error: atlaserrors.ErrCIFailed,
		},
	}
	mockHandler := &mockCIFailureHandler{hasHandler: true}

	executor := NewCIExecutor(
		WithCIHubRunner(mockRunner),
		WithCIFailureHandlerInterface(mockHandler),
	)

	task := &domain.Task{
		ID:          "task-test-failure-handler",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name:    "ci-wait",
		Type:    domain.StepTypeCI,
		Timeout: 30 * time.Minute,
	}

	result, err := executor.Execute(context.Background(), task, step)

	// With handler, we return awaiting_approval instead of failed
	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Contains(t, result.Output, "CI checks failed")
}

func TestCIExecutor_Execute_Timeout(t *testing.T) {
	// Setup mock with timeout
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusTimeout,
			ElapsedTime: 30 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / test", Bucket: "pending", State: "PENDING"},
			},
			Error: atlaserrors.ErrCITimeout,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-test-timeout",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name:    "ci-wait",
		Type:    domain.StepTypeCI,
		Timeout: 30 * time.Minute,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Contains(t, result.Output, "timed out")
}

func TestCIExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := NewCIExecutor(WithCIHubRunner(&ciMockHubRunner{}))
	task := &domain.Task{ID: "task-123", Metadata: map[string]any{"pr_number": 42}}
	step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

	_, err := executor.Execute(ctx, task, step)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCIExecutor_Execute_MissingHubRunner(t *testing.T) {
	executor := NewCIExecutor() // No HubRunner

	task := &domain.Task{
		ID:          "task-no-runner",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name: "ci-wait",
		Type: domain.StepTypeCI,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrExecutorNotFound)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "HubRunner")
}

func TestCIExecutor_Execute_MissingPRNumber(t *testing.T) {
	executor := NewCIExecutor(WithCIHubRunner(&ciMockHubRunner{}))

	testCases := []struct {
		name     string
		metadata map[string]any
	}{
		{"nil metadata", nil},
		{"empty metadata", map[string]any{}},
		{"wrong type", map[string]any{"pr_number": "not-a-number"}},
		{"zero value", map[string]any{"pr_number": 0}},
		{"negative value", map[string]any{"pr_number": -1}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{
				ID:       "task-missing-pr",
				Metadata: tc.metadata,
			}
			step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

			result, err := executor.Execute(context.Background(), task, step)

			require.Error(t, err)
			require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
			assert.Equal(t, "failed", result.Status)
		})
	}
}

func TestCIExecutor_Execute_PRNumberTypes(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	testCases := []struct {
		name     string
		prNumber any
	}{
		{"int", 42},
		{"int64", int64(42)},
		{"float64", float64(42)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{
				ID:       "task-pr-type",
				Metadata: map[string]any{"pr_number": tc.prNumber},
			}
			step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

			result, err := executor.Execute(context.Background(), task, step)

			require.NoError(t, err)
			assert.Equal(t, "success", result.Status)
		})
	}
}

func TestCIExecutor_Execute_PollIntervalConfig(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	testCases := []struct {
		name   string
		config map[string]any
	}{
		{"duration type", map[string]any{"poll_interval": 5 * time.Second}},
		{"string type", map[string]any{"poll_interval": "5s"}},
		{"int seconds", map[string]any{"poll_interval": 5}},
		{"int64 seconds", map[string]any{"poll_interval": int64(5)}},
		{"float64 seconds", map[string]any{"poll_interval": float64(5)}},
		{"nil config", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{
				ID:       "task-poll-config",
				Metadata: map[string]any{"pr_number": 42},
			}
			step := &domain.StepDefinition{
				Name:   "ci",
				Type:   domain.StepTypeCI,
				Config: tc.config,
			}

			result, err := executor.Execute(context.Background(), task, step)

			require.NoError(t, err)
			assert.Equal(t, "success", result.Status)
		})
	}
}

func TestCIExecutor_UsesRuntimeConfig(t *testing.T) {
	// Create CI config with custom values
	ciConfig := &config.CIConfig{
		PollInterval: 45 * time.Second,
		Timeout:      15 * time.Minute,
		GracePeriod:  30 * time.Second,
	}

	var capturedOpts git.CIWatchOptions
	mockRunner := &ciMockHubRunner{
		watchFn: func(_ context.Context, opts git.CIWatchOptions) (*git.CIWatchResult, error) {
			capturedOpts = opts
			return &git.CIWatchResult{
				Status:      git.CIStatusSuccess,
				ElapsedTime: time.Second,
			}, nil
		},
	}

	executor := NewCIExecutor(
		WithCIHubRunner(mockRunner),
		WithCIConfig(ciConfig),
	)

	task := &domain.Task{
		ID:       "test-runtime",
		Metadata: map[string]any{"pr_number": 42},
	}

	step := &domain.StepDefinition{
		Name: "ci",
		Type: domain.StepTypeCI,
		// No step.Config override - should use runtime config
	}

	result, err := executor.Execute(context.Background(), task, step)
	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)

	// Verify runtime config was used
	assert.Equal(t, 45*time.Second, capturedOpts.Interval, "should use runtime poll_interval")
	assert.Equal(t, 15*time.Minute, capturedOpts.Timeout, "should use runtime timeout")
}

func TestCIExecutor_Execute_WorkflowFiltering(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:       "task-workflow-filter",
		Metadata: map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name: "ci",
		Type: domain.StepTypeCI,
		Config: map[string]any{
			"workflows": []string{"CI", "Lint"},
		},
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}

func TestCIExecutor_Execute_WorkflowFilteringAnySlice(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:       "task-workflow-filter-any",
		Metadata: map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name: "ci",
		Type: domain.StepTypeCI,
		Config: map[string]any{
			"workflows": []any{"CI", "Lint"},
		},
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}

func TestCIExecutor_Execute_WatchError(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchErr: errTestNetwork,
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:       "task-watch-error",
		Metadata: map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

	_, err := executor.Execute(context.Background(), task, step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to watch PR checks")
}

func TestCIExecutor_Execute_ArtifactSaving(t *testing.T) {
	saver := newTestArtifactSaver()

	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: 5 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / lint", Bucket: "pass", State: "SUCCESS", Duration: time.Minute},
				{Name: "CI / test", Bucket: "pass", State: "SUCCESS"},
			},
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner), WithCIArtifactSaver(saver))

	task := &domain.Task{
		ID:          "task-artifact-save",
		WorkspaceID: "test-ws",
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{Name: "ci-wait", Type: domain.StepTypeCI}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.NotEmpty(t, result.ArtifactPath)

	// Verify artifact was saved via the saver
	expectedFilename := filepath.Join("ci-wait", "ci-result.json")
	data, ok := saver.savedArtifacts[expectedFilename]
	require.True(t, ok, "artifact should have been saved")
	assert.Contains(t, string(data), "success")
}

func TestCIExecutor_Execute_FailureArtifactWithFailedChecks(t *testing.T) {
	saver := newTestArtifactSaver()

	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusFailure,
			ElapsedTime: 3 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / lint", Bucket: "pass", State: "SUCCESS"},
				{Name: "CI / test", Bucket: "fail", State: "FAILURE", URL: "https://github.com/logs"},
				{Name: "CI / build", Bucket: "cancel", State: "CANCELED"},
			},
			Error: atlaserrors.ErrCIFailed,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner), WithCIArtifactSaver(saver))

	task := &domain.Task{
		ID:          "task-failure-artifact",
		WorkspaceID: "test-ws",
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{Name: "ci-wait", Type: domain.StepTypeCI}

	result, err := executor.Execute(context.Background(), task, step)

	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.NotEmpty(t, result.ArtifactPath)

	// Verify artifact was saved with failure info
	expectedFilename := filepath.Join("ci-wait", "ci-result.json")
	data, ok := saver.savedArtifacts[expectedFilename]
	require.True(t, ok, "artifact should have been saved")
	assert.Contains(t, string(data), "failure")
	assert.Contains(t, string(data), "failed_checks")
}

func TestCIExecutor_Execute_Timing(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:       "task-timing",
		Metadata: map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.True(t, result.CompletedAt.After(result.StartedAt) || result.CompletedAt.Equal(result.StartedAt))
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestCIExecutor_Execute_StepIndex(t *testing.T) {
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-step-index",
		CurrentStep: 5,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, 5, result.StepIndex)
}

func TestExtractDuration(t *testing.T) {
	defaultVal := 10 * time.Second

	testCases := []struct {
		name     string
		config   map[string]any
		key      string
		expected time.Duration
	}{
		{"nil config", nil, "key", defaultVal},
		{"missing key", map[string]any{}, "key", defaultVal},
		{"duration type", map[string]any{"key": 5 * time.Second}, "key", 5 * time.Second},
		{"string type", map[string]any{"key": "5s"}, "key", 5 * time.Second},
		{"invalid string", map[string]any{"key": "invalid"}, "key", defaultVal},
		{"int type", map[string]any{"key": 5}, "key", 5 * time.Second},
		{"int64 type", map[string]any{"key": int64(5)}, "key", 5 * time.Second},
		{"float64 type", map[string]any{"key": float64(5)}, "key", 5 * time.Second},
		{"unsupported type", map[string]any{"key": []int{5}}, "key", defaultVal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractDuration(tc.config, tc.key, defaultVal)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractStringSlice(t *testing.T) {
	testCases := []struct {
		name     string
		config   map[string]any
		key      string
		expected []string
	}{
		{"nil config", nil, "key", nil},
		{"missing key", map[string]any{}, "key", nil},
		{"string slice", map[string]any{"key": []string{"a", "b"}}, "key", []string{"a", "b"}},
		{"any slice", map[string]any{"key": []any{"a", "b"}}, "key", []string{"a", "b"}},
		{"mixed any slice", map[string]any{"key": []any{"a", 123, "b"}}, "key", []string{"a", "b"}},
		{"unsupported type", map[string]any{"key": "single"}, "key", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractStringSlice(tc.config, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCIExecutor_FormatCIFailureMessage(t *testing.T) {
	executor := NewCIExecutor()

	result := &git.CIWatchResult{
		CheckResults: []git.CheckResult{
			{Name: "CI / lint", Bucket: "pass", State: "SUCCESS"},
			{Name: "CI / test", Bucket: "fail", State: "FAILURE", URL: "https://github.com/logs/1"},
			{Name: "CI / build", Bucket: "cancel", State: "CANCELED", URL: "https://github.com/logs/2"},
		},
	}

	msg := executor.formatCIFailureMessage(result)

	assert.Contains(t, msg, "CI checks failed")
	assert.Contains(t, msg, "CI / test")
	assert.Contains(t, msg, "CI / build")
	assert.Contains(t, msg, "https://github.com/logs/1")
	assert.NotContains(t, msg, "CI / lint") // Pass should not be included
}

func TestCIExecutor_Execute_FailureHandler_NotConfigured(t *testing.T) {
	// Handler exists but returns HasHandler() = false
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusFailure,
			ElapsedTime: time.Second,
			Error:       atlaserrors.ErrCIFailed,
		},
	}
	mockHandler := &mockCIFailureHandler{hasHandler: false}

	executor := NewCIExecutor(
		WithCIHubRunner(mockRunner),
		WithCIFailureHandlerInterface(mockHandler),
	)

	task := &domain.Task{
		ID:       "task-handler-not-configured",
		Metadata: map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{Name: "ci", Type: domain.StepTypeCI}

	result, err := executor.Execute(context.Background(), task, step)

	// Should return simple failure when handler not properly configured
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrCIFailed)
	assert.Equal(t, "failed", result.Status)
}

func TestCIExecutor_Execute_SkipsWhenNoChanges(t *testing.T) {
	// Test that CI step is skipped when skip_git_steps metadata is set
	// This happens when the commit step finds no changes to commit
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-no-changes",
		CurrentStep: 5,
		Metadata: map[string]any{
			"skip_git_steps": true, // Set by commit step when no changes
		},
	}
	step := &domain.StepDefinition{
		Name: "ci-wait",
		Type: domain.StepTypeCI,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "skipped", result.Status)
	assert.Equal(t, 5, result.StepIndex)
	assert.Equal(t, "ci-wait", result.StepName)
	assert.Contains(t, result.Output, "no PR was created")
	assert.Contains(t, result.Output, "no changes to commit")
	// Verify WatchPRChecks was never called
	assert.Equal(t, 0, mockRunner.callCount)
}

func TestCIExecutor_Execute_DoesNotSkipWhenFlagFalse(t *testing.T) {
	// Test that CI step runs normally when skip_git_steps is false
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusSuccess,
			ElapsedTime: time.Second,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-with-changes",
		CurrentStep: 0,
		Metadata: map[string]any{
			"skip_git_steps": false,
			"pr_number":      42,
		},
	}
	step := &domain.StepDefinition{
		Name: "ci-wait",
		Type: domain.StepTypeCI,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	// Verify WatchPRChecks was called
	assert.Equal(t, 1, mockRunner.callCount)
}

func TestCIExecutor_Execute_FetchError(t *testing.T) {
	// Test that CIStatusFetchError transitions to awaiting_approval
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusFetchError,
			ElapsedTime: 5 * time.Minute,
			Error:       atlaserrors.ErrCIFetchFailed,
		},
	}

	executor := NewCIExecutor(WithCIHubRunner(mockRunner))

	task := &domain.Task{
		ID:          "task-fetch-error",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name:    "ci-wait",
		Type:    domain.StepTypeCI,
		Timeout: 30 * time.Minute,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.Contains(t, result.Output, "Unable to fetch CI status")
	assert.Equal(t, "ci-wait", result.StepName)

	// Verify metadata contains failure_type for dispatch
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "ci_fetch_error", result.Metadata["failure_type"])
}

func TestCIExecutor_Execute_FetchError_WithArtifact(t *testing.T) {
	// Test that fetch error saves artifact with error details
	saver := newTestArtifactSaver()
	mockRunner := &ciMockHubRunner{
		watchResult: &git.CIWatchResult{
			Status:      git.CIStatusFetchError,
			ElapsedTime: 5 * time.Minute,
			Error:       atlaserrors.ErrCIFetchFailed,
		},
	}

	executor := NewCIExecutor(
		WithCIHubRunner(mockRunner),
		WithCIArtifactSaver(saver),
	)

	task := &domain.Task{
		ID:          "task-fetch-error-artifact",
		WorkspaceID: "test-workspace",
		CurrentStep: 0,
		Metadata:    map[string]any{"pr_number": 42},
	}
	step := &domain.StepDefinition{
		Name: "ci-wait",
		Type: domain.StepTypeCI,
	}

	result, err := executor.Execute(context.Background(), task, step)

	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", result.Status)
	assert.NotEmpty(t, result.ArtifactPath)

	// Verify artifact was saved to the artifact saver
	assert.Len(t, saver.savedArtifacts, 1)
	artifactKey := "ci-wait/ci-result.json"
	assert.Contains(t, saver.savedArtifacts, artifactKey)
}
