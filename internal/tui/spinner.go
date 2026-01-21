// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// flushWriter attempts to flush the writer if it supports flushing.
// This ensures escape sequences are sent immediately to the terminal,
// preventing multi-line output when the terminal is in the background.
func flushWriter(w io.Writer) {
	type syncer interface {
		Sync() error
	}
	if s, ok := w.(syncer); ok {
		_ = s.Sync()
	}
}

// spinnerFrames are the animation frames for the spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} //nolint:gochecknoglobals // Package-level constant for spinner animation

// SpinnerFrames returns the animation frames for testing.
func SpinnerFrames() []string {
	return spinnerFrames
}

// SpinnerInterval is the default update interval for spinner animation.
const SpinnerInterval = 100 * time.Millisecond

// ElapsedTimeThreshold is the duration after which elapsed time is shown in spinner.
const ElapsedTimeThreshold = 30 * time.Second

// SpinnerMessageThrottle is the minimum interval between spinner message updates.
// This prevents excessive flashing during high-frequency activity events.
const SpinnerMessageThrottle = 200 * time.Millisecond

// spinnerManager is the singleton instance for tracking active spinners.
var spinnerManager = &SpinnerManager{} //nolint:gochecknoglobals // Singleton for global spinner tracking

// SpinnerManager tracks the currently active spinner to allow coordinated
// line clearing before log writes. This prevents log messages from appearing
// on the same line as the spinner animation.
type SpinnerManager struct {
	mu     sync.Mutex
	active *TerminalSpinner
}

// SetActive registers the given spinner as the currently active spinner.
func (m *SpinnerManager) SetActive(s *TerminalSpinner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = s
}

// ClearActive removes the currently active spinner.
func (m *SpinnerManager) ClearActive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = nil
}

// GetActive returns the currently active spinner, or nil if no spinner is active.
func (m *SpinnerManager) GetActive() *TerminalSpinner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// GlobalSpinnerManager returns the global spinner manager instance.
// This allows coordination between log writers and spinner animations.
func GlobalSpinnerManager() *SpinnerManager {
	return spinnerManager
}

// TerminalSpinner provides animated progress indication for terminal output.
// This is the concrete implementation of spinner functionality.
type TerminalSpinner struct {
	w       io.Writer
	styles  *OutputStyles
	message string
	started time.Time
	done    chan struct{}
	mu      sync.Mutex
	running bool
	stopped bool // tracks if Stop() has been called for current cycle

	// Throttling for message updates to prevent excessive flashing
	lastMessageUpdate time.Time
	throttleInterval  time.Duration
}

// NewTerminalSpinner creates a new spinner that writes to w.
func NewTerminalSpinner(w io.Writer) *TerminalSpinner {
	return &TerminalSpinner{
		w:                w,
		styles:           NewOutputStyles(),
		throttleInterval: SpinnerMessageThrottle,
	}
}

// Start begins the spinner animation with the given message.
// This method is safe to call multiple times; subsequent calls update the message.
func (s *TerminalSpinner) Start(ctx context.Context, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.message = message
	s.started = time.Now()
	s.lastMessageUpdate = time.Now() // Initialize throttle timestamp

	// If already running, just update the message
	if s.running {
		return
	}

	s.running = true
	s.stopped = false // Reset stopped flag for this new Start() cycle
	s.done = make(chan struct{})

	// Register with the global spinner manager for log coordination
	spinnerManager.SetActive(s)

	// Capture the done channel before starting the goroutine
	// to avoid race with potential Stop() calls
	done := s.done
	go s.animate(ctx, done)
}

// UpdateMessage changes the spinner message without stopping the animation.
// Updates are throttled to prevent excessive terminal I/O during high-frequency events.
func (s *TerminalSpinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip if message hasn't actually changed (deduplication)
	if s.message == message {
		return
	}

	// Throttle: only update if enough time has passed since last update
	// This prevents flashing during high-frequency activity events
	now := time.Now()
	if now.Sub(s.lastMessageUpdate) < s.throttleInterval {
		return
	}

	s.message = message
	s.lastMessageUpdate = now
}

// Stop stops the spinner animation and clears the line.
func (s *TerminalSpinner) Stop() {
	s.mu.Lock()
	if !s.running || s.stopped {
		s.mu.Unlock()
		return
	}

	// Mark as stopped to ensure we only close the done channel once
	// This prevents races between Stop() and context cancellation
	s.stopped = true
	s.running = false
	done := s.done
	s.mu.Unlock()

	close(done)

	// Clear the spinner line BEFORE marking inactive
	// This ensures any logs that come through after ClearActive()
	// will write to an already-cleared line
	_, _ = fmt.Fprint(s.w, "\r\033[K")
	flushWriter(s.w)

	// Now safe to mark as inactive
	spinnerManager.ClearActive()
}

// StopWithSuccess stops the spinner and displays a success message.
func (s *TerminalSpinner) StopWithSuccess(message string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.w, s.styles.Success.Render("✓ "+message))
}

// StopWithError stops the spinner and displays an error message.
func (s *TerminalSpinner) StopWithError(message string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.w, s.styles.Error.Render("✗ "+message))
}

// StopWithWarning stops the spinner and displays a warning message.
func (s *TerminalSpinner) StopWithWarning(message string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.w, s.styles.Warning.Render("⚠ "+message))
}

// animate runs the spinner animation loop.
// The done channel is passed as a parameter to avoid race conditions with s.done field.
func (s *TerminalSpinner) animate(ctx context.Context, done <-chan struct{}) {
	ticker := time.NewTicker(SpinnerInterval)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-done:
			// Stopped explicitly via Stop() - don't write, Stop() handles cleanup
			return
		case <-ctx.Done():
			// Context canceled - mark as stopped and clear line
			s.mu.Lock()
			wasRunning := s.running && !s.stopped
			if wasRunning {
				s.running = false
				s.stopped = true
			}
			s.mu.Unlock()

			if wasRunning {
				// Clear line BEFORE marking inactive
				_, _ = fmt.Fprint(s.w, "\r\033[K")
				flushWriter(s.w)
				spinnerManager.ClearActive()
			}
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.running {
				s.mu.Unlock()
				return
			}

			msg := s.message
			elapsed := time.Since(s.started)
			if elapsed > ElapsedTimeThreshold {
				msg = fmt.Sprintf("%s %s", s.message, formatElapsedTime(elapsed))
			}

			// Render spinner frame with message
			spinnerFrame := s.styles.Info.Render(spinnerFrames[frame%len(spinnerFrames)])
			_, _ = fmt.Fprintf(s.w, "\r\033[K%s %s", spinnerFrame, msg)
			flushWriter(s.w)
			s.mu.Unlock()

			frame++
		}
	}
}

// formatElapsedTime formats duration in human-readable form for display.
func formatElapsedTime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("(%ds elapsed)", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("(%dm %ds elapsed)", minutes, seconds)
}

// FormatDuration formats a duration in milliseconds for display (e.g., "1.2s").
func FormatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

// SpinnerAdapter wraps the TerminalSpinner to satisfy the Output.Spinner interface.
// This provides a bridge between the existing spinner implementation and the
// Output interface contract.
type SpinnerAdapter struct {
	spinner *TerminalSpinner
	cancel  context.CancelFunc
}

// NewSpinnerAdapter creates a new SpinnerAdapter for TTY output (AC: #6).
// Uses the custom TerminalSpinner implementation which provides animated
// spinner functionality similar to the Bubbles spinner library.
// Context is used for cancellation propagation per architecture rules.
func NewSpinnerAdapter(ctx context.Context, w io.Writer, msg string) *SpinnerAdapter {
	ctx, cancel := context.WithCancel(ctx)
	s := NewTerminalSpinner(w)
	s.Start(ctx, msg)
	return &SpinnerAdapter{
		spinner: s,
		cancel:  cancel,
	}
}

// Update changes the spinner message (AC: #6).
func (a *SpinnerAdapter) Update(msg string) {
	a.spinner.UpdateMessage(msg)
}

// Stop terminates the spinner (AC: #6).
func (a *SpinnerAdapter) Stop() {
	a.cancel()
	a.spinner.Stop()
}

// NoopSpinner is a no-op spinner for JSON/non-TTY output (AC: #6).
type NoopSpinner struct{}

// Update is a no-op for NoopSpinner.
func (*NoopSpinner) Update(_ string) {}

// Stop is a no-op for NoopSpinner.
func (*NoopSpinner) Stop() {}
