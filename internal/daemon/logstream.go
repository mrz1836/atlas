package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cache "github.com/mrz1836/go-cache"
)

// logKeyPrefix is the key prefix for task log streams: atlas:log:{taskID}.
const logKeyPrefix = "log:"

// LogEntry represents a single structured log line for a task.
type LogEntry struct {
	ID        string    `json:"id,omitempty"` // Redis stream ID (set on read, not write)
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // debug, info, warn, error
	Message   string    `json:"message"`
	Step      string    `json:"step,omitempty"`   // e.g., "analyze", "validate"
	Source    string    `json:"source,omitempty"` // runner, agent, validation
}

// LogWriter writes structured log entries to a Redis Stream for a task.
type LogWriter struct {
	client *cache.Client
	prefix string // key prefix (e.g., "atlas:")
	maxLen int64  // max stream entries per task (MAXLEN cap)
}

// NewLogWriter creates a LogWriter using the given cache client, key prefix and max length.
func NewLogWriter(client *cache.Client, prefix string, maxLen int64) *LogWriter {
	if maxLen <= 0 {
		maxLen = 10000
	}
	return &LogWriter{
		client: client,
		prefix: prefix,
		maxLen: maxLen,
	}
}

// Write appends a log entry to the stream for taskID.
// The entry's Timestamp is set to now if zero. The stream is capped at maxLen.
func (w *LogWriter) Write(ctx context.Context, taskID string, entry LogEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("logwriter: marshal entry: %w", err)
	}
	key := w.streamKey(taskID)
	fields := map[string]string{
		"data": string(payload),
	}
	if _, err = cache.StreamAddCapped(ctx, w.client, key, w.maxLen, fields); err != nil {
		return fmt.Errorf("logwriter: stream add: %w", err)
	}
	return nil
}

// streamKey returns the Redis Stream key for the given task ID.
func (w *LogWriter) streamKey(taskID string) string {
	return w.prefix + logKeyPrefix + taskID
}

// LogReader reads log entries from a Redis Stream with optional blocking support.
type LogReader struct {
	client *cache.Client
	prefix string
}

// NewLogReader creates a LogReader using the given cache client and key prefix.
func NewLogReader(client *cache.Client, prefix string) *LogReader {
	return &LogReader{
		client: client,
		prefix: prefix,
	}
}

// Read returns up to count log entries starting from startID (non-blocking).
// Use "0" as startID to read from the beginning of the stream.
func (r *LogReader) Read(ctx context.Context, taskID, startID string, count int64) ([]LogEntry, error) {
	if startID == "" {
		startID = "0"
	}
	key := r.streamKey(taskID)
	entries, err := cache.StreamRead(ctx, r.client, key, startID, count)
	if err != nil {
		return nil, fmt.Errorf("logreader: stream read: %w", err)
	}
	return parseEntries(entries)
}

// Tail blocks for up to blockMs milliseconds waiting for new entries after lastID.
// Use "$" as lastID to receive only entries added after the call.
// Use "0" to read from the beginning with blocking.
func (r *LogReader) Tail(ctx context.Context, taskID, lastID string, count, blockMs int64) ([]LogEntry, error) {
	if lastID == "" {
		lastID = "$"
	}
	key := r.streamKey(taskID)
	entries, err := cache.StreamReadBlock(ctx, r.client, key, lastID, count, blockMs)
	if err != nil {
		return nil, fmt.Errorf("logreader: stream read block: %w", err)
	}
	return parseEntries(entries)
}

// streamKey returns the Redis Stream key for the given task ID.
func (r *LogReader) streamKey(taskID string) string {
	return r.prefix + logKeyPrefix + taskID
}

// parseEntries converts cache.StreamEntry slice into LogEntry slice.
// Each stream entry holds a "data" field containing a JSON-encoded LogEntry.
func parseEntries(raw []cache.StreamEntry) ([]LogEntry, error) {
	entries := make([]LogEntry, 0, len(raw))
	for _, r := range raw {
		data, ok := r.Fields["data"]
		if !ok {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			return nil, fmt.Errorf("logreader: unmarshal entry %s: %w", r.ID, err)
		}
		entry.ID = r.ID
		entries = append(entries, entry)
	}
	return entries, nil
}
