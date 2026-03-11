package git

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestDefaultLockRetryConfig(t *testing.T) {
	config := DefaultLockRetryConfig()

	if config.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", config.MaxAttempts)
	}
	if config.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", config.InitialDelay)
	}
	if config.MaxDelay != 2*time.Second {
		t.Errorf("MaxDelay = %v, want 2s", config.MaxDelay)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", config.Multiplier)
	}
}

func TestRunWithLockRetry_Success(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	result, err := RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
		callCount++
		return "success", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("result = %q, want %q", result, "success")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRunWithLockRetry_LockErrorThenSuccess(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	result, err := RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("fatal: unable to create '/path/.git/index.lock': file exists") //nolint:err113 // test error
		}
		return "success after retries", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success after retries" {
		t.Errorf("result = %q, want %q", result, "success after retries")
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRunWithLockRetry_NonLockError(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	_, err := RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
		callCount++
		return "", errors.New("permission denied") //nolint:err113 // test error
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (should not retry non-lock errors)", callCount)
	}
}

func TestRunWithLockRetry_ExhaustedRetries(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	_, err := RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
		callCount++
		return "", errors.New("another git process seems to be running in this repository") //nolint:err113 // test error
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3 (max attempts)", callCount)
	}
}

func TestRunWithLockRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := LockRetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
		callCount++
		return "", errors.New("index.lock exists") //nolint:err113 // test error
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunWithLockRetryVoid_Success(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	err := RunWithLockRetryVoid(ctx, config, logger, func(_ context.Context) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRunWithLockRetryVoid_LockErrorThenSuccess(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	callCount := 0
	err := RunWithLockRetryVoid(ctx, config, logger, func(_ context.Context) error {
		callCount++
		if callCount < 2 {
			return errors.New("unable to create lock file") //nolint:err113 // test error
		}
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestRunWithLockRetry_MaxDelayRespected(t *testing.T) {
	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  10,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     20 * time.Millisecond,
		Multiplier:   10.0, // Would quickly exceed max without capping
	}
	logger := zerolog.Nop()

	start := time.Now()
	callCount := 0
	_, _ = RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
		callCount++
		if callCount < 4 {
			return "", errors.New("index.lock") //nolint:err113 // test error
		}
		return "done", nil
	})

	elapsed := time.Since(start)

	// With 3 retries (attempts 1-3 fail, 4 succeeds):
	// - After attempt 1: wait 10ms
	// - After attempt 2: wait 20ms (10ms * 10 = 100ms, capped to 20ms)
	// - After attempt 3: wait 20ms (capped)
	// Total wait: ~50ms (with some tolerance)
	if elapsed > 150*time.Millisecond {
		t.Errorf("elapsed time = %v, expected < 150ms (max delay should be respected)", elapsed)
	}
}

func TestRunWithLockRetry_VariousLockErrorMessages(t *testing.T) {
	lockErrors := []string{
		"fatal: Unable to create '/path/.git/index.lock': File exists.",
		"Another git process seems to be running in this repository",
		"Unable to create '/path/.git/refs/heads/main.lock'",
		"error: could not lock file",
		".lock': file exists",
	}

	ctx := context.Background()
	config := LockRetryConfig{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}
	logger := zerolog.Nop()

	for i, lockErr := range lockErrors {
		name := lockErr
		if len(name) > 30 {
			name = name[:30]
		}
		t.Run(name, func(t *testing.T) {
			_ = i // avoid unused variable
			callCount := 0
			_, err := RunWithLockRetry(ctx, config, logger, func(_ context.Context) (string, error) {
				callCount++
				if callCount < 2 {
					return "", errors.New(lockErr) //nolint:err113 // test error
				}
				return "success", nil
			})
			if err != nil {
				t.Errorf("unexpected error for lock message %q: %v", lockErr, err)
			}
			if callCount != 2 {
				t.Errorf("callCount = %d, want 2 for lock message %q", callCount, lockErr)
			}
		})
	}
}
