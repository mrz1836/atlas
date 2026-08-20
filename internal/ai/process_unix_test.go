//go:build !windows

package ai

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pidAlive reports whether a process with the given PID currently exists.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// signal 0 performs error checking without delivering a signal.
	return syscall.Kill(pid, 0) == nil
}

// waitForChildPID polls pidFile (written by the fake command) until the spawned
// child's PID is available, then returns it.
func waitForChildPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil { //nolint:gosec // G304: pidFile is a test-controlled t.TempDir path
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child PID was not written to %s within the deadline", pidFile)
	return 0
}

// spawnsOutlivingChildScript returns a shell script that starts a long-lived
// grandchild (which would be orphaned if only the parent shell is killed),
// records its PID, and then blocks so the parent shell stays alive.
func spawnsOutlivingChildScript(pidFile string) string {
	// The grandchild sleeps far longer than the test. `wait` keeps the parent
	// shell alive so termination targets a live process tree.
	return "sleep 300 & echo $! > " + pidFile + "; wait"
}

// terminatable is satisfied by both executors under test.
type terminatable interface {
	Execute(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error)
	TerminateProcess() error
}

// assertTerminateKillsChildGroup runs a fake command that spawns an outliving
// child, terminates it via the executor, and asserts the CHILD (a grandchild of
// the test) is killed — proving the whole process group is terminated rather than
// just the CLI. Before the process-group fix, the child would be orphaned.
func assertTerminateKillsChildGroup(t *testing.T, exec terminatable) {
	t.Helper()

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")

	ctx := context.Background()
	cmd := exec2(ctx, pidFile)

	done := make(chan struct{})
	go func() {
		_, _, _ = exec.Execute(ctx, cmd)
		close(done)
	}()

	childPID := waitForChildPID(t, pidFile)
	// Ensure the child is cleaned up even if the assertion below fails.
	t.Cleanup(func() {
		if pidAlive(childPID) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
	})

	require.True(t, pidAlive(childPID), "child should be alive before termination")

	require.NoError(t, exec.TerminateProcess())

	assert.Eventually(t, func() bool { return !pidAlive(childPID) }, 5*time.Second, 50*time.Millisecond,
		"terminating the command must kill its spawned child (process group), not orphan it")

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after termination")
	}
}

// exec2 builds the fake command used by the process-group tests.
func exec2(ctx context.Context, pidFile string) *exec.Cmd {
	//nolint:gosec // G204: fixed shell script over a test-controlled temp path
	return exec.CommandContext(ctx, "sh", "-c", spawnsOutlivingChildScript(pidFile))
}

func TestDefaultExecutor_TerminateKillsChildProcessGroup(t *testing.T) {
	t.Parallel()
	assertTerminateKillsChildGroup(t, &DefaultExecutor{})
}

func TestStreamingExecutor_TerminateKillsChildProcessGroup(t *testing.T) {
	t.Parallel()
	assertTerminateKillsChildGroup(t, NewStreamingExecutor(ActivityOptions{}))
}

// TestContextCancel_KillsChildProcessGroup verifies that context cancellation
// (the timeout path) terminates the whole group via the overridden cmd.Cancel,
// so a spawned child is not left running after a task times out.
func TestContextCancel_KillsChildProcessGroup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec2(ctx, pidFile)

	exec := &DefaultExecutor{}
	done := make(chan struct{})
	go func() {
		_, _, _ = exec.Execute(ctx, cmd)
		close(done)
	}()

	childPID := waitForChildPID(t, pidFile)
	t.Cleanup(func() {
		if pidAlive(childPID) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
	})

	cancel()

	assert.Eventually(t, func() bool { return !pidAlive(childPID) }, 5*time.Second, 50*time.Millisecond,
		"context cancellation must kill the spawned child via the process group")

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after context cancellation")
	}
}
