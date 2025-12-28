// Package cli provides the command-line interface for atlas.
package cli

import (
	"fmt"
	"io"
	"os"
)

// EmitBell writes the BEL character to stdout to trigger the terminal bell.
// This works on most terminals including iTerm2, Terminal.app, tmux, etc.
func EmitBell() {
	EmitBellTo(os.Stdout)
}

// EmitBellTo writes the BEL character to the specified writer.
// This allows testing without actually emitting to stdout.
func EmitBellTo(w io.Writer) {
	_, _ = fmt.Fprint(w, "\a") // BEL character (ASCII 7)
}

// ShouldNotify checks if a notification should be triggered for an event.
// Returns true if bell is enabled and the event is in the configured events list.
func ShouldNotify(event string, cfg *NotificationConfig) bool {
	if cfg == nil {
		return false
	}
	if !cfg.BellEnabled {
		return false
	}
	for _, e := range cfg.Events {
		if e == event {
			return true
		}
	}
	return false
}

// NotifyIfEnabled emits a bell if the event should trigger a notification.
// This is a convenience function combining ShouldNotify and EmitBell.
func NotifyIfEnabled(event string, cfg *NotificationConfig) {
	if ShouldNotify(event, cfg) {
		EmitBell()
	}
}

// NotifyIfEnabledTo emits a bell to the specified writer if the event should trigger a notification.
// This allows testing without actually emitting to stdout.
func NotifyIfEnabledTo(w io.Writer, event string, cfg *NotificationConfig) {
	if ShouldNotify(event, cfg) {
		EmitBellTo(w)
	}
}
