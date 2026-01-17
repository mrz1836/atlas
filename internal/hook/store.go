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
func (fl *fileLock) LockWithTimeout(timeout time.Duration) error {
	var err error
	fl.file, err = os.OpenFile(fl.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	interval := 50 * time.Millisecond

	for {
		err = flock.Exclusive(fl.file.Fd())
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			_ = fl.file.Close()
			return fmt.Errorf("%w after %v", atlaserrors.ErrLockTimedOut, timeout)
		}

		time.Sleep(interval)
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
	// Returns ErrHookNotFound if hook does not exist.
	Get(ctx context.Context, taskID string) (*domain.Hook, error)

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
func (fs *FileStore) Create(ctx context.Context, taskID, workspaceID string) (*domain.Hook, error) {
	hookPath := fs.hookPath(taskID)

	// Check if hook already exists
	if _, err := os.Stat(hookPath); err == nil {
		return nil, ErrHookExists
	}

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

	// Save the new hook
	if err := fs.Save(ctx, hook); err != nil {
		return nil, fmt.Errorf("failed to save new hook: %w", err)
	}

	return hook, nil
}

// Get retrieves a hook by task ID.
func (fs *FileStore) Get(_ context.Context, taskID string) (*domain.Hook, error) {
	hookPath := fs.hookPath(taskID)

	// Read file with lock
	data, err := fs.readWithLock(hookPath)
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

// Save persists the hook state atomically.
//
// Note: This does NOT take a lock on read, so it's susceptible to overwriting concurrent changes
// if not used carefully. Prefer Update for read-modify-write cycles.
//
//nolint:godox // Note explains intentional behavior
func (fs *FileStore) Save(_ context.Context, hook *domain.Hook) error {
	hookPath := fs.hookPath(hook.TaskID)

	// Ensure directory exists
	dir := filepath.Dir(hookPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Update timestamp
	hook.UpdatedAt = time.Now().UTC()

	// Marshal to JSON
	data, err := json.MarshalIndent(hook, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hook: %w", err)
	}

	// Write atomically with lock
	if err := fs.writeWithLock(hookPath, data); err != nil {
		return fmt.Errorf("failed to write hook file: %w", err)
	}

	// Regenerate HOOK.md if generator is available
	if fs.markdownGenerator != nil {
		mdPath := fs.markdownPath(hook.TaskID)
		mdContent, genErr := fs.markdownGenerator.Generate(hook)
		if genErr == nil {
			// Only attempt write if generation succeeded
			_ = fs.atomicWrite(mdPath, mdContent) // Ignore error - markdown is non-critical
		}
		// Intentionally ignore errors - markdown generation is non-critical for hook persistence
	}

	return nil
}

// Update performs an atomic read-modify-write operation.
func (fs *FileStore) Update(_ context.Context, taskID string, modifier func(*domain.Hook) error) error {
	hookPath := fs.hookPath(taskID)

	// Acquire lock for the duration of read-modify-write
	lock := newFileLock(hookPath + ".lock")
	if err := lock.LockWithTimeout(fs.lockTimeout); err != nil {
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

// readWithLock reads a file with a lock.
func (fs *FileStore) readWithLock(path string) ([]byte, error) {
	lock := newFileLock(path + ".lock")
	if err := lock.LockWithTimeout(fs.lockTimeout); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	return os.ReadFile(path) //nolint:gosec // path is validated by caller through hookPath/markdownPath methods
}

// writeWithLock writes a file atomically with an exclusive lock.
func (fs *FileStore) writeWithLock(path string, data []byte) error {
	lock := newFileLock(path + ".lock")
	if err := lock.LockWithTimeout(fs.lockTimeout); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	return fs.atomicWrite(path, data)
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
