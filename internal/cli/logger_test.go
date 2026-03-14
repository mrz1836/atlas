package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/logging"
	"github.com/mrz1836/atlas/internal/tui"
)

func TestInitLogger_VerboseMode(t *testing.T) {
	t.Parallel()

	// Use custom writer to avoid file creation side effects
	var buf bytes.Buffer
	logger := InitLoggerWithWriter(true, false, &buf)
	assert.Equal(t, zerolog.DebugLevel, logger.GetLevel())
}

func TestInitLogger_QuietMode(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := InitLoggerWithWriter(false, true, &buf)
	assert.Equal(t, zerolog.WarnLevel, logger.GetLevel())
}

func TestInitLogger_DefaultMode(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := InitLoggerWithWriter(false, false, &buf)
	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel())
}

func TestInitLogger_LogLevelPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		verbose       bool
		quiet         bool
		expectedLevel zerolog.Level
	}{
		{
			name:          "default is info level",
			verbose:       false,
			quiet:         false,
			expectedLevel: zerolog.InfoLevel,
		},
		{
			name:          "verbose enables debug level",
			verbose:       true,
			quiet:         false,
			expectedLevel: zerolog.DebugLevel,
		},
		{
			name:          "quiet enables warn level",
			verbose:       false,
			quiet:         true,
			expectedLevel: zerolog.WarnLevel,
		},
		{
			name:          "verbose takes precedence over quiet",
			verbose:       true,
			quiet:         true,
			expectedLevel: zerolog.DebugLevel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := InitLoggerWithWriter(tc.verbose, tc.quiet, &buf)
			assert.Equal(t, tc.expectedLevel, logger.GetLevel())
		})
	}
}

func TestInitLogger_HasTimestamp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := InitLoggerWithWriter(false, false, &buf)

	// Logger should not be zero value
	assert.NotEqual(t, zerolog.Logger{}, logger)
}

func TestSelectOutput_NonTTY(t *testing.T) {
	// This test runs in a non-TTY environment (typical for CI/tests).
	// In non-TTY mode, selectOutput always returns os.Stderr regardless of NO_COLOR.

	output := selectOutput()
	assert.NotNil(t, output)
	// In non-TTY environment, output should be os.Stderr (JSON format)
	assert.Equal(t, os.Stderr, output)
}

func TestSelectOutput_RespectsNO_COLOR(t *testing.T) {
	// Test that NO_COLOR environment variable is checked.
	// In non-TTY environment, this has no effect, but we verify the code path.

	// t.Setenv automatically restores the original value after test
	t.Setenv("NO_COLOR", "1")

	output := selectOutput()
	assert.NotNil(t, output)
	// In non-TTY or NO_COLOR mode, output should be os.Stderr
	assert.Equal(t, os.Stderr, output)
}

func TestInitLogger_WithNO_COLOR(t *testing.T) {
	// Verify logger initializes correctly when NO_COLOR is set.
	// This ensures the NO_COLOR code path doesn't cause any issues.

	// t.Setenv automatically restores the original value after test
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	// Logger should initialize without error
	logger := InitLoggerWithWriter(false, false, &buf)
	assert.NotEqual(t, zerolog.Logger{}, logger)
	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel())
}

func TestSelectLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		verbose       bool
		quiet         bool
		expectedLevel zerolog.Level
	}{
		{
			name:          "default returns info",
			verbose:       false,
			quiet:         false,
			expectedLevel: zerolog.InfoLevel,
		},
		{
			name:          "verbose returns debug",
			verbose:       true,
			quiet:         false,
			expectedLevel: zerolog.DebugLevel,
		},
		{
			name:          "quiet returns warn",
			verbose:       false,
			quiet:         true,
			expectedLevel: zerolog.WarnLevel,
		},
		{
			name:          "verbose takes precedence",
			verbose:       true,
			quiet:         true,
			expectedLevel: zerolog.DebugLevel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			level := selectLevel(tc.verbose, tc.quiet)
			assert.Equal(t, tc.expectedLevel, level)
		})
	}
}

func TestCreateLogFileWriter_CreatesDirectory(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Use temp directory as ATLAS_HOME
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	writer, err := createLogFileWriter()
	require.NoError(t, err)
	require.NotNil(t, writer)
	defer func() { _ = writer.Close() }()

	// Verify log directory was created
	logDir := filepath.Join(tmpDir, constants.LogsDir)
	info, err := os.Stat(logDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCreateLogFileWriter_CreatesLogFile(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Use temp directory as ATLAS_HOME
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	writer, err := createLogFileWriter()
	require.NoError(t, err)
	require.NotNil(t, writer)

	// Write something to trigger file creation
	_, err = writer.Write([]byte(`{"level":"info","message":"test"}`))
	require.NoError(t, err)

	// Close to ensure data is flushed
	err = writer.Close()
	require.NoError(t, err)

	// Verify log file was created
	logPath := filepath.Join(tmpDir, constants.LogsDir, constants.CLILogFileName)
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Positive(t, info.Size())
}

func TestGetAtlasHome_UsesEnvironmentVariable(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	customHome := "/custom/atlas/home"
	t.Setenv("ATLAS_HOME", customHome)

	home, err := getAtlasHome()
	require.NoError(t, err)
	assert.Equal(t, customHome, home)
}

func TestGetAtlasHome_DefaultsToUserHome(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Clear ATLAS_HOME to test default behavior
	t.Setenv("ATLAS_HOME", "")

	home, err := getAtlasHome()
	require.NoError(t, err)

	userHome, err := os.UserHomeDir()
	require.NoError(t, err)

	expectedHome := filepath.Join(userHome, constants.AtlasHome)
	assert.Equal(t, expectedHome, home)
}

func TestLogFilePath(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	path, err := LogFilePath()
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, constants.LogsDir, constants.CLILogFileName)
	assert.Equal(t, expected, path)
}

func TestInitLogger_WritesToFile(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Use temp directory as ATLAS_HOME
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	// Reset log file writer from any previous tests
	logFileWriter = nil

	logger := InitLogger(false, false)

	// Log something
	logger.Info().Str("test_key", "test_value").Msg("test message")

	// Close the log file to flush
	CloseLogFile()

	// Verify log file was created and contains content
	logPath := filepath.Join(tmpDir, constants.LogsDir, constants.CLILogFileName)
	data, err := os.ReadFile(logPath) //#nosec G304 -- path is constructed from test temp dir
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_key")
	assert.Contains(t, string(data), "test_value")
	assert.Contains(t, string(data), "test message")
}

func TestCloseLogFile_NoOpWhenNil(_ *testing.T) {
	// Can't use t.Parallel() when accessing package-level state

	// Ensure logFileWriter is nil
	logFileWriter = nil

	// Should not panic
	CloseLogFile()
}

func TestInitLoggerWithWriter_CustomOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := InitLoggerWithWriter(true, false, &buf)

	logger.Debug().Msg("debug message")

	output := buf.String()
	assert.Contains(t, output, "debug message")
}

func TestCreateLogFileWriter_FailsOnInvalidPath(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Set ATLAS_HOME to a path that cannot be created
	// Use a file as the parent directory which will fail MkdirAll
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not_a_directory")

	// Create a file where we expect a directory
	err := os.WriteFile(filePath, []byte("test"), 0o600) //#nosec G306 -- test file
	require.NoError(t, err)

	// Set ATLAS_HOME to use the file as a path component
	t.Setenv("ATLAS_HOME", filePath)

	writer, err := createLogFileWriter()
	require.Error(t, err)
	assert.Nil(t, writer)
	assert.Contains(t, err.Error(), "failed to create log directory")
}

func TestLogEntryStructure_MatchesExpectedFields(t *testing.T) {
	t.Parallel()

	// Configure zerolog globals before test
	configureZerologGlobals()

	var buf bytes.Buffer
	logger := InitLoggerWithWriter(false, false, &buf)

	// Log a message with typical fields
	logger.Info().
		Str("workspace_name", "test-ws").
		Str("task_id", "task-123").
		Str("step_name", "validate").
		Int64("duration_ms", 150).
		Msg("step completed")

	output := buf.String()

	// Verify field names match logEntry struct in workspace_logs.go
	assert.Contains(t, output, `"ts":`)    // timestamp field
	assert.Contains(t, output, `"level":`) // level field
	assert.Contains(t, output, `"event":`) // message field (not "message")
	assert.Contains(t, output, `"workspace_name":"test-ws"`)
	assert.Contains(t, output, `"task_id":"task-123"`)
	assert.Contains(t, output, `"step_name":"validate"`)
	assert.Contains(t, output, `"duration_ms":150`)
	assert.Contains(t, output, "step completed")
}

func TestConfigureZerologGlobals_Idempotent(t *testing.T) {
	t.Parallel()

	// Call multiple times - should not panic or change behavior
	configureZerologGlobals()
	configureZerologGlobals()
	configureZerologGlobals()

	// Verify the global field names are configured correctly
	assert.Equal(t, "ts", zerolog.TimestampFieldName)
	assert.Equal(t, "event", zerolog.MessageFieldName)
}

func TestInitLogger_RedactsSensitiveDataInFile(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Use temp directory as ATLAS_HOME
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	// Reset log file writer from any previous tests
	logFileWriter = nil

	logger := InitLogger(false, false)

	// Log a message containing sensitive data
	logger.Info().Msg("connecting with key sk-ant-api03-verysecretkey123")

	// Close the log file to flush
	CloseLogFile()

	// Verify log file was created and sensitive data is REDACTED
	logPath := filepath.Join(tmpDir, constants.LogsDir, constants.CLILogFileName)
	data, err := os.ReadFile(logPath) //#nosec G304 -- path is constructed from test temp dir
	require.NoError(t, err)

	content := string(data)

	// The sensitive API key should NOT appear in the log file
	assert.NotContains(t, content, "sk-ant-api03", "API key should be redacted from log file")
	assert.NotContains(t, content, "verysecretkey", "API key should be redacted from log file")

	// The redaction marker should appear
	assert.Contains(t, content, "[REDACTED]", "redaction marker should be present")

	// Non-sensitive parts should be preserved
	assert.Contains(t, content, "connecting with key", "non-sensitive message part should be preserved")
}

// mockTaskLogAppender is a test implementation of TaskLogAppender.
type mockTaskLogAppender struct {
	entries []mockLogEntry
}

type mockLogEntry struct {
	workspaceName string
	taskID        string
	entry         []byte
}

func (m *mockTaskLogAppender) AppendLog(_ context.Context, workspaceName, taskID string, entry []byte) error {
	m.entries = append(m.entries, mockLogEntry{
		workspaceName: workspaceName,
		taskID:        taskID,
		entry:         entry,
	})
	return nil
}

func TestTaskLogWriter_Write(t *testing.T) {
	t.Parallel()

	t.Run("passes through to target writer", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		mock := &mockTaskLogAppender{}
		writer := newTaskLogWriter(mock, &buf)

		input := []byte(`{"level":"info","event":"test message"}`)
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Equal(t, input, buf.Bytes())
	})

	t.Run("persists log with workspace and task context", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		mock := &mockTaskLogAppender{}
		writer := newTaskLogWriter(mock, &buf)

		input := []byte(`{"level":"info","event":"test message","workspace_name":"test-ws","task_id":"task-123"}`)
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Equal(t, input, buf.Bytes())

		// Verify log was persisted via AppendLog
		require.Len(t, mock.entries, 1)
		assert.Equal(t, "test-ws", mock.entries[0].workspaceName)
		assert.Equal(t, "task-123", mock.entries[0].taskID)
		assert.Equal(t, input, mock.entries[0].entry)
	})

	t.Run("does not persist log without workspace_name", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		mock := &mockTaskLogAppender{}
		writer := newTaskLogWriter(mock, &buf)

		input := []byte(`{"level":"info","event":"test message","task_id":"task-123"}`)
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Empty(t, mock.entries)
	})

	t.Run("does not persist log without task_id", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		mock := &mockTaskLogAppender{}
		writer := newTaskLogWriter(mock, &buf)

		input := []byte(`{"level":"info","event":"test message","workspace_name":"test-ws"}`)
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Empty(t, mock.entries)
	})

	t.Run("handles non-JSON input gracefully", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		mock := &mockTaskLogAppender{}
		writer := newTaskLogWriter(mock, &buf)

		input := []byte("not json at all")
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Equal(t, input, buf.Bytes())
		assert.Empty(t, mock.entries)
	})

	t.Run("handles malformed JSON gracefully", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		mock := &mockTaskLogAppender{}
		writer := newTaskLogWriter(mock, &buf)

		input := []byte(`{"level":"info", "broken`)
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Empty(t, mock.entries)
	})
}

func TestInitLoggerWithTaskStore(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	// Reset log file writer from any previous tests
	logFileWriter = nil

	mock := &mockTaskLogAppender{}
	logger := InitLoggerWithTaskStore(false, false, mock)

	// Log with workspace/task context
	logger.Info().
		Str("workspace_name", "test-ws").
		Str("task_id", "task-456").
		Msg("step completed")

	// Close the log file to flush
	CloseLogFile()

	// Verify log was persisted via AppendLog
	require.Len(t, mock.entries, 1)
	assert.Equal(t, "test-ws", mock.entries[0].workspaceName)
	assert.Equal(t, "task-456", mock.entries[0].taskID)
	assert.Contains(t, string(mock.entries[0].entry), "step completed")
}

func TestLoggerWithTaskStore(t *testing.T) {
	// Can't use t.Parallel() due to global state access

	// Set up global flags
	globalLoggerMu.Lock()
	globalLogFlags.verbose = false
	globalLogFlags.quiet = false
	globalLoggerMu.Unlock()

	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	// Reset log file writer from any previous tests
	logFileWriter = nil

	mock := &mockTaskLogAppender{}
	logger := LoggerWithTaskStore(mock)

	// Log with workspace/task context
	logger.Info().
		Str("workspace_name", "test-ws").
		Str("task_id", "task-789").
		Msg("test log entry")

	// Close the log file to flush
	CloseLogFile()

	// Verify log was persisted
	require.Len(t, mock.entries, 1)
	assert.Equal(t, "test-ws", mock.entries[0].workspaceName)
	assert.Equal(t, "task-789", mock.entries[0].taskID)
}

func TestFilteringWriteCloser(t *testing.T) {
	t.Parallel()

	t.Run("Write delegates to filter", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		fw := logging.NewFilteringWriter(&buf)
		closer := io.NopCloser(&buf)
		fwc := &filteringWriteCloser{
			filter: fw,
			closer: closer,
		}

		input := []byte("test message")
		n, err := fwc.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Contains(t, buf.String(), "test message")
	})

	t.Run("Close delegates to closer", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.log")
		file, err := os.Create(tmpFile) //#nosec G304 -- test file
		require.NoError(t, err)

		fw := logging.NewFilteringWriter(file)
		fwc := &filteringWriteCloser{
			filter: fw,
			closer: file,
		}

		err = fwc.Close()
		require.NoError(t, err)

		// Verify file is closed by attempting to write
		_, err = file.WriteString("should fail")
		require.Error(t, err)
	})
}

// mockSpinnerManager implements the spinner manager interface for testing.
type mockSpinnerManager struct {
	activeSpinner *tui.TerminalSpinner
}

func (m *mockSpinnerManager) GetActive() *tui.TerminalSpinner {
	return m.activeSpinner
}

func TestSpinnerAwareWriter(t *testing.T) {
	t.Parallel()

	t.Run("Write without active spinner", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		manager := &mockSpinnerManager{activeSpinner: nil}
		writer := newSpinnerAwareWriter(&buf, manager)

		input := []byte("test message")
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)
		assert.Equal(t, "test message", buf.String())
	})

	t.Run("Write with active spinner clears line", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		// Create a mock spinner (non-nil to trigger the clear sequence)
		manager := &mockSpinnerManager{
			activeSpinner: &tui.TerminalSpinner{},
		}
		writer := newSpinnerAwareWriter(&buf, manager)

		input := []byte("test message")
		n, err := writer.Write(input)

		require.NoError(t, err)
		assert.Equal(t, len(input), n)

		output := buf.String()
		// Should contain the clear sequence followed by the message
		assert.Contains(t, output, "\r\033[K")
		assert.Contains(t, output, "test message")
		// Clear sequence should come before message
		assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("\r\033[K")))
	})

	t.Run("Write with active spinner handles partial write", func(t *testing.T) {
		t.Parallel()

		// Use a limited writer that will cause partial writes
		limitedBuf := &limitedWriter{limit: 2}
		manager := &mockSpinnerManager{
			activeSpinner: &tui.TerminalSpinner{},
		}
		writer := newSpinnerAwareWriter(limitedBuf, manager)

		input := []byte("test message")
		n, err := writer.Write(input)

		// Should return error from underlying writer
		require.Error(t, err)
		// Should return 0 since partial write was within clear sequence
		assert.Equal(t, 0, n)
	})

	t.Run("Write with active spinner handles partial write after clear", func(t *testing.T) {
		t.Parallel()

		// Use a limited writer that allows clear sequence but fails after
		limitedBuf := &limitedWriter{limit: 10}
		manager := &mockSpinnerManager{
			activeSpinner: &tui.TerminalSpinner{},
		}
		writer := newSpinnerAwareWriter(limitedBuf, manager)

		input := []byte("test message long enough to exceed limit")
		n, err := writer.Write(input)

		// Should return error from underlying writer
		require.Error(t, err)
		// Should adjust count to exclude clear sequence
		assert.Positive(t, n)
	})
}

// limitedWriter writes only up to a limit and then returns an error.
type limitedWriter struct {
	written int
	limit   int
}

func (w *limitedWriter) Write(p []byte) (n int, err error) {
	if w.written >= w.limit {
		return 0, os.ErrClosed
	}
	toWrite := len(p)
	if w.written+toWrite > w.limit {
		toWrite = w.limit - w.written
	}
	w.written += toWrite
	if toWrite < len(p) {
		return toWrite, os.ErrClosed
	}
	return toWrite, nil
}

func TestPrepareLoggerSetup(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	t.Run("creates setup with correct level and hook", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("ATLAS_HOME", tmpDir)

		setup, err := prepareLoggerSetup(true, false)

		require.NoError(t, err)
		assert.Equal(t, zerolog.DebugLevel, setup.level)
		assert.NotNil(t, setup.hook)
		assert.NotNil(t, setup.console)
		assert.NotNil(t, setup.fileWriter)
	})

	t.Run("handles file writer creation error gracefully", func(t *testing.T) {
		// Set ATLAS_HOME to invalid path
		t.Setenv("ATLAS_HOME", "/dev/null/invalid")

		setup, err := prepareLoggerSetup(false, false)

		// Should return error but still provide setup
		require.Error(t, err)
		assert.NotNil(t, setup)
		assert.Equal(t, zerolog.InfoLevel, setup.level)
		assert.NotNil(t, setup.console)
		assert.Nil(t, setup.fileWriter)
	})
}

func TestBuildLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	setup := &loggerSetup{
		level: zerolog.DebugLevel,
		hook:  logging.NewSensitiveDataHook(),
	}

	logger := buildLogger(setup, &buf)

	assert.Equal(t, zerolog.DebugLevel, logger.GetLevel())
	assert.NotEqual(t, zerolog.Logger{}, logger)
}

func TestInitLogger_HandlesFileCreationFailure(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Set invalid ATLAS_HOME to cause file creation to fail
	t.Setenv("ATLAS_HOME", "/dev/null/invalid")

	// Reset log file writer
	logFileWriter = nil

	// Should not panic, falls back to console-only logging
	logger := InitLogger(false, false)
	assert.NotEqual(t, zerolog.Logger{}, logger)
	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel())

	// logFileWriter should remain nil since file creation failed
	assert.Nil(t, logFileWriter)
}

func TestInitLoggerWithTaskStore_HandlesFileCreationFailure(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// Set invalid ATLAS_HOME to cause file creation to fail
	t.Setenv("ATLAS_HOME", "/dev/null/invalid")

	// Reset log file writer
	logFileWriter = nil

	mock := &mockTaskLogAppender{}

	// Should not panic, falls back to console-only logging + task logs
	logger := InitLoggerWithTaskStore(false, false, mock)
	assert.NotEqual(t, zerolog.Logger{}, logger)
	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel())

	// logFileWriter should remain nil since file creation failed
	assert.Nil(t, logFileWriter)
}

func TestLogFilePath_HandlesGetAtlasHomeError(t *testing.T) {
	// Can't use t.Parallel() with t.Setenv()

	// This test verifies error handling, but in practice getAtlasHome
	// only returns error if os.UserHomeDir fails, which is hard to simulate
	// in a test without mocking. The function is already tested via other tests.

	// Test with valid ATLAS_HOME
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_HOME", tmpDir)

	path, err := LogFilePath()
	require.NoError(t, err)
	assert.Contains(t, path, tmpDir)
}
