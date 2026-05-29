// Package lifecycle provides shared lifecycle utilities for daemon and direct Atlas modes.
package lifecycle

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	cache "github.com/mrz1836/go-cache"
)

// WorktreeLockRedisKey returns the Redis lock key for a given worktree path.
// Format: <prefix>lock:worktree:<first-16-bytes-of-sha256-hex>
func WorktreeLockRedisKey(prefix, worktreePath string) string {
	h := sha256.Sum256([]byte(worktreePath))
	return fmt.Sprintf("%slock:worktree:%x", prefix, h[:16])
}

// FilesystemLockPath returns the path of the filesystem lock file within worktreePath.
// Lock file location: <worktreePath>/.atlas/lock
func FilesystemLockPath(worktreePath string) string {
	return filepath.Join(worktreePath, ".atlas", "lock")
}

// IsFilesystemLocked returns true if a live process holds the filesystem lock
// at the given worktree path. Returns false if the lock file is absent or the
// owning process is dead (stale lock).
func IsFilesystemLocked(worktreePath string) bool {
	lockPath := FilesystemLockPath(worktreePath)
	data, err := os.ReadFile(lockPath) //nolint:gosec // lockPath is constructed from validated worktree paths
	if err != nil {
		return false
	}
	// File format: "<pid>\n<ownerID>\n"
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) == 0 {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// signal(0) tests process existence without sending a real signal.
	if sigErr := proc.Signal(syscall.Signal(0)); sigErr != nil {
		return false // process dead — stale lock
	}
	return true
}

// WriteFilesystemLock writes a filesystem lock for the current process at the given
// worktree path. Creates the .atlas subdirectory if it does not exist.
func WriteFilesystemLock(worktreePath, ownerID string) error {
	lockDir := filepath.Join(worktreePath, ".atlas")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return fmt.Errorf("create lock dir %q: %w", lockDir, err)
	}
	content := fmt.Sprintf("%d\n%s\n", os.Getpid(), ownerID)
	return os.WriteFile(FilesystemLockPath(worktreePath), []byte(content), 0o600) //nolint:gosec // content is PID + ownerID
}

// RemoveFilesystemLock removes the filesystem lock. Errors are silently ignored
// because lock removal is always best-effort cleanup.
func RemoveFilesystemLock(worktreePath string) {
	_ = os.Remove(FilesystemLockPath(worktreePath))
}

// AcquireWorktreeRedisLock attempts to acquire a Redis-backed worktree exclusivity lock.
// Returns (true, nil) if the lock was acquired, (false, nil) if already held by another owner.
// ttl is the lock TTL in seconds; ownerID identifies the lock holder (e.g., daemon task ID).
func AcquireWorktreeRedisLock(ctx context.Context, client *cache.Client, prefix, worktreePath, ownerID string, ttl int64) (bool, error) {
	key := WorktreeLockRedisKey(prefix, worktreePath)
	locked, err := cache.WriteLock(ctx, client, key, ownerID, ttl)
	if err != nil {
		// ErrLockMismatch means the key is held by a different owner — treat as "not acquired".
		if errors.Is(err, cache.ErrLockMismatch) {
			return false, nil
		}
		return false, err
	}
	return locked, nil
}

// ReleaseWorktreeRedisLock releases a Redis-backed worktree lock held by ownerID.
// Errors are returned but should be treated as best-effort — the TTL ensures
// eventual expiry even if release fails.
func ReleaseWorktreeRedisLock(ctx context.Context, client *cache.Client, prefix, worktreePath, ownerID string) error {
	key := WorktreeLockRedisKey(prefix, worktreePath)
	_, err := cache.ReleaseLock(ctx, client, key, ownerID)
	return err
}
