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

// Test error variables for err113 compliance.
var (
	errNoChecksReportedGH    = errors.New("gh failed [no checks reported on the 'main' branch]: github operation failed")
	errNoChecksReportedLower = errors.New("no checks reported")
	errNoChecksReportedMixed = errors.New("no checks reported on branch")
	errUnexpectedCall        = errors.New("unexpected mock call")
	errUnexpectedCommand     = errors.New("unexpected command")
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
		{PRErrorOther, true}, // PRErrorOther is now retryable since unknown gh errors may be transient
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

// =============================================================================
// CI Status Monitoring Tests (Story 6.6)
// =============================================================================

func TestCIStatus_String(t *testing.T) {
	tests := []struct {
		status   CIStatus
		expected string
	}{
		{CIStatusPending, "pending"},
		{CIStatusSuccess, "success"},
		{CIStatusFailure, "failure"},
		{CIStatusTimeout, "timeout"},
		{CIStatus(99), "unknown"}, // Unknown type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestCLIGitHubRunner_WatchPRChecks_Success(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// First poll: pending
				return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
			}
			// Second poll: success
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond, // Fast for tests
		Timeout:  1 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Equal(t, 2, callCount)
	assert.NoError(t, result.Error)
}

func TestCLIGitHubRunner_WatchPRChecks_Failure(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[{"name":"CI","state":"FAILURE","bucket":"fail"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusFailure, result.Status)
	assert.ErrorIs(t, result.Error, atlaserrors.ErrCIFailed)
}

func TestCLIGitHubRunner_WatchPRChecks_Timeout(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			// Always pending
			return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusTimeout, result.Status)
	assert.ErrorIs(t, result.Error, atlaserrors.ErrCITimeout)
}

func TestCLIGitHubRunner_WatchPRChecks_RequiredChecksFilter(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[
				{"name":"CI / lint","state":"SUCCESS","bucket":"pass"},
				{"name":"CI / test","state":"FAILURE","bucket":"fail"},
				{"name":"Optional","state":"SUCCESS","bucket":"pass"}
			]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	// Only require CI checks - should fail because CI / test failed
	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:       42,
		RequiredChecks: []string{"CI*"},
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusFailure, result.Status)
	assert.Len(t, result.CheckResults, 2) // Only CI checks
}

func TestCLIGitHubRunner_WatchPRChecks_RequiredChecksPass(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[
				{"name":"CI / lint","state":"SUCCESS","bucket":"pass"},
				{"name":"CI / test","state":"SUCCESS","bucket":"pass"},
				{"name":"Optional","state":"FAILURE","bucket":"fail"}
			]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	// Only require CI checks - should pass even though Optional failed
	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:       42,
		RequiredChecks: []string{"CI*"},
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Len(t, result.CheckResults, 2) // Only CI checks
}

func TestCLIGitHubRunner_WatchPRChecks_ProgressCallback(t *testing.T) {
	callCount := 0
	progressCalls := 0

	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			if callCount < 3 {
				return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
			}
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond,
		ProgressCallback: func(_ time.Duration, checks []CheckResult) {
			progressCalls++
			assert.NotEmpty(t, checks)
		},
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Equal(t, 3, progressCalls) // Called on each poll
}

func TestCLIGitHubRunner_WatchPRChecks_ContextCancellation(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := runner.WatchPRChecks(ctx, CIWatchOptions{
		PRNumber: 42,
		Interval: 100 * time.Millisecond,
	})

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCLIGitHubRunner_WatchPRChecks_ContextCancelledAtStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := NewCLIGitHubRunner("/test/dir")

	_, err := runner.WatchPRChecks(ctx, CIWatchOptions{
		PRNumber: 42,
	})

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCLIGitHubRunner_WatchPRChecks_InvalidPRNumber(t *testing.T) {
	runner := NewCLIGitHubRunner("/test/dir")

	tests := []int{0, -1, -100}
	for _, prNum := range tests {
		t.Run(fmt.Sprintf("PR %d", prNum), func(t *testing.T) {
			_, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
				PRNumber: prNum,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
		})
	}
}

func TestCLIGitHubRunner_WatchPRChecks_RetryOnNetworkError(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("could not resolve host: %w", atlaserrors.ErrGitHubOperation)
			}
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
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

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Equal(t, 3, attempts)
}

func TestCLIGitHubRunner_WatchPRChecks_NoRetryOnAuth(t *testing.T) {
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

	_, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.Error(t, err)
	assert.Equal(t, 1, attempts) // No retry for auth errors
}

func TestCLIGitHubRunner_WatchPRChecks_RetryOnOtherError(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			if attempts < 3 {
				// Generic error that doesn't match known patterns
				return nil, fmt.Errorf("gh failed: %w", atlaserrors.ErrGitHubOperation)
			}
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
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

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Equal(t, 3, attempts) // PRErrorOther should be retried
}

func TestCLIGitHubRunner_WatchPRChecks_OtherErrorExhaustsRetriesReturnsFetchError(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
			// Track only "gh pr checks" calls, not fallback "gh pr view" calls
			if len(args) >= 2 && args[0] == "pr" && args[1] == "checks" {
				attempts++
			}
			// For fallback "gh pr view", also return error
			if len(args) >= 2 && args[0] == "pr" && args[1] == "view" {
				return nil, fmt.Errorf("gh failed: %w", atlaserrors.ErrGitHubOperation)
			}
			// Always return error that classifies as PRErrorOther
			return nil, fmt.Errorf("gh failed: %w", atlaserrors.ErrGitHubOperation)
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

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond,
	})

	// Should NOT return error - should return CIStatusFetchError result
	require.NoError(t, err)
	assert.Equal(t, CIStatusFetchError, result.Status)
	assert.Equal(t, 3, attempts) // All retries exhausted
	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "CI status fetch failed")
}

func TestCLIGitHubRunner_WatchPRChecks_OtherErrorFallbackSucceeds(t *testing.T) {
	checkAttempts := 0
	fallbackCalled := false
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "pr" && args[1] == "checks" {
				checkAttempts++
				// Always fail checks with PRErrorOther
				return nil, fmt.Errorf("gh failed: %w", atlaserrors.ErrGitHubOperation)
			}
			// Fallback "gh pr view" succeeds with SUCCESS status
			if len(args) >= 2 && args[0] == "pr" && args[1] == "view" {
				fallbackCalled = true
				return []byte(`{"number":42,"state":"OPEN","mergeable":"MERGEABLE","statusCheckRollup":[{"conclusion":"SUCCESS"}]}`), nil
			}
			return nil, errUnexpectedCommand
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

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		Interval: 10 * time.Millisecond,
	})

	// Fallback should succeed
	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.True(t, fallbackCalled, "fallback via gh pr view should have been called")
	assert.GreaterOrEqual(t, checkAttempts, 3, "should have retried checks at least 3 times before fallback")
}

func TestCLIGitHubRunner_WatchPRChecks_EmptyChecks(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			// No CI configured
			return []byte(`[]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	// Empty checks means success
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Empty(t, result.CheckResults)
}

func TestCLIGitHubRunner_WatchPRChecks_CancelBucket(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[{"name":"CI","state":"CANCELED","bucket":"cancel"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusFailure, result.Status)
	assert.ErrorIs(t, result.Error, atlaserrors.ErrCIFailed)
}

func TestCLIGitHubRunner_WatchPRChecks_SkippingBucket(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[
				{"name":"CI","state":"SUCCESS","bucket":"pass"},
				{"name":"Optional","state":"SKIPPED","bucket":"skipping"}
			]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	// Skipping is treated as pass
	assert.Equal(t, CIStatusSuccess, result.Status)
}

func TestCLIGitHubRunner_WatchPRChecks_DefaultsApplied(t *testing.T) {
	// Just validate defaults are applied without erroring
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
		// No Interval or Timeout set - should use defaults
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
}

func TestFilterChecks_WildcardMatching(t *testing.T) {
	checks := []CheckResult{
		{Name: "CI / lint"},
		{Name: "CI / test"},
		{Name: "Security Scan"},
	}

	tests := []struct {
		name     string
		patterns []string
		expected int
	}{
		{"all", nil, 3},
		{"all empty slice", []string{}, 3},
		{"exact match", []string{"CI / lint"}, 1},
		{"wildcard", []string{"CI*"}, 2},
		{"wildcard with space", []string{"CI /*"}, 2},
		{"multiple patterns", []string{"CI / lint", "Security*"}, 2},
		{"no matches", []string{"Nonexistent*"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterChecks(checks, tt.patterns)
			assert.Len(t, filtered, tt.expected)
		})
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		patterns []string
		expected bool
	}{
		{"exact match", "CI / lint", []string{"CI / lint"}, true},
		{"wildcard prefix", "CI / lint", []string{"CI*"}, true},
		{"wildcard prefix with space", "CI / lint", []string{"CI /*"}, true},
		{"no match", "CI / lint", []string{"Security*"}, false},
		{"empty patterns", "CI / lint", []string{}, false},
		{"partial match without wildcard", "CI / lint", []string{"CI"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesAnyPattern(tt.input, tt.patterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineOverallCIStatus(t *testing.T) {
	tests := []struct {
		name     string
		checks   []CheckResult
		expected CIStatus
	}{
		{
			name:     "empty checks",
			checks:   []CheckResult{},
			expected: CIStatusSuccess,
		},
		{
			name: "all pass",
			checks: []CheckResult{
				{Name: "CI", Bucket: "pass"},
				{Name: "Lint", Bucket: "pass"},
			},
			expected: CIStatusSuccess,
		},
		{
			name: "one fail",
			checks: []CheckResult{
				{Name: "CI", Bucket: "pass"},
				{Name: "Lint", Bucket: "fail"},
			},
			expected: CIStatusFailure,
		},
		{
			name: "cancel treated as fail",
			checks: []CheckResult{
				{Name: "CI", Bucket: "cancel"},
			},
			expected: CIStatusFailure,
		},
		{
			name: "pending",
			checks: []CheckResult{
				{Name: "CI", Bucket: "pass"},
				{Name: "Lint", Bucket: "pending"},
			},
			expected: CIStatusPending,
		},
		{
			name: "skipping treated as pass",
			checks: []CheckResult{
				{Name: "CI", Bucket: "pass"},
				{Name: "Optional", Bucket: "skipping"},
			},
			expected: CIStatusSuccess,
		},
		{
			name: "unknown bucket treated as pending",
			checks: []CheckResult{
				{Name: "CI", Bucket: "unknown_bucket_value"},
			},
			expected: CIStatusPending,
		},
		{
			name: "fail takes precedence over pending",
			checks: []CheckResult{
				{Name: "CI", Bucket: "fail"},
				{Name: "Lint", Bucket: "pending"},
			},
			expected: CIStatusFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineOverallCIStatus(tt.checks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCheckResults(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expected    int
		expectError bool
	}{
		{
			name:     "valid output",
			output:   `[{"name":"CI","state":"SUCCESS","bucket":"pass","workflow":"CI"}]`,
			expected: 1,
		},
		{
			name:     "multiple checks",
			output:   `[{"name":"CI","bucket":"pass"},{"name":"Lint","bucket":"fail"}]`,
			expected: 2,
		},
		{
			name:     "empty array",
			output:   `[]`,
			expected: 0,
		},
		{
			name:     "empty output",
			output:   ``,
			expected: 0,
		},
		{
			name:     "whitespace only",
			output:   `   `,
			expected: 0,
		},
		{
			name:        "invalid JSON",
			output:      `{invalid json`,
			expectError: true,
		},
		{
			name:     "with timestamps",
			output:   `[{"name":"CI","bucket":"pass","startedAt":"2025-12-30T10:00:00Z","completedAt":"2025-12-30T10:05:00Z"}]`,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := parseCheckResults([]byte(tt.output))
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, results, tt.expected)
		})
	}
}

func TestCalculateCheckDuration(t *testing.T) {
	tests := []struct {
		name        string
		startedAt   string
		completedAt string
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{
			name:        "completed check",
			startedAt:   "2025-12-30T10:00:00Z",
			completedAt: "2025-12-30T10:05:00Z",
			minDuration: 5 * time.Minute,
			maxDuration: 5 * time.Minute,
		},
		{
			name:        "empty start",
			startedAt:   "",
			completedAt: "2025-12-30T10:05:00Z",
			minDuration: 0,
			maxDuration: 0,
		},
		{
			name:        "invalid start format",
			startedAt:   "not-a-date",
			completedAt: "2025-12-30T10:05:00Z",
			minDuration: 0,
			maxDuration: 0,
		},
		{
			name:        "invalid completed format",
			startedAt:   "2025-12-30T10:00:00Z",
			completedAt: "not-a-date",
			minDuration: 0,
			maxDuration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := calculateCheckDuration(tt.startedAt, tt.completedAt)
			assert.GreaterOrEqual(t, duration, tt.minDuration)
			assert.LessOrEqual(t, duration, tt.maxDuration+time.Minute) // Allow some variance for running checks
		})
	}
}

func TestValidateCIWatchOptions(t *testing.T) {
	tests := []struct {
		name        string
		opts        CIWatchOptions
		expectError bool
	}{
		{
			name:        "valid options",
			opts:        CIWatchOptions{PRNumber: 42},
			expectError: false,
		},
		{
			name:        "zero PR number",
			opts:        CIWatchOptions{PRNumber: 0},
			expectError: true,
		},
		{
			name:        "negative PR number",
			opts:        CIWatchOptions{PRNumber: -1},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCIWatchOptions(&tt.opts)
			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCLIGitHubRunner_WatchPRChecks_MaxRetriesExhausted(t *testing.T) {
	attempts := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			attempts++
			return nil, fmt.Errorf("network is unreachable: %w", atlaserrors.ErrGitHubOperation)
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

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	// With graceful degradation, transient errors after max retries return CIStatusFetchError
	// instead of an error, allowing the caller to handle it appropriately
	require.NoError(t, err)
	assert.Equal(t, CIStatusFetchError, result.Status)
	assert.Contains(t, result.Error.Error(), "failed to fetch PR checks after 3 attempts")
	// 3 fetch attempts + 1 fallback verification attempt = 4 total
	assert.Equal(t, 4, attempts)
}

func TestCLIGitHubRunner_WatchPRChecks_ElapsedTimeTracked(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	// Elapsed time should be non-zero but very small
	assert.Greater(t, result.ElapsedTime, time.Duration(0))
	assert.Less(t, result.ElapsedTime, time.Second)
}

func TestCLIGitHubRunner_WatchPRChecks_CheckResultsPopulated(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[
				{"name":"CI / lint","state":"SUCCESS","bucket":"pass","workflow":"CI","link":"https://example.com/1"},
				{"name":"CI / test","state":"SUCCESS","bucket":"pass","workflow":"CI","link":"https://example.com/2"}
			]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	assert.Len(t, result.CheckResults, 2)
	assert.Equal(t, "CI / lint", result.CheckResults[0].Name)
	assert.Equal(t, "pass", result.CheckResults[0].Bucket)
	assert.Equal(t, "CI", result.CheckResults[0].Workflow)
	assert.Equal(t, "https://example.com/1", result.CheckResults[0].URL)
}

func TestCLIGitHubRunner_WatchPRChecks_RequiredChecksNotFound(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			// Checks exist but none match the required pattern
			return []byte(`[
				{"name":"CI / lint","state":"SUCCESS","bucket":"pass"},
				{"name":"CI / test","state":"SUCCESS","bucket":"pass"}
			]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	// Require checks that don't exist - should error
	_, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:       42,
		RequiredChecks: []string{"NonExistent*"},
	})

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrCICheckNotFound)
	assert.Contains(t, err.Error(), "NonExistent*")
}

func TestCLIGitHubRunner_WatchPRChecks_RequiredChecksNotFoundEmptyChecks(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			// No checks at all - should succeed (no CI configured)
			return []byte(`[]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	// Even with required checks, if there are no checks at all, succeed
	// (this means no CI is configured, which is valid)
	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:       42,
		RequiredChecks: []string{"NonExistent*"},
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
}

func TestFormatCIProgressMessage(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		checks   []CheckResult
		expected string
	}{
		{
			name:     "no checks",
			elapsed:  5 * time.Minute,
			checks:   []CheckResult{},
			expected: "Waiting for CI... (5m elapsed, no checks found)",
		},
		{
			name:    "single check with workflow",
			elapsed: 30 * time.Second,
			checks: []CheckResult{
				{Name: "CI / lint", Workflow: "CI"},
			},
			expected: "Waiting for CI... (30s elapsed, checking: CI)",
		},
		{
			name:    "multiple checks same workflow",
			elapsed: 2 * time.Minute,
			checks: []CheckResult{
				{Name: "CI / lint", Workflow: "CI"},
				{Name: "CI / test", Workflow: "CI"},
			},
			expected: "Waiting for CI... (2m elapsed, checking: CI)",
		},
		{
			name:    "multiple different workflows",
			elapsed: 5 * time.Minute,
			checks: []CheckResult{
				{Name: "CI / lint", Workflow: "CI"},
				{Name: "Security Scan", Workflow: "Security"},
			},
			expected: "Waiting for CI... (5m elapsed, checking: CI, Security)",
		},
		{
			name:    "check without workflow uses name",
			elapsed: 1 * time.Minute,
			checks: []CheckResult{
				{Name: "Lint"},
			},
			expected: "Waiting for CI... (1m elapsed, checking: Lint)",
		},
		{
			name:    "hour duration",
			elapsed: 65 * time.Minute,
			checks: []CheckResult{
				{Name: "Long Test", Workflow: "Test"},
			},
			expected: "Waiting for CI... (1h 5m elapsed, checking: Test)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCIProgressMessage(tt.elapsed, tt.checks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{1 * time.Minute, "1m"},
		{5 * time.Minute, "5m"},
		{1*time.Minute + 15*time.Second, "1m 15s"},
		{2*time.Minute + 30*time.Second, "2m 30s"},
		{59 * time.Minute, "59m"},
		{59*time.Minute + 45*time.Second, "59m 45s"},
		{60 * time.Minute, "1h"},
		{65 * time.Minute, "1h 5m"},
		{65*time.Minute + 30*time.Second, "1h 5m"},
		{125 * time.Minute, "2h 5m"},
		{2 * time.Hour, "2h"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCLIGitHubRunner_ConvertToDraft_Success(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, name string, args ...string) ([]byte, error) {
			assert.Equal(t, "gh", name)
			assert.Equal(t, []string{"pr", "ready", "--undo", "42"}, args)
			return []byte{}, nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.NoError(t, err)
	assert.Equal(t, 1, mock.callCount)
}

func TestCLIGitHubRunner_ConvertToDraft_InvalidPRNumber(t *testing.T) {
	runner := NewCLIGitHubRunner("/test/dir")

	tests := []int{0, -1, -100}
	for _, prNum := range tests {
		t.Run(fmt.Sprintf("PR %d", prNum), func(t *testing.T) {
			err := runner.ConvertToDraft(context.Background(), prNum)
			require.Error(t, err)
			assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
		})
	}
}

func TestCLIGitHubRunner_ConvertToDraft_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := NewCLIGitHubRunner("/test/dir")

	err := runner.ConvertToDraft(ctx, 42)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCLIGitHubRunner_ConvertToDraft_AlreadyDraft(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("already a draft: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.NoError(t, err) // Should succeed silently
}

func TestCLIGitHubRunner_ConvertToDraft_PRMerged(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("pull request is already merged: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.NoError(t, err) // Should succeed silently
}

func TestCLIGitHubRunner_ConvertToDraft_PRClosed(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("pull request is closed: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.NoError(t, err) // Should succeed silently
}

func TestCLIGitHubRunner_ConvertToDraft_PRNotFound(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("pull request not found: %w", atlaserrors.ErrPRNotFound)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrPRNotFound)
}

func TestCLIGitHubRunner_ConvertToDraft_AuthFailed(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("gh auth login - not logged into any GitHub hosts: %w", atlaserrors.ErrGHAuthFailed)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrGHAuthFailed)
}

func TestCLIGitHubRunner_ConvertToDraft_OtherError(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("some other error: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.ConvertToDraft(context.Background(), 42)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to convert PR to draft")
}

// Tests for grace period and "no checks reported" handling

func TestClassifyGHError_NoChecksReported(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected PRErrorType
	}{
		{
			name:     "no checks reported error",
			err:      errNoChecksReportedGH,
			expected: PRErrorNoChecksYet,
		},
		{
			name:     "no checks reported lowercase",
			err:      errNoChecksReportedLower,
			expected: PRErrorNoChecksYet,
		},
		{
			name:     "no checks reported mixed case",
			err:      errNoChecksReportedMixed,
			expected: PRErrorNoChecksYet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGHError(tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPRErrorType_NoChecksYet_String(t *testing.T) {
	assert.Equal(t, "no_checks_yet", PRErrorNoChecksYet.String())
}

func TestIsGHNoChecksReportedError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"exact match", "no checks reported", true},
		{"with branch name", "no checks reported on the 'main' branch", true},
		{"full gh error", "gh failed [no checks reported on the 'task/foo' branch]: github operation failed", true},
		{"unrelated error", "authentication required", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGHNoChecksReportedError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCLIGitHubRunner_WatchPRChecks_NoChecksYetDuringGracePeriod(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			if callCount < 3 {
				// First two calls return "no checks reported" error
				return nil, fmt.Errorf("gh failed [no checks reported on the 'main' branch]: %w", atlaserrors.ErrGitHubOperation)
			}
			// Third call returns actual checks
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:           42,
		InitialGracePeriod: 1 * time.Second, // Short grace period for test
		GracePollInterval:  10 * time.Millisecond,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.GreaterOrEqual(t, callCount, 3) // Should have polled multiple times
}

func TestCLIGitHubRunner_WatchPRChecks_ChecksAppearAfterDelay(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			if callCount < 3 {
				// First two calls return "no checks reported" error (checks not registered yet)
				return nil, fmt.Errorf("gh failed [no checks reported on the 'main' branch]: %w", atlaserrors.ErrGitHubOperation)
			}
			// Third call returns pending checks
			if callCount == 3 {
				return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
			}
			// Subsequent calls return success
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:           42,
		Interval:           10 * time.Millisecond,
		InitialGracePeriod: 1 * time.Second,
		GracePollInterval:  10 * time.Millisecond,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.GreaterOrEqual(t, callCount, 3)
}

func TestCLIGitHubRunner_WatchPRChecks_NoChecksAfterGracePeriod(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			// Always return "no checks reported" error
			return nil, fmt.Errorf("gh failed [no checks reported on the 'main' branch]: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:           42,
		InitialGracePeriod: 50 * time.Millisecond, // Very short grace period
		GracePollInterval:  10 * time.Millisecond,
	})

	require.NoError(t, err)
	// After grace period, no checks = no CI configured = success
	assert.Equal(t, CIStatusSuccess, result.Status)
}

func TestCLIGitHubRunner_WatchPRChecks_EmptyChecksImmediateSuccess(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			// Always return empty checks
			return []byte(`[]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:           42,
		InitialGracePeriod: 1 * time.Second, // Grace period doesn't matter for empty checks
		GracePollInterval:  10 * time.Millisecond,
	})

	require.NoError(t, err)
	// Empty checks (not error) = no CI configured = immediate success
	assert.Equal(t, CIStatusSuccess, result.Status)
	assert.Equal(t, 1, callCount) // Should return immediately after first successful fetch
}

func TestCLIGitHubRunner_WatchPRChecks_GracePeriodRespectsTimeout(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			// Always return "no checks reported" error
			return nil, fmt.Errorf("gh failed [no checks reported on the 'main' branch]: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber:           42,
		Timeout:            30 * time.Millisecond, // Timeout before grace period ends
		InitialGracePeriod: 1 * time.Second,
		GracePollInterval:  10 * time.Millisecond,
	})

	require.NoError(t, err)
	// Should timeout, not wait for full grace period
	assert.Equal(t, CIStatusTimeout, result.Status)
}

func TestCLIGitHubRunner_WatchPRChecks_GracePeriodDefaults(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	// Don't set grace period options, verify defaults are applied
	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	assert.Equal(t, CIStatusSuccess, result.Status)
}

// Test MergePR method
func TestCLIGitHubRunner_MergePR_Success(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, name string, args ...string) ([]byte, error) {
			assert.Equal(t, "gh", name)
			assert.Contains(t, args, "pr")
			assert.Contains(t, args, "merge")
			assert.Contains(t, args, "42")
			assert.Contains(t, args, "--squash")
			assert.Contains(t, args, "--delete-branch=false")
			return []byte{}, nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.MergePR(context.Background(), 42, "squash", false)
	require.NoError(t, err)
}

func TestCLIGitHubRunner_MergePR_AdminBypass(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
			assert.Contains(t, args, "--admin")
			return []byte{}, nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.MergePR(context.Background(), 42, "squash", true)
	require.NoError(t, err)
}

func TestCLIGitHubRunner_MergePR_MergeMethods(t *testing.T) {
	tests := []struct {
		method      string
		expectedArg string
	}{
		{"squash", "--squash"},
		{"merge", "--merge"},
		{"rebase", "--rebase"},
		{"invalid", "--squash"}, // Defaults to squash
		{"", "--squash"},        // Empty defaults to squash
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			mock := &mockCommandExecutor{
				executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
					assert.Contains(t, args, tt.expectedArg)
					return []byte{}, nil
				},
			}

			runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
			err := runner.MergePR(context.Background(), 42, tt.method, false)
			require.NoError(t, err)
		})
	}
}

func TestCLIGitHubRunner_MergePR_InvalidPRNumber(t *testing.T) {
	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.MergePR(context.Background(), 0, "squash", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

func TestCLIGitHubRunner_MergePR_NotFound(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			//nolint:err113 // Test mock simulating external error
			return nil, fmt.Errorf("PR #999 not found")
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.MergePR(context.Background(), 999, "squash", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrPRNotFound)
}

func TestCLIGitHubRunner_MergePR_AuthFailed(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			//nolint:err113 // Test mock simulating external error
			return nil, errors.New("authentication required")
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.MergePR(context.Background(), 42, "squash", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrGHAuthFailed)
}

func TestCLIGitHubRunner_MergePR_GenericError(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			//nolint:err113 // Test mock simulating external error
			return nil, errors.New("merge conflict detected")
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.MergePR(context.Background(), 42, "squash", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrPRMergeFailed)
}

func TestCLIGitHubRunner_MergePR_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.MergePR(ctx, 42, "squash", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// Test AddPRReview method
func TestCLIGitHubRunner_AddPRReview_Success(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, name string, args ...string) ([]byte, error) {
			assert.Equal(t, "gh", name)
			assert.Contains(t, args, "pr")
			assert.Contains(t, args, "review")
			assert.Contains(t, args, "42")
			assert.Contains(t, args, "--approve")
			assert.Contains(t, args, "--body")
			assert.Contains(t, args, "LGTM")
			return []byte{}, nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRReview(context.Background(), 42, "LGTM", "APPROVE")
	require.NoError(t, err)
}

func TestCLIGitHubRunner_AddPRReview_EventTypes(t *testing.T) {
	tests := []struct {
		event       string
		expectedArg string
	}{
		{"APPROVE", "--approve"},
		{"approve", "--approve"}, // Case insensitive
		{"REQUEST_CHANGES", "--request-changes"},
		{"COMMENT", "--comment"},
		{"invalid", "--approve"}, // Defaults to approve
		{"", "--approve"},        // Empty defaults to approve
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			mock := &mockCommandExecutor{
				executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
					assert.Contains(t, args, tt.expectedArg)
					return []byte{}, nil
				},
			}

			runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
			err := runner.AddPRReview(context.Background(), 42, "test body", tt.event)
			require.NoError(t, err)
		})
	}
}

func TestCLIGitHubRunner_AddPRReview_OwnPR(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
	}{
		{"cannot approve", "cannot approve your own pull request"},
		{"author error", "author cannot review"},
		{"own pull request", "cannot request changes on own pull request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCommandExecutor{
				executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
					//nolint:err113 // Test mock simulating external error
					return nil, errors.New(tt.errMsg)
				},
			}

			runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
			err := runner.AddPRReview(context.Background(), 42, "LGTM", "APPROVE")
			require.Error(t, err)
			assert.ErrorIs(t, err, atlaserrors.ErrPRReviewNotAllowed)
		})
	}
}

func TestCLIGitHubRunner_AddPRReview_InvalidPRNumber(t *testing.T) {
	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.AddPRReview(context.Background(), 0, "test", "APPROVE")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

func TestCLIGitHubRunner_AddPRReview_EmptyBody(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, args ...string) ([]byte, error) {
			// Body should not be in args if empty
			assert.NotContains(t, args, "--body")
			return []byte{}, nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRReview(context.Background(), 42, "", "APPROVE")
	require.NoError(t, err)
}

func TestCLIGitHubRunner_AddPRReview_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRReview(ctx, 42, "test", "APPROVE")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// Test AddPRComment method
func TestCLIGitHubRunner_AddPRComment_Success(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, name string, args ...string) ([]byte, error) {
			assert.Equal(t, "gh", name)
			assert.Contains(t, args, "pr")
			assert.Contains(t, args, "comment")
			assert.Contains(t, args, "42")
			assert.Contains(t, args, "--body")
			assert.Contains(t, args, "Approved and Merged by ATLAS")
			return []byte{}, nil
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRComment(context.Background(), 42, "Approved and Merged by ATLAS")
	require.NoError(t, err)
}

func TestCLIGitHubRunner_AddPRComment_InvalidPRNumber(t *testing.T) {
	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.AddPRComment(context.Background(), 0, "test")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

func TestCLIGitHubRunner_AddPRComment_EmptyBody(t *testing.T) {
	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))

	err := runner.AddPRComment(context.Background(), 42, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

func TestCLIGitHubRunner_AddPRComment_NotFound(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			//nolint:err113 // Test mock simulating external error
			return nil, fmt.Errorf("PR #999 not found")
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRComment(context.Background(), 999, "test comment")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrPRNotFound)
}

func TestCLIGitHubRunner_AddPRComment_AuthFailed(t *testing.T) {
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			//nolint:err113 // Test mock simulating external error
			return nil, errors.New("authentication required")
		},
	}

	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRComment(context.Background(), 42, "test comment")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrGHAuthFailed)
}

func TestCLIGitHubRunner_AddPRComment_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockCommandExecutor{}
	runner := NewCLIGitHubRunner("/test/dir", WithGHCommandExecutor(mock))
	err := runner.AddPRComment(ctx, 42, "test")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAddJitter(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		factor   float64
	}{
		{"zero factor", 100 * time.Millisecond, 0},
		{"small factor", 100 * time.Millisecond, 0.1},
		{"medium factor", 100 * time.Millisecond, 0.25},
		{"large factor", 100 * time.Millisecond, 0.5},
		{"second scale", 5 * time.Second, 0.2},
		{"minute scale", 1 * time.Minute, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to verify jitter behavior
			results := make([]time.Duration, 100)
			for i := range 100 {
				results[i] = addJitter(tt.duration, tt.factor)
			}

			if tt.factor == 0 {
				verifyZeroFactorResults(t, results, tt.duration)
			} else {
				verifyNonZeroFactorResults(t, results, tt.duration, tt.factor)
			}
		})
	}
}

// verifyZeroFactorResults checks that all results match the expected duration.
func verifyZeroFactorResults(t *testing.T, results []time.Duration, expected time.Duration) {
	t.Helper()
	for _, r := range results {
		assert.Equal(t, expected, r, "zero factor should return exact duration")
	}
}

// verifyNonZeroFactorResults checks that results are within bounds and have variance.
func verifyNonZeroFactorResults(t *testing.T, results []time.Duration, duration time.Duration, factor float64) {
	t.Helper()
	minExpected := time.Duration(float64(duration) * (1 - factor))
	maxExpected := time.Duration(float64(duration) * (1 + factor))

	// All results should be within bounds
	for i, r := range results {
		assert.GreaterOrEqual(t, r, minExpected, "result %d below min bound", i)
		assert.LessOrEqual(t, r, maxExpected, "result %d above max bound", i)
	}

	// Check that there's actual variance (not all same value)
	allSame := true
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "jitter should produce variance across calls")
}

func TestAddJitter_NegativeFactor(t *testing.T) {
	// Negative factor should be treated as zero (no jitter)
	d := 100 * time.Millisecond
	for range 10 {
		result := addJitter(d, -0.1)
		assert.Equal(t, d, result, "negative factor should return exact duration")
	}
}

func TestCIStatusFetchError_String(t *testing.T) {
	assert.Equal(t, "fetch_error", CIStatusFetchError.String())
}

func TestCLIGitHubRunner_WatchPRChecks_FetchError_FallbackSuccess(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, cmd string, args ...string) ([]byte, error) {
			callCount++
			// First 3 calls are fetch retries (gh pr checks), then fallback (gh pr view)
			if callCount <= 3 {
				// Simulate network error during fetch
				return nil, fmt.Errorf("network timeout: %w", atlaserrors.ErrGitHubOperation)
			}
			// Fallback call - gh pr view succeeds
			if cmd == "gh" && len(args) > 2 && args[1] == "view" {
				return []byte(`{"state":"MERGED","statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"}]}`), nil
			}
			return nil, fmt.Errorf("%w: %s %v", errUnexpectedCall, cmd, args)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}),
	)

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	require.NoError(t, err)
	// Fallback determined CI passed
	assert.Equal(t, CIStatusSuccess, result.Status)
}

func TestCLIGitHubRunner_WatchPRChecks_FetchError_FallbackAlsoFails(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			// All calls fail (fetch retries + fallback)
			return nil, fmt.Errorf("network timeout: %w", atlaserrors.ErrGitHubOperation)
		},
	}

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}),
	)

	result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	// When fallback also fails, we return CIStatusFetchError (graceful degradation)
	require.NoError(t, err)
	assert.Equal(t, CIStatusFetchError, result.Status)
	assert.Error(t, result.Error)
}

func TestCLIGitHubRunner_WatchPRChecks_AuthError_NoRetry(t *testing.T) {
	callCount := 0
	mock := &mockCommandExecutor{
		executeFunc: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			callCount++
			return nil, atlaserrors.ErrGHAuthFailed
		},
	}

	runner := NewCLIGitHubRunner("/test/dir",
		WithGHCommandExecutor(mock),
		WithGHRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}),
	)

	_, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
		PRNumber: 42,
	})

	// Auth errors should not retry and should return error immediately
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrGHAuthFailed)
	assert.Equal(t, 1, callCount, "auth errors should not retry")
}
