// Package git provides Git operations for ATLAS.
package git

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockCommandExecutor is a test double for CommandExecutor.
type mockCommandExecutor struct {
	executeFunc func(ctx context.Context, workDir, name string, args ...string) ([]byte, error)
	callCount   int
	lastArgs    []string
}

func (m *mockCommandExecutor) Execute(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
	m.callCount++
	m.lastArgs = args
	if m.executeFunc != nil {
		return m.executeFunc(ctx, workDir, name, args...)
	}
	return nil, atlaserrors.ErrCommandNotConfigured
}

func TestPRErrorType_String(t *testing.T) {
	tests := []struct {
		errType  PRErrorType
		expected string
	}{
		{PRErrorNone, "none"},
		{PRErrorAuth, "auth"},
		{PRErrorRateLimit, "rate_limit"},
		{PRErrorNetwork, "network"},
		{PRErrorNotFound, "not_found"},
		{PRErrorOther, "other"},
		{PRErrorType(99), "other"}, // Unknown type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.errType.String())
		})
	}
}

func TestNewCLIGitHubRunner(t *testing.T) {
	t.Run("creates runner with defaults", func(t *testing.T) {
		runner := NewCLIGitHubRunner("/test/dir")

		assert.NotNil(t, runner)
		assert.Equal(t, "/test/dir", runner.workDir)
		assert.Equal(t, 3, runner.config.MaxAttempts)
	})

	t.Run("applies options", func(t *testing.T) {
		logger := zerolog.Nop()
		config := RetryConfig{MaxAttempts: 5, InitialDelay: time.Second}
		mock := &mockCommandExecutor{}

		runner := NewCLIGitHubRunner("/test/dir",
			WithGHLogger(logger),
			WithGHRetryConfig(config),
			WithGHCommandExecutor(mock),
		)

		assert.Equal(t, 5, runner.config.MaxAttempts)
		assert.Equal(t, time.Second, runner.config.InitialDelay)
	})
}

func TestCLIGitHubRunner_CreatePR_Success(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte("https://github.com/owner/repo/pull/42\n"), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "fix(config): handle nil options",
		Body:       "Test body content",
		BaseBranch: "main",
		HeadBranch: "fix/test-branch",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 42, result.Number)
	assert.Equal(t, "https://github.com/owner/repo/pull/42", result.URL)
	assert.Equal(t, "open", result.State)
	assert.Equal(t, 1, result.Attempts)
}

func TestCLIGitHubRunner_CreatePR_DraftPR(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
			// Verify --draft flag is present
			hasDraft := false
			for _, arg := range args {
				if arg == "--draft" {
					hasDraft = true
					break
				}
			}
			assert.True(t, hasDraft, "expected --draft flag")
			return []byte("https://github.com/owner/repo/pull/42\n"), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "feat: new feature",
		Body:       "Test body",
		BaseBranch: "main",
		HeadBranch: "feat/new",
		Draft:      true,
	})

	require.NoError(t, err)
	assert.Equal(t, "draft", result.State)
}

func TestCLIGitHubRunner_CreatePR_ValidationErrors(t *testing.T) {
	runner := NewCLIGitHubRunner("/test/dir")

	tests := []struct {
		name string
		opts PRCreateOptions
	}{
		{
			name: "empty title",
			opts: PRCreateOptions{
				Title:      "",
				Body:       "body",
				HeadBranch: "feat/test",
			},
		},
		{
			name: "empty body",
			opts: PRCreateOptions{
				Title:      "title",
				Body:       "",
				HeadBranch: "feat/test",
			},
		},
		{
			name: "empty head branch",
			opts: PRCreateOptions{
				Title: "title",
				Body:  "body",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runner.CreatePR(context.Background(), tt.opts)
			require.Error(t, err)
			assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
		})
	}
}

func TestCLIGitHubRunner_CreatePR_DefaultBaseBranch(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
			// Verify --base main is present
			for i, arg := range args {
				if arg == "--base" && i+1 < len(args) {
					assert.Equal(t, "main", args[i+1])
				}
			}
			return []byte("https://github.com/owner/repo/pull/1\n"), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	_, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "title",
		Body:       "body",
		HeadBranch: "feat/test",
		// BaseBranch not set, should default to "main"
	})

	require.NoError(t, err)
}

func TestCLIGitHubRunner_CreatePR_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := NewCLIGitHubRunner("/test/dir")

	_, err := runner.CreatePR(ctx, PRCreateOptions{
		Title:      "title",
		Body:       "body",
		HeadBranch: "feat/test",
	})

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCLIGitHubRunner_CreatePR_RetryOnRateLimit(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("API rate limit exceeded: %w", atlaserrors.ErrGHRateLimited)
			}
			return []byte("https://github.com/owner/repo/pull/42\n"), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond, // Fast for tests
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}),
	)

	result, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "title",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feat/test",
	})

	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, 3, result.Attempts)
}

func TestCLIGitHubRunner_CreatePR_NoRetryOnAuth(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			return nil, fmt.Errorf("gh auth login - not logged into any GitHub hosts: %w", atlaserrors.ErrGHAuthFailed)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}),
	)

	result, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "title",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feat/test",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrGHAuthFailed)
	assert.Equal(t, 1, attempts) // No retry for auth errors
	assert.Equal(t, PRErrorAuth, result.ErrorType)
}

func TestCLIGitHubRunner_CreatePR_MaxRetriesExhausted(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			return nil, fmt.Errorf("rate limit exceeded: %w", atlaserrors.ErrGHRateLimited)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}),
	)

	result, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "title",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feat/test",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrGHRateLimited)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, 3, result.Attempts)
}

func TestCLIGitHubRunner_CreatePR_ContextCancelledDuringRetry(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			return nil, fmt.Errorf("rate limit exceeded: %w", atlaserrors.ErrGHRateLimited)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 500 * time.Millisecond, // Long delay so we can cancel during wait
			MaxDelay:     time.Second,
			Multiplier:   2.0,
		}),
	)

	// Cancel context after a short delay (after first attempt starts waiting)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := runner.CreatePR(ctx, PRCreateOptions{
		Title:      "title",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feat/test",
	})

	// Should have been canceled during retry wait
	require.ErrorIs(t, err, context.Canceled)
	// Only one attempt should have been made before cancellation during wait
	assert.Equal(t, 1, attempts)
}

func TestCLIGitHubRunner_GetPRStatus_Success(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"number":42,"state":"OPEN","mergeable":"MERGEABLE","statusCheckRollup":[{"conclusion":"SUCCESS"}]}`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	status, err := runner.GetPRStatus(context.Background(), 42)

	require.NoError(t, err)
	assert.Equal(t, 42, status.Number)
	assert.Equal(t, "open", status.State)
	assert.True(t, status.Mergeable)
	assert.True(t, status.ChecksPass)
	assert.Equal(t, "success", status.CIStatus)
}

func TestCLIGitHubRunner_GetPRStatus_InvalidPRNumber(t *testing.T) {
	runner := NewCLIGitHubRunner("/test/dir")

	tests := []int{0, -1, -100}
	for _, prNum := range tests {
		t.Run("invalid PR number", func(t *testing.T) {
			_, err := runner.GetPRStatus(context.Background(), prNum)
			require.Error(t, err)
			assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
		})
	}
}

func TestCLIGitHubRunner_GetPRStatus_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewCLIGitHubRunner("/test/dir")

	_, err := runner.GetPRStatus(ctx, 42)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCLIGitHubRunner_GetPRStatus_NotFound(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("pull request not found: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	_, err := runner.GetPRStatus(context.Background(), 999)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrPRNotFound)
}

func TestCLIGitHubRunner_GetPRStatus_OtherError(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("some random error: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	_, err := runner.GetPRStatus(context.Background(), 42)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get PR status")
}

func TestCLIGitHubRunner_GetPRStatus_InvalidJSON(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`{invalid json`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	_, err := runner.GetPRStatus(context.Background(), 42)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse PR status JSON")
}

//nolint:err113 // test table uses dynamic errors to test error message pattern matching
func TestClassifyGHError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected PRErrorType
	}{
		{"nil error", nil, PRErrorNone},
		{"context deadline", context.DeadlineExceeded, PRErrorNetwork},
		{"rate limit exceeded", errors.New("API rate limit exceeded"), PRErrorRateLimit},
		{"secondary rate limit", errors.New("secondary rate limit"), PRErrorRateLimit},
		{"abuse detection", errors.New("abuse detection triggered"), PRErrorRateLimit},
		{"too many requests", errors.New("too many requests"), PRErrorRateLimit},
		{"auth required", errors.New("authentication required"), PRErrorAuth},
		{"bad credentials", errors.New("bad credentials"), PRErrorAuth},
		{"not logged into", errors.New("not logged into any hosts"), PRErrorAuth},
		{"gh auth login", errors.New("gh auth login required"), PRErrorAuth},
		{"invalid token", errors.New("invalid token"), PRErrorAuth},
		{"token expired", errors.New("token expired"), PRErrorAuth},
		{"could not resolve host", errors.New("could not resolve host"), PRErrorNetwork},
		{"connection refused", errors.New("connection refused"), PRErrorNetwork},
		{"network unreachable", errors.New("network is unreachable"), PRErrorNetwork},
		{"connection timed out", errors.New("connection timed out"), PRErrorNetwork},
		{"timeout", errors.New("timeout waiting for response"), PRErrorNetwork},
		{"not found", errors.New("pull request not found"), PRErrorNotFound},
		{"repository not found", errors.New("repository not found"), PRErrorNotFound},
		{"does not exist", errors.New("branch does not exist"), PRErrorNotFound},
		{"unknown error", errors.New("some random error"), PRErrorOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGHError(tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParsePRCreateOutput(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		expectedURL    string
		expectedNumber int
	}{
		{
			name:           "standard output",
			output:         "https://github.com/owner/repo/pull/42\n",
			expectedURL:    "https://github.com/owner/repo/pull/42",
			expectedNumber: 42,
		},
		{
			name:           "multiline output",
			output:         "Creating PR...\nhttps://github.com/owner/repo/pull/123\nDone!",
			expectedURL:    "https://github.com/owner/repo/pull/123",
			expectedNumber: 123,
		},
		{
			name:           "with extra whitespace",
			output:         "  https://github.com/owner/repo/pull/1  \n",
			expectedURL:    "https://github.com/owner/repo/pull/1",
			expectedNumber: 1,
		},
		{
			name:           "no URL",
			output:         "Error: something went wrong",
			expectedURL:    "",
			expectedNumber: 0,
		},
		{
			name:           "empty output",
			output:         "",
			expectedURL:    "",
			expectedNumber: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, number := parsePRCreateOutput(tt.output)
			assert.Equal(t, tt.expectedURL, url)
			assert.Equal(t, tt.expectedNumber, number)
		})
	}
}

func TestParsePRStatusOutput(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		expectedNumber int
		expectedState  string
		expectedCI     string
		expectError    bool
	}{
		{
			name:           "success status",
			output:         `{"number":42,"state":"OPEN","mergeable":"MERGEABLE","statusCheckRollup":[{"conclusion":"SUCCESS"}]}`,
			expectedNumber: 42,
			expectedState:  "open",
			expectedCI:     "success",
		},
		{
			name:           "failure status",
			output:         `{"number":10,"state":"OPEN","mergeable":"CONFLICTING","statusCheckRollup":[{"conclusion":"FAILURE"}]}`,
			expectedNumber: 10,
			expectedState:  "open",
			expectedCI:     "failure",
		},
		{
			name:           "no checks",
			output:         `{"number":5,"state":"MERGED","mergeable":"MERGEABLE","statusCheckRollup":[]}`,
			expectedNumber: 5,
			expectedState:  "merged",
			expectedCI:     "success", // No checks = pass
		},
		{
			name:           "null checks",
			output:         `{"number":1,"state":"CLOSED","statusCheckRollup":null}`,
			expectedNumber: 1,
			expectedState:  "closed",
			expectedCI:     "success", // Null checks = pass
		},
		{
			name:           "pending status",
			output:         `{"number":99,"state":"OPEN","statusCheckRollup":[{"state":"PENDING"}]}`,
			expectedNumber: 99,
			expectedState:  "open",
			expectedCI:     "pending",
		},
		{
			name:           "timed out status",
			output:         `{"number":77,"state":"OPEN","statusCheckRollup":[{"conclusion":"TIMED_OUT"}]}`,
			expectedNumber: 77,
			expectedState:  "open",
			expectedCI:     "failure",
		},
		{
			name:           "in progress status",
			output:         `{"number":88,"state":"OPEN","statusCheckRollup":[{"state":"IN_PROGRESS"}]}`,
			expectedNumber: 88,
			expectedState:  "open",
			expectedCI:     "pending",
		},
		{
			name:        "invalid JSON",
			output:      `{invalid json`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := parsePRStatusOutput([]byte(tt.output))
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedNumber, status.Number)
			assert.Equal(t, tt.expectedState, status.State)
			assert.Equal(t, tt.expectedCI, status.CIStatus)
		})
	}
}

func TestDetermineCIStatus(t *testing.T) {
	tests := []struct {
		name         string
		checks       []ghStatusCheckEntry
		expectedCI   string
		expectedPass bool
	}{
		{
			name:         "empty checks",
			checks:       []ghStatusCheckEntry{},
			expectedCI:   "success",
			expectedPass: true,
		},
		{
			name:         "all success",
			checks:       []ghStatusCheckEntry{{Conclusion: "SUCCESS"}, {Conclusion: "SUCCESS"}},
			expectedCI:   "success",
			expectedPass: true,
		},
		{
			name:         "one failure",
			checks:       []ghStatusCheckEntry{{Conclusion: "SUCCESS"}, {Conclusion: "FAILURE"}},
			expectedCI:   "failure",
			expectedPass: false,
		},
		{
			name:         "canceled",
			checks:       []ghStatusCheckEntry{{Conclusion: "CANCELED"}},
			expectedCI:   "failure",
			expectedPass: false,
		},
		{
			name:         "queued",
			checks:       []ghStatusCheckEntry{{State: "QUEUED"}},
			expectedCI:   "pending",
			expectedPass: false,
		},
		{
			name:         "mixed pending and success",
			checks:       []ghStatusCheckEntry{{Conclusion: "SUCCESS"}, {State: "PENDING"}},
			expectedCI:   "pending",
			expectedPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, pass := determineCIStatus(tt.checks)
			assert.Equal(t, tt.expectedCI, status)
			assert.Equal(t, tt.expectedPass, pass)
		})
	}
}

func TestShouldRetryPR(t *testing.T) {
	tests := []struct {
		errType  PRErrorType
		expected bool
	}{
		{PRErrorNone, false},
		{PRErrorAuth, false},
		{PRErrorRateLimit, true},
		{PRErrorNetwork, true},
		{PRErrorNotFound, false},
		{PRErrorOther, false},
	}

	for _, tt := range tests {
		t.Run(tt.errType.String(), func(t *testing.T) {
			assert.Equal(t, tt.expected, shouldRetryPR(tt.errType))
		})
	}
}

func TestBuildPRFinalError(t *testing.T) {
	tests := []struct {
		name        string
		result      *PRResult
		expectedErr error
		errContains string
	}{
		{
			name:        "no error",
			result:      &PRResult{ErrorType: PRErrorNone},
			expectedErr: nil,
		},
		{
			name:        "auth error",
			result:      &PRResult{ErrorType: PRErrorAuth, Attempts: 1},
			expectedErr: atlaserrors.ErrGHAuthFailed,
		},
		{
			name:        "rate limit error",
			result:      &PRResult{ErrorType: PRErrorRateLimit, Attempts: 3},
			expectedErr: atlaserrors.ErrGHRateLimited,
		},
		{
			name:        "network error",
			result:      &PRResult{ErrorType: PRErrorNetwork, Attempts: 3},
			expectedErr: atlaserrors.ErrPRCreationFailed,
		},
		{
			name:        "not found error",
			result:      &PRResult{ErrorType: PRErrorNotFound, Attempts: 1},
			expectedErr: atlaserrors.ErrPRCreationFailed,
		},
		{
			name:        "other error with FinalErr",
			result:      &PRResult{ErrorType: PRErrorOther, Attempts: 1, FinalErr: errors.New("custom error")}, //nolint:err113 // test uses dynamic error
			errContains: "custom error",
		},
		{
			name:        "unknown error type defaults to other",
			result:      &PRResult{ErrorType: PRErrorType(99), Attempts: 1, FinalErr: errors.New("unknown type error")}, //nolint:err113 // test uses dynamic error
			errContains: "unknown type error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := buildPRFinalError(tt.result)
			if tt.expectedErr == nil && tt.errContains == "" {
				assert.NoError(t, err)
			} else if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func TestValidatePROptions(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("valid options", func(t *testing.T) {
		opts := &PRCreateOptions{
			Title:      "title",
			Body:       "body",
			BaseBranch: "main",
			HeadBranch: "feat/test",
		}
		err := validatePROptions(opts, logger)
		require.NoError(t, err)
	})

	t.Run("sets default base branch", func(t *testing.T) {
		opts := &PRCreateOptions{
			Title:      "title",
			Body:       "body",
			HeadBranch: "feat/test",
		}
		err := validatePROptions(opts, logger)
		require.NoError(t, err)
		assert.Equal(t, "main", opts.BaseBranch)
	})
}

func TestCLIGitHubRunner_CreatePR_MaxDelayCapReached(t *testing.T) {
	// Test that delay is capped at MaxDelay when multiplier would exceed it
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("rate limit exceeded: %w", atlaserrors.ErrGHRateLimited)
			}
			return []byte("https://github.com/owner/repo/pull/42\n"), nil
		},
	}

	// Configure so that delay would exceed MaxDelay after first retry
	// InitialDelay=50ms, Multiplier=100 -> second delay would be 5000ms
	// But MaxDelay=100ms should cap it
	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond, // Cap at 100ms
			Multiplier:   100.0,                  // Would make 5000ms without cap
		}),
	)

	start := time.Now()
	result, err := runner.CreatePR(context.Background(), PRCreateOptions{
		Title:      "title",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feat/test",
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 3, result.Attempts)

	// If MaxDelay wasn't applied, this would take >5 seconds
	// With MaxDelay of 100ms, total should be ~150ms (50ms + 100ms)
	assert.Less(t, elapsed, 500*time.Millisecond, "delay should be capped by MaxDelay")
}
