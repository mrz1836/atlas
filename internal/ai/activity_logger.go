package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ErrLoggerClosed is returned when attempting to log to a closed logger.
var ErrLoggerClosed = errors.New("logger is closed")

// ActivityLogEntry represents a single entry in the activity log.
type ActivityLogEntry struct {
	Timestamp time.Time    `json:"timestamp"`
	Type      ActivityType `json:"type"`
	Message   string       `json:"message"`
	File      string       `json:"file,omitempty"`
	Phase     string       `json:"phase,omitempty"`
}

// ActivityLogger writes activity events to a JSONL file for later analysis.
type ActivityLogger struct {
	logDir      string
	taskID      string
	file        *os.File
	encoder     *json.Encoder
	mu          sync.Mutex
	eventCount  int
	maxLogFiles int
}

// ActivityLoggerConfig configures the activity logger.
type ActivityLoggerConfig struct {
	// LogDir is the directory where logs are stored.
	// Defaults to ~/.atlas/logs
	LogDir string

	// TaskID is used to name the log file.
	TaskID string

	// MaxLogFiles is the maximum number of log files to keep.
	// Defaults to 50.
	MaxLogFiles int
}

// DefaultActivityLoggerConfig returns the default configuration.
func DefaultActivityLoggerConfig() ActivityLoggerConfig {
	homeDir, _ := os.UserHomeDir()
	return ActivityLoggerConfig{
		LogDir:      filepath.Join(homeDir, ".atlas", "logs"),
		MaxLogFiles: 50,
	}
}

// NewActivityLogger creates a new ActivityLogger.
// If config.LogDir is empty, it uses the default log directory.
func NewActivityLogger(config ActivityLoggerConfig) (*ActivityLogger, error) {
	if config.LogDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		config.LogDir = filepath.Join(homeDir, ".atlas", "logs")
	}

	if config.MaxLogFiles <= 0 {
		config.MaxLogFiles = 50
	}

	// Ensure log directory exists with secure permissions
	if err := os.MkdirAll(config.LogDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Determine log file name based on task ID or timestamp
	var fileName string
	if config.TaskID != "" {
		fileName = fmt.Sprintf("activity-%s.jsonl", config.TaskID)
	} else {
		fileName = fmt.Sprintf("activity-%s.jsonl", time.Now().Format("20060102-150405"))
	}

	logPath := filepath.Join(config.LogDir, fileName)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // Path is constructed from validated config
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger := &ActivityLogger{
		logDir:      config.LogDir,
		taskID:      config.TaskID,
		file:        file,
		encoder:     json.NewEncoder(file),
		maxLogFiles: config.MaxLogFiles,
	}

	// Clean up old log files
	logger.cleanupOldLogs()

	return logger, nil
}

// Log writes an activity event to the log file.
// Thread-safe for concurrent writes.
func (l *ActivityLogger) Log(event ActivityEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return ErrLoggerClosed
	}

	// Convert ActivityEvent to ActivityLogEntry (same underlying structure)
	entry := ActivityLogEntry(event)

	if err := l.encoder.Encode(entry); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	l.eventCount++
	return nil
}

// Close closes the log file.
func (l *ActivityLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	err := l.file.Close()
	l.file = nil
	return err
}

// EventCount returns the number of events logged.
func (l *ActivityLogger) EventCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.eventCount
}

// LogPath returns the path to the current log file.
func (l *ActivityLogger) LogPath() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return ""
	}
	return l.file.Name()
}

// CreateCallback returns an ActivityCallback that logs to this logger.
// This is a convenience method for easy integration.
func (l *ActivityLogger) CreateCallback() ActivityCallback {
	return func(event ActivityEvent) {
		_ = l.Log(event) // Silently ignore errors
	}
}

// cleanupOldLogs removes old log files if there are more than maxLogFiles.
func (l *ActivityLogger) cleanupOldLogs() {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return // Silently ignore errors during cleanup
	}

	// Filter for activity log files
	var logFiles []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			if len(entry.Name()) > 9 && entry.Name()[:9] == "activity-" {
				logFiles = append(logFiles, entry)
			}
		}
	}

	// If under the limit, nothing to do
	if len(logFiles) <= l.maxLogFiles {
		return
	}

	// Sort by modification time (oldest first)
	sort.Slice(logFiles, func(i, j int) bool {
		infoI, errI := logFiles[i].Info()
		infoJ, errJ := logFiles[j].Info()
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Remove oldest files
	toRemove := len(logFiles) - l.maxLogFiles
	for i := 0; i < toRemove; i++ {
		path := filepath.Join(l.logDir, logFiles[i].Name())
		_ = os.Remove(path) // Silently ignore errors
	}
}

// CombineCallbacks combines multiple ActivityCallbacks into one.
// Useful for sending events to both the UI and the logger.
func CombineCallbacks(callbacks ...ActivityCallback) ActivityCallback {
	return func(event ActivityEvent) {
		for _, cb := range callbacks {
			if cb != nil {
				cb(event)
			}
		}
	}
}
