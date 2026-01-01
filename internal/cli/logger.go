// Package cli provides the command-line interface for atlas.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/term"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/logging"
)

// LogFileWriter holds the log file writer for cleanup purposes.
// This is package-level to enable cleanup during shutdown.
var logFileWriter io.WriteCloser //nolint:gochecknoglobals // Needed for cleanup

// zerologConfigured tracks whether zerolog global settings have been applied.
var zerologConfigured bool //nolint:gochecknoglobals // One-time configuration flag

// configureZerologGlobals sets zerolog global field names to match our log entry structure.
// This is called once before any logger is created.
func configureZerologGlobals() {
	if zerologConfigured {
		return
	}
	// Use "ts" for timestamp to match logEntry struct in workspace_logs.go
	zerolog.TimestampFieldName = "ts"
	// Use "event" for message to match logEntry struct
	zerolog.MessageFieldName = "event"
	zerologConfigured = true
}

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
//
// The logger also writes to ~/.atlas/logs/atlas.log with rotation enabled.
// If the log file cannot be created, the logger will continue with console-only output.
func InitLogger(verbose, quiet bool) zerolog.Logger {
	// Configure zerolog global field names on first call
	configureZerologGlobals()

	level := selectLevel(verbose, quiet)
	consoleOutput := selectOutput()

	// Create sensitive data filter hook
	hook := logging.NewSensitiveDataHook()

	// Create file writer for global log with rotation
	fileWriter, err := createLogFileWriter()
	if err != nil {
		// Log file creation failed; continue with console-only output
		// This can happen if ATLAS_HOME is not writable
		return zerolog.New(consoleOutput).Level(level).Hook(hook).With().Timestamp().Logger()
	}

	// Store file writer for cleanup
	logFileWriter = fileWriter

	// Multi-writer: console + file
	multi := zerolog.MultiLevelWriter(consoleOutput, fileWriter)
	return zerolog.New(multi).Level(level).Hook(hook).With().Timestamp().Logger()
}

// InitLoggerWithWriter creates and configures a zerolog.Logger with a custom writer.
// This is primarily intended for testing purposes.
func InitLoggerWithWriter(verbose, quiet bool, w io.Writer) zerolog.Logger {
	// Configure zerolog global field names on first call
	configureZerologGlobals()

	level := selectLevel(verbose, quiet)
	hook := logging.NewSensitiveDataHook()
	return zerolog.New(w).Level(level).Hook(hook).With().Timestamp().Logger()
}

// CloseLogFile closes the global log file writer if it was opened.
// This should be called during application shutdown for clean cleanup.
func CloseLogFile() {
	if logFileWriter != nil {
		_ = logFileWriter.Close()
		logFileWriter = nil
	}
}

// selectLevel determines the appropriate log level based on flags.
func selectLevel(verbose, quiet bool) zerolog.Level {
	switch {
	case verbose:
		return zerolog.DebugLevel
	case quiet:
		return zerolog.WarnLevel
	default:
		return zerolog.InfoLevel
	}
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

// filteringWriteCloser wraps a WriteCloser with sensitive data filtering.
// It implements io.WriteCloser so it can be used as a drop-in replacement.
type filteringWriteCloser struct {
	filter *logging.FilteringWriter
	closer io.Closer
}

// Write implements io.Writer by delegating to the filtering writer.
func (fwc *filteringWriteCloser) Write(p []byte) (n int, err error) {
	return fwc.filter.Write(p)
}

// Close implements io.Closer by delegating to the underlying closer.
func (fwc *filteringWriteCloser) Close() error {
	return fwc.closer.Close()
}

// createLogFileWriter creates a rotating file writer for the global CLI log.
// Returns a lumberjack logger configured with rotation settings, wrapped with
// a filtering writer to ensure sensitive data is never written to disk.
func createLogFileWriter() (io.WriteCloser, error) {
	atlasHome, err := getAtlasHome()
	if err != nil {
		return nil, err
	}

	logDir := filepath.Join(atlasHome, constants.LogsDir)
	logPath := filepath.Join(logDir, constants.CLILogFileName)

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create rotating log file writer
	lj := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    constants.LogMaxSizeMB,
		MaxBackups: constants.LogMaxBackups,
		MaxAge:     constants.LogMaxAgeDays,
		Compress:   constants.LogCompress,
	}

	// Wrap with filtering writer to redact sensitive data
	return &filteringWriteCloser{
		filter: logging.NewFilteringWriter(lj),
		closer: lj,
	}, nil
}

// getAtlasHome returns the atlas home directory path.
// If ATLAS_HOME environment variable is set, it uses that.
// Otherwise, it defaults to ~/.atlas.
func getAtlasHome() (string, error) {
	if atlasHome := os.Getenv("ATLAS_HOME"); atlasHome != "" {
		return atlasHome, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(home, constants.AtlasHome), nil
}

// GetLogFilePath returns the path to the global CLI log file.
// This is useful for displaying the log location to users.
func GetLogFilePath() (string, error) {
	atlasHome, err := getAtlasHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(atlasHome, constants.LogsDir, constants.CLILogFileName), nil
}
