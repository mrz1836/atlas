// Package git provides Git operations for ATLAS.
// This file tests the shared retry logic.
package git

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Static test errors for err113 compliance.
var (
	errNetworkTest    = errors.New("network error")
	errPersistentTest = errors.New("persistent network error")
	errAuthTest       = errors.New("authentication failed")
	errRetryableTest  = errors.New("retryable")
	errAnyTest        = errors.New("any error")
)

func TestExecuteWithRetry_Success(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	attemptCount := 0
	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			attemptCount++
			return "success", true, nil
		},
		ShouldRetryFunc: func(_ error) bool {
			return false
		},
	}

	result, attempts, err := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())

	require.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 1, attempts)
	assert.Equal(t, 1, attemptCount)
}

func TestExecuteWithRetry_RetriesOnFailure(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	attemptCount := 0

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			attemptCount++
			if attemptCount < 3 {
				return "", false, errNetworkTest
			}
			return "success after retries", true, nil
		},
		ShouldRetryFunc: func(err error) bool {
			return errors.Is(err, errNetworkTest)
		},
	}

	result, attempts, err := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())

	require.NoError(t, err)
	assert.Equal(t, "success after retries", result)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, 3, attemptCount)
}

func TestExecuteWithRetry_ExhaustsRetries(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	attemptCount := 0

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			attemptCount++
			return "failed", false, errPersistentTest
		},
		ShouldRetryFunc: func(_ error) bool {
			return true // Always retry
		},
	}

	result, attempts, err := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())

	require.Error(t, err)
	require.ErrorIs(t, err, errPersistentTest)
	assert.Equal(t, "failed", result) // Last attempt result
	assert.Equal(t, 3, attempts)
	assert.Equal(t, 3, attemptCount)
}

func TestExecuteWithRetry_NoRetryOnNonRetryableError(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	attemptCount := 0

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			attemptCount++
			return "auth_failed", false, errAuthTest
		},
		ShouldRetryFunc: func(_ error) bool {
			return false // Never retry auth errors
		},
	}

	result, attempts, err := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())

	require.Error(t, err)
	require.ErrorIs(t, err, errAuthTest)
	assert.Equal(t, "auth_failed", result)
	assert.Equal(t, 1, attempts)
	assert.Equal(t, 1, attemptCount)
}

func TestExecuteWithRetry_ContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond, // Long delay to allow cancellation
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	attemptCount := 0

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			attemptCount++
			if attemptCount == 1 {
				// Cancel after first attempt
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
			}
			return "", false, errRetryableTest
		},
		ShouldRetryFunc: func(_ error) bool {
			return true
		},
	}

	_, attempts, err := ExecuteWithRetry(ctx, config, op, zerolog.Nop())

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, attempts)
}

func TestExecuteWithRetry_ExponentialBackoff(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	var delays []time.Duration
	lastAttemptTime := time.Now()

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, attempt int) (string, bool, error) {
			now := time.Now()
			if attempt > 1 {
				delays = append(delays, now.Sub(lastAttemptTime))
			}
			lastAttemptTime = now
			return "", false, errRetryableTest
		},
		ShouldRetryFunc: func(_ error) bool {
			return true
		},
		OnRetryWaitFunc: func(_ int, _ time.Duration) {
			// This is called before wait
		},
	}

	_, attempts, _ := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())

	assert.Equal(t, 4, attempts)
	require.Len(t, delays, 3)

	// Verify exponential backoff (with tolerance for CI scheduling variance)
	// Expected: 10ms, 20ms, 40ms
	assert.InDelta(t, 10, delays[0].Milliseconds(), 15)
	assert.InDelta(t, 20, delays[1].Milliseconds(), 15)
	assert.InDelta(t, 40, delays[2].Milliseconds(), 20)
}

func TestExecuteWithRetry_MaxDelayCapApplied(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     60 * time.Millisecond, // Low cap
		Multiplier:   2.0,
	}

	var delays []time.Duration
	lastAttemptTime := time.Now()

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, attempt int) (string, bool, error) {
			now := time.Now()
			if attempt > 1 {
				delays = append(delays, now.Sub(lastAttemptTime))
			}
			lastAttemptTime = now
			return "", false, errRetryableTest
		},
		ShouldRetryFunc: func(_ error) bool {
			return true
		},
	}

	_, attempts, _ := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())

	assert.Equal(t, 5, attempts)
	require.Len(t, delays, 4)

	// All delays should be capped at MaxDelay (60ms)
	for _, d := range delays[1:] { // After first delay (50ms), subsequent should be capped
		assert.LessOrEqual(t, d.Milliseconds(), int64(70)) // Allow 10ms tolerance
	}
}

func TestExecuteWithRetry_OnRetryWaitCalled(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	var onWaitCalls []struct {
		attempt int
		delay   time.Duration
	}

	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			return "", false, errRetryableTest
		},
		ShouldRetryFunc: func(_ error) bool {
			return true
		},
		OnRetryWaitFunc: func(attempt int, delay time.Duration) {
			onWaitCalls = append(onWaitCalls, struct {
				attempt int
				delay   time.Duration
			}{attempt, delay})
		},
	}

	_, _, err := ExecuteWithRetry(context.Background(), config, op, zerolog.Nop())
	require.Error(t, err) // Expect error since all attempts fail

	// OnRetryWait should be called before each wait (not on last attempt)
	require.Len(t, onWaitCalls, 2) // Called for attempts 1 and 2 (before retrying)
	assert.Equal(t, 1, onWaitCalls[0].attempt)
	assert.Equal(t, 2, onWaitCalls[1].attempt)
}

func TestSimpleRetryOperation_NilFuncs(t *testing.T) {
	// Test that SimpleRetryOperation handles nil function pointers gracefully
	op := &SimpleRetryOperation[string]{
		AttemptFunc: func(_ context.Context, _ int) (string, bool, error) {
			return "test", true, nil
		},
		// ShouldRetryFunc and OnRetryWaitFunc are nil
	}

	// ShouldRetry returns false when function is nil
	assert.False(t, op.ShouldRetry(errAnyTest))

	// OnRetryWait doesn't panic when function is nil
	assert.NotPanics(t, func() {
		op.OnRetryWait(1, time.Second)
	})
}
