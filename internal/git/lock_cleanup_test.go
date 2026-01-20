package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Helper function to create a lock file with a specific age
func createLockFileWithAge(t *testing.T, tmpDir string, age time.Duration) string {
	t.Helper()
	lockPath := filepath.Join(tmpDir, "index.lock")
	if err := os.WriteFile(lockPath, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to create test lock file: %v", err)
	}
	if age > 0 {
		oldTime := time.Now().Add(-age)
		if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
			t.Fatalf("failed to change file time: %v", err)
		}
	}
	return lockPath
}

func TestDetectStaleLockFile(t *testing.T) {
	t.Run("NonExistentFile", func(t *testing.T) {
		isStale, err := DetectStaleLockFile("/nonexistent/lock.file", time.Minute)
		if err != nil {
			t.Errorf("expected no error for nonexistent file, got: %v", err)
		}
		if isStale {
			t.Error("expected false for nonexistent file")
		}
	})

	t.Run("FreshLockFile", func(t *testing.T) {
		lockPath := createLockFileWithAge(t, t.TempDir(), 0)
		isStale, err := DetectStaleLockFile(lockPath, time.Minute)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if isStale {
			t.Error("expected fresh file to not be stale")
		}
	})

	t.Run("StaleLockFile", func(t *testing.T) {
		lockPath := createLockFileWithAge(t, t.TempDir(), 2*time.Minute)
		isStale, err := DetectStaleLockFile(lockPath, time.Minute)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !isStale {
			t.Error("expected old file to be stale")
		}
	})

	t.Run("LockFileAtThreshold", func(t *testing.T) {
		lockPath := createLockFileWithAge(t, t.TempDir(), time.Minute)
		time.Sleep(10 * time.Millisecond)
		isStale, err := DetectStaleLockFile(lockPath, time.Minute)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !isStale {
			t.Error("expected file at threshold to be stale")
		}
	})
}

func TestRemoveStaleLockFile(t *testing.T) {
	t.Run("SuccessfulRemoval", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "index.lock")

		// Create a stale lock file
		if err := os.WriteFile(lockPath, []byte("test"), 0o600); err != nil {
			t.Fatalf("failed to create test lock file: %v", err)
		}
		oldTime := time.Now().Add(-2 * time.Minute)
		if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
			t.Fatalf("failed to change file time: %v", err)
		}

		// Remove it
		logger := zerolog.Nop()
		err := RemoveStaleLockFile(ctx, lockPath, time.Minute, logger)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify it's gone
		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Error("expected lock file to be removed")
		}
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		ctx := context.Background()
		logger := zerolog.Nop()

		// Should succeed silently for nonexistent files
		err := RemoveStaleLockFile(ctx, "/nonexistent/lock.file", time.Minute, logger)
		if err != nil {
			t.Errorf("expected no error for nonexistent file, got: %v", err)
		}
	})

	t.Run("FreshFileNotRemoved", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "index.lock")

		// Create a fresh lock file
		if err := os.WriteFile(lockPath, []byte("test"), 0o600); err != nil {
			t.Fatalf("failed to create test lock file: %v", err)
		}

		// Try to remove it - should fail because it's not stale
		logger := zerolog.Nop()
		err := RemoveStaleLockFile(ctx, lockPath, time.Minute, logger)
		if err == nil {
			t.Error("expected error when removing fresh lock file")
		}

		// Verify it still exists
		if _, err := os.Stat(lockPath); err != nil {
			t.Error("expected lock file to still exist")
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "index.lock")
		logger := zerolog.Nop()

		err := RemoveStaleLockFile(ctx, lockPath, time.Minute, logger)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	})
}

// Helper to create a git directory structure
func setupGitDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("failed to create git dir: %v", err)
	}
	return gitDir
}

// Helper to create a lock file at a specific path with a specific age
func createLockFileAt(t *testing.T, path string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	if age > 0 {
		oldTime := time.Now().Add(-age)
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatalf("failed to change file time: %v", err)
		}
	}
}

// Helper to verify if a file exists or not
func assertFileExists(t *testing.T, path string, shouldExist bool) {
	t.Helper()
	_, err := os.Stat(path)
	exists := !os.IsNotExist(err)
	if exists != shouldExist {
		if shouldExist {
			t.Errorf("expected file %s to exist", path)
		} else {
			t.Errorf("expected file %s to not exist", path)
		}
	}
}

// setupWorktreeGitStructure creates a worktree git structure for testing
func setupWorktreeGitStructure(t *testing.T) (gitFilePath, lockPath string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create the main repo's .git structure
	mainGitDir := filepath.Join(tmpDir, "main-repo", ".git")
	worktreeGitDir := filepath.Join(mainGitDir, "worktrees", "my-worktree")
	if err := os.MkdirAll(worktreeGitDir, 0o750); err != nil {
		t.Fatalf("failed to create worktree git dir: %v", err)
	}

	// Create a stale lock file in the worktree's git dir
	lockPath = filepath.Join(worktreeGitDir, "index.lock")
	createLockFileAt(t, lockPath, 2*time.Minute)

	// Create the worktree directory with a .git file
	worktreeDir := filepath.Join(tmpDir, "worktree-dir")
	if err := os.MkdirAll(worktreeDir, 0o750); err != nil {
		t.Fatalf("failed to create worktree dir: %v", err)
	}

	// Create the .git file that points to the actual git dir
	gitFilePath = filepath.Join(worktreeDir, ".git")
	gitFileContent := fmt.Sprintf("gitdir: %s\n", worktreeGitDir)
	if err := os.WriteFile(gitFilePath, []byte(gitFileContent), 0o600); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}

	return gitFilePath, lockPath
}

func TestCleanupStaleLockFiles(t *testing.T) {
	t.Run("NoLockFiles", func(t *testing.T) {
		gitDir := setupGitDir(t)
		err := CleanupStaleLockFiles(context.Background(), gitDir, time.Minute, zerolog.Nop())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("SingleStaleLockFile", func(t *testing.T) {
		gitDir := setupGitDir(t)
		lockPath := filepath.Join(gitDir, "index.lock")
		createLockFileAt(t, lockPath, 2*time.Minute)

		err := CleanupStaleLockFiles(context.Background(), gitDir, time.Minute, zerolog.Nop())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertFileExists(t, lockPath, false)
	})

	t.Run("MultipleWorktreeLocks", func(t *testing.T) {
		gitDir := setupGitDir(t)
		worktreesDir := filepath.Join(gitDir, "worktrees")

		wt1Dir := filepath.Join(worktreesDir, "worktree1")
		wt2Dir := filepath.Join(worktreesDir, "worktree2")
		if err := os.MkdirAll(wt1Dir, 0o750); err != nil {
			t.Fatalf("failed to create worktree1 dir: %v", err)
		}
		if err := os.MkdirAll(wt2Dir, 0o750); err != nil {
			t.Fatalf("failed to create worktree2 dir: %v", err)
		}

		lock1 := filepath.Join(wt1Dir, "index.lock")
		lock2 := filepath.Join(wt2Dir, "index.lock")
		createLockFileAt(t, lock1, 2*time.Minute)
		createLockFileAt(t, lock2, 2*time.Minute)

		err := CleanupStaleLockFiles(context.Background(), gitDir, time.Minute, zerolog.Nop())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertFileExists(t, lock1, false)
		assertFileExists(t, lock2, false)
	})

	t.Run("MixedFreshAndStaleLocks", func(t *testing.T) {
		gitDir := setupGitDir(t)
		staleLock := filepath.Join(gitDir, "index.lock")
		createLockFileAt(t, staleLock, 2*time.Minute)

		refsHeadsDir := filepath.Join(gitDir, "refs", "heads")
		if err := os.MkdirAll(refsHeadsDir, 0o750); err != nil {
			t.Fatalf("failed to create refs/heads dir: %v", err)
		}
		freshLock := filepath.Join(refsHeadsDir, "branch.lock")
		createLockFileAt(t, freshLock, 0)

		_ = CleanupStaleLockFiles(context.Background(), gitDir, time.Minute, zerolog.Nop())
		assertFileExists(t, staleLock, false)
		assertFileExists(t, freshLock, true)
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		gitDir := setupGitDir(t)
		err := CleanupStaleLockFiles(ctx, gitDir, time.Minute, zerolog.Nop())
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	})

	t.Run("NonexistentGitDir", func(_ *testing.T) {
		_ = CleanupStaleLockFiles(context.Background(), "/nonexistent/.git", time.Minute, zerolog.Nop())
	})

	t.Run("WorktreeGitFile", func(t *testing.T) {
		gitFilePath, lockPath := setupWorktreeGitStructure(t)
		err := CleanupStaleLockFiles(context.Background(), gitFilePath, time.Minute, zerolog.Nop())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		assertFileExists(t, lockPath, false)
	})
}
