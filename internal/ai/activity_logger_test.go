package ai

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewActivityLogger(t *testing.T) {
	t.Parallel()

	// Create temp directory
	tmpDir := t.TempDir()

	logger, err := NewActivityLogger(ActivityLoggerConfig{
		LogDir:      tmpDir,
		TaskID:      "test-task-123",
		MaxLogFiles: 10,
	})
	if err != nil {
		t.Fatalf("NewActivityLogger failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Errorf("Failed to close logger: %v", closeErr)
		}
	})

	// Verify log file was created
	logPath := logger.LogPath()
	if logPath == "" {
		t.Error("LogPath() returned empty string")
	}

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logPath)
	}

	// Verify filename contains task ID
	if filepath.Base(logPath) != "activity-test-task-123.jsonl" {
		t.Errorf("Log filename = %s, want activity-test-task-123.jsonl", filepath.Base(logPath))
	}
}

//nolint:gocognit // Test function complexity is acceptable
func TestActivityLogger_Log(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	logger, err := NewActivityLogger(ActivityLoggerConfig{
		LogDir: tmpDir,
		TaskID: "log-test",
	})
	if err != nil {
		t.Fatalf("NewActivityLogger failed: %v", err)
	}

	// Log some events
	events := []ActivityEvent{
		{
			Timestamp: time.Now(),
			Type:      ActivityReading,
			Message:   "Reading test file",
			File:      "test.go",
		},
		{
			Timestamp: time.Now(),
			Type:      ActivityWriting,
			Message:   "Writing output",
			Phase:     "implementation",
		},
		{
			Timestamp: time.Now(),
			Type:      ActivityThinking,
			Message:   "Thinking...",
		},
	}

	for _, e := range events {
		if logErr := logger.Log(e); logErr != nil {
			t.Errorf("Log failed: %v", logErr)
		}
	}

	// Verify event count
	if count := logger.EventCount(); count != len(events) {
		t.Errorf("EventCount() = %d, want %d", count, len(events))
	}

	// Close and read back
	logPath := logger.LogPath()
	if closeErr := logger.Close(); closeErr != nil {
		t.Errorf("Failed to close logger: %v", closeErr)
	}

	// Read and verify log entries
	file, err := os.Open(logPath) //nolint:gosec // Test file path is controlled
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Errorf("Failed to close file: %v", closeErr)
		}
	})

	scanner := bufio.NewScanner(file)
	i := 0
	for scanner.Scan() {
		var entry ActivityLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Errorf("Failed to parse log entry %d: %v", i, err)
			continue
		}

		if entry.Type != events[i].Type {
			t.Errorf("Entry %d type = %s, want %s", i, entry.Type, events[i].Type)
		}
		if entry.Message != events[i].Message {
			t.Errorf("Entry %d message = %q, want %q", i, entry.Message, events[i].Message)
		}
		if entry.File != events[i].File {
			t.Errorf("Entry %d file = %q, want %q", i, entry.File, events[i].File)
		}
		if entry.Phase != events[i].Phase {
			t.Errorf("Entry %d phase = %q, want %q", i, entry.Phase, events[i].Phase)
		}

		i++
	}

	if i != len(events) {
		t.Errorf("Read %d entries, want %d", i, len(events))
	}
}

func TestActivityLogger_Close(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	logger, err := NewActivityLogger(ActivityLoggerConfig{
		LogDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewActivityLogger failed: %v", err)
	}

	// Close should succeed
	if err := logger.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Close again should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}

	// Logging after close should fail
	if err := logger.Log(ActivityEvent{Type: ActivityReading}); err == nil {
		t.Error("Log after Close should fail")
	}

	// LogPath should return empty after close
	if path := logger.LogPath(); path != "" {
		t.Errorf("LogPath after Close = %q, want empty", path)
	}
}

func TestActivityLogger_CreateCallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	logger, err := NewActivityLogger(ActivityLoggerConfig{
		LogDir: tmpDir,
		TaskID: "callback-test",
	})
	if err != nil {
		t.Fatalf("NewActivityLogger failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Errorf("Failed to close logger: %v", closeErr)
		}
	})

	callback := logger.CreateCallback()

	// Use the callback
	callback(ActivityEvent{
		Timestamp: time.Now(),
		Type:      ActivityReading,
		Message:   "Via callback",
	})

	if logger.EventCount() != 1 {
		t.Errorf("EventCount() = %d, want 1", logger.EventCount())
	}
}

func TestActivityLogger_CleanupOldLogs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create some old log files
	for i := 0; i < 5; i++ {
		fileName := filepath.Join(tmpDir, "activity-old"+string(rune('0'+i))+".jsonl")
		if err := os.WriteFile(fileName, []byte("test"), 0o600); err != nil {
			t.Fatalf("Failed to create old log file: %v", err)
		}
		// Add a small delay to ensure different modification times
		time.Sleep(10 * time.Millisecond)
	}

	// Create logger with max 3 files - this should trigger cleanup
	logger, err := NewActivityLogger(ActivityLoggerConfig{
		LogDir:      tmpDir,
		TaskID:      "new-log",
		MaxLogFiles: 3,
	})
	if err != nil {
		t.Fatalf("NewActivityLogger failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Errorf("Failed to close logger: %v", closeErr)
		}
	})

	// Count remaining log files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	logCount := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".jsonl" {
			logCount++
		}
	}

	// Should have at most maxLogFiles + 1 (the new one)
	if logCount > 4 { // 3 old + 1 new
		t.Errorf("Found %d log files, want <= 4", logCount)
	}
}

func TestCombineCallbacks(t *testing.T) {
	t.Parallel()

	var calls1, calls2 int

	cb1 := func(_ ActivityEvent) {
		calls1++
	}
	cb2 := func(_ ActivityEvent) {
		calls2++
	}

	combined := CombineCallbacks(cb1, cb2)

	// Call the combined callback
	combined(ActivityEvent{Type: ActivityReading})

	if calls1 != 1 {
		t.Errorf("cb1 called %d times, want 1", calls1)
	}
	if calls2 != 1 {
		t.Errorf("cb2 called %d times, want 1", calls2)
	}

	// Call again
	combined(ActivityEvent{Type: ActivityWriting})

	if calls1 != 2 {
		t.Errorf("cb1 called %d times, want 2", calls1)
	}
	if calls2 != 2 {
		t.Errorf("cb2 called %d times, want 2", calls2)
	}
}

func TestCombineCallbacks_WithNil(t *testing.T) {
	t.Parallel()

	var calls int

	cb := func(_ ActivityEvent) {
		calls++
	}

	// Combine with nil should not panic
	combined := CombineCallbacks(cb, nil, cb)
	combined(ActivityEvent{Type: ActivityReading})

	if calls != 2 {
		t.Errorf("Callback called %d times, want 2", calls)
	}
}

func TestDefaultActivityLoggerConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultActivityLoggerConfig()

	if cfg.MaxLogFiles != 50 {
		t.Errorf("MaxLogFiles = %d, want 50", cfg.MaxLogFiles)
	}

	if cfg.LogDir == "" {
		t.Error("LogDir should not be empty")
	}

	// Should contain .atlas/logs
	if !filepath.IsAbs(cfg.LogDir) {
		t.Error("LogDir should be an absolute path")
	}
}
