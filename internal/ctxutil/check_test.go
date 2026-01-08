package ctxutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mrz1836/atlas/internal/ctxutil"
)

func TestCanceled(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for active context", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		err := ctxutil.Canceled(ctx)
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("returns error for canceled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := ctxutil.Canceled(ctx)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("returns error for deadline exceeded", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		// Wait for timeout
		<-ctx.Done()
		err := ctxutil.Canceled(ctx)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
	})
}
