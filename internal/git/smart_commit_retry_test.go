package git

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

var (
	errTimeout           = errors.New("timeout")
	errPersistentTimeout = errors.New("persistent timeout")
	errPersistentFailure = errors.New("persistent failure")
	errNetworkTimeout    = errors.New("network timeout")
)

// mockAIRunnerWithAttempts tracks call attempts for retry testing.
type mockAIRunnerWithAttempts struct {
	attempts      int
	maxAttempts   int
	successOn     int // Which attempt should succeed (0 = all fail)
	response      *domain.AIResult
	err           error
	recordedCalls []time.Duration // Record the timeout for each call
}

func (m *mockAIRunnerWithAttempts) Run(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	m.attempts++
	m.recordedCalls = append(m.recordedCalls, req.Timeout)

	if m.successOn > 0 && m.attempts == m.successOn {
		return m.response, nil
	}

	if m.attempts < m.maxAttempts || m.successOn == 0 {
		return nil, m.err
	}

	return m.response, nil
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_SuccessFirstAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunnerWithAttempts{
		successOn: 1,
		response: &domain.AIResult{
			Success: true,
			Output:  "feat(git): add retry logic\n\nImplements exponential backoff for AI requests.",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(30*time.Second),
		WithMaxRetries(2),
		WithRetryBackoffFactor(1.5),
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/smart_commit.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	message, err := runner.generateAIMessageWithRetry(context.Background(), group)
	require.NoError(t, err)
	assert.Contains(t, message, "feat(git): add retry logic")
	assert.Equal(t, 1, mockAI.attempts, "should succeed on first attempt")
	assert.Len(t, mockAI.recordedCalls, 1, "should only make one call")
	assert.Equal(t, 30*time.Second, mockAI.recordedCalls[0], "first attempt should use base timeout")
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_SuccessSecondAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunnerWithAttempts{
		maxAttempts: 2,
		successOn:   2,
		err:         errTimeout,
		response: &domain.AIResult{
			Success: true,
			Output:  "feat(git): add retry logic\n\nImplements exponential backoff for AI requests.",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(30*time.Second),
		WithMaxRetries(2),
		WithRetryBackoffFactor(1.5),
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/smart_commit.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	message, err := runner.generateAIMessageWithRetry(context.Background(), group)
	require.NoError(t, err)
	assert.Contains(t, message, "feat(git): add retry logic")
	assert.Equal(t, 2, mockAI.attempts, "should succeed on second attempt")
	assert.Len(t, mockAI.recordedCalls, 2, "should make two calls")

	// Verify exponential backoff
	assert.Equal(t, 30*time.Second, mockAI.recordedCalls[0], "first attempt uses base timeout")
	assert.Equal(t, 45*time.Second, mockAI.recordedCalls[1], "second attempt uses 30s * 1.5 = 45s")
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_SuccessThirdAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunnerWithAttempts{
		maxAttempts: 3,
		successOn:   3,
		err:         errTimeout,
		response: &domain.AIResult{
			Success: true,
			Output:  "feat(git): add retry logic\n\nImplements exponential backoff for AI requests.",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(20*time.Second),
		WithMaxRetries(2),
		WithRetryBackoffFactor(2.0),
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/smart_commit.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	message, err := runner.generateAIMessageWithRetry(context.Background(), group)
	require.NoError(t, err)
	assert.Contains(t, message, "feat(git): add retry logic")
	assert.Equal(t, 3, mockAI.attempts, "should succeed on third attempt")
	assert.Len(t, mockAI.recordedCalls, 3, "should make three calls")

	// Verify exponential backoff with factor 2.0
	assert.Equal(t, 20*time.Second, mockAI.recordedCalls[0], "first attempt: 20s")
	assert.Equal(t, 40*time.Second, mockAI.recordedCalls[1], "second attempt: 20s * 2.0 = 40s")
	assert.Equal(t, 80*time.Second, mockAI.recordedCalls[2], "third attempt: 40s * 2.0 = 80s")
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_AllAttemptsFail(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunnerWithAttempts{
		successOn: 0, // Never succeed
		err:       errPersistentTimeout,
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(10*time.Second),
		WithMaxRetries(2),
		WithRetryBackoffFactor(1.5),
		WithFallbackEnabled(false), // Disable fallback to test retry behavior only
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/smart_commit.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	_, err = runner.generateAIMessageWithRetry(context.Background(), group)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI generation failed after trying all fallback models")
	assert.Equal(t, 3, mockAI.attempts, "should try all 3 attempts (initial + 2 retries)")
	assert.Len(t, mockAI.recordedCalls, 3, "should make three calls")

	// Verify all timeouts were applied
	assert.Equal(t, 10*time.Second, mockAI.recordedCalls[0])
	assert.Equal(t, 15*time.Second, mockAI.recordedCalls[1])
	assert.Equal(t, time.Duration(22.5*float64(time.Second)), mockAI.recordedCalls[2])
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_ZeroRetries(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunnerWithAttempts{
		successOn: 0, // Never succeed
		err:       errTimeout,
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(30*time.Second),
		WithMaxRetries(0), // No retries
		WithRetryBackoffFactor(1.5),
		WithFallbackEnabled(false), // Disable fallback to test retry behavior only
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/smart_commit.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	_, err = runner.generateAIMessageWithRetry(context.Background(), group)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI generation failed after trying all fallback models")
	assert.Equal(t, 1, mockAI.attempts, "should only try once with maxRetries=0")
	assert.Len(t, mockAI.recordedCalls, 1, "should only make one call")
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_CustomBackoffFactor(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	mockAI := &mockAIRunnerWithAttempts{
		maxAttempts: 4,
		successOn:   4,
		err:         errTimeout,
		response: &domain.AIResult{
			Success: true,
			Output:  "feat(git): add retry logic\n\nImplements exponential backoff for AI requests.",
		},
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(10*time.Second),
		WithMaxRetries(3),
		WithRetryBackoffFactor(3.0), // Aggressive backoff
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/smart_commit.go", Status: ChangeModified},
		},
		CommitType: CommitTypeFeat,
	}

	message, err := runner.generateAIMessageWithRetry(context.Background(), group)
	require.NoError(t, err)
	assert.Contains(t, message, "feat(git): add retry logic")

	// Verify aggressive exponential backoff with factor 3.0
	assert.Equal(t, 10*time.Second, mockAI.recordedCalls[0], "attempt 1: 10s")
	assert.Equal(t, 30*time.Second, mockAI.recordedCalls[1], "attempt 2: 10s * 3.0 = 30s")
	assert.Equal(t, 90*time.Second, mockAI.recordedCalls[2], "attempt 3: 30s * 3.0 = 90s")
	assert.Equal(t, 270*time.Second, mockAI.recordedCalls[3], "attempt 4: 90s * 3.0 = 270s")
}

func TestSmartCommitRunner_DefaultRetrySettings(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	// Create runner without specifying timeout/retry options
	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil)

	// Verify defaults
	assert.Equal(t, 30*time.Second, runner.timeout, "default timeout should be 30s")
	assert.Equal(t, 2, runner.maxRetries, "default maxRetries should be 2")
	assert.InEpsilon(t, 1.5, runner.retryBackoffFactor, 0.001, "default backoff factor should be 1.5")
}

func TestSmartCommitRunner_WithTimeoutOption(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil,
		WithTimeout(60*time.Second),
	)

	assert.Equal(t, 60*time.Second, runner.timeout)
	assert.Equal(t, 2, runner.maxRetries, "should use default maxRetries")
	assert.InEpsilon(t, 1.5, runner.retryBackoffFactor, 0.001, "should use default backoff factor")
}

func TestSmartCommitRunner_WithMaxRetriesOption(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil,
		WithMaxRetries(5),
	)

	assert.Equal(t, 30*time.Second, runner.timeout, "should use default timeout")
	assert.Equal(t, 5, runner.maxRetries)
	assert.InEpsilon(t, 1.5, runner.retryBackoffFactor, 0.001, "should use default backoff factor")
}

func TestSmartCommitRunner_WithRetryBackoffFactorOption(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	runner := NewSmartCommitRunner(gitRunner, tmpDir, nil,
		WithRetryBackoffFactor(2.5),
	)

	assert.Equal(t, 30*time.Second, runner.timeout, "should use default timeout")
	assert.Equal(t, 2, runner.maxRetries, "should use default maxRetries")
	assert.InEpsilon(t, 2.5, runner.retryBackoffFactor, 0.001)
}

func TestSmartCommitRunner_GenerateAIMessageWithRetry_AIErrorTypes(t *testing.T) {
	tests := []struct {
		name    string
		aiError error
	}{
		{
			name:    "AI runner error",
			aiError: atlaserrors.ErrAIError,
		},
		{
			name:    "empty response error",
			aiError: atlaserrors.ErrAIEmptyResponse,
		},
		{
			name:    "invalid format error",
			aiError: atlaserrors.ErrAIInvalidFormat,
		},
		{
			name:    "generic error",
			aiError: errNetworkTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			initGitRepo(t, tmpDir)

			gitRunner, err := NewRunner(context.Background(), tmpDir)
			require.NoError(t, err)

			mockAI := &mockAIRunnerWithAttempts{
				successOn: 0,
				err:       tt.aiError,
			}

			runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
				WithTimeout(10*time.Second),
				WithMaxRetries(1),
				WithFallbackEnabled(false), // Disable fallback to test retry behavior only
			)

			group := FileGroup{
				Package: "internal/git",
				Files:   []FileChange{{Path: "test.go", Status: ChangeModified}},
			}

			_, err = runner.generateAIMessageWithRetry(context.Background(), group)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AI generation failed after trying all fallback models")
			assert.Equal(t, 2, mockAI.attempts)
		})
	}
}

func TestSmartCommitRunner_GenerateCommitMessage_FallbackLogsMessage(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	gitRunner, err := NewRunner(context.Background(), tmpDir)
	require.NoError(t, err)

	// AI runner that always fails
	mockAI := &mockAIRunnerWithAttempts{
		successOn: 0,
		err:       errPersistentFailure,
	}

	runner := NewSmartCommitRunner(gitRunner, tmpDir, mockAI,
		WithTimeout(10*time.Second),
		WithMaxRetries(1),
		WithFallbackEnabled(false), // Disable fallback to test retry behavior only
	)

	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeAdded},
		},
		CommitType: CommitTypeFeat,
	}

	// This should fallback to template-based message
	message := runner.generateCommitMessage(context.Background(), group)

	// Verify fallback message is correct
	assert.Contains(t, message, "feat(git): add runner.go")
	assert.Contains(t, message, "Updated runner.go in internal/git.")

	// Verify retry attempts were made
	assert.Equal(t, 2, mockAI.attempts, "should have tried initial + 1 retry")
}
