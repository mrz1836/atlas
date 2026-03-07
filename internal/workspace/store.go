// Package workspace provides workspace persistence and management for ATLAS.
// This package implements the storage layer for workspace state files,
// with atomic writes and file locking for data integrity.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/flock"
)

// CurrentSchemaVersion is the current version of the workspace schema.
// This enables forward-compatible schema migrations.
const CurrentSchemaVersion = 1

// validNameRegex matches valid workspace names (alphanumeric, dash, underscore).
var validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Store defines the interface for workspace persistence operations.
type Store interface {
	// Create persists a new workspace. Returns ErrWorkspaceExists if workspace already exists.
	Create(ctx context.Context, ws *domain.Workspace) error

	// Get retrieves a workspace by name. Returns ErrWorkspaceNotFound if not found.
	Get(ctx context.Context, name string) (*domain.Workspace, error)

	// Update persists changes to an existing workspace. Returns ErrWorkspaceNotFound if not found.
	Update(ctx context.Context, ws *domain.Workspace) error

	// List returns all workspaces. Returns empty slice if none exist.
	List(ctx context.Context) ([]*domain.Workspace, error)

	// Delete removes a workspace and its data. Returns ErrWorkspaceNotFound if not found.
	Delete(ctx context.Context, name string) error

	// ResetMetadata removes only the workspace metadata file, preserving task history.
	// Returns ErrWorkspaceNotFound if not found.
	ResetMetadata(ctx context.Context, name string) error

	// Exists returns true if a workspace with the given name exists.
	Exists(ctx context.Context, name string) (bool, error)
}

// FileStore implements Store using the local filesystem.
type FileStore struct {
	baseDir string // Usually ~/.atlas
}

// NewFileStore creates a new FileStore with the given base directory.
// If baseDir is empty, uses the default ~/.atlas directory.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		baseDir = filepath.Join(home, constants.AtlasHome)
	}
	return &FileStore{baseDir: baseDir}, nil
}

// RepoHash computes a deterministic short hash of a repository path.
// It resolves symlinks, computes SHA-256, and returns the first 12 hex characters.
func RepoHash(repoPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repo path: %w", err)
	}
	h := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(h[:])[:12], nil
}

// NewRepoScopedFileStore creates a FileStore scoped to a specific repository.
// Storage path: ~/.atlas/repos/{repo-hash}/
// This prevents workspace name collisions across different repositories.
func NewRepoScopedFileStore(repoPath string) (*FileStore, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repo path cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	hash, err := RepoHash(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute repo hash: %w", err)
	}
	baseDir := filepath.Join(home, constants.AtlasHome, constants.ReposDir, hash)
	return &FileStore{baseDir: baseDir}, nil
}

// Create persists a new workspace.
func (s *FileStore) Create(ctx context.Context, ws *domain.Workspace) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate workspace name
	if err := validateName(ws.Name); err != nil {
		return fmt.Errorf("failed to create workspace '%s': %w", ws.Name, err)
	}

	wsPath := s.workspacePath(ws.Name)
	wsFile := s.workspaceFilePath(ws.Name)

	// Check if workspace.json already exists (directory may exist with preserved tasks)
	if _, err := os.Stat(wsFile); err == nil {
		return fmt.Errorf("failed to create workspace '%s': %w", ws.Name, atlaserrors.ErrWorkspaceExists)
	}

	// Create workspace directory (may already exist if recreating closed workspace)
	if err := os.MkdirAll(wsPath, constants.WorkspaceDirPerm); err != nil {
		return fmt.Errorf("failed to create workspace directory '%s': %w", ws.Name, err)
	}

	// Set schema version before saving
	ws.SchemaVersion = CurrentSchemaVersion

	// Set path field
	ws.Path = wsPath

	// Acquire lock for write operation
	lockFile, err := s.acquireLock(ctx, ws.Name)
	if err != nil {
		// Clean up directory on lock failure
		_ = os.RemoveAll(wsPath)
		return fmt.Errorf("failed to create workspace '%s': %w", ws.Name, err)
	}
	defer func() { _ = s.releaseLock(lockFile) }()

	// Marshal workspace to JSON
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		_ = os.RemoveAll(wsPath)
		return fmt.Errorf("failed to create workspace '%s': %w", ws.Name, err)
	}

	// Write workspace file atomically
	if err := atomicWrite(wsFile, data, constants.WorkspaceFilePerm); err != nil {
		_ = os.RemoveAll(wsPath)
		return fmt.Errorf("failed to create workspace '%s': %w", ws.Name, err)
	}

	return nil
}

// Get retrieves a workspace by name.
func (s *FileStore) Get(ctx context.Context, name string) (*domain.Workspace, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate name
	if err := validateName(name); err != nil {
		return nil, fmt.Errorf("failed to read workspace '%s': %w", name, err)
	}

	ws, err := s.getFromDir(ctx, name, s.workspacePath(name))
	if err == nil {
		return ws, nil
	}

	// Legacy fallback: if not found in repo-scoped path, check legacy ~/.atlas/workspaces/
	if isRepoScoped(s.baseDir) {
		legacyBase, legacyErr := legacyBaseDir()
		if legacyErr == nil {
			legacyPath := filepath.Join(legacyBase, constants.WorkspacesDir, name)
			if legacyWs, legacyGetErr := s.getFromDir(ctx, name, legacyPath); legacyGetErr == nil {
				return legacyWs, nil
			}
		}
	}

	return nil, err
}

// Update persists changes to an existing workspace.
func (s *FileStore) Update(ctx context.Context, ws *domain.Workspace) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate workspace name
	if err := validateName(ws.Name); err != nil {
		return fmt.Errorf("failed to update workspace '%s': %w", ws.Name, err)
	}

	wsPath := s.workspacePath(ws.Name)

	// Check if workspace exists
	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		return fmt.Errorf("failed to update workspace '%s': %w", ws.Name, atlaserrors.ErrWorkspaceNotFound)
	}

	// Acquire lock for write operation
	lockFile, err := s.acquireLock(ctx, ws.Name)
	if err != nil {
		return fmt.Errorf("failed to update workspace '%s': %w", ws.Name, err)
	}
	defer func() { _ = s.releaseLock(lockFile) }()

	// Update timestamp
	ws.UpdatedAt = time.Now()

	// Marshal workspace to JSON
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to update workspace '%s': %w", ws.Name, err)
	}

	// Write workspace file atomically
	wsFile := s.workspaceFilePath(ws.Name)
	if err := atomicWrite(wsFile, data, constants.WorkspaceFilePerm); err != nil {
		return fmt.Errorf("failed to update workspace '%s': %w", ws.Name, err)
	}

	return nil
}

// List returns all workspaces.
func (s *FileStore) List(ctx context.Context) ([]*domain.Workspace, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	wsDir := s.workspacesDir()

	var entries []os.DirEntry
	if _, err := os.Stat(wsDir); !os.IsNotExist(err) {
		var readErr error
		entries, readErr = os.ReadDir(wsDir)
		if readErr != nil {
			return nil, fmt.Errorf("failed to list workspaces: %w", readErr)
		}
	}

	workspaces := make([]*domain.Workspace, 0, len(entries))
	seen := make(map[string]bool, len(entries))

	for _, entry := range entries {
		// Skip non-directories
		if !entry.IsDir() {
			continue
		}

		// Check for cancellation during iteration
		if err := ctxutil.Canceled(ctx); err != nil {
			return nil, err
		}

		// Try to read workspace
		ws, err := s.Get(ctx, entry.Name())
		if err != nil {
			// Skip directories without valid workspace.json (log warning in production)
			continue
		}

		workspaces = append(workspaces, ws)
		seen[ws.Name] = true
	}

	// Legacy fallback: also list workspaces from legacy ~/.atlas/workspaces/
	if isRepoScoped(s.baseDir) {
		legacyWs, legacyErr := s.listLegacyWorkspaces(ctx, seen)
		if legacyErr != nil {
			return nil, legacyErr
		}
		workspaces = append(workspaces, legacyWs...)
	}

	return workspaces, nil
}

// Delete removes a workspace and its data.
func (s *FileStore) Delete(ctx context.Context, name string) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate name
	if err := validateName(name); err != nil {
		return fmt.Errorf("failed to delete workspace '%s': %w", name, err)
	}

	wsPath := s.workspacePath(name)

	// Check if workspace exists
	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		return fmt.Errorf("failed to delete workspace '%s': %w", name, atlaserrors.ErrWorkspaceNotFound)
	}

	// Remove entire workspace directory
	if err := os.RemoveAll(wsPath); err != nil {
		return fmt.Errorf("failed to delete workspace '%s': %w", name, err)
	}

	return nil
}

// ResetMetadata removes only the workspace metadata file, preserving task history.
// Use this when recreating a closed workspace to allow workspace name reuse while
// keeping historical task data.
func (s *FileStore) ResetMetadata(ctx context.Context, name string) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Validate name
	if err := validateName(name); err != nil {
		return fmt.Errorf("failed to reset workspace '%s': %w", name, err)
	}

	metadataPath := s.workspaceFilePath(name)

	// Check if workspace metadata exists
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return fmt.Errorf("failed to reset workspace '%s': %w", name, atlaserrors.ErrWorkspaceNotFound)
	}

	// Remove only the metadata file, preserving tasks directory
	if err := os.Remove(metadataPath); err != nil {
		return fmt.Errorf("failed to reset workspace '%s': %w", name, err)
	}

	return nil
}

// Exists returns true if a workspace with the given name exists.
func (s *FileStore) Exists(ctx context.Context, name string) (bool, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return false, err
	}

	// Validate name
	if err := validateName(name); err != nil {
		return false, fmt.Errorf("failed to check workspace '%s': %w", name, err)
	}

	wsPath := s.workspacePath(name)
	wsFile := s.workspaceFilePath(name)

	// Check if both directory and workspace.json exist
	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		return false, nil
	}
	if _, err := os.Stat(wsFile); os.IsNotExist(err) {
		return false, nil
	}

	return true, nil
}

// listLegacyWorkspaces lists workspaces from the legacy ~/.atlas/workspaces/ path.
func (s *FileStore) listLegacyWorkspaces(ctx context.Context, seen map[string]bool) ([]*domain.Workspace, error) {
	legacyBase, err := legacyBaseDir()
	if err != nil {
		return nil, nil //nolint:nilerr // legacy fallback is best-effort
	}

	legacyDir := filepath.Join(legacyBase, constants.WorkspacesDir)
	legacyEntries, readErr := os.ReadDir(legacyDir)
	if readErr != nil {
		return nil, nil //nolint:nilerr // legacy dir may not exist
	}

	var workspaces []*domain.Workspace
	for _, entry := range legacyEntries {
		if !entry.IsDir() || seen[entry.Name()] {
			continue
		}
		if err := ctxutil.Canceled(ctx); err != nil {
			return nil, err
		}
		legacyPath := filepath.Join(legacyDir, entry.Name())
		ws, getErr := s.getFromDir(ctx, entry.Name(), legacyPath)
		if getErr != nil {
			continue
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

// workspacesDir returns the path to the workspaces directory.
func (s *FileStore) workspacesDir() string {
	return filepath.Join(s.baseDir, constants.WorkspacesDir)
}

// workspacePath returns the path to a specific workspace directory.
func (s *FileStore) workspacePath(name string) string {
	return filepath.Join(s.workspacesDir(), name)
}

// workspaceFilePath returns the path to a workspace's JSON file.
func (s *FileStore) workspaceFilePath(name string) string {
	return filepath.Join(s.workspacePath(name), constants.WorkspaceFileName)
}

// lockFilePath returns the path to a workspace's lock file.
func (s *FileStore) lockFilePath(name string) string {
	return filepath.Join(s.workspacePath(name), constants.WorkspaceFileName+".lock")
}

// validateName checks if a workspace name is valid.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("workspace name cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	if len(name) > constants.MaxWorkspaceNameLength {
		return fmt.Errorf("workspace name too long (max %d characters): %w", constants.MaxWorkspaceNameLength, atlaserrors.ErrValueOutOfRange)
	}
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("workspace name contains invalid characters (use alphanumeric, dash, underscore): %w", atlaserrors.ErrValueOutOfRange)
	}
	// Check for path traversal attempts
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("workspace name contains invalid path characters: %w", atlaserrors.ErrValueOutOfRange)
	}
	return nil
}

// acquireLock acquires an exclusive file lock for the workspace.
// It respects context cancellation during the lock acquisition retry loop.
func (s *FileStore) acquireLock(ctx context.Context, name string) (*os.File, error) {
	lockPath := s.lockFilePath(name)

	// Ensure workspace directory exists for lock file
	wsPath := s.workspacePath(name)
	if err := os.MkdirAll(wsPath, constants.WorkspaceDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, constants.WorkspaceFilePerm) //#nosec G302,G304 -- lock file needs write access, path is constructed from validated name
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire lock with timeout
	deadline := time.Now().Add(constants.WorkspaceLockTimeout)
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		default:
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

// getFromDir reads a workspace from a specific directory path.
func (s *FileStore) getFromDir(_ context.Context, name, wsPath string) (*domain.Workspace, error) {
	// Check if workspace directory exists
	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read workspace '%s': %w", name, atlaserrors.ErrWorkspaceNotFound)
	}

	// Read workspace file (no lock needed for reads — atomic writes guarantee consistency)
	wsFile := filepath.Join(wsPath, constants.WorkspaceFileName)
	data, err := os.ReadFile(wsFile) //#nosec G304 -- path is validated and constructed from trusted base
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read workspace '%s': %w", name, atlaserrors.ErrWorkspaceNotFound)
		}
		return nil, fmt.Errorf("failed to read workspace '%s': %w", name, err)
	}

	// Parse JSON
	var ws domain.Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("workspace '%s' has corrupted state file: %w. Consider deleting %s/", name, atlaserrors.ErrWorkspaceCorrupted, wsPath)
	}

	return &ws, nil
}

// isRepoScoped returns true if the baseDir is under the repos/ directory structure.
func isRepoScoped(baseDir string) bool {
	return strings.Contains(baseDir, string(filepath.Separator)+constants.ReposDir+string(filepath.Separator))
}

// legacyBaseDir returns the legacy ~/.atlas base directory.
func legacyBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.AtlasHome), nil
}

// atomicWrite writes data to a file atomically using write-then-rename.
//
//nolint:unparam // perm is designed for flexibility, currently only uses constants.WorkspaceFilePerm
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	// Write to temp file
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //#nosec G304 -- path is constructed internally
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
