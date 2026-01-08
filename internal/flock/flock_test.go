//go:build unix

package flock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mrz1836/atlas/internal/flock"
)

//nolint:gocognit // Test complexity is acceptable for comprehensive lock testing
func TestExclusiveLock(t *testing.T) {
	t.Parallel()

	t.Run("acquires lock on new file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		lockFile := filepath.Join(tmpDir, "test.lock")

		f, err := os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- test code using safe temp dir
		if err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				t.Errorf("failed to close file: %v", closeErr)
			}
		}()

		err = flock.Exclusive(f.Fd())
		if err != nil {
			t.Errorf("expected to acquire lock, got error: %v", err)
		}

		err = flock.Unlock(f.Fd())
		if err != nil {
			t.Errorf("expected to release lock, got error: %v", err)
		}
	})

	t.Run("fails to acquire lock when already held", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		lockFile := filepath.Join(tmpDir, "test.lock")

		// First process acquires the lock
		f1, err := os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- test code using safe temp dir
		if err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
		defer func() {
			if closeErr := f1.Close(); closeErr != nil {
				t.Errorf("failed to close file: %v", closeErr)
			}
		}()

		err = flock.Exclusive(f1.Fd())
		if err != nil {
			t.Fatalf("first lock acquisition failed: %v", err)
		}
		defer func() {
			if unlockErr := flock.Unlock(f1.Fd()); unlockErr != nil {
				t.Errorf("failed to unlock: %v", unlockErr)
			}
		}()

		// Second attempt should fail (non-blocking)
		f2, err := os.OpenFile(lockFile, os.O_RDWR, 0o600) // #nosec G304 -- test code using safe temp dir
		if err != nil {
			t.Fatalf("failed to open lock file: %v", err)
		}
		defer func() {
			if closeErr := f2.Close(); closeErr != nil {
				t.Errorf("failed to close file: %v", closeErr)
			}
		}()

		err = flock.Exclusive(f2.Fd())
		if err == nil {
			t.Error("expected lock acquisition to fail, but it succeeded")
			if unlockErr := flock.Unlock(f2.Fd()); unlockErr != nil {
				t.Errorf("failed to unlock: %v", unlockErr)
			}
		}
	})

	t.Run("lock can be reacquired after unlock", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		lockFile := filepath.Join(tmpDir, "test.lock")

		f, err := os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- test code using safe temp dir
		if err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				t.Errorf("failed to close file: %v", closeErr)
			}
		}()

		// Acquire and release
		err = flock.Exclusive(f.Fd())
		if err != nil {
			t.Fatalf("first lock failed: %v", err)
		}
		err = flock.Unlock(f.Fd())
		if err != nil {
			t.Fatalf("unlock failed: %v", err)
		}

		// Reacquire
		err = flock.Exclusive(f.Fd())
		if err != nil {
			t.Errorf("second lock failed: %v", err)
		}
		if unlockErr := flock.Unlock(f.Fd()); unlockErr != nil {
			t.Errorf("failed to unlock: %v", unlockErr)
		}
	})
}
