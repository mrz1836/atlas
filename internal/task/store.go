// Package task provides task persistence and execution for ATLAS.
// This package implements the storage layer for task state files,
// with atomic writes and file locking for data integrity.
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// LockTimeout is the maximum duration to wait for acquiring a file lock.
const LockTimeout = 5 * time.Second

// Directory and file permission constants.
const (
	dirPerm  = 0o750 // Secure directory permissions
	filePerm = 0o600 // Secure file permissions
)

// validTaskIDRegex matches valid task IDs (task-YYYYMMDD-HHMMSS with optional ms suffix).
var validTaskIDRegex = regexp.MustCompile(`^task-\d{8}-\d{6}(-\d{3})?$`)

// Store defines the interface for task persistence operations.
type Store interface {
	// Create creates a new task in the workspace.
	// Returns error if task already exists.
	Create(ctx context.Context, workspaceName string, task *domain.Task) error

	// Get retrieves a task by ID from the workspace.
	// Returns ErrTaskNotFound if task doesn't exist.
	Get(ctx context.Context, workspaceName, taskID string) (*domain.Task, error)

	// Update saves the current task state (atomic write).
	// Returns error if task doesn't exist.
	Update(ctx context.Context, workspaceName string, task *domain.Task) error

	// List returns all tasks for a workspace, sorted by creation time (newest first).
	List(ctx context.Context, workspaceName string) ([]*domain.Task, error)

	// Delete removes a task and all its artifacts.
	Delete(ctx context.Context, workspaceName, taskID string) error

	// AppendLog appends a log entry to the task's log file (JSON-lines format).
	AppendLog(ctx context.Context, workspaceName, taskID string, entry []byte) error

	// SaveArtifact saves an artifact file for the task.
	SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error

	// SaveVersionedArtifact saves an artifact with version suffix (e.g., validation.1.json).
	// Returns the actual filename used.
	SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)

	// GetArtifact retrieves an artifact file.
	GetArtifact(ctx context.Context, workspaceName, taskID, filename string) ([]byte, error)

	// ListArtifacts lists all artifact files for a task.
	ListArtifacts(ctx context.Context, workspaceName, taskID string) ([]string, error)
}

// FileStore implements Store using the local filesystem.
type FileStore struct {
	atlasHome string // Usually ~/.atlas
}

// NewFileStore creates a new FileStore with the given atlas home directory.
// If atlasHome is empty, uses the default ~/.atlas directory.
func NewFileStore(atlasHome string) (*FileStore, error) {
	if atlasHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		atlasHome = filepath.Join(home, constants.AtlasHome)
	}
	return &FileStore{atlasHome: atlasHome}, nil
}

// Create creates a new task in the workspace.
func (s *FileStore) Create(ctx context.Context, workspaceName string, task *domain.Task) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return fmt.Errorf("failed to create task: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if task == nil {
		return fmt.Errorf("failed to create task: task %w", atlaserrors.ErrEmptyValue)
	}
	if task.ID == "" {
		return fmt.Errorf("failed to create task: task ID %w", atlaserrors.ErrEmptyValue)
	}

	taskDir := s.taskDir(workspaceName, task.ID)

	// Check if task already exists
	if _, err := os.Stat(taskDir); err == nil {
		return fmt.Errorf("failed to create task '%s': %w", task.ID, atlaserrors.ErrTaskExists)
	}

	// Create task directory
	if err := os.MkdirAll(taskDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create task directory: %w", err)
	}

	// Set schema version before saving
	task.SchemaVersion = constants.TaskSchemaVersion

	// Acquire lock for write operation
	lockFile, err := s.acquireLock(ctx, workspaceName, task.ID)
	if err != nil {
		// Clean up directory on lock failure
		_ = os.RemoveAll(taskDir)
		return fmt.Errorf("failed to create task '%s': %w", task.ID, err)
	}
	defer func() { _ = s.releaseLock(lockFile) }()

	// Marshal task to JSON
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		_ = os.RemoveAll(taskDir)
		return fmt.Errorf("failed to create task '%s': %w", task.ID, err)
	}

	// Write task file atomically
	taskFile := s.taskFilePath(workspaceName, task.ID)
	if err := atomicWrite(taskFile, data); err != nil {
		_ = os.RemoveAll(taskDir)
		return fmt.Errorf("failed to create task '%s': %w", task.ID, err)
	}

	return nil
}

// Get retrieves a task by ID from the workspace.
func (s *FileStore) Get(ctx context.Context, workspaceName, taskID string) (*domain.Task, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return nil, fmt.Errorf("failed to get task: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return nil, fmt.Errorf("failed to get task: task ID %w", atlaserrors.ErrEmptyValue)
	}

	taskDir := s.taskDir(workspaceName, taskID)

	// Check if task directory exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to get task '%s': %w", taskID, atlaserrors.ErrTaskNotFound)
	}

	// Acquire lock for read operation
	lockFile, err := s.acquireLock(ctx, workspaceName, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task '%s': %w", taskID, err)
	}
	defer func() { _ = s.releaseLock(lockFile) }()

	// Read task file
	taskFile := s.taskFilePath(workspaceName, taskID)
	data, err := os.ReadFile(taskFile) //#nosec G304 -- path is validated and constructed from trusted base
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to get task '%s': %w", taskID, atlaserrors.ErrTaskNotFound)
		}
		return nil, fmt.Errorf("failed to read task '%s': %w", taskID, err)
	}

	// Parse JSON
	var task domain.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to parse task '%s': corrupted state file: %w", taskID, err)
	}

	return &task, nil
}

// Update saves the current task state (atomic write).
func (s *FileStore) Update(ctx context.Context, workspaceName string, task *domain.Task) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return fmt.Errorf("failed to update task: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if task == nil {
		return fmt.Errorf("failed to update task: task %w", atlaserrors.ErrEmptyValue)
	}
	if task.ID == "" {
		return fmt.Errorf("failed to update task: task ID %w", atlaserrors.ErrEmptyValue)
	}

	taskDir := s.taskDir(workspaceName, task.ID)

	// Check if task exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return fmt.Errorf("failed to update task '%s': %w", task.ID, atlaserrors.ErrTaskNotFound)
	}

	// Acquire lock for write operation
	lockFile, err := s.acquireLock(ctx, workspaceName, task.ID)
	if err != nil {
		return fmt.Errorf("failed to update task '%s': %w", task.ID, err)
	}
	defer func() { _ = s.releaseLock(lockFile) }()

	// Update timestamp
	task.UpdatedAt = time.Now().UTC()

	// Marshal task to JSON
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to update task '%s': %w", task.ID, err)
	}

	// Write task file atomically
	taskFile := s.taskFilePath(workspaceName, task.ID)
	if err := atomicWrite(taskFile, data); err != nil {
		return fmt.Errorf("failed to update task '%s': %w", task.ID, err)
	}

	return nil
}

// List returns all tasks for a workspace, sorted by creation time (newest first).
func (s *FileStore) List(ctx context.Context, workspaceName string) ([]*domain.Task, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return nil, fmt.Errorf("failed to list tasks: workspace name %w", atlaserrors.ErrEmptyValue)
	}

	tasksDir := s.tasksDir(workspaceName)

	// Return empty slice if tasks directory doesn't exist
	if _, err := os.Stat(tasksDir); os.IsNotExist(err) {
		return []*domain.Task{}, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	tasks := make([]*domain.Task, 0, len(entries))

	for _, entry := range entries {
		// Skip non-directories
		if !entry.IsDir() {
			continue
		}

		// Skip invalid task IDs
		if !validTaskIDRegex.MatchString(entry.Name()) {
			continue
		}

		// Check for cancellation during iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Try to read task
		task, err := s.Get(ctx, workspaceName, entry.Name())
		if err != nil {
			// Skip directories without valid task.json (log warning in production)
			continue
		}

		tasks = append(tasks, task)
	}

	// Sort by creation time (newest first)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	return tasks, nil
}

// Delete removes a task and all its artifacts.
func (s *FileStore) Delete(ctx context.Context, workspaceName, taskID string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return fmt.Errorf("failed to delete task: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return fmt.Errorf("failed to delete task: task ID %w", atlaserrors.ErrEmptyValue)
	}

	taskDir := s.taskDir(workspaceName, taskID)

	// Check if task exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return fmt.Errorf("failed to delete task '%s': %w", taskID, atlaserrors.ErrTaskNotFound)
	}

	// Acquire lock to prevent concurrent access during deletion
	lockFile, err := s.acquireLock(ctx, workspaceName, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task '%s': %w", taskID, err)
	}
	// Release lock before removal since lock file is inside task directory
	_ = s.releaseLock(lockFile)

	// Remove entire task directory
	if err := os.RemoveAll(taskDir); err != nil {
		return fmt.Errorf("failed to delete task '%s': %w", taskID, err)
	}

	return nil
}

// AppendLog appends a log entry to the task's log file (JSON-lines format).
func (s *FileStore) AppendLog(ctx context.Context, workspaceName, taskID string, entry []byte) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return fmt.Errorf("failed to append log: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return fmt.Errorf("failed to append log: task ID %w", atlaserrors.ErrEmptyValue)
	}

	taskDir := s.taskDir(workspaceName, taskID)

	// Check if task exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return fmt.Errorf("failed to append log: task '%s' %w", taskID, atlaserrors.ErrTaskNotFound)
	}

	// Acquire lock to prevent concurrent log writes
	lockFile, err := s.acquireLock(ctx, workspaceName, taskID)
	if err != nil {
		return fmt.Errorf("failed to append log: %w", err)
	}
	defer func() { _ = s.releaseLock(lockFile) }()

	logPath := s.logFilePath(workspaceName, taskID)

	// Open file for append (create if not exists)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm) //#nosec G304 -- path is constructed internally
	if err != nil {
		return fmt.Errorf("failed to append log: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Ensure entry ends with newline for JSON-lines format
	if len(entry) > 0 && entry[len(entry)-1] != '\n' {
		entry = append(entry, '\n')
	}

	// Write log entry
	if _, err := f.Write(entry); err != nil {
		return fmt.Errorf("failed to append log: %w", err)
	}

	// Sync to disk
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync log: %w", err)
	}

	return nil
}

// SaveArtifact saves an artifact file for the task.
func (s *FileStore) SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return fmt.Errorf("failed to save artifact: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return fmt.Errorf("failed to save artifact: task ID %w", atlaserrors.ErrEmptyValue)
	}
	if filename == "" {
		return fmt.Errorf("failed to save artifact: filename %w", atlaserrors.ErrEmptyValue)
	}

	// Prevent path traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("failed to save artifact: %w", atlaserrors.ErrPathTraversal)
	}

	taskDir := s.taskDir(workspaceName, taskID)

	// Check if task exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return fmt.Errorf("failed to save artifact: task '%s' %w", taskID, atlaserrors.ErrTaskNotFound)
	}

	// Ensure artifacts directory exists
	artifactDir := s.artifactsDir(workspaceName, taskID)
	if err := os.MkdirAll(artifactDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	// Write artifact file atomically to prevent partial writes on crash
	artifactPath := filepath.Join(artifactDir, filename)
	if err := atomicWrite(artifactPath, data); err != nil {
		return fmt.Errorf("failed to save artifact '%s': %w", filename, err)
	}

	return nil
}

// SaveVersionedArtifact saves an artifact with automatic version numbering.
// For example, if "validation.json" exists, saves as "validation.1.json",
// then "validation.2.json", etc.
func (s *FileStore) SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return "", fmt.Errorf("failed to save versioned artifact: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return "", fmt.Errorf("failed to save versioned artifact: task ID %w", atlaserrors.ErrEmptyValue)
	}
	if baseName == "" {
		return "", fmt.Errorf("failed to save versioned artifact: base name %w", atlaserrors.ErrEmptyValue)
	}

	// Prevent path traversal
	if strings.Contains(baseName, "..") || strings.Contains(baseName, "/") || strings.Contains(baseName, "\\") {
		return "", fmt.Errorf("failed to save versioned artifact: %w", atlaserrors.ErrPathTraversal)
	}

	taskDir := s.taskDir(workspaceName, taskID)

	// Check if task exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return "", fmt.Errorf("failed to save versioned artifact: task '%s' %w", taskID, atlaserrors.ErrTaskNotFound)
	}

	// Ensure artifacts directory exists
	artifactDir := s.artifactsDir(workspaceName, taskID)
	if err := os.MkdirAll(artifactDir, dirPerm); err != nil {
		return "", fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	// Find next version number
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	version := 1
	for {
		filename := fmt.Sprintf("%s.%d%s", nameWithoutExt, version, ext)
		fullPath := filepath.Join(artifactDir, filename)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// This version doesn't exist, use it
			// Write atomically to prevent partial writes on crash
			if err := atomicWrite(fullPath, data); err != nil {
				return "", fmt.Errorf("failed to save versioned artifact: %w", err)
			}
			return filename, nil
		}
		version++

		// Safety limit to prevent infinite loop
		if version > 10000 {
			return "", fmt.Errorf("failed to save versioned artifact: %w", atlaserrors.ErrTooManyVersions)
		}
	}
}

// GetArtifact retrieves an artifact file.
func (s *FileStore) GetArtifact(ctx context.Context, workspaceName, taskID, filename string) ([]byte, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return nil, fmt.Errorf("failed to get artifact: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return nil, fmt.Errorf("failed to get artifact: task ID %w", atlaserrors.ErrEmptyValue)
	}
	if filename == "" {
		return nil, fmt.Errorf("failed to get artifact: filename %w", atlaserrors.ErrEmptyValue)
	}

	// Prevent path traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return nil, fmt.Errorf("failed to get artifact: %w", atlaserrors.ErrPathTraversal)
	}

	artifactPath := filepath.Join(s.artifactsDir(workspaceName, taskID), filename)

	data, err := os.ReadFile(artifactPath) //#nosec G304 -- path is validated and constructed from trusted base
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("artifact '%s': %w", filename, atlaserrors.ErrArtifactNotFound)
		}
		return nil, fmt.Errorf("failed to read artifact '%s': %w", filename, err)
	}

	return data, nil
}

// ListArtifacts lists all artifact files for a task.
func (s *FileStore) ListArtifacts(ctx context.Context, workspaceName, taskID string) ([]string, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if workspaceName == "" {
		return nil, fmt.Errorf("failed to list artifacts: workspace name %w", atlaserrors.ErrEmptyValue)
	}
	if taskID == "" {
		return nil, fmt.Errorf("failed to list artifacts: task ID %w", atlaserrors.ErrEmptyValue)
	}

	artifactDir := s.artifactsDir(workspaceName, taskID)

	// Return empty slice if artifacts directory doesn't exist
	if _, err := os.Stat(artifactDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}

	filenames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			filenames = append(filenames, entry.Name())
		}
	}

	// Sort for consistent ordering
	sort.Strings(filenames)

	return filenames, nil
}

// Helper methods for path construction

// tasksDir returns the path to a workspace's tasks directory.
func (s *FileStore) tasksDir(workspaceName string) string {
	return filepath.Join(
		s.atlasHome,
		constants.WorkspacesDir,
		workspaceName,
		constants.TasksDir,
	)
}

// taskDir returns the path to a specific task's directory.
func (s *FileStore) taskDir(workspaceName, taskID string) string {
	return filepath.Join(s.tasksDir(workspaceName), taskID)
}

// taskFilePath returns the path to a task's JSON file.
func (s *FileStore) taskFilePath(workspaceName, taskID string) string {
	return filepath.Join(s.taskDir(workspaceName, taskID), constants.TaskFileName)
}

// logFilePath returns the path to a task's log file.
func (s *FileStore) logFilePath(workspaceName, taskID string) string {
	return filepath.Join(s.taskDir(workspaceName, taskID), constants.TaskLogFileName)
}

// artifactsDir returns the path to a task's artifacts directory.
func (s *FileStore) artifactsDir(workspaceName, taskID string) string {
	return filepath.Join(s.taskDir(workspaceName, taskID), constants.ArtifactsDir)
}

// lockFilePath returns the path to a task's lock file.
func (s *FileStore) lockFilePath(workspaceName, taskID string) string {
	return filepath.Join(s.taskDir(workspaceName, taskID), constants.TaskFileName+".lock")
}

// acquireLock acquires an exclusive file lock for the task.
// It respects context cancellation during the lock acquisition retry loop.
func (s *FileStore) acquireLock(ctx context.Context, workspaceName, taskID string) (*os.File, error) {
	lockPath := s.lockFilePath(workspaceName, taskID)

	// Ensure task directory exists for lock file
	taskDir := s.taskDir(workspaceName, taskID)
	if err := os.MkdirAll(taskDir, dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, filePerm) //#nosec G302,G304 -- lock file needs write access, path is constructed from validated name
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire lock with timeout
	deadline := time.Now().Add(LockTimeout)
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		default:
		}

		// LOCK_EX = exclusive lock, LOCK_NB = non-blocking
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}

		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("failed to acquire lock: %w", atlaserrors.ErrLockTimeout)
		}

		// Wait a bit before retrying
		time.Sleep(50 * time.Millisecond)
	}
}

// releaseLock releases a file lock.
func (s *FileStore) releaseLock(f *os.File) error {
	if f == nil {
		return nil
	}

	// Release the lock
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		// Still try to close the file
		_ = f.Close()
		return fmt.Errorf("failed to release lock: %w", err)
	}

	return f.Close()
}

// atomicWrite writes data to a file atomically using write-then-rename.
// Uses filePerm (0o600) for secure file permissions.
func atomicWrite(path string, data []byte) error {
	// Write to temp file
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm) //#nosec G304 -- path is constructed internally
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write data
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Sync to disk (ensure data is persisted before rename)
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	// Close file before rename
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// GenerateTaskID generates a task ID with format task-YYYYMMDD-HHMMSS.
// IDs generated within the same second will be identical.
// Use GenerateTaskIDUnique for scenarios requiring uniqueness checks.
func GenerateTaskID() string {
	now := time.Now().UTC()
	return fmt.Sprintf("task-%s-%s",
		now.Format("20060102"),
		now.Format("150405"))
}

// GenerateTaskIDUnique generates a task ID, adding milliseconds if needed for uniqueness.
// It checks against the provided map of existing IDs.
//
// IMPORTANT: This function provides best-effort uniqueness based on a snapshot of IDs.
// It does NOT guarantee uniqueness in concurrent scenarios. The recommended pattern is:
//
//	id := GenerateTaskIDUnique(existingIDs)
//	err := store.Create(ctx, workspaceName, task)
//	if errors.Is(err, ErrTaskExists) {
//	    // Regenerate and retry
//	}
//
// The Create method handles the actual uniqueness guarantee via filesystem checks.
func GenerateTaskIDUnique(existingIDs map[string]bool) string {
	id := GenerateTaskID()
	if !existingIDs[id] {
		return id
	}
	// Add milliseconds for uniqueness
	now := time.Now().UTC()
	return fmt.Sprintf("task-%s-%s-%03d",
		now.Format("20060102"),
		now.Format("150405"),
		now.Nanosecond()/1000000)
}
