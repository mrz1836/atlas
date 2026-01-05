package git

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test errors for push operations.
var (
	errTestNetworkTimeout  = fmt.Errorf("fatal: unable to access: connection timed out: %w", atlaserrors.ErrGitOperation)
	errTestNetworkHost     = fmt.Errorf("fatal: unable to access 'https://github.com/...': Could not resolve host: github.com: %w", atlaserrors.ErrGitOperation)
	errTestAuthFailed      = fmt.Errorf("fatal: Authentication failed for 'https://github.com': %w", atlaserrors.ErrGitOperation)
	errTestUnknown         = fmt.Errorf("some unknown error: %w", atlaserrors.ErrGitOperation)
	errTestTUI             = fmt.Errorf("TUI error: %w", atlaserrors.ErrGitOperation)
	errTestCouldNotRead    = fmt.Errorf("fatal: could not read Username for 'https://github.com': terminal prompts disabled: %w", atlaserrors.ErrGitOperation)
	errTestPermissionDeny  = fmt.Errorf("permission denied (publickey) - fatal: could not read from remote repository: %w", atlaserrors.ErrGitOperation)
	errTestInvalidPassword = fmt.Errorf("remote: invalid username or password - fatal: authentication failed: %w", atlaserrors.ErrGitOperation)
	errTestAccessDenied    = fmt.Errorf("access denied: %w", atlaserrors.ErrGitOperation)
	errTestConnRefused     = fmt.Errorf("fatal: unable to access 'https://github.com/...': Failed to connect to github.com port 443: Connection refused: %w", atlaserrors.ErrGitOperation)
	errTestNetworkUnreach  = fmt.Errorf("ssh: connect to host github.com port 22: Network is unreachable: %w", atlaserrors.ErrGitOperation)
	errTestConnTimeout     = fmt.Errorf("fatal: unable to access 'https://github.com/...': Connection timed out after 30001 milliseconds: %w", atlaserrors.ErrGitOperation)
	errTestOpTimeout       = fmt.Errorf("fatal: unable to access 'https://github.com/...': Operation timed out after 30001 milliseconds: %w", atlaserrors.ErrGitOperation)
	errTestUnableAccess    = fmt.Errorf("fatal: unable to access 'https://github.com/...': %w", atlaserrors.ErrGitOperation)
	errTestNoRoute         = fmt.Errorf("ssh: connect to host github.com port 22: No route to host: %w", atlaserrors.ErrGitOperation)
	errTestFailedConnect   = fmt.Errorf("fatal: failed to connect to github.com: %w", atlaserrors.ErrGitOperation)
	errTestSomethingWrong  = fmt.Errorf("something went wrong: %w", atlaserrors.ErrGitOperation)
	errTestRefspec         = fmt.Errorf("error: src refspec main does not match any: %w", atlaserrors.ErrGitOperation)
)

// MockRunner implements Runner interface for testing.
type MockRunner struct {
	PushFunc          func(ctx context.Context, remote, branch string, setUpstream bool) error
	StatusFunc        func(ctx context.Context) (*Status, error)
	AddFunc           func(ctx context.Context, paths []string) error
	CommitFunc        func(ctx context.Context, message string, trailers map[string]string) error
	CurrentBranchFunc func(ctx context.Context) (string, error)
	CreateBranchFunc  func(ctx context.Context, name, baseBranch string) error
	DiffFunc          func(ctx context.Context, cached bool) (string, error)
	BranchExistsFunc  func(ctx context.Context, name string) (bool, error)
	FetchFunc         func(ctx context.Context, remote string) error
	RebaseFunc        func(ctx context.Context, onto string) error
	RebaseAbortFunc   func(ctx context.Context) error
	ResetFunc         func(ctx context.Context) error
}

func (m *MockRunner) Push(ctx context.Context, remote, branch string, setUpstream bool) error {
	if m.PushFunc != nil {
		return m.PushFunc(ctx, remote, branch, setUpstream)
	}
	return nil
}

func (m *MockRunner) Status(ctx context.Context) (*Status, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx)
	}
	return &Status{}, nil
}

func (m *MockRunner) Add(ctx context.Context, paths []string) error {
	if m.AddFunc != nil {
		return m.AddFunc(ctx, paths)
	}
	return nil
}

func (m *MockRunner) Commit(ctx context.Context, message string, trailers map[string]string) error {
	if m.CommitFunc != nil {
		return m.CommitFunc(ctx, message, trailers)
	}
	return nil
}

func (m *MockRunner) CurrentBranch(ctx context.Context) (string, error) {
	if m.CurrentBranchFunc != nil {
		return m.CurrentBranchFunc(ctx)
	}
	return "main", nil
}

func (m *MockRunner) CreateBranch(ctx context.Context, name, baseBranch string) error {
	if m.CreateBranchFunc != nil {
		return m.CreateBranchFunc(ctx, name, baseBranch)
	}
	return nil
}

func (m *MockRunner) Diff(ctx context.Context, cached bool) (string, error) {
	if m.DiffFunc != nil {
		return m.DiffFunc(ctx, cached)
	}
	return "", nil
}

func (m *MockRunner) BranchExists(ctx context.Context, name string) (bool, error) {
	if m.BranchExistsFunc != nil {
		return m.BranchExistsFunc(ctx, name)
	}
	return false, nil
}

func (m *MockRunner) Fetch(ctx context.Context, remote string) error {
	if m.FetchFunc != nil {
		return m.FetchFunc(ctx, remote)
	}
	return nil
}

func (m *MockRunner) Rebase(ctx context.Context, onto string) error {
	if m.RebaseFunc != nil {
		return m.RebaseFunc(ctx, onto)
	}
	return nil
}

func (m *MockRunner) RebaseAbort(ctx context.Context) error {
	if m.RebaseAbortFunc != nil {
		return m.RebaseAbortFunc(ctx)
	}
	return nil
}

func (m *MockRunner) Reset(ctx context.Context) error {
	if m.ResetFunc != nil {
		return m.ResetFunc(ctx)
	}
	return nil
}

func TestPushErrorType_String(t *testing.T) {
	tests := []struct {
		name     string
		errType  PushErrorType
		expected string
	}{
		{
			name:     "none",
			errType:  PushErrorNone,
			expected: "none",
		},
		{
			name:     "auth",
			errType:  PushErrorAuth,
			expected: "auth",
		},
		{
			name:     "network",
			errType:  PushErrorNetwork,
			expected: "network",
		},
		{
			name:     "timeout",
			errType:  PushErrorTimeout,
			expected: "timeout",
		},
		{
			name:     "other",
			errType:  PushErrorOther,
			expected: "other",
		},
		{
			name:     "unknown value",
			errType:  PushErrorType(99),
			expected: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errType.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 2*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.InDelta(t, 2.0, config.Multiplier, 0.001)
}

func TestNewPushRunner(t *testing.T) {
	t.Run("creates with defaults", func(t *testing.T) {
		mockRunner := &MockRunner{}
		pr := NewPushRunner(mockRunner)

		require.NotNil(t, pr)
		assert.Equal(t, mockRunner, pr.runner)
		assert.Equal(t, DefaultRetryConfig(), pr.config)
	})

	t.Run("with custom logger", func(t *testing.T) {
		mockRunner := &MockRunner{}
		logger := zerolog.Nop()
		pr := NewPushRunner(mockRunner, WithPushLogger(logger))

		require.NotNil(t, pr)
		assert.Equal(t, logger, pr.logger)
	})

	t.Run("with custom retry config", func(t *testing.T) {
		mockRunner := &MockRunner{}
		customConfig := RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 1 * time.Second,
			MaxDelay:     60 * time.Second,
			Multiplier:   3.0,
		}
		pr := NewPushRunner(mockRunner, WithPushRetryConfig(customConfig))

		require.NotNil(t, pr)
		assert.Equal(t, customConfig, pr.config)
	})
}

func TestPushRunner_Push_Success(t *testing.T) {
	t.Run("successful push without upstream", func(t *testing.T) {
		pushCalled := false
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, branch string, setUpstream bool) error {
				pushCalled = true
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "feat/test", branch)
				assert.False(t, setUpstream)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:      "origin",
			Branch:      "feat/test",
			SetUpstream: false,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, pushCalled)
		assert.True(t, result.Success)
		assert.Empty(t, result.Upstream)
		assert.Equal(t, 1, result.Attempts)
	})

	t.Run("successful push with upstream", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, setUpstream bool) error {
				assert.True(t, setUpstream)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:      "origin",
			Branch:      "feat/test",
			SetUpstream: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "origin/feat/test", result.Upstream)
		assert.Equal(t, 1, result.Attempts)
	})

	t.Run("default remote is origin", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, _ string, _ bool) error {
				assert.Equal(t, "origin", remote)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Branch: "feat/test",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})
}

func TestPushRunner_Push_EmptyBranch(t *testing.T) {
	mockRunner := &MockRunner{}
	pr := NewPushRunner(mockRunner)

	_, err := pr.Push(context.Background(), PushOptions{
		Remote: "origin",
		Branch: "",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	assert.Contains(t, err.Error(), "branch name cannot be empty")
}

func TestPushRunner_Push_ContextCancellation(t *testing.T) {
	t.Run("canceled at entry", func(t *testing.T) {
		mockRunner := &MockRunner{}
		pr := NewPushRunner(mockRunner)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := pr.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("canceled during retry wait", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				return errTestNetworkTimeout
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
		}))

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		_, err := pr.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestPushRunner_Push_ConfirmationCallback(t *testing.T) {
	t.Run("confirmation approved", func(t *testing.T) {
		callbackCalled := false
		pushCalled := false

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				pushCalled = true
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test",
			ConfirmBeforePush: true,
			ConfirmCallback: func(remote, branch string) (bool, error) {
				callbackCalled = true
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "feat/test", branch)
				return true, nil
			},
		})

		require.NoError(t, err)
		assert.True(t, callbackCalled)
		assert.True(t, pushCalled)
		assert.True(t, result.Success)
	})

	t.Run("confirmation denied", func(t *testing.T) {
		pushCalled := false

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				pushCalled = true
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		_, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test",
			ConfirmBeforePush: true,
			ConfirmCallback: func(_, _ string) (bool, error) {
				return false, nil
			},
		})

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrOperationCanceled)
		assert.False(t, pushCalled)
	})

	t.Run("confirmation callback error", func(t *testing.T) {
		mockRunner := &MockRunner{}
		pr := NewPushRunner(mockRunner)

		_, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test",
			ConfirmBeforePush: true,
			ConfirmCallback: func(_, _ string) (bool, error) {
				return false, errTestTUI
			},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to confirm push")
	})

	t.Run("no callback when ConfirmBeforePush is false", func(t *testing.T) {
		callbackCalled := false

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test",
			ConfirmBeforePush: false,
			ConfirmCallback: func(_, _ string) (bool, error) {
				callbackCalled = true
				return true, nil
			},
		})

		require.NoError(t, err)
		assert.False(t, callbackCalled)
		assert.True(t, result.Success)
	})
}

func TestPushRunner_Push_ProgressCallback(t *testing.T) {
	t.Run("progress callback called on success", func(t *testing.T) {
		progressMessages := []string{}
		var mu sync.Mutex

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
			ProgressCallback: func(progress string) {
				mu.Lock()
				progressMessages = append(progressMessages, progress)
				mu.Unlock()
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, progressMessages, "Push attempt 1/3...")
		assert.Contains(t, progressMessages, "Push completed successfully")
	})

	t.Run("progress callback called during retries", func(t *testing.T) {
		progressMessages := []string{}
		var mu sync.Mutex
		attempt := 0

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				if attempt < 3 {
					return errTestNetworkTimeout
				}
				return nil
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
			ProgressCallback: func(progress string) {
				mu.Lock()
				progressMessages = append(progressMessages, progress)
				mu.Unlock()
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, 3, result.Attempts)

		// Should have attempt messages and retry messages
		assert.Contains(t, progressMessages, "Push attempt 1/3...")
		assert.Contains(t, progressMessages, "Push attempt 2/3...")
		assert.Contains(t, progressMessages, "Push attempt 3/3...")
		assert.Contains(t, progressMessages, "Push completed successfully")
	})
}

func TestPushRunner_Push_RetryLogic(t *testing.T) {
	t.Run("retries on network error", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				if attempt < 3 {
					return errTestNetworkHost
				}
				return nil
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, 3, result.Attempts)
	})

	t.Run("retries exhausted on network error", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				return errTestNetworkHost
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrPushNetworkFailed)
		assert.Contains(t, err.Error(), "push failed after 3 attempts")
		assert.False(t, result.Success)
		assert.Equal(t, 3, result.Attempts)
		assert.Equal(t, PushErrorNetwork, result.ErrorType)
	})

	t.Run("no retry on auth error", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				return errTestAuthFailed
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrPushAuthFailed)
		assert.Contains(t, err.Error(), "authentication failed")
		assert.False(t, result.Success)
		assert.Equal(t, 1, attempt, "should not retry on auth error")
		assert.Equal(t, PushErrorAuth, result.ErrorType)
	})

	t.Run("no retry on other errors", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				return errTestUnknown
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to push")
		assert.False(t, result.Success)
		assert.Equal(t, 1, attempt, "should not retry on unknown error")
		assert.Equal(t, PushErrorOther, result.ErrorType)
	})
}

func TestClassifyPushError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected PushErrorType
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: PushErrorNone,
		},
		// Authentication errors
		{
			name:     "authentication failed",
			err:      errTestAuthFailed,
			expected: PushErrorAuth,
		},
		{
			name:     "could not read username",
			err:      errTestCouldNotRead,
			expected: PushErrorAuth,
		},
		{
			name:     "permission denied",
			err:      errTestPermissionDeny,
			expected: PushErrorAuth,
		},
		{
			name:     "invalid username or password",
			err:      errTestInvalidPassword,
			expected: PushErrorAuth,
		},
		{
			name:     "access denied",
			err:      errTestAccessDenied,
			expected: PushErrorAuth,
		},
		// Network errors
		{
			name:     "could not resolve host",
			err:      errTestNetworkHost,
			expected: PushErrorNetwork,
		},
		{
			name:     "connection refused",
			err:      errTestConnRefused,
			expected: PushErrorNetwork,
		},
		{
			name:     "network is unreachable",
			err:      errTestNetworkUnreach,
			expected: PushErrorNetwork,
		},
		{
			name:     "connection timed out",
			err:      errTestConnTimeout,
			expected: PushErrorNetwork,
		},
		{
			name:     "operation timed out",
			err:      errTestOpTimeout,
			expected: PushErrorNetwork,
		},
		{
			name:     "unable to access",
			err:      errTestUnableAccess,
			expected: PushErrorNetwork,
		},
		{
			name:     "no route to host",
			err:      errTestNoRoute,
			expected: PushErrorNetwork,
		},
		{
			name:     "failed to connect",
			err:      errTestFailedConnect,
			expected: PushErrorNetwork,
		},
		// Timeout error
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: PushErrorTimeout,
		},
		// Other errors
		{
			name:     "unknown error",
			err:      errTestSomethingWrong,
			expected: PushErrorOther,
		},
		{
			name:     "branch does not exist",
			err:      errTestRefspec,
			expected: PushErrorOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyPushError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPushRunner_Push_TimeoutClassification(t *testing.T) {
	t.Run("retries on timeout error", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				if attempt < 3 {
					return context.DeadlineExceeded
				}
				return nil
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, 3, result.Attempts)
	})

	t.Run("timeout exhausted retries", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return context.DeadlineExceeded
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test",
		})

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrPushNetworkFailed)
		assert.False(t, result.Success)
		assert.Equal(t, 3, result.Attempts)
		assert.Equal(t, PushErrorTimeout, result.ErrorType)
	})
}

func TestPushRunner_Push_DelayIncreases(t *testing.T) {
	// Track the time between attempts to verify exponential backoff
	var attemptTimes []time.Time
	var mu sync.Mutex
	attempt := 0

	mockRunner := &MockRunner{
		PushFunc: func(_ context.Context, _, _ string, _ bool) error {
			mu.Lock()
			attemptTimes = append(attemptTimes, time.Now())
			attempt++
			mu.Unlock()
			if attempt < 3 {
				return errTestNetworkTimeout
			}
			return nil
		},
	}

	pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}))

	result, err := pr.Push(context.Background(), PushOptions{
		Remote: "origin",
		Branch: "feat/test",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify exponential backoff
	require.Len(t, attemptTimes, 3)

	// First delay should be around 50ms
	firstDelay := attemptTimes[1].Sub(attemptTimes[0])
	assert.GreaterOrEqual(t, firstDelay, 40*time.Millisecond)
	assert.Less(t, firstDelay, 100*time.Millisecond)

	// Second delay should be around 100ms (2x the first)
	secondDelay := attemptTimes[2].Sub(attemptTimes[1])
	assert.GreaterOrEqual(t, secondDelay, 80*time.Millisecond)
	assert.Less(t, secondDelay, 200*time.Millisecond)
}

func TestPushRunner_Push_MaxDelayRespected(t *testing.T) {
	var attemptTimes []time.Time
	var mu sync.Mutex

	mockRunner := &MockRunner{
		PushFunc: func(_ context.Context, _, _ string, _ bool) error {
			mu.Lock()
			attemptTimes = append(attemptTimes, time.Now())
			count := len(attemptTimes)
			mu.Unlock()
			if count < 5 {
				return errTestNetworkTimeout
			}
			return nil
		},
	}

	pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 30 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Multiplier:   2.0,
	}))

	result, err := pr.Push(context.Background(), PushOptions{
		Remote: "origin",
		Branch: "feat/test",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify delays are capped at MaxDelay
	require.Len(t, attemptTimes, 5)

	// Later delays should be capped at MaxDelay (50ms)
	lastDelay := attemptTimes[4].Sub(attemptTimes[3])
	assert.GreaterOrEqual(t, lastDelay, 40*time.Millisecond)
	assert.Less(t, lastDelay, 100*time.Millisecond, "delay should be capped at MaxDelay")
}

func TestPushOptions(t *testing.T) {
	// Test that PushOptions struct works correctly
	opts := PushOptions{
		Remote:            "upstream",
		Branch:            "feat/my-feature",
		SetUpstream:       true,
		ConfirmBeforePush: true,
		ConfirmCallback: func(_, _ string) (bool, error) {
			return true, nil
		},
		ProgressCallback: func(_ string) {},
	}

	assert.Equal(t, "upstream", opts.Remote)
	assert.Equal(t, "feat/my-feature", opts.Branch)
	assert.True(t, opts.SetUpstream)
	assert.True(t, opts.ConfirmBeforePush)
	assert.NotNil(t, opts.ConfirmCallback)
	assert.NotNil(t, opts.ProgressCallback)
}

func TestPushResult(t *testing.T) {
	// Test that PushResult struct works correctly
	result := PushResult{
		Success:   true,
		Upstream:  "origin/feat/test",
		ErrorType: PushErrorNone,
		Attempts:  1,
		FinalErr:  nil,
	}

	assert.True(t, result.Success)
	assert.Equal(t, "origin/feat/test", result.Upstream)
	assert.Equal(t, PushErrorNone, result.ErrorType)
	assert.Equal(t, 1, result.Attempts)
	assert.NoError(t, result.FinalErr)
}

func TestRetryConfig(t *testing.T) {
	// Test that RetryConfig struct works correctly
	config := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   3.0,
	}

	assert.Equal(t, 5, config.MaxAttempts)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 60*time.Second, config.MaxDelay)
	assert.InDelta(t, 3.0, config.Multiplier, 0.001)
}

func TestPushRunner_Push_CustomRemote(t *testing.T) {
	mockRunner := &MockRunner{
		PushFunc: func(_ context.Context, remote, _ string, _ bool) error {
			assert.Equal(t, "upstream", remote)
			return nil
		},
	}

	pr := NewPushRunner(mockRunner)
	result, err := pr.Push(context.Background(), PushOptions{
		Remote: "upstream",
		Branch: "feat/test",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestPushRunner_InterfaceCompliance(_ *testing.T) {
	// Verify that PushRunner implements PushService
	var _ PushService = (*PushRunner)(nil)
}
