package testfakes_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/daemon"
	"github.com/mrz1836/atlas/internal/daemon/testfakes"
)

// -- Redis primitives: hash --

func TestRedisFixture_Hash(t *testing.T) {
	t.Parallel()
	f := testfakes.NewRedisFixture(t, "")
	ctx := context.Background()

	pairs := [][2]any{{"status", "queued"}, {"template", "bug"}}
	require.NoError(t, cache.HashMapSet(ctx, f.Client, "test:task:h1", pairs))

	val, err := cache.HashGet(ctx, f.Client, "test:task:h1", "status")
	require.NoError(t, err)
	assert.Equal(t, "queued", val)

	val2, err := cache.HashGet(ctx, f.Client, "test:task:h1", "template")
	require.NoError(t, err)
	assert.Equal(t, "bug", val2)
}

// -- Redis primitives: set --

func TestRedisFixture_Set(t *testing.T) {
	t.Parallel()
	f := testfakes.NewRedisFixture(t, "")
	ctx := context.Background()

	require.NoError(t, cache.SetAdd(ctx, f.Client, "test:active", "task-001"))
	require.NoError(t, cache.SetAdd(ctx, f.Client, "test:active", "task-002"))

	members, err := cache.SetMembers(ctx, f.Client, "test:active")
	require.NoError(t, err)
	assert.Len(t, members, 2)

	require.NoError(t, cache.SetRemoveMember(ctx, f.Client, "test:active", "task-001"))
	members, err = cache.SetMembers(ctx, f.Client, "test:active")
	require.NoError(t, err)
	assert.Len(t, members, 1)
}

// -- Redis primitives: sorted set (via RedisQueue) --

func TestRedisFixture_SortedSet(t *testing.T) {
	t.Parallel()
	f := testfakes.NewRedisFixture(t, "")
	ctx := context.Background()

	q := daemon.NewRedisQueue(f.Client, f.Prefix)
	require.NoError(t, q.Submit(ctx, "sorted-task-001", daemon.PriorityNormal))
	require.NoError(t, q.Submit(ctx, "sorted-task-002", daemon.PriorityUrgent))

	// Urgent pops first.
	id, prio, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "sorted-task-002", id)
	assert.Equal(t, daemon.PriorityUrgent, prio)

	// Normal is next.
	id2, prio2, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "sorted-task-001", id2)
	assert.Equal(t, daemon.PriorityNormal, prio2)

	// Queue empty.
	empty, _, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// -- Redis primitives: pub/sub (via EventPublisher) --

func TestRedisFixture_PubSub(t *testing.T) {
	t.Parallel()
	f := testfakes.NewRedisFixture(t, "")
	ctx := context.Background()

	// Publish must succeed without error; full round-trip (subscribe + receive)
	// is covered by events_test.go in the daemon package.
	pub := daemon.NewEventPublisher(f.Client, "testfakes:smoke")
	evt := daemon.TaskEvent{Type: daemon.EventTaskSubmitted, TaskID: "pub-smoke-001"}
	require.NoError(t, pub.Publish(ctx, evt), "pub/sub publish must not error")
}

// -- Redis primitives: stream (via LogWriter / LogReader) --

func TestRedisFixture_Stream(t *testing.T) {
	t.Parallel()
	f := testfakes.NewRedisFixture(t, "")
	ctx := context.Background()

	writer := daemon.NewLogWriter(f.Client, f.Prefix, 1000)
	entry := daemon.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     "info",
		Message:   "hello from smoke test",
		Source:    "testfakes",
		Step:      "smoke",
	}
	require.NoError(t, writer.Write(ctx, "stream-smoke-001", entry))

	reader := daemon.NewLogReader(f.Client, f.Prefix)
	entries, err := reader.Read(ctx, "stream-smoke-001", "0", 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello from smoke test", entries[0].Message)
	assert.Equal(t, "info", entries[0].Level)
	assert.Equal(t, "testfakes", entries[0].Source)
}

// -- FakeExecutor --

func TestFakeExecutor_Execute_Completed(t *testing.T) {
	t.Parallel()
	exec := &testfakes.FakeExecutor{
		EngineTaskID: "eng-42",
		FinalStatus:  "completed",
	}
	ctx := context.Background()
	job := daemon.TaskJob{TaskID: "task-001", Description: "fix the thing"}

	eid, status, err := exec.Execute(ctx, job)
	require.NoError(t, err)
	assert.Equal(t, "eng-42", eid)
	assert.Equal(t, "completed", status)
	assert.Equal(t, 1, exec.CallCount())
	require.Len(t, exec.ExecuteCalls, 1)
	assert.Equal(t, "task-001", exec.ExecuteCalls[0].TaskID)
}

func TestFakeExecutor_Execute_DefaultStatus(t *testing.T) {
	t.Parallel()
	// When FinalStatus is empty, Execute returns "completed".
	exec := &testfakes.FakeExecutor{}
	_, status, err := exec.Execute(context.Background(), daemon.TaskJob{TaskID: "task-default"})
	require.NoError(t, err)
	assert.Equal(t, "completed", status)
}

func TestFakeExecutor_Execute_Error(t *testing.T) {
	t.Parallel()
	execErr := errors.New("fake engine error") //nolint:err113 // test-local sentinel
	exec := &testfakes.FakeExecutor{ExecErr: execErr}

	_, _, err := exec.Execute(context.Background(), daemon.TaskJob{TaskID: "task-err"})
	require.ErrorIs(t, err, execErr)
	assert.Equal(t, 1, exec.CallCount())
}

func TestFakeExecutor_Execute_BlockAndCancel(t *testing.T) {
	t.Parallel()
	exec := &testfakes.FakeExecutor{BlockUntilCtxDone: true}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, _, err := exec.Execute(ctx, daemon.TaskJob{TaskID: "task-cancel"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestFakeExecutor_Execute_Delay(t *testing.T) {
	t.Parallel()
	exec := &testfakes.FakeExecutor{ExecDelay: 50 * time.Millisecond, FinalStatus: "awaiting_approval"}
	_, status, err := exec.Execute(context.Background(), daemon.TaskJob{TaskID: "task-delay"})
	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", status)
}

func TestFakeExecutor_Abandon(t *testing.T) {
	t.Parallel()
	exec := &testfakes.FakeExecutor{}
	job := daemon.TaskJob{TaskID: "abandon-task"}

	err := exec.Abandon(context.Background(), job, "test abandonment")
	require.NoError(t, err)
	require.Len(t, exec.AbandonCalls, 1)
	assert.Equal(t, "abandon-task", exec.AbandonCalls[0].TaskID)
}

func TestFakeExecutor_Abandon_Error(t *testing.T) {
	t.Parallel()
	abandonErr := errors.New("abandon failed") //nolint:err113 // test-local sentinel
	exec := &testfakes.FakeExecutor{AbandonErr: abandonErr}

	err := exec.Abandon(context.Background(), daemon.TaskJob{TaskID: "err-task"}, "reason")
	require.ErrorIs(t, err, abandonErr)
}

// -- TempGitRepo --

func TestTempGitRepo(t *testing.T) {
	t.Parallel()
	dir := testfakes.TempGitRepo(t)

	// .git directory must exist.
	_, err := os.Stat(filepath.Join(dir, ".git"))
	require.NoError(t, err, ".git directory should exist after TempGitRepo")

	// README.md from the initial commit must be present.
	_, err = os.Stat(filepath.Join(dir, "README.md"))
	require.NoError(t, err, "README.md should exist in the initial commit")
}

// -- TempAtlasEnv --

func TestTempAtlasEnv(t *testing.T) {
	t.Parallel()
	dir := testfakes.TempAtlasEnv(t)

	for _, sub := range []string{"tasks", "workspaces"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		require.NoError(t, err, "%s subdirectory should exist", sub)
		assert.True(t, info.IsDir(), "%s should be a directory", sub)
	}
}

// -- NewClient --

func TestRedisFixture_NewClient(t *testing.T) {
	t.Parallel()
	f := testfakes.NewRedisFixture(t, "")
	ctx := context.Background()

	// Additional client should be able to read keys written by the primary client.
	pairs := [][2]any{{"extra", "yes"}}
	require.NoError(t, cache.HashMapSet(ctx, f.Client, "test:extra:key", pairs))

	client2 := f.NewClient(t)
	val, err := cache.HashGet(ctx, client2, "test:extra:key", "extra")
	require.NoError(t, err)
	assert.Equal(t, "yes", val)
}
