package hook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/flock"
)

// fileLock wraps a file descriptor for locking operations.
type fileLock struct {
	path string
	file *os.File
}

// New creates a new fileLock for the given path.
func newFileLock(path string) *fileLock {
	return &fileLock{path: path}
}

// LockWithTimeout acquires an exclusive lock with timeout using retry.
// Deprecated: Use LockWithContext for context-aware cancellation support.
func (fl *fileLock) LockWithTimeout(timeout time.Duration) error {
	return fl.LockWithContext(context.Background(), timeout)
}

// LockWithContext acquires an exclusive lock with timeout and context cancellation support.
// The lock acquisition can be interrupted by canceling the context, which is important
// for responsive shutdown handling.
func (fl *fileLock) LockWithContext(ctx context.Context, timeout time.Duration) error {
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

// Unlock releases the lock and closes the file.
func (fl *fileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}
	_ = flock.Unlock(fl.file.Fd())
	return fl.file.Close()
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

// NewFileStore creates a new FileStore.
func NewFileStore(basePath string, opts ...FileStoreOption) *FileStore {
	fs := &FileStore{
		basePath:    basePath,
		lockTimeout: 5 * time.Second,
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
	lock := newFileLock(hookPath + ".lock")
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

	// Generate markdown if available
	if fs.markdownGenerator != nil {
		mdPath := fs.markdownPath(taskID)
		mdContent, genErr := fs.markdownGenerator.Generate(hook)
		if genErr == nil {
			_ = fs.atomicWrite(mdPath, mdContent)
		}
	}

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
	return hook.DeepCopy(), nil
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
	lock := newFileLock(hookPath + ".lock")
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

	// Regenerate HOOK.md inside lock for consistency
	if fs.markdownGenerator != nil {
		mdPath := fs.markdownPath(hook.TaskID)
		mdContent, genErr := fs.markdownGenerator.Generate(hook)
		if genErr == nil {
			_ = fs.atomicWrite(mdPath, mdContent) // Ignore error - markdown is non-critical
		}
	}

	return nil
}

// Update performs an atomic read-modify-write operation.
func (fs *FileStore) Update(ctx context.Context, taskID string, modifier func(*domain.Hook) error) error {
	hookPath := fs.hookPath(taskID)

	// Acquire lock for the duration of read-modify-write
	lock := newFileLock(hookPath + ".lock")
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

	// Regenerate HOOK.md if generator is available
	//nolint:godox // Note explains intentional behavior
	// Note: We do this inside the lock to ensure consistency, though it might slow down slightly
	if fs.markdownGenerator != nil {
		mdPath := fs.markdownPath(hook.TaskID)
		mdContent, genErr := fs.markdownGenerator.Generate(&hook)
		if genErr == nil {
			_ = fs.atomicWrite(mdPath, mdContent)
		}
	}

	return nil
}

// Delete removes the hook files.
func (fs *FileStore) Delete(_ context.Context, taskID string) error {
	hookPath := fs.hookPath(taskID)
	mdPath := fs.markdownPath(taskID)

	// Remove hook.json
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete hook file: %w", err)
	}

	// Remove HOOK.md
	if err := os.Remove(mdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete markdown file: %w", err)
	}

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
func (fs *FileStore) ListStale(_ context.Context, threshold time.Duration) ([]*domain.Hook, error) {
	var staleHooks []*domain.Hook

	// Walk through all hook files
	err := filepath.WalkDir(fs.basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only process hook.json files
		if d.IsDir() || d.Name() != constants.HookFileName {
			return nil
		}

		// Read the hook
		data, readErr := os.ReadFile(path) //nolint:gosec // path is from filepath.WalkDir, safe within basePath
		if readErr != nil {
			return nil //nolint:nilerr // Skip unreadable files in WalkDir, continue processing others
		}

		var hook domain.Hook
		if unmarshalErr := json.Unmarshal(data, &hook); unmarshalErr != nil {
			return nil //nolint:nilerr // Skip invalid hook files in WalkDir, continue processing others
		}

		// Check if stale and not terminal
		if !domain.IsTerminalState(hook.State) && time.Since(hook.UpdatedAt) > threshold {
			staleHooks = append(staleHooks, &hook)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list stale hooks: %w", err)
	}

	return staleHooks, nil
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
	lock := newFileLock(path + ".lock")
	if err := lock.LockWithContext(ctx, fs.lockTimeout); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	return os.ReadFile(path) //nolint:gosec // path is validated by caller through hookPath/markdownPath methods
}

// atomicWrite writes data to a file atomically using temp file + rename.
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
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// MarkdownGenerator generates HOOK.md content from hook state.
type MarkdownGenerator interface {
	// Generate creates the HOOK.md content from hook state.
	Generate(hook *domain.Hook) ([]byte, error)
}
