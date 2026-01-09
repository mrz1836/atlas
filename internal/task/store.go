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
	"time"

	"github.com/google/uuid"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/flock"
)

// LockTimeout is the maximum duration to wait for acquiring a file lock.
const LockTimeout = 5 * time.Second

// Directory and file permission constants.
const (
	dirPerm  = 0o750 // Secure directory permissions
	filePerm = 0o600 // Secure file permissions
)

// validTaskIDRegex matches valid task IDs (task-{uuid}).
// Format: task-[8 hex]-[4 hex]-[4 hex]-[4 hex]-[12 hex]
var validTaskIDRegex = regexp.MustCompile(`^task-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

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

	// ReadLog reads the task's log file.
	ReadLog(ctx context.Context, workspaceName, taskID string) ([]byte, error)

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
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate inputs
	if err := validateWorkspaceName("create task", workspaceName); err != nil {
		return err
	}
	if err := validateTaskWithID("create task", task); err != nil {
		return err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("get task", workspaceName, taskID); err != nil {
		return nil, err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate inputs
	if err := validateWorkspaceName("update task", workspaceName); err != nil {
		return err
	}
	if err := validateTaskWithID("update task", task); err != nil {
		return err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if err := validateWorkspaceName("list tasks", workspaceName); err != nil {
		return nil, err
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

		if err := ctxutil.Canceled(ctx); err != nil {
			return nil, err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("delete task", workspaceName, taskID); err != nil {
		return err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("append log", workspaceName, taskID); err != nil {
		return err
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

// ReadLog reads the task's log file.
func (s *FileStore) ReadLog(ctx context.Context, workspaceName, taskID string) ([]byte, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("read log", workspaceName, taskID); err != nil {
		return nil, err
	}

	logPath := s.logFilePath(workspaceName, taskID)

	data, err := os.ReadFile(logPath) //#nosec G304 -- path is constructed internally
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("log file: %w", atlaserrors.ErrArtifactNotFound)
		}
		return nil, fmt.Errorf("failed to read log: %w", err)
	}

	return data, nil
}

// SaveArtifact saves an artifact file for the task.
func (s *FileStore) SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("save artifact", workspaceName, taskID); err != nil {
		return err
	}
	if err := validateFilename("save artifact", filename); err != nil {
		return err
	}

	// Prevent path traversal - reject absolute paths and ".." sequences
	// Use filepath.Clean to normalize the path and detect traversal attempts
	cleanFilename := filepath.Clean(filename)
	if filepath.IsAbs(cleanFilename) || strings.HasPrefix(cleanFilename, "..") {
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

	// Build artifact path and ensure it stays within artifacts directory
	artifactPath := filepath.Join(artifactDir, cleanFilename)

	// Create subdirectories if needed (e.g., for "sdd/spec.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), dirPerm); err != nil {
		return fmt.Errorf("failed to create artifact subdirectory: %w", err)
	}
	if err := atomicWrite(artifactPath, data); err != nil {
		return fmt.Errorf("failed to save artifact '%s': %w", filename, err)
	}

	return nil
}

// SaveVersionedArtifact saves an artifact with automatic version numbering.
// For example, if "validation.json" exists, saves as "validation.1.json",
// then "validation.2.json", etc.
func (s *FileStore) SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return "", err
	}

	// Validate inputs
	if err := validateVersionedArtifactInputs(workspaceName, taskID, baseName); err != nil {
		return "", err
	}

	// Prevent path traversal - reject absolute paths and ".." sequences
	cleanBaseName := filepath.Clean(baseName)
	if filepath.IsAbs(cleanBaseName) || strings.HasPrefix(cleanBaseName, "..") {
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

	// Find and save the next versioned file
	filename, err := s.saveNextVersionedFile(artifactDir, cleanBaseName, data)
	if err != nil {
		return "", err
	}

	return filename, nil
}

// GetArtifact retrieves an artifact file.
func (s *FileStore) GetArtifact(ctx context.Context, workspaceName, taskID, filename string) ([]byte, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("get artifact", workspaceName, taskID); err != nil {
		return nil, err
	}
	if err := validateFilename("get artifact", filename); err != nil {
		return nil, err
	}

	// Prevent path traversal - reject absolute paths and ".." sequences
	// Use filepath.Clean to normalize the path and detect traversal attempts
	cleanFilename := filepath.Clean(filename)
	if filepath.IsAbs(cleanFilename) || strings.HasPrefix(cleanFilename, "..") {
		return nil, fmt.Errorf("failed to get artifact: %w", atlaserrors.ErrPathTraversal)
	}

	artifactPath := filepath.Join(s.artifactsDir(workspaceName, taskID), cleanFilename)

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
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if err := validateWorkspaceAndTaskID("list artifacts", workspaceName, taskID); err != nil {
		return nil, err
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

// Validation helper functions to reduce code duplication.

// validateWorkspaceName validates that workspace name is not empty.
func validateWorkspaceName(operation, workspaceName string) error {
	if workspaceName == "" {
		return fmt.Errorf("failed to %s: workspace name %w", operation, atlaserrors.ErrEmptyValue)
	}
	return nil
}

// validateTaskID validates that task ID is not empty.
func validateTaskID(operation, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("failed to %s: task ID %w", operation, atlaserrors.ErrEmptyValue)
	}
	return nil
}

// validateTask validates that task pointer is not nil.
func validateTask(operation string, task *domain.Task) error {
	if task == nil {
		return fmt.Errorf("failed to %s: task %w", operation, atlaserrors.ErrEmptyValue)
	}
	return nil
}

// validateTaskWithID validates that task pointer is not nil and has a non-empty ID.
func validateTaskWithID(operation string, task *domain.Task) error {
	if err := validateTask(operation, task); err != nil {
		return err
	}
	if task.ID == "" {
		return fmt.Errorf("failed to %s: task ID %w", operation, atlaserrors.ErrEmptyValue)
	}
	return nil
}

// validateFilename validates that filename is not empty.
func validateFilename(operation, filename string) error {
	if filename == "" {
		return fmt.Errorf("failed to %s: filename %w", operation, atlaserrors.ErrEmptyValue)
	}
	return nil
}

// validateWorkspaceAndTaskID validates both workspace name and task ID.
func validateWorkspaceAndTaskID(operation, workspaceName, taskID string) error {
	if err := validateWorkspaceName(operation, workspaceName); err != nil {
		return err
	}
	return validateTaskID(operation, taskID)
}

// validateVersionedArtifactInputs validates the input parameters for SaveVersionedArtifact.
func validateVersionedArtifactInputs(workspaceName, taskID, baseName string) error {
	if err := validateWorkspaceAndTaskID("save versioned artifact", workspaceName, taskID); err != nil {
		return err
	}
	if baseName == "" {
		return fmt.Errorf("failed to save versioned artifact: base name %w", atlaserrors.ErrEmptyValue)
	}
	return nil
}

// saveNextVersionedFile finds the next available version number and saves the file.
func (s *FileStore) saveNextVersionedFile(artifactDir, cleanBaseName string, data []byte) (string, error) {
	dir := filepath.Dir(cleanBaseName)
	base := filepath.Base(cleanBaseName)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	version := 1
	for {
		versionedName := fmt.Sprintf("%s.%d%s", nameWithoutExt, version, ext)
		filename := s.buildVersionedFilename(dir, versionedName)
		fullPath := filepath.Join(artifactDir, filename)

		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// This version doesn't exist, use it
			if err := s.writeVersionedFile(fullPath, data); err != nil {
				return "", err
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

// buildVersionedFilename constructs the filename with directory preservation.
func (s *FileStore) buildVersionedFilename(dir, versionedName string) string {
	if dir == "." {
		return versionedName
	}
	return filepath.Join(dir, versionedName)
}

// writeVersionedFile writes the versioned file atomically, creating subdirectories if needed.
func (s *FileStore) writeVersionedFile(fullPath string, data []byte) error {
	// Create subdirectories if needed (e.g., for "sdd/spec.md")
	if err := os.MkdirAll(filepath.Dir(fullPath), dirPerm); err != nil {
		return fmt.Errorf("failed to create artifact subdirectory: %w", err)
	}
	// Write atomically to prevent partial writes on crash
	if err := atomicWrite(fullPath, data); err != nil {
		return fmt.Errorf("failed to save versioned artifact: %w", err)
	}
	return nil
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
		if err := ctxutil.Canceled(ctx); err != nil {
			_ = f.Close()
			return nil, err
		}

		// Attempt to acquire exclusive non-blocking lock
		err := flock.Exclusive(f.Fd())
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
	if err := flock.Unlock(f.Fd()); err != nil {
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

// GenerateTaskID generates a globally unique task ID using UUID v4.
// Format: task-{uuid} (e.g., task-a1b2c3d4-e5f6-4789-g0h1-i2j3k4l5m6n7)
// Collision probability is negligible (1 in 2^122).
func GenerateTaskID() string {
	return "task-" + uuid.New().String()
}
