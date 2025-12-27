// Package cli provides the command-line interface for atlas.
package cli

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/term"
)

// InitLogger creates and configures a zerolog.Logger based on verbosity flags.
//
// Log levels are set as follows:
//   - verbose=true: Debug level (most detailed)
//   - quiet=true: Warn level (errors and warnings only)
//   - default: Info level (normal operation)
//
// Output format is determined by the terminal:
//   - TTY with colors enabled: Console writer with timestamps
//   - Non-TTY or NO_COLOR set: JSON output to stderr
func InitLogger(verbose, quiet bool) zerolog.Logger {
	var level zerolog.Level
	switch {
	case verbose:
		level = zerolog.DebugLevel
	case quiet:
		level = zerolog.WarnLevel
	default:
		level = zerolog.InfoLevel
	}

	output := selectOutput()

	return zerolog.New(output).Level(level).With().Timestamp().Logger()
}

// selectOutput determines the appropriate output writer based on
// terminal capabilities and environment settings.
func selectOutput() io.Writer {
	// Use console writer for TTY without NO_COLOR
	if term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("NO_COLOR") == "" {
		return zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.Kitchen,
		}
	}

	// Default to JSON output for non-TTY or when NO_COLOR is set
	return os.Stderr
}
