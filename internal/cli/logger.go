// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/logging"
)

// LogFileWriter holds the log file writer for cleanup purposes.
// This is package-level to enable cleanup during shutdown.
var logFileWriter io.WriteCloser //nolint:gochecknoglobals // Needed for cleanup

// zerologConfigOnce ensures zerolog global settings are configured exactly once.
var zerologConfigOnce sync.Once //nolint:gochecknoglobals // One-time configuration

// zerologGlobalMu protects concurrent writes to the zerolog global logger.
// This is separate from globalLoggerMu to avoid deadlocks.
var zerologGlobalMu sync.Mutex //nolint:gochecknoglobals // Protects zerolog global

// configureZerologGlobals sets zerolog global field names to match our log entry structure.
// This is called once before any logger is created and is safe for concurrent use.
func configureZerologGlobals() {
	zerologConfigOnce.Do(func() {
		// Use "ts" for timestamp to match logEntry struct in workspace_logs.go
		zerolog.TimestampFieldName = "ts"
		// Use "event" for message to match logEntry struct
		zerolog.MessageFieldName = "event"
	})
}

// loggerSetup holds the common components needed to create a logger.
type loggerSetup struct {
	level      zerolog.Level
	hook       zerolog.Hook
	fileWriter io.WriteCloser
	console    io.Writer
}

// prepareLoggerSetup creates the common logger components.
// Returns the setup and any error from file writer creation.
// The error is non-fatal - callers can proceed with console-only logging.
func prepareLoggerSetup(verbose, quiet bool) (*loggerSetup, error) {
	configureZerologGlobals()

	setup := &loggerSetup{
		level:   selectLevel(verbose, quiet),
		hook:    logging.NewSensitiveDataHook(),
		console: selectOutput(),
	}

	fileWriter, err := createLogFileWriter()
	if err == nil {
		setup.fileWriter = fileWriter
	}
	return setup, err
}

// buildLogger creates a zerolog.Logger from the setup and writer.
func buildLogger(setup *loggerSetup, writer io.Writer) zerolog.Logger {
	return zerolog.New(writer).Level(setup.level).Hook(setup.hook).With().Timestamp().Logger()
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
	setup, err := prepareLoggerSetup(verbose, quiet)

	var writer io.Writer
	if err != nil || setup.fileWriter == nil {
		// Log file creation failed; continue with console-only output
		writer = setup.console
	} else {
		// Store file writer for cleanup
		logFileWriter = setup.fileWriter
		// Multi-writer: console + file
		writer = zerolog.MultiLevelWriter(setup.console, setup.fileWriter)
	}

	logger := buildLogger(setup, writer)
	setGlobalLogger(logger)
	return logger
}

// setGlobalLogger configures the global zerolog logger to match our CLI logger config.
// This ensures that any code using log.Debug(), log.Info(), etc. from the
// github.com/rs/zerolog/log package uses the same formatting as our CLI logger.
// This function is safe for concurrent use.
func setGlobalLogger(cliLogger zerolog.Logger) {
	zerologGlobalMu.Lock()
	defer zerologGlobalMu.Unlock()
	log.Logger = cliLogger
}

// InitLoggerWithWriter creates and configures a zerolog.Logger with a custom writer.
// This is primarily intended for testing purposes.
func InitLoggerWithWriter(verbose, quiet bool, w io.Writer) zerolog.Logger {
	// Configure zerolog global field names on first call
	configureZerologGlobals()

	level := selectLevel(verbose, quiet)
	hook := logging.NewSensitiveDataHook()
	logger := zerolog.New(w).Level(level).Hook(hook).With().Timestamp().Logger()

	// Configure global logger to match CLI logger settings
	setGlobalLogger(logger)

	return logger
}

// TaskLogAppender is a minimal interface for appending logs to task-specific log files.
// This interface is satisfied by task.Store.
type TaskLogAppender interface {
	AppendLog(ctx context.Context, workspaceName, taskID string, entry []byte) error
}

// InitLoggerWithTaskStore creates a logger that persists task-specific logs.
// Log entries with workspace_name and task_id fields are written to the task's log file.
// All logs continue to go to console and global log file as normal.
func InitLoggerWithTaskStore(verbose, quiet bool, store TaskLogAppender) zerolog.Logger {
	setup, err := prepareLoggerSetup(verbose, quiet)

	var baseWriter io.Writer
	if err != nil || setup.fileWriter == nil {
		// Log file creation failed; continue with console-only output + task logs
		baseWriter = setup.console
	} else {
		// Store file writer for cleanup
		logFileWriter = setup.fileWriter
		// Multi-writer: console + file
		baseWriter = zerolog.MultiLevelWriter(setup.console, setup.fileWriter)
	}

	// Wrap with task log writer to persist task-specific logs
	taskLogWriter := newTaskLogWriter(store, baseWriter)

	logger := buildLogger(setup, taskLogWriter)
	setGlobalLogger(logger)
	return logger
}

// taskLogWriter wraps an io.Writer and persists log entries with workspace_name
// and task_id fields to the task-specific log file.
type taskLogWriter struct {
	store  TaskLogAppender
	target io.Writer
}

// newTaskLogWriter creates a taskLogWriter that persists logs with workspace/task
// context to the task store while passing all writes to the target writer.
func newTaskLogWriter(store TaskLogAppender, target io.Writer) *taskLogWriter {
	return &taskLogWriter{
		store:  store,
		target: target,
	}
}

// logFields represents the minimal fields we need to extract from log entries.
type logFields struct {
	WorkspaceName string `json:"workspace_name"`
	TaskID        string `json:"task_id"`
}

// Write implements io.Writer. It parses JSON log entries to extract workspace_name
// and task_id, persisting matching entries to the task log file.
func (w *taskLogWriter) Write(p []byte) (n int, err error) {
	// Try to persist to task log if entry has workspace/task context
	w.persistToTaskLog(p)

	// Always pass through to target writer
	return w.target.Write(p)
}

// persistToTaskLog attempts to parse the log entry and persist it to the task log.
// Failures are silently ignored to avoid disrupting normal logging.
func (w *taskLogWriter) persistToTaskLog(p []byte) {
	// Parse JSON to extract workspace_name and task_id
	var fields logFields
	if err := json.Unmarshal(p, &fields); err != nil {
		// Not valid JSON or doesn't have our fields - skip silently
		return
	}

	// Only persist if both workspace_name and task_id are present
	if fields.WorkspaceName == "" || fields.TaskID == "" {
		return
	}

	// Persist to task log - errors are ignored to avoid disrupting logging
	_ = w.store.AppendLog(context.Background(), fields.WorkspaceName, fields.TaskID, p)
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

// LogFilePath returns the path to the global CLI log file.
// This is useful for displaying the log location to users.
func LogFilePath() (string, error) {
	atlasHome, err := getAtlasHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(atlasHome, constants.LogsDir, constants.CLILogFileName), nil
}
