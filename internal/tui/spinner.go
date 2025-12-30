// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

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

// Spinner provides animated progress indication for terminal output.
type Spinner struct {
	w       io.Writer
	styles  *OutputStyles
	message string
	started time.Time
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

// NewSpinner creates a new spinner that writes to w.
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{
		w:      w,
		styles: NewOutputStyles(),
	}
}

// Start begins the spinner animation with the given message.
// This method is safe to call multiple times; subsequent calls update the message.
func (s *Spinner) Start(ctx context.Context, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.message = message
	s.started = time.Now()

	// If already running, just update the message
	if s.running {
		return
	}

	s.running = true
	s.done = make(chan struct{})

	go s.animate(ctx)
}

// UpdateMessage changes the spinner message without stopping the animation.
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// Stop stops the spinner animation and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.done)
	s.mu.Unlock()

	// Clear the spinner line
	_, _ = fmt.Fprint(s.w, "\r\033[K")
}

// StopWithSuccess stops the spinner and displays a success message.
func (s *Spinner) StopWithSuccess(message string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.w, s.styles.Success.Render("✓ "+message))
}

// StopWithError stops the spinner and displays an error message.
func (s *Spinner) StopWithError(message string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.w, s.styles.Error.Render("✗ "+message))
}

// StopWithWarning stops the spinner and displays a warning message.
func (s *Spinner) StopWithWarning(message string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.w, s.styles.Warning.Render("⚠ "+message))
}

// animate runs the spinner animation loop.
func (s *Spinner) animate(ctx context.Context) {
	ticker := time.NewTicker(SpinnerInterval)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-s.done:
			// Stopped explicitly via Stop() - don't write, Stop() handles cleanup
			return
		case <-ctx.Done():
			// Context canceled - mark as not running and clear line
			s.mu.Lock()
			if s.running {
				s.running = false
				s.mu.Unlock()
				_, _ = fmt.Fprint(s.w, "\r\033[K")
			} else {
				s.mu.Unlock()
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
