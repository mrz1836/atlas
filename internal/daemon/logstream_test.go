package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLogWriter tests that NewLogWriter initializes with correct defaults.
func TestNewLogWriter(t *testing.T) {
	t.Parallel()

	w := NewLogWriter(nil, "atlas:", 5000)
	assert.NotNil(t, w)
	assert.Equal(t, "atlas:", w.prefix)
	assert.Equal(t, int64(5000), w.maxLen)
}

// TestNewLogWriter_DefaultsMaxLen tests that maxLen<=0 is clamped to 10000.
func TestNewLogWriter_DefaultsMaxLen(t *testing.T) {
	t.Parallel()

	w := NewLogWriter(nil, "atlas:", 0)
	assert.Equal(t, int64(10000), w.maxLen)

	w2 := NewLogWriter(nil, "atlas:", -1)
	assert.Equal(t, int64(10000), w2.maxLen)
}

// TestLogWriter_StreamKey verifies the key format.
func TestLogWriter_StreamKey(t *testing.T) {
	t.Parallel()

	w := NewLogWriter(nil, "atlas:", 1000)
	assert.Equal(t, "atlas:log:task-123", w.streamKey("task-123"))
}

// TestNewLogReader tests that NewLogReader initializes with correct fields.
func TestNewLogReader(t *testing.T) {
	t.Parallel()

	r := NewLogReader(nil, "atlas:")
	assert.NotNil(t, r)
	assert.Equal(t, "atlas:", r.prefix)
}

// TestLogReader_StreamKey verifies the key format.
func TestLogReader_StreamKey(t *testing.T) {
	t.Parallel()

	r := NewLogReader(nil, "atlas:")
	assert.Equal(t, "atlas:log:task-456", r.streamKey("task-456"))
}

// TestLogEntry_Marshal verifies that LogEntry round-trips through JSON correctly.
func TestLogEntry_Marshal(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	entry := LogEntry{
		ID:        "1-0",
		Timestamp: ts,
		Level:     "info",
		Message:   "task started",
		Step:      "analyze",
		Source:    "runner",
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var parsed LogEntry
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, entry.ID, parsed.ID)
	assert.Equal(t, entry.Level, parsed.Level)
	assert.Equal(t, entry.Message, parsed.Message)
	assert.Equal(t, entry.Step, parsed.Step)
	assert.Equal(t, entry.Source, parsed.Source)
	assert.True(t, entry.Timestamp.Equal(parsed.Timestamp), "timestamps should match")
}

// TestLogWriter_WithRedis tests write+read round-trip using a real Redis test connection.
func TestLogWriter_WithRedis(t *testing.T) {
	t.Parallel()

	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	prefix := "test:logstream:"
	w := NewLogWriter(d.redis, prefix, 100)
	r := NewLogReader(d.redis, prefix)

	taskID := "roundtrip-task"

	entry := LogEntry{
		Level:   "info",
		Message: "hello from logstream test",
		Step:    "setup",
		Source:  "test",
	}
	err := w.Write(context.Background(), taskID, entry)
	require.NoError(t, err)

	entries, err := r.Read(context.Background(), taskID, "0", 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.Equal(t, "info", got.Level)
	assert.Equal(t, "hello from logstream test", got.Message)
	assert.Equal(t, "setup", got.Step)
	assert.Equal(t, "test", got.Source)
	assert.NotEmpty(t, got.ID, "ID should be set from stream entry")
	assert.False(t, got.Timestamp.IsZero(), "Timestamp should be set on write")
}

// TestLogWriter_WithRedis_MultipleEntries verifies ordering is preserved.
func TestLogWriter_WithRedis_MultipleEntries(t *testing.T) {
	t.Parallel()

	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	prefix := "test:logstream:multi:"
	w := NewLogWriter(d.redis, prefix, 100)
	r := NewLogReader(d.redis, prefix)

	taskID := "multi-task"
	messages := []string{"first", "second", "third"}
	for _, msg := range messages {
		err := w.Write(context.Background(), taskID, LogEntry{Level: "debug", Message: msg, Source: "test"})
		require.NoError(t, err)
	}

	entries, err := r.Read(context.Background(), taskID, "0", 10)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	for i, e := range entries {
		assert.Equal(t, messages[i], e.Message, "entry %d should match", i)
	}
}
