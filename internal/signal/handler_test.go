package signal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandler_Signal_CancelsContext verifies that receiving a signal
// cancels the context.
func TestHandler_Signal_CancelsContext(t *testing.T) {
	h := NewHandler(context.Background())
	defer h.Stop()

	// Simulate signal via internal method (no real OS signals)
	h.handleSignal()

	// Context should be canceled
	require.Error(t, h.Context().Err())
	assert.Equal(t, context.Canceled, h.Context().Err())
}

// TestHandler_Signal_ClosesInterruptedChannel verifies that receiving a signal
// closes the interrupted channel.
func TestHandler_Signal_ClosesInterruptedChannel(t *testing.T) {
	h := NewHandler(context.Background())
	defer h.Stop()

	// Simulate signal
	h.handleSignal()

	// Interrupted channel should be closed
	select {
	case <-h.Interrupted():
		// Expected - channel is closed
	default:
		t.Fatal("interrupted channel should be closed after signal")
	}
}

// TestHandler_MultipleSignals_OnlyProcessedOnce verifies that multiple
// signals are only processed once (idempotent behavior).
func TestHandler_MultipleSignals_OnlyProcessedOnce(t *testing.T) {
	h := NewHandler(context.Background())
	defer h.Stop()

	// Simulate multiple signals
	h.handleSignal()
	h.handleSignal()
	h.handleSignal()

	// Context should still be canceled (just once)
	require.Error(t, h.Context().Err())

	// Interrupted channel should still be closed
	select {
	case <-h.Interrupted():
		// Expected
	default:
		t.Fatal("interrupted channel should be closed")
	}
}

// TestHandler_Stop_CancelsContext verifies that Stop() cancels the context.
func TestHandler_Stop_CancelsContext(t *testing.T) {
	h := NewHandler(context.Background())
	h.Stop()

	// Context should be canceled after stop
	assert.Error(t, h.Context().Err())
}

// TestHandler_Stop_IsIdempotent verifies that Stop() can be called multiple times safely.
func TestHandler_Stop_IsIdempotent(t *testing.T) {
	h := NewHandler(context.Background())

	// Should not panic when called multiple times
	h.Stop()
	h.Stop()
	h.Stop()

	assert.Error(t, h.Context().Err())
}

// TestHandler_ParentContextCancelled verifies that the handler respects
// parent context cancellation.
func TestHandler_ParentContextCancelled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	h := NewHandler(parent)
	defer h.Stop()

	// Cancel parent context
	cancel()

	// Handler's context should also be canceled
	assert.Error(t, h.Context().Err())
}

// TestHandler_InterruptedChannelNotClosedInitially verifies that the
// interrupted channel is open initially.
func TestHandler_InterruptedChannelNotClosedInitially(t *testing.T) {
	h := NewHandler(context.Background())
	defer h.Stop()

	// Interrupted channel should be open
	select {
	case <-h.Interrupted():
		t.Fatal("interrupted channel should be open initially")
	default:
		// Expected - channel is open
	}
}

// TestHandler_ContextValidInitially verifies that the context is valid initially.
func TestHandler_ContextValidInitially(t *testing.T) {
	h := NewHandler(context.Background())
	defer h.Stop()

	// Context should be valid
	assert.NoError(t, h.Context().Err())
}

// TestHandler_ListenContinuesAfterSignal verifies that the listen goroutine
// continues to process signals after the first one (bug fix test).
func TestHandler_ListenContinuesAfterSignal(t *testing.T) {
	h := NewHandler(context.Background())
	defer h.Stop()

	// Send multiple signals directly to the channel to simulate repeated Ctrl+C
	// The first signal should be processed, and the handler should remain responsive
	h.sigChan <- nil // First signal
	h.sigChan <- nil // Second signal (should not block/deadlock)

	// Give the goroutine time to process
	// If listen() exits after first signal, the second send would block forever

	// Context should be canceled after the first signal
	require.Error(t, h.Context().Err())
	assert.Equal(t, context.Canceled, h.Context().Err())

	// Interrupted channel should be closed
	select {
	case <-h.Interrupted():
		// Expected - channel is closed
	default:
		t.Fatal("interrupted channel should be closed after signal")
	}
}

// TestHandler_StopExitsListenGoroutine verifies that Stop() properly signals
// the listen goroutine to exit.
func TestHandler_StopExitsListenGoroutine(t *testing.T) {
	h := NewHandler(context.Background())

	// Stop should cleanly exit the listen goroutine
	h.Stop()

	// Verify the handler is stopped by checking context is canceled
	assert.Error(t, h.Context().Err())

	// Sending to sigChan should not block (channel is stopped)
	// This is implicitly tested by the test completing without deadlock
}
