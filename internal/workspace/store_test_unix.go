//go:build unix

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestFileStore_LockTimeout tests that ErrLockTimeout is returned when lock cannot be acquired.
func TestFileStore_LockTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lock timeout test in short mode")
	}

	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace first
	ws := &domain.Workspace{
		Name:      "lock-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Manually acquire the lock file to simulate contention
	lockPath := filepath.Join(tmpDir, constants.WorkspacesDir, "lock-test", constants.WorkspaceFileName+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, filePerm) //#nosec G302,G304 -- test lock file
	require.NoError(t, err)
	defer func() { _ = lockFile.Close() }()

	// Acquire exclusive lock (blocking)
	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	// Now try to update the workspace - should timeout
	// Use a shorter timeout context to speed up the test
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ws.Branch = "new-branch"
	err = store.Update(ctx, ws)

	// Should fail - either with context deadline exceeded or lock timeout
	require.Error(t, err)
	// The error could be context.DeadlineExceeded (if context expires first)
	// or ErrLockTimeout (if lock timeout expires first)
	assert.True(t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, atlaserrors.ErrLockTimeout),
		"expected deadline exceeded or lock timeout, got: %v", err)
}

// TestFileStore_ContextCancellationDuringLock tests that context cancellation
// is respected during the lock acquisition retry loop.
func TestFileStore_ContextCancellationDuringLock(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace first
	ws := &domain.Workspace{
		Name:      "ctx-cancel-lock-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Manually acquire the lock to create contention
	lockPath := filepath.Join(tmpDir, constants.WorkspacesDir, "ctx-cancel-lock-test", constants.WorkspaceFileName+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, filePerm) //#nosec G302,G304 -- test lock file
	require.NoError(t, err)
	defer func() { _ = lockFile.Close() }()

	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	// Create a context that will be canceled quickly
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay (before lock timeout would occur)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Try to update - should fail with context.Canceled
	ws.Branch = "should-not-update"
	err = store.Update(ctx, ws)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
