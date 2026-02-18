package hook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/flock"
)

// slowLockThreshold is the duration above which lock acquisition is considered slow.
const slowLockThreshold = 100 * time.Millisecond

// fileLock wraps a file descriptor for locking operations.
type fileLock struct {
	path   string
	file   *os.File
	logger *zerolog.Logger // Optional, for slow lock logging
}

// newFileLock creates a new fileLock for the given path.
// The logger is optional and used for slow lock acquisition warnings.
func newFileLock(path string, logger *zerolog.Logger) *fileLock {
	return &fileLock{path: path, logger: logger}
}

// LockWithContext acquires an exclusive lock with timeout and context cancellation support.
// The lock acquisition can be interrupted by canceling the context, which is important
// for responsive shutdown handling.
func (fl *fileLock) LockWithContext(ctx context.Context, timeout time.Duration) error {
	start := time.Now()
	defer func() {
		if elapsed := time.Since(start); elapsed > slowLockThreshold && fl.logger != nil {
			fl.logger.Warn().
				Dur("elapsed", elapsed).
				Str("path", fl.path).
				Msg("slow lock acquisition")
		}
	}()

	var err error
	fl.file, err = os.OpenFile(fl.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	interval := 50 * time.Millisecond

	for {
		// Check context cancellation first
		select {
		case <-ctx.Done():
			_ = fl.file.Close()
			return ctx.Err()
		default:
		}

		err = flock.Exclusive(fl.file.Fd())
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			_ = fl.file.Close()
			return fmt.Errorf("%w after %v", atlaserrors.ErrLockTimedOut, timeout)
		}

		// Use timer instead of Sleep for context-awareness
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = fl.file.Close()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Unlock releases the lock, closes the file, and cleans up the lock file.
// Lock file removal is best-effort to prevent accumulation of stale lock files.
func (fl *fileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}
	_ = flock.Unlock(fl.file.Fd())
	err := fl.file.Close()

	// Clean up lock file (best-effort, ignore errors)
	// This prevents accumulation of stale .lock files over time.
	// Other processes waiting for the lock will recreate it via O_CREATE.
	_ = os.Remove(fl.path)

	return err
}

// ErrHookNotFound is returned when a hook file does not exist.
var ErrHookNotFound = errors.New("hook not found")

// ErrHookExists is returned when trying to create a hook that already exists.
var ErrHookExists = errors.New("hook already exists")

// ErrInvalidHook is returned when a hook file is corrupted or invalid.
var ErrInvalidHook = errors.New("invalid hook file")

// Store defines the persistence interface for Hook state.
type Store interface {
	// Create initializes a new hook for a task.
	// Returns error if hook already exists.
	Create(ctx context.Context, taskID, workspaceID string) (*domain.Hook, error)

	// Get retrieves a hook by task ID.
	// WARNING: Returns a snapshot that may become stale. Use Update() for
	// read-modify-write operations, or GetSnapshot() for explicit read-only access.
	// Returns ErrHookNotFound if hook does not exist.
	Get(ctx context.Context, taskID string) (*domain.Hook, error)

	// GetSnapshot retrieves a deep copy of the hook for read-only inspection.
	// The returned hook is safe to read but modifications will NOT be persisted.
	// Use this when you need to inspect state without risk of accidental mutation.
	// Returns ErrHookNotFound if hook does not exist.
	GetSnapshot(ctx context.Context, taskID string) (*domain.Hook, error)

	// Save persists the hook state atomically.
	// Also regenerates HOOK.md from the updated state.
	Save(ctx context.Context, hook *domain.Hook) error

	// Delete removes the hook files (hook.json and HOOK.md).
	Delete(ctx context.Context, taskID string) error

	// Exists checks if a hook exists for the given task.
	Exists(ctx context.Context, taskID string) (bool, error)

	// Update performs an atomic read-modify-write operation.
	Update(ctx context.Context, taskID string, modifier func(*domain.Hook) error) error
}

// FileStore implements Store with file-based persistence.
// It stores hook.json alongside task.json in the task directory.
type FileStore struct {
	// basePath is the path to the workspaces directory (~/.atlas/workspaces).
	basePath string

	// markdownGenerator generates HOOK.md content.
	markdownGenerator MarkdownGenerator

	// lockTimeout is the timeout for acquiring file locks.
	lockTimeout time.Duration

	// logger for diagnostic messages (optional).
	logger *zerolog.Logger
}

// FileStoreOption configures a FileStore.
type FileStoreOption func(*FileStore)

// WithMarkdownGenerator sets a custom markdown generator.
func WithMarkdownGenerator(gen MarkdownGenerator) FileStoreOption {
	return func(fs *FileStore) {
		fs.markdownGenerator = gen
	}
}

// WithLockTimeout sets a custom lock timeout.
func WithLockTimeout(timeout time.Duration) FileStoreOption {
	return func(fs *FileStore) {
		fs.lockTimeout = timeout
	}
}

// WithLogger sets a logger for diagnostic messages.
// If not set, markdown generation errors are silently ignored.
func WithLogger(logger *zerolog.Logger) FileStoreOption {
	return func(fs *FileStore) {
		fs.logger = logger
	}
}

// NewFileStore creates a new FileStore.
func NewFileStore(basePath string, opts ...FileStoreOption) *FileStore {
	nopLogger := zerolog.Nop()
	fs := &FileStore{
		basePath:    basePath,
		lockTimeout: 5 * time.Second,
		logger:      &nopLogger, // Default to nop logger, never nil
	}

	for _, opt := range opts {
		opt(fs)
	}

	return fs
}

// Create initializes a new hook for a task.
// The lock is acquired BEFORE the existence check to prevent TOCTOU race conditions
// where multiple goroutines could pass the existence check and overwrite each other.
func (fs *FileStore) Create(ctx context.Context, taskID, workspaceID string) (*domain.Hook, error) {
	hookPath := fs.hookPath(taskID)

	// Ensure directory exists BEFORE acquiring lock (lock file needs the directory)
	dir := filepath.Dir(hookPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// ACQUIRE LOCK FIRST to prevent TOCTOU race condition
	lock := newFileLock(hookPath+".lock", fs.logger)
	if err := lock.LockWithContext(ctx, fs.lockTimeout); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// NOW check existence (under lock) - this is atomic with the write
	if _, err := os.Stat(hookPath); err == nil {
		return nil, ErrHookExists
	}

	// Create the hook (still under lock)
	now := time.Now().UTC()
	hook := &domain.Hook{
		Version:       constants.HookSchemaVersion,
		TaskID:        taskID,
		WorkspaceID:   workspaceID,
		CreatedAt:     now,
		UpdatedAt:     now,
		State:         domain.HookStateInitializing,
		History:       []domain.HookEvent{},
		Checkpoints:   []domain.StepCheckpoint{},
		Receipts:      []domain.ValidationReceipt{},
		SchemaVersion: constants.HookSchemaVersion,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(hook, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hook: %w", err)
	}

	// Write atomically (we already hold the lock)
	if err := fs.atomicWrite(hookPath, data); err != nil {
		return nil, fmt.Errorf("failed to write hook file: %w", err)
	}

	// Generate markdown if available (non-critical, log errors but don't fail)
	fs.writeMarkdown(hook)

	return hook, nil
}

// Get retrieves a hook by task ID.
//
// WARNING: The returned hook is a point-in-time snapshot and becomes stale
// immediately after the lock is released. Any modifications to the returned
// hook will NOT be persisted unless followed by a Save() call, but this
// pattern is dangerous because concurrent modifications may be lost.
//
// For read-modify-write operations, use Update() instead which provides
// atomic guarantees. For read-only inspection where you want to be explicit
// about the snapshot semantics, use GetSnapshot() which returns a deep copy.
func (fs *FileStore) Get(ctx context.Context, taskID string) (*domain.Hook, error) {
	hookPath := fs.hookPath(taskID)

	// Read file with lock
	data, err := fs.readWithLock(ctx, hookPath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return nil, ErrHookNotFound
		}
		return nil, fmt.Errorf("failed to read hook file: %w", err)
	}

	var hook domain.Hook
	if err := json.Unmarshal(data, &hook); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidHook, err)
	}

	return &hook, nil
}

// GetSnapshot retrieves a deep copy of the hook for read-only inspection.
// The returned hook is safe to read but modifications will NOT be persisted.
// This method is preferred over Get() when you need to be explicit about
// the read-only nature of the access and avoid accidental mutations.
func (fs *FileStore) GetSnapshot(ctx context.Context, taskID string) (*domain.Hook, error) {
	hook, err := fs.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	snapshot, err := hook.DeepCopy()
	if err != nil {
		return nil, fmt.Errorf("failed to create hook snapshot: %w", err)
	}
	return snapshot, nil
}

// Save persists the hook state atomically with full locking.
// The lock is acquired before any operations to ensure thread-safety.
func (fs *FileStore) Save(ctx context.Context, hook *domain.Hook) error {
	hookPath := fs.hookPath(hook.TaskID)

	// Ensure directory exists BEFORE acquiring lock (lock file needs the directory)
	dir := filepath.Dir(hookPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Acquire lock BEFORE modifying timestamp to ensure thread-safety
	lock := newFileLock(hookPath+".lock", fs.logger)
	if err := lock.LockWithContext(ctx, fs.lockTimeout); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Now safe to modify timestamp
	hook.UpdatedAt = time.Now().UTC()

	// Marshal to JSON
	data, err := json.MarshalIndent(hook, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hook: %w", err)
	}

	// Use atomicWrite directly (we already hold the lock)
	if err := fs.atomicWrite(hookPath, data); err != nil {
		return fmt.Errorf("failed to write hook file: %w", err)
	}

	// Regenerate HOOK.md inside lock for consistency (non-critical, log errors but don't fail)
	fs.writeMarkdown(hook)

	return nil
}

// Update performs an atomic read-modify-write operation.
func (fs *FileStore) Update(ctx context.Context, taskID string, modifier func(*domain.Hook) error) error {
	hookPath := fs.hookPath(taskID)

	// Acquire lock for the duration of read-modify-write
	lock := newFileLock(hookPath+".lock", fs.logger)
	if err := lock.LockWithContext(ctx, fs.lockTimeout); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Read
	//nolint:gosec // G304: Path is constructed from trusted taskID
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrHookNotFound
		}
		return fmt.Errorf("failed to read hook file: %w", err)
	}

	var hook domain.Hook
	if unmarshalErr := json.Unmarshal(data, &hook); unmarshalErr != nil {
		return fmt.Errorf("%w: %w", ErrInvalidHook, unmarshalErr)
	}

	// Modify
	if modErr := modifier(&hook); modErr != nil {
		return modErr
	}

	// Update timestamp
	hook.UpdatedAt = time.Now().UTC()

	// Write
	updatedData, err := json.MarshalIndent(&hook, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hook: %w", err)
	}

	// Use atomicWrite directly since we already hold the lock
	if err := fs.atomicWrite(hookPath, updatedData); err != nil {
		return fmt.Errorf("failed to write hook file: %w", err)
	}

	// Regenerate HOOK.md if generator is available (non-critical, log errors but don't fail)
	// We do this inside the lock to ensure consistency, though it might slow down slightly
	fs.writeMarkdown(&hook)

	return nil
}

// Delete removes the hook files.
// The lock is acquired to prevent race conditions with concurrent read/write operations.
func (fs *FileStore) Delete(ctx context.Context, taskID string) error {
	hookPath := fs.hookPath(taskID)

	// Check if hook exists before trying to acquire lock.
	// This handles the common case of deleting non-existent hooks without
	// requiring the directory to exist for the lock file.
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		// Hook doesn't exist, nothing to delete - this is success
		return nil
	}

	// Acquire lock to prevent race with concurrent read/write operations
	lock := newFileLock(hookPath+".lock", fs.logger)
	if err := lock.LockWithContext(ctx, fs.lockTimeout); err != nil {
		return fmt.Errorf("failed to acquire lock for delete: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Remove hook.json (under lock)
	// Re-check with IsNotExist in case it was deleted between stat and lock
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete hook file: %w", err)
	}

	// Remove HOOK.md (under lock)
	mdPath := fs.markdownPath(taskID)
	if err := os.Remove(mdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete markdown file: %w", err)
	}

	// Clean up lock file
	_ = os.Remove(hookPath + ".lock")

	return nil
}

// Exists checks if a hook exists for the given task.
func (fs *FileStore) Exists(_ context.Context, taskID string) (bool, error) {
	hookPath := fs.hookPath(taskID)
	_, err := os.Stat(hookPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ListStale returns all hooks that haven't been updated within threshold.
// Uses GetSnapshot for each hook to ensure consistent, locked reads.
func (fs *FileStore) ListStale(ctx context.Context, threshold time.Duration) ([]*domain.Hook, error) {
	var staleHooks []*domain.Hook

	// Check if basePath exists, return empty list if not
	if _, err := os.Stat(fs.basePath); os.IsNotExist(err) {
		return staleHooks, nil
	}

	// Walk through all hook files to discover taskIDs
	err := filepath.WalkDir(fs.basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only process hook.json files
		if d.IsDir() || d.Name() != constants.HookFileName {
			return nil
		}

		// Extract taskID from path for proper locking
		taskID := fs.extractTaskIDFromPath(path)
		if taskID == "" {
			return nil // Skip if we can't determine taskID
		}

		// Use GetSnapshot for locked, consistent read
		hook, getErr := fs.GetSnapshot(ctx, taskID)
		if getErr != nil {
			// Skip unreadable hooks (may have been deleted between walk and read)
			return nil //nolint:nilerr // Continue processing other hooks
		}

		// Check if stale and not terminal
		if !domain.IsTerminalState(hook.State) && time.Since(hook.UpdatedAt) > threshold {
			staleHooks = append(staleHooks, hook)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list stale hooks: %w", err)
	}

	return staleHooks, nil
}

// extractTaskIDFromPath extracts the taskID from a hook.json path.
// Path format: basePath/taskID/hook.json
func (fs *FileStore) extractTaskIDFromPath(hookPath string) string {
	// Remove the hook.json filename to get the task directory
	taskDir := filepath.Dir(hookPath)

	// Get the relative path from basePath
	relPath, err := filepath.Rel(fs.basePath, taskDir)
	if err != nil {
		return ""
	}

	return relPath
}

// hookPath returns the path to hook.json for a task.
func (fs *FileStore) hookPath(taskID string) string {
	// Hook files are stored alongside task files in the task directory
	// Path: basePath/workspaces/<workspace>/tasks/<taskID>/hook.json
	// For simplicity, we assume taskID includes the full path information or
	// we need to search for it. In practice, the task system provides this.
	return filepath.Join(fs.basePath, taskID, constants.HookFileName)
}

// markdownPath returns the path to HOOK.md for a task.
func (fs *FileStore) markdownPath(taskID string) string {
	return filepath.Join(fs.basePath, taskID, constants.HookMarkdownFileName)
}

// readWithLock reads a file with a lock, respecting context cancellation.
func (fs *FileStore) readWithLock(ctx context.Context, path string) ([]byte, error) {
	lock := newFileLock(path+".lock", fs.logger)
	if err := lock.LockWithContext(ctx, fs.lockTimeout); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	return os.ReadFile(path) //nolint:gosec // path is validated by caller through hookPath/markdownPath methods
}

// atomicWrite writes data to a file atomically using temp file + rename.
// After the rename, the parent directory is synced to ensure durability on POSIX systems.
func (fs *FileStore) atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)

	// Create temp file in same directory
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}() // Clean up on failure

	// Write data
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Ensure data is on disk
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil { //nolint:gosec // G703: path is from internal store, not user input
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Sync parent directory to ensure rename is durable on POSIX systems.
	// This ensures the directory metadata (which records the rename) is on disk.
	// Failure here is logged but not fatal - the data is likely safe.
	_ = syncDir(dir)

	return nil
}

// syncDir ensures directory metadata is flushed to disk.
// This is required for durable atomic file operations on POSIX systems,
// as os.Rename() may only update kernel buffer cache without persisting
// the directory entry change to disk.
func syncDir(dirPath string) error {
	d, err := os.Open(dirPath) // #nosec G304 -- dirPath is controlled by the application
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

// writeMarkdown generates and writes HOOK.md for the given hook.
// Errors are logged but don't cause operation failure (markdown is non-critical).
func (fs *FileStore) writeMarkdown(hook *domain.Hook) {
	if fs.markdownGenerator == nil {
		return
	}

	mdPath := fs.markdownPath(hook.TaskID)
	mdContent, genErr := fs.markdownGenerator.Generate(hook)
	if genErr != nil {
		fs.logger.Warn().
			Str("task_id", hook.TaskID).
			Err(genErr).
			Msg("failed to generate HOOK.md")
		return
	}

	if writeErr := fs.atomicWrite(mdPath, mdContent); writeErr != nil {
		fs.logger.Warn().
			Str("task_id", hook.TaskID).
			Err(writeErr).
			Msg("failed to write HOOK.md")
	}
}

// MarkdownGenerator generates HOOK.md content from hook state.
type MarkdownGenerator interface {
	// Generate creates the HOOK.md content from hook state.
	Generate(hook *domain.Hook) ([]byte, error)
}
