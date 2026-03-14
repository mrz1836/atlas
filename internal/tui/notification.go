package tui

import (
	"fmt"
	"io"
	"os"
)

// Notifier handles user notifications such as terminal bells.
// It respects configuration settings for bell enabled and quiet mode.
type Notifier struct {
	bellEnabled bool
	quiet       bool
	writer      io.Writer
}

// NewNotifier creates a notifier with the given settings.
// It writes to os.Stdout by default.
func NewNotifier(bellEnabled, quiet bool) *Notifier {
	return NewNotifierWithWriter(bellEnabled, quiet, os.Stdout)
}

// NewNotifierWithWriter creates a notifier with a custom writer.
// This is useful for testing.
func NewNotifierWithWriter(bellEnabled, quiet bool, w io.Writer) *Notifier {
	return &Notifier{
		bellEnabled: bellEnabled,
		quiet:       quiet,
		writer:      w,
	}
}

// Bell emits a terminal bell character (\a) if enabled and not in quiet mode.
// The bell character (ASCII 7, BEL) causes most terminals to play an alert sound.
func (n *Notifier) Bell() {
	if n.bellEnabled && !n.quiet {
		_, _ = fmt.Fprint(n.writer, "\a")
	}
}
