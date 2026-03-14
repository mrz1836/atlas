package git

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// These tests were previously integration tests that required real git commands.
// They have been refactored to use MockRunner for faster, more reliable testing.

// TestPushRunner_FullWorkflow tests a complete push workflow scenario.
func TestPushRunner_FullWorkflow(t *testing.T) {
	t.Run("successful push with upstream sets tracking branch", func(t *testing.T) {
		pushCalls := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, branch string, setUpstream bool) error {
				pushCalls++
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "feat/test-push", branch)
				assert.True(t, setUpstream)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:      "origin",
			Branch:      "feat/test-push",
			SetUpstream: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "origin/feat/test-push", result.Upstream)
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, 1, pushCalls)
	})

	t.Run("subsequent push without changes succeeds", func(t *testing.T) {
		// Simulates pushing when there are no new commits - should still succeed
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil // Already up to date is not an error
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:      "origin",
			Branch:      "feat/test-push",
			SetUpstream: false,
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Empty(t, result.Upstream) // No upstream set when SetUpstream is false
	})

	t.Run("push with new commits", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, branch string, _ bool) error {
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "feat/test-push", branch)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test-push",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})
}

// TestPushRunner_ProgressCallback_Scenarios tests various progress callback scenarios.
func TestPushRunner_ProgressCallback_Scenarios(t *testing.T) {
	t.Run("progress callback receives all expected messages", func(t *testing.T) {
		var progressMessages []string
		var mu sync.Mutex

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test-push",
			ProgressCallback: func(progress string) {
				mu.Lock()
				progressMessages = append(progressMessages, progress)
				mu.Unlock()
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)

		mu.Lock()
		defer mu.Unlock()
		assert.Contains(t, progressMessages, "Push attempt 1/3...")
		assert.Contains(t, progressMessages, "Push completed successfully")
	})

	t.Run("progress callback shows retry attempts", func(t *testing.T) {
		var progressMessages []string
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
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/test-push",
			ProgressCallback: func(progress string) {
				mu.Lock()
				progressMessages = append(progressMessages, progress)
				mu.Unlock()
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, 3, result.Attempts)

		mu.Lock()
		defer mu.Unlock()
		assert.Contains(t, progressMessages, "Push attempt 1/3...")
		assert.Contains(t, progressMessages, "Push attempt 2/3...")
		assert.Contains(t, progressMessages, "Push attempt 3/3...")
	})

	t.Run("progress callback nil does not panic", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:           "origin",
			Branch:           "feat/test",
			ProgressCallback: nil, // No callback
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})
}

// TestPushRunner_ConfirmationCallback_Scenarios tests confirmation callback scenarios.
func TestPushRunner_ConfirmationCallback_Scenarios(t *testing.T) {
	t.Run("confirmation approved proceeds with push", func(t *testing.T) {
		confirmCalled := false
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
			Branch:            "feat/test-push",
			ConfirmBeforePush: true,
			ConfirmCallback: func(remote, branch string) (bool, error) {
				confirmCalled = true
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "feat/test-push", branch)
				return true, nil
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.True(t, confirmCalled, "confirmation callback should have been called")
		assert.True(t, pushCalled, "push should have been called after confirmation")
	})

	t.Run("confirmation denied cancels push", func(t *testing.T) {
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
			Branch:            "feat/test-push",
			ConfirmBeforePush: true,
			ConfirmCallback: func(_, _ string) (bool, error) {
				return false, nil // User denies
			},
		})

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrOperationCanceled)
		assert.False(t, pushCalled, "push should not be called when confirmation denied")
	})

	t.Run("confirmation callback error propagates", func(t *testing.T) {
		mockRunner := &MockRunner{}
		pr := NewPushRunner(mockRunner)

		_, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test-push",
			ConfirmBeforePush: true,
			ConfirmCallback: func(_, _ string) (bool, error) {
				return false, errTestTUINotAvailable
			},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to confirm push")
	})

	t.Run("confirmation skipped when ConfirmBeforePush is false", func(t *testing.T) {
		confirmCalled := false

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test-push",
			ConfirmBeforePush: false,
			ConfirmCallback: func(_, _ string) (bool, error) {
				confirmCalled = true
				return true, nil
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.False(t, confirmCalled, "confirmation should be skipped")
	})

	t.Run("nil confirmation callback with ConfirmBeforePush true skips confirmation", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:            "origin",
			Branch:            "feat/test-push",
			ConfirmBeforePush: true,
			ConfirmCallback:   nil, // No callback provided
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})
}

// TestPushRunner_InvalidRemote tests pushing to an invalid remote.
func TestPushRunner_InvalidRemote(t *testing.T) {
	t.Run("push to nonexistent remote fails with other error", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, _ string, _ bool) error {
				assert.Equal(t, "nonexistent", remote)
				return errTestNonexistentRepo
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "nonexistent",
			Branch: "main",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, PushErrorOther, result.ErrorType)
		assert.Equal(t, 1, result.Attempts) // No retry for "other" errors
	})
}

// TestPushRunner_ConcurrentPushes tests thread safety of PushRunner.
func TestPushRunner_ConcurrentPushes(t *testing.T) {
	t.Run("concurrent pushes to different branches", func(t *testing.T) {
		var pushCount int32

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				atomic.AddInt32(&pushCount, 1)
				time.Sleep(10 * time.Millisecond) // Simulate network latency
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)

		var wg sync.WaitGroup
		branches := []string{"feat/a", "feat/b", "feat/c", "feat/d", "feat/e"}
		results := make([]*PushResult, len(branches))
		errs := make([]error, len(branches))

		for i, branch := range branches {
			wg.Add(1)
			go func(idx int, b string) {
				defer wg.Done()
				results[idx], errs[idx] = pr.Push(context.Background(), PushOptions{
					Remote: "origin",
					Branch: b,
				})
			}(i, branch)
		}

		wg.Wait()

		// All pushes should succeed
		for i, err := range errs {
			require.NoError(t, err, "push %d failed", i)
			assert.True(t, results[i].Success)
		}

		assert.Equal(t, 5, int(atomic.LoadInt32(&pushCount)))
	})
}

// TestPushRunner_EdgeCases tests various edge cases.
func TestPushRunner_EdgeCases(t *testing.T) {
	t.Run("empty remote defaults to origin", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, _ string, _ bool) error {
				assert.Equal(t, "origin", remote)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "", // Empty remote
			Branch: "main",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("branch with special characters", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, branch string, _ bool) error {
				assert.Equal(t, "feat/JIRA-123_add-feature", branch)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "feat/JIRA-123_add-feature",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("custom remote name", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, _ string, _ bool) error {
				assert.Equal(t, "upstream", remote)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "upstream",
			Branch: "main",
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("push result contains final error on failure", func(t *testing.T) {
		expectedErr := errTestAuthFailed

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return expectedErr
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "main",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, expectedErr, result.FinalErr)
	})
}

// TestPushRunner_RetryBehavior tests retry behavior in detail.
func TestPushRunner_RetryBehavior(t *testing.T) {
	t.Run("retries on transient network errors", func(t *testing.T) {
		networkErrors := []error{
			errTestNetworkHost,
			errTestConnRefused,
			errTestNetworkUnreach,
		}

		for _, netErr := range networkErrors {
			t.Run(netErr.Error()[:30], func(t *testing.T) {
				attempt := 0
				mockRunner := &MockRunner{
					PushFunc: func(_ context.Context, _, _ string, _ bool) error {
						attempt++
						if attempt < 2 {
							return netErr
						}
						return nil
					},
				}

				pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
					MaxAttempts:  3,
					InitialDelay: 1 * time.Millisecond,
					MaxDelay:     10 * time.Millisecond,
					Multiplier:   2.0,
				}))

				result, err := pr.Push(context.Background(), PushOptions{
					Remote: "origin",
					Branch: "main",
				})

				require.NoError(t, err)
				assert.True(t, result.Success)
				assert.Equal(t, 2, result.Attempts)
			})
		}
	})

	t.Run("does not retry on authentication errors", func(t *testing.T) {
		authErrors := []error{
			errTestAuthFailed,
			errTestPermissionDeny,
			errTestInvalidPassword,
			errTestAccessDenied,
		}

		for _, authErr := range authErrors {
			t.Run(authErr.Error()[:30], func(t *testing.T) {
				attempt := 0
				mockRunner := &MockRunner{
					PushFunc: func(_ context.Context, _, _ string, _ bool) error {
						attempt++
						return authErr
					},
				}

				pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
					MaxAttempts:  3,
					InitialDelay: 1 * time.Millisecond,
					MaxDelay:     10 * time.Millisecond,
					Multiplier:   2.0,
				}))

				result, err := pr.Push(context.Background(), PushOptions{
					Remote: "origin",
					Branch: "main",
				})

				require.Error(t, err)
				assert.False(t, result.Success)
				assert.Equal(t, 1, attempt, "should not retry on auth errors")
				assert.Equal(t, PushErrorAuth, result.ErrorType)
			})
		}
	})

	t.Run("tracks all attempts even when exhausted", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				return errTestNetworkTimeout
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "main",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, 5, result.Attempts)
		assert.Equal(t, PushErrorNetwork, result.ErrorType)
	})
}

// TestPushRunner_ContextHandling tests context cancellation scenarios.
func TestPushRunner_ContextHandling(t *testing.T) {
	t.Run("respects context cancellation before push", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				t.Fatal("push should not be called")
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel before push

		_, err := pr.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "main",
		})

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("respects context timeout", func(t *testing.T) {
		mockRunner := &MockRunner{
			PushFunc: func(ctx context.Context, _, _ string, _ bool) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return nil
				}
			},
		}

		pr := NewPushRunner(mockRunner)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, err := pr.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "main",
		})

		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
	})

	t.Run("cancellation during retry wait", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				return errTestNetworkTimeout
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 100 * time.Millisecond, // Long delay to allow cancellation
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
		}))

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after first attempt
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		_, err := pr.Push(ctx, PushOptions{
			Remote: "origin",
			Branch: "main",
		})

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, attempt, "should cancel after first attempt")
	})
}

// TestPushRunner_AllOptions tests using all options together.
func TestPushRunner_AllOptions(t *testing.T) {
	t.Run("all options enabled", func(t *testing.T) {
		var progressMessages []string
		var mu sync.Mutex
		confirmCalled := false
		pushCalled := false

		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, remote, branch string, setUpstream bool) error {
				pushCalled = true
				assert.Equal(t, "upstream", remote)
				assert.Equal(t, "feat/full-test", branch)
				assert.True(t, setUpstream)
				return nil
			},
		}

		pr := NewPushRunner(mockRunner)
		result, err := pr.Push(context.Background(), PushOptions{
			Remote:            "upstream",
			Branch:            "feat/full-test",
			SetUpstream:       true,
			ConfirmBeforePush: true,
			ConfirmCallback: func(remote, branch string) (bool, error) {
				confirmCalled = true
				assert.Equal(t, "upstream", remote)
				assert.Equal(t, "feat/full-test", branch)
				return true, nil
			},
			ProgressCallback: func(progress string) {
				mu.Lock()
				progressMessages = append(progressMessages, progress)
				mu.Unlock()
			},
		})

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "upstream/feat/full-test", result.Upstream)
		assert.True(t, confirmCalled)
		assert.True(t, pushCalled)

		mu.Lock()
		defer mu.Unlock()
		assert.NotEmpty(t, progressMessages)
	})
}

// TestPushRunner_SingleAttemptConfig tests with single attempt configuration.
func TestPushRunner_SingleAttemptConfig(t *testing.T) {
	t.Run("single attempt fails immediately on error", func(t *testing.T) {
		attempt := 0
		mockRunner := &MockRunner{
			PushFunc: func(_ context.Context, _, _ string, _ bool) error {
				attempt++
				return errTestNetworkTimeout
			},
		}

		pr := NewPushRunner(mockRunner, WithPushRetryConfig(RetryConfig{
			MaxAttempts:  1, // Single attempt
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}))

		result, err := pr.Push(context.Background(), PushOptions{
			Remote: "origin",
			Branch: "main",
		})

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, 1, attempt)
		assert.Equal(t, 1, result.Attempts)
	})
}
