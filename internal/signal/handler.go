// Package signal provides graceful shutdown handling for ATLAS CLI commands.
//
// Import rules:
//   - CAN import: std lib only
//   - MUST NOT import: internal packages (to avoid circular dependencies)
package signal

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Handler manages graceful shutdown by listening for interrupt signals.
// It wraps a context and cancels it when SIGINT or SIGTERM is received.
type Handler struct {
	ctx         context.Context //nolint:containedctx // intentional: handler manages context lifecycle
	cancel      context.CancelFunc
	interrupted chan struct{}
	done        chan struct{} // signals listen() to exit cleanly
	once        sync.Once
	stopOnce    sync.Once
	sigChan     chan os.Signal
}

// NewHandler creates a signal handler that listens for SIGINT and SIGTERM.
// When a signal is received, the handler cancels the context and closes
// the interrupted channel.
//
// Usage:
//
//	h := signal.NewHandler(ctx)
//	defer h.Stop()
//	ctx = h.Context()
//
//	// ... do work with ctx ...
//
//	select {
//	case <-h.Interrupted():
//	    // Handle interruption
//	default:
//	}
func NewHandler(parent context.Context) *Handler {
	ctx, cancel := context.WithCancel(parent)
	h := &Handler{
		ctx:         ctx,
		cancel:      cancel,
		interrupted: make(chan struct{}),
		done:        make(chan struct{}),
		// Buffer of 1 ensures signal.Notify doesn't drop signals if handler is busy.
		// See: https://pkg.go.dev/os/signal#Notify
		sigChan: make(chan os.Signal, 1),
	}

	signal.Notify(h.sigChan, syscall.SIGINT, syscall.SIGTERM)
	go h.listen()

	return h
}

// Context returns the cancellable context.
// Use this context for all operations that should be interruptible.
func (h *Handler) Context() context.Context {
	return h.ctx
}

// Interrupted returns a channel that closes when an interrupt signal is received.
// Use this to detect when the user pressed Ctrl+C.
func (h *Handler) Interrupted() <-chan struct{} {
	return h.interrupted
}

// Stop cleans up the signal handler and stops listening for signals.
// Always call this when done to prevent resource leaks.
func (h *Handler) Stop() {
	h.stopOnce.Do(func() {
		signal.Stop(h.sigChan)
		close(h.done) // Signal listen() to exit before closing sigChan
		h.cancel()
	})
}

// handleSignal processes a received signal.
// This method is exported for testing purposes.
func (h *Handler) handleSignal() {
	h.once.Do(func() {
		h.cancel()
		close(h.interrupted)
	})
}

// listen waits for signals and handles them.
// It loops continuously to handle multiple signals until Stop() is called
// or the context is canceled.
func (h *Handler) listen() {
	for {
		select {
		case <-h.ctx.Done():
			// Context was canceled externally
			return
		case <-h.done:
			// Stop() was called - exit cleanly
			return
		case <-h.sigChan:
			h.handleSignal()
			// Continue looping to drain signal channel. Note: only the first signal
			// has effect due to sync.Once in handleSignal(); subsequent signals are
			// received but ignored to avoid blocking signal delivery.
		}
	}
}
