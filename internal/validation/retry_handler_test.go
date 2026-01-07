package validation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// MockAIRunner is a mock implementation of AIRunner for testing.
type MockAIRunner struct {
	RunFn func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}

func (m *MockAIRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	if m.RunFn != nil {
		return m.RunFn(ctx, req)
	}
	return &domain.AIResult{Success: true}, nil
}

// MockCommandRunnerForRetry is a mock command runner for testing retry scenarios.
// It is safe for concurrent use due to the parallel lint+test execution.
type MockCommandRunnerForRetry struct {
	Results     []Result
	Error       error
	ShouldFix   bool // If true, return success after first call
	callCount   int64
	mu          sync.Mutex
	callHistory []string
}

func (m *MockCommandRunnerForRetry) Run(_ context.Context, _, command string) (stdout, stderr string, exitCode int, err error) {
	count := atomic.AddInt64(&m.callCount, 1)

	m.mu.Lock()
	m.callHistory = append(m.callHistory, command)
	m.mu.Unlock()

	// If ShouldFix is true, succeed after first call (simulating AI fixed the issue)
	if m.ShouldFix && count > 1 {
		return "success", "", 0, nil
	}

	if m.Error != nil {
		return "", m.Error.Error(), 1, m.Error
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Results) > 0 {
		r := m.Results[0]
		if len(m.Results) > 1 {
			m.Results = m.Results[1:]
		}
		return r.Stdout, r.Stderr, r.ExitCode, nil
	}

	return "success", "", 0, nil
}

func (m *MockCommandRunnerForRetry) CallCount() int {
	return int(atomic.LoadInt64(&m.callCount))
}

func (m *MockCommandRunnerForRetry) CallHistory() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.callHistory))
	copy(result, m.callHistory)
	return result
}

func TestNewRetryHandler(t *testing.T) {
	mockAI := &MockAIRunner{}
	executor := NewExecutor(time.Minute)
	config := DefaultRetryConfig()
	logger := zerolog.Nop()

	handler := NewRetryHandler(mockAI, executor, config, logger)

	require.NotNil(t, handler)
	assert.Equal(t, 3, handler.MaxAttempts())
	assert.True(t, handler.IsEnabled())
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.True(t, config.Enabled)
}

func TestRetryHandler_RetryWithAI_InvokesAIAndRerunsValidation(t *testing.T) {
	var capturedPrompt string
	mockAI := &MockAIRunner{
		RunFn: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			capturedPrompt = req.Prompt
			return &domain.AIResult{
				Success:      true,
				FilesChanged: []string{"internal/foo.go"},
			}, nil
		},
	}

	// Mock command runner that succeeds after AI fix
	mockRunner := &MockCommandRunnerForRetry{
		ShouldFix: true,
	}
	executor := NewExecutorWithRunner(time.Minute, mockRunner)

	handler := NewRetryHandler(mockAI, executor, DefaultRetryConfig(), zerolog.Nop())

	failedResult := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		LintResults: []Result{
			{Command: "golangci-lint run", Success: false, ExitCode: 1, Stderr: "undefined: foo"},
		},
	}

	result, err := handler.RetryWithAI(context.Background(), failedResult, "/tmp", 1, nil, domain.AgentClaude, "sonnet")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.AttemptNumber)
	assert.NotNil(t, result.PipelineResult)
	assert.NotNil(t, result.AIResult)

	// Verify prompt contains error context
	assert.Contains(t, capturedPrompt, "validation failed")
	assert.Contains(t, capturedPrompt, "lint")
	assert.Contains(t, capturedPrompt, "undefined: foo")
}

func TestRetryHandler_RetryWithAI_RespectsMaxAttempts(t *testing.T) {
	handler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 3, Enabled: true}, zerolog.Nop())

	// Attempt 4 should fail
	_, err := handler.RetryWithAI(context.Background(), &PipelineResult{}, "/tmp", 4, nil, domain.AgentClaude, "sonnet")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrMaxRetriesExceeded)
	assert.Contains(t, err.Error(), "attempt 4 exceeds max 3")
}

func TestRetryHandler_RetryWithAI_DisabledRetry(t *testing.T) {
	handler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 3, Enabled: false}, zerolog.Nop())

	_, err := handler.RetryWithAI(context.Background(), &PipelineResult{}, "/tmp", 1, nil, domain.AgentClaude, "sonnet")

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrRetryDisabled)
}

func TestRetryHandler_RetryWithAI_AIFailure(t *testing.T) {
	mockAI := &MockAIRunner{
		RunFn: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return nil, atlaserrors.ErrClaudeInvocation
		},
	}

	handler := NewRetryHandler(mockAI, nil, DefaultRetryConfig(), zerolog.Nop())

	_, err := handler.RetryWithAI(context.Background(), &PipelineResult{}, "/tmp", 1, nil, domain.AgentClaude, "sonnet")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI fix failed")
}

func TestRetryHandler_RetryWithAI_ValidationStillFails(t *testing.T) {
	mockAI := &MockAIRunner{
		RunFn: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return &domain.AIResult{Success: true}, nil
		},
	}

	// Mock runner that always fails
	mockRunner := &MockCommandRunnerForRetry{
		Error: atlaserrors.ErrCommandFailed,
	}
	executor := NewExecutorWithRunner(time.Minute, mockRunner)

	handler := NewRetryHandler(mockAI, executor, DefaultRetryConfig(), zerolog.Nop())

	failedResult := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
	}

	result, err := handler.RetryWithAI(context.Background(), failedResult, "/tmp", 1, nil, domain.AgentClaude, "sonnet")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.AttemptNumber)
}

func TestRetryHandler_RetryWithAI_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	handler := NewRetryHandler(nil, nil, DefaultRetryConfig(), zerolog.Nop())

	_, err := handler.RetryWithAI(ctx, &PipelineResult{}, "/tmp", 1, nil, domain.AgentClaude, "sonnet")

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetryHandler_RetryWithAI_PassesWorkDir(t *testing.T) {
	// Create a real temporary directory for the test
	workDir := t.TempDir()

	var capturedWorkDir string
	mockAI := &MockAIRunner{
		RunFn: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			capturedWorkDir = req.WorkingDir
			return &domain.AIResult{Success: true}, nil
		},
	}

	mockRunner := &MockCommandRunnerForRetry{ShouldFix: true}
	executor := NewExecutorWithRunner(time.Minute, mockRunner)

	handler := NewRetryHandler(mockAI, executor, DefaultRetryConfig(), zerolog.Nop())

	_, err := handler.RetryWithAI(context.Background(), &PipelineResult{}, workDir, 1, nil, domain.AgentClaude, "sonnet")

	require.NoError(t, err)
	assert.Equal(t, workDir, capturedWorkDir)
}

func TestRetryHandler_CanRetry(t *testing.T) {
	handler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 3, Enabled: true}, zerolog.Nop())

	assert.True(t, handler.CanRetry(1))
	assert.True(t, handler.CanRetry(2))
	assert.True(t, handler.CanRetry(3))
	assert.False(t, handler.CanRetry(4))

	// Disabled handler
	disabledHandler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 3, Enabled: false}, zerolog.Nop())
	assert.False(t, disabledHandler.CanRetry(1))
}

func TestRetryHandler_MaxAttempts(t *testing.T) {
	handler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 5, Enabled: true}, zerolog.Nop())
	assert.Equal(t, 5, handler.MaxAttempts())
}

func TestRetryHandler_IsEnabled(t *testing.T) {
	enabledHandler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 3, Enabled: true}, zerolog.Nop())
	assert.True(t, enabledHandler.IsEnabled())

	disabledHandler := NewRetryHandler(nil, nil, RetryConfig{MaxAttempts: 3, Enabled: false}, zerolog.Nop())
	assert.False(t, disabledHandler.IsEnabled())
}

func TestRetryHandler_LogsAttemptNumber(t *testing.T) {
	// This test verifies the logging includes attempt numbers.
	// We're mainly testing that the code doesn't panic and the logic flows correctly.
	mockAI := &MockAIRunner{
		RunFn: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			// Verify attempt info is in prompt
			assert.Contains(t, req.Prompt, "Attempt 2 of 3")
			return &domain.AIResult{Success: true}, nil
		},
	}

	mockRunner := &MockCommandRunnerForRetry{ShouldFix: true}
	executor := NewExecutorWithRunner(time.Minute, mockRunner)

	handler := NewRetryHandler(mockAI, executor, DefaultRetryConfig(), zerolog.Nop())

	_, err := handler.RetryWithAI(context.Background(), &PipelineResult{
		FailedStepName: "test",
	}, "/tmp", 2, nil, domain.AgentClaude, "sonnet")

	require.NoError(t, err)
}

func TestRetryHandler_UsesRunnerConfig(t *testing.T) {
	mockAI := &MockAIRunner{
		RunFn: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return &domain.AIResult{Success: true}, nil
		},
	}

	mockRunner := &MockCommandRunnerForRetry{
		ShouldFix: true,
	}
	executor := NewExecutorWithRunner(time.Minute, mockRunner)

	handler := NewRetryHandler(mockAI, executor, DefaultRetryConfig(), zerolog.Nop())

	// Custom runner config with specific commands
	runnerConfig := &RunnerConfig{
		FormatCommands: []string{"custom-format"},
		LintCommands:   []string{"custom-lint"},
	}

	_, err := handler.RetryWithAI(context.Background(), &PipelineResult{}, "/tmp", 1, runnerConfig, domain.AgentClaude, "sonnet")

	require.NoError(t, err)
	// Verify commands were run (at least one command should have executed)
	assert.Positive(t, mockRunner.CallCount())
}
