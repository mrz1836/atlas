package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/lifecycle"
)

// newTestDaemonForLock creates a minimal Daemon with Redis for worktree-lock tests.
func newTestDaemonForLock(t *testing.T, mr *miniredis.Miniredis) (*Daemon, *cache.Client) {
	t.Helper()
	tmp := t.TempDir()
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			Enabled:          true,
			SocketPath:       filepath.Join(tmp, "daemon.sock"),
			PIDFile:          filepath.Join(tmp, "daemon.pid"),
			MaxParallelTasks: 2,
			ShutdownTimeout:  2 * time.Second,
			TaskTimeout:      45 * time.Minute,
		},
		Redis: config.RedisConfig{
			Addr:         mr.Addr(),
			KeyPrefix:    "atlas:",
			PoolSize:     5,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
	}
	ctx := context.Background()
	client, err := NewRedisClient(ctx, RedisConfig{
		Addr:         mr.Addr(),
		PoolSize:     5,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err)

	d := New(cfg, zerolog.Nop())
	d.redis = client
	d.queue = NewRedisQueue(client, "atlas:")
	d.events = NewEventPublisher(client, "")
	return d, client
}

// submitTaskWithRepoPath calls handleTaskSubmit with repo_path set.
func submitTaskWithRepoPath(t *testing.T, d *Daemon, repoPath string) (string, error) { //nolint:unparam // taskID may be used by future callers
	t.Helper()
	params, err := json.Marshal(TaskSubmitRequest{
		Description: "test task",
		Template:    "task",
		RepoPath:    repoPath,
	})
	require.NoError(t, err)
	resp, err := d.handleTaskSubmit(context.Background(), params)
	if err != nil {
		return "", err
	}
	sr, ok := resp.(TaskSubmitResponse)
	require.True(t, ok)
	return sr.TaskID, nil
}

// TestWorktreeLock_TwoDaemonSubmitsSameWorktree verifies the second submit is rejected (scenario a).
func TestWorktreeLock_TwoDaemonSubmitsSameWorktree(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	repoPath := t.TempDir() // unique per-test path

	// First submit should succeed.
	_, err := submitTaskWithRepoPath(t, d, repoPath)
	require.NoError(t, err, "first submit should succeed")

	// Second submit for the same repo path should fail.
	_, err = submitTaskWithRepoPath(t, d, repoPath)
	require.Error(t, err, "second submit for same worktree should be rejected")
	assert.ErrorIs(t, err, errWorktreeLocked, "error should be errWorktreeLocked")
}

// TestWorktreeLock_DirectModeInterlock verifies daemon submit is rejected when direct mode
// has a live filesystem lock (scenario b).
func TestWorktreeLock_DirectModeInterlock(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	repoPath := t.TempDir()

	// Simulate direct mode holding the filesystem lock.
	require.NoError(t, lifecycle.WriteFilesystemLock(repoPath, "direct-mode-owner"))
	defer lifecycle.RemoveFilesystemLock(repoPath)

	// Daemon submit should be rejected.
	_, err := submitTaskWithRepoPath(t, d, repoPath)
	require.Error(t, err, "daemon submit should be rejected when direct mode holds filesystem lock")
	assert.ErrorIs(t, err, errWorktreeLocked)
}

// TestWorktreeLock_DirectModeChecksRedisLock verifies the Redis worktree lock key exists
// after a successful daemon submit, which direct mode should check (scenario c verification).
func TestWorktreeLock_DirectModeChecksRedisLock(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	repoPath := t.TempDir()

	// Daemon submit succeeds — acquires Redis worktree lock.
	_, err := submitTaskWithRepoPath(t, d, repoPath)
	require.NoError(t, err)

	// The Redis worktree lock key should exist.
	lockKey := lifecycle.WorktreeLockRedisKey("atlas:", repoPath)
	exists, existsErr := cache.Exists(context.Background(), client, lockKey)
	require.NoError(t, existsErr)
	assert.True(t, exists, "Redis worktree lock should exist after daemon submit")
}

// TestWorktreeLock_TwoWorktreesSameUpstreamAllowed verifies concurrent tasks on different
// worktrees of the same upstream repo are both allowed (scenario d).
func TestWorktreeLock_TwoWorktreesSameUpstreamAllowed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	// Two distinct worktree paths (simulating two worktrees of the same upstream repo).
	repoPath1 := t.TempDir()
	repoPath2 := t.TempDir()

	// Both submits should succeed because they're on different worktree paths.
	_, err1 := submitTaskWithRepoPath(t, d, repoPath1)
	require.NoError(t, err1, "first worktree submit should succeed")

	_, err2 := submitTaskWithRepoPath(t, d, repoPath2)
	require.NoError(t, err2, "second worktree submit to different path should succeed")
}

// TestWorktreeLock_NoRepoPath verifies that tasks without a repo_path bypass the lock.
func TestWorktreeLock_NoRepoPath(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	// Submit without repo_path — should succeed (no lock applied).
	params, err := json.Marshal(TaskSubmitRequest{
		Description: "task without repo",
		Template:    "task",
	})
	require.NoError(t, err)
	resp, err := d.handleTaskSubmit(context.Background(), params)
	require.NoError(t, err)
	_, ok := resp.(TaskSubmitResponse)
	assert.True(t, ok)
}

// hookDegradedExecutor is a local test double that returns degraded hook state.
type hookDegradedExecutor struct {
	reason string
}

func (e *hookDegradedExecutor) Execute(_ context.Context, _ TaskJob) (string, string, error) {
	return "", "completed", nil
}

func (e *hookDegradedExecutor) Abandon(_ context.Context, _ TaskJob, _ string) error { return nil }

func (e *hookDegradedExecutor) InitHooks(_ context.Context, _ TaskJob) (bool, string) {
	return true, e.reason
}

// TestWorktreeLock_HookDegraded verifies that the runner marks crash_recovery=degraded
// when the executor's InitHooks reports failure.
func TestWorktreeLock_HookDegraded(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			MaxParallelTasks: 1,
			ShutdownTimeout:  2 * time.Second,
			TaskTimeout:      5 * time.Second,
		},
		Redis: config.RedisConfig{
			Addr:      mr.Addr(),
			KeyPrefix: "atlas:",
		},
	}

	ctx := context.Background()
	client, err := NewRedisClient(ctx, RedisConfig{
		Addr:        mr.Addr(),
		PoolSize:    5,
		DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	exec := &hookDegradedExecutor{reason: "hook dir unwritable"}
	q := NewRedisQueue(client, "atlas:")
	events := NewEventPublisher(client, "")
	r := NewRunner(cfg, client, q, events, zerolog.Nop(), exec)

	// Seed a task.
	taskID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	mr.HSet(hashKey, "status", "queued")
	mr.HSet(hashKey, "priority", "normal")
	mr.HSet(hashKey, "description", "degraded hook test")
	_, _ = mr.SAdd("atlas:active", taskID)
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	r.Start(ctx)

	deadline := time.After(3 * time.Second)
	for {
		status, _ := cache.HashGet(ctx, client, hashKey, "status")
		if status == "completed" || status == "failed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("task did not complete within deadline")
		case <-time.After(50 * time.Millisecond):
		}
	}
	r.Stop()

	crashRecovery, err := cache.HashGet(ctx, client, hashKey, "crash_recovery")
	require.NoError(t, err)
	assert.Equal(t, "degraded", crashRecovery, "crash_recovery should be degraded when hook init fails")
}
