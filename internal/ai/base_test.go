package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test errors for static error definitions
var (
	errTestTemporaryNetwork = errors.New("temporary network error")
	errTestAuthFailed       = errors.New("authentication failed: invalid API key")
	errTestSome             = errors.New("some error")
	errTestOriginal         = errors.New("original error")
	errTestWrapped          = errors.New("wrapped error")
	errTestErrorType        = errors.New("test error type")
)

func TestBaseRunner_ResolveTimeout(t *testing.T) {
	t.Parallel()

	t.Run("request timeout takes precedence", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{
			Config: &config.AIConfig{Timeout: 5 * time.Minute},
		}
		req := &domain.AIRequest{Timeout: 10 * time.Minute}
		assert.Equal(t, 10*time.Minute, b.ResolveTimeout(req))
	})

	t.Run("config timeout used when request has none", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{
			Config: &config.AIConfig{Timeout: 5 * time.Minute},
		}
		req := &domain.AIRequest{}
		assert.Equal(t, 5*time.Minute, b.ResolveTimeout(req))
	})

	t.Run("default timeout used when no config", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		req := &domain.AIRequest{}
		assert.Equal(t, constants.DefaultAITimeout, b.ResolveTimeout(req))
	})

	t.Run("default timeout used when config timeout is zero", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{
			Config: &config.AIConfig{Timeout: 0},
		}
		req := &domain.AIRequest{}
		assert.Equal(t, constants.DefaultAITimeout, b.ResolveTimeout(req))
	})
}

func TestBaseRunner_RunWithTimeout(t *testing.T) {
	t.Parallel()

	t.Run("returns error on canceled context", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{ErrType: atlaserrors.ErrClaudeInvocation}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := b.RunWithTimeout(ctx, &domain.AIRequest{}, func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			t.Fatal("execute should not be called on canceled context")
			return nil, errTestSome
		})

		require.Error(t, err)
		require.Equal(t, context.Canceled, err)
	})

	t.Run("executes successfully", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{
			Config:  &config.AIConfig{Timeout: 1 * time.Minute},
			ErrType: atlaserrors.ErrClaudeInvocation,
		}
		expected := &domain.AIResult{Output: "test output"}

		result, err := b.RunWithTimeout(context.Background(), &domain.AIRequest{}, func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return expected, nil
		})

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("retries on retryable error", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{
			Config:  &config.AIConfig{Timeout: 1 * time.Minute},
			ErrType: atlaserrors.ErrClaudeInvocation,
		}
		attempts := 0
		expected := &domain.AIResult{Output: "success"}
		// Use a generic network error which is considered retryable
		retryableErr := errTestTemporaryNetwork

		result, err := b.RunWithTimeout(context.Background(), &domain.AIRequest{}, func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			attempts++
			if attempts < 2 {
				return nil, retryableErr
			}
			return expected, nil
		})

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		assert.Equal(t, 2, attempts)
	})

	t.Run("does not retry on non-retryable error", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{
			Config:  &config.AIConfig{Timeout: 1 * time.Minute},
			ErrType: atlaserrors.ErrClaudeInvocation,
		}
		attempts := 0
		// Use authentication error which is explicitly non-retryable
		nonRetryableErr := errTestAuthFailed

		_, err := b.RunWithTimeout(context.Background(), &domain.AIRequest{}, func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			attempts++
			return nil, nonRetryableErr
		})

		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}

func TestBaseRunner_HandleExecutionError(t *testing.T) {
	t.Parallel()

	t.Run("returns context error when canceled", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := b.HandleExecutionError(ctx, errTestSome, nil, func(e error) error { return e })

		assert.Equal(t, context.Canceled, err)
	})

	t.Run("uses tryParse when successful", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		expected := &domain.AIResult{Output: "parsed"}

		result, err := b.HandleExecutionError(
			context.Background(),
			errTestSome,
			func() (*domain.AIResult, bool) { return expected, true },
			func(e error) error { return e },
		)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("uses wrapErr when tryParse fails", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		originalErr := errTestOriginal
		wrappedErr := errTestWrapped

		_, err := b.HandleExecutionError(
			context.Background(),
			originalErr,
			func() (*domain.AIResult, bool) { return nil, false },
			func(_ error) error { return wrappedErr },
		)

		assert.Equal(t, wrappedErr, err)
	})

	t.Run("handles nil tryParse", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		wrappedErr := errTestWrapped

		_, err := b.HandleExecutionError(
			context.Background(),
			errTestOriginal,
			nil,
			func(_ error) error { return wrappedErr },
		)

		assert.Equal(t, wrappedErr, err)
	})
}

func TestBaseRunner_HandleProviderExecutionError(t *testing.T) {
	t.Parallel()

	testInfo := CLIInfo{
		Name:        "test-cli",
		ErrType:     errTestErrorType,
		InstallHint: "Install with: brew install test-cli",
	}

	t.Run("returns context error when canceled", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := b.HandleProviderExecutionError(ctx, testInfo, errTestSome, nil, nil)

		assert.Equal(t, context.Canceled, err)
	})

	t.Run("uses tryParse when successful", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}
		expected := &domain.AIResult{Output: "parsed"}

		result, err := b.HandleProviderExecutionError(
			context.Background(),
			testInfo,
			errTestSome,
			[]byte("stderr"),
			func() (*domain.AIResult, bool) { return expected, true },
		)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("wraps error with CLI info when tryParse fails", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}

		_, err := b.HandleProviderExecutionError(
			context.Background(),
			testInfo,
			errTestOriginal,
			[]byte("test stderr"),
			func() (*domain.AIResult, bool) { return nil, false },
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "test stderr")
	})

	t.Run("handles nil tryParse", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}

		_, err := b.HandleProviderExecutionError(
			context.Background(),
			testInfo,
			errTestOriginal,
			[]byte("stderr output"),
			nil,
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "stderr output")
	})
}

func TestBaseRunner_ValidateWorkingDir(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for empty working directory", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}

		err := b.ValidateWorkingDir("")

		assert.NoError(t, err)
	})

	t.Run("returns nil for existing directory", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}

		// Use the current test directory which definitely exists
		err := b.ValidateWorkingDir(".")

		assert.NoError(t, err)
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		t.Parallel()
		b := &BaseRunner{}

		err := b.ValidateWorkingDir("/non/existent/directory/path")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "working directory missing")
		assert.ErrorIs(t, err, atlaserrors.ErrWorktreeNotFound)
	})
}
