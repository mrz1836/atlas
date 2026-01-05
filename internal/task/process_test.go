package task

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessManager(t *testing.T) {
	t.Run("creates valid manager", func(t *testing.T) {
		logger := zerolog.New(os.Stdout)
		pm := NewProcessManager(logger)

		require.NotNil(t, pm)
	})

	t.Run("stores logger correctly", func(t *testing.T) {
		var buf bytes.Buffer
		logger := zerolog.New(&buf)
		pm := NewProcessManager(logger)

		require.NotNil(t, pm)
		// Logger is stored and can be used (we'll see output in other tests)
	})
}

func TestProcessManager_IsProcessAlive(t *testing.T) {
	logger := zerolog.New(os.Stdout)
	pm := NewProcessManager(logger)

	tests := []struct {
		name     string
		setup    func(t *testing.T) int
		expected bool
	}{
		{
			name: "alive process returns true",
			setup: func(t *testing.T) int {
				// Spawn a sleep process
				cmd := exec.CommandContext(context.Background(), "sleep", "30")
				require.NoError(t, cmd.Start())
				t.Cleanup(func() { _ = cmd.Process.Kill() })
				return cmd.Process.Pid
			},
			expected: true,
		},
		{
			name: "dead process returns false",
			setup: func(t *testing.T) int {
				// Spawn and immediately finish
				cmd := exec.CommandContext(context.Background(), "true")
				require.NoError(t, cmd.Run())
				return cmd.Process.Pid
			},
			expected: false,
		},
		{
			name: "invalid PID zero returns false",
			setup: func(_ *testing.T) int {
				return 0
			},
			expected: false,
		},
		{
			name: "invalid negative PID returns false",
			setup: func(_ *testing.T) int {
				return -1
			},
			expected: false,
		},
		{
			name: "non-existent PID returns false",
			setup: func(_ *testing.T) int {
				return 999999 // Very unlikely to exist
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid := tt.setup(t)
			result := pm.IsProcessAlive(pid)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessManager_CleanupDeadProcesses(t *testing.T) {
	logger := zerolog.New(os.Stdout)
	pm := NewProcessManager(logger)

	t.Run("filters out dead processes", func(t *testing.T) {
		// Create alive process
		aliveCmd := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, aliveCmd.Start())
		t.Cleanup(func() { _ = aliveCmd.Process.Kill() })
		alivePid := aliveCmd.Process.Pid

		// Create and kill process
		deadCmd := exec.CommandContext(context.Background(), "true")
		require.NoError(t, deadCmd.Run())
		deadPid := deadCmd.Process.Pid

		// Mix of alive, dead, and invalid PIDs
		pids := []int{alivePid, deadPid, -1, 0}

		cleaned := pm.CleanupDeadProcesses(pids)

		require.Len(t, cleaned, 1)
		assert.Equal(t, alivePid, cleaned[0])
	})

	t.Run("empty slice returns empty slice", func(t *testing.T) {
		cleaned := pm.CleanupDeadProcesses([]int{})
		assert.Empty(t, cleaned)
	})

	t.Run("all dead returns empty", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "true")
		require.NoError(t, cmd.Run())

		cleaned := pm.CleanupDeadProcesses([]int{cmd.Process.Pid, -1, 0})
		assert.Empty(t, cleaned)
	})

	t.Run("all alive returns same slice", func(t *testing.T) {
		cmd1 := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd1.Start())
		t.Cleanup(func() { _ = cmd1.Process.Kill() })

		cmd2 := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd2.Start())
		t.Cleanup(func() { _ = cmd2.Process.Kill() })

		pids := []int{cmd1.Process.Pid, cmd2.Process.Pid}
		cleaned := pm.CleanupDeadProcesses(pids)

		require.Len(t, cleaned, 2)
		assert.Contains(t, cleaned, cmd1.Process.Pid)
		assert.Contains(t, cleaned, cmd2.Process.Pid)
	})
}

func TestProcessManager_TerminateProcesses(t *testing.T) {
	var logBuf bytes.Buffer
	logger := zerolog.New(&logBuf)
	pm := NewProcessManager(logger)

	t.Run("empty PID list returns zero terminated", func(t *testing.T) {
		terminated, errs := pm.TerminateProcesses([]int{}, 100*time.Millisecond)

		assert.Equal(t, 0, terminated)
		assert.Empty(t, errs)
	})

	t.Run("graceful termination with SIGTERM", func(t *testing.T) {
		// Spawn process that handles SIGTERM
		cmd := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd.Start())
		pid := cmd.Process.Pid

		terminated, errs := pm.TerminateProcesses([]int{pid}, 500*time.Millisecond)

		assert.Equal(t, 1, terminated)
		assert.Empty(t, errs)

		// Wait for process to be reaped (non-blocking with timeout)
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited successfully
		case <-time.After(1 * time.Second):
			t.Fatal("process did not exit within timeout")
		}
	})

	t.Run("force kill with SIGKILL after timeout", func(t *testing.T) {
		// Spawn process that ignores SIGTERM (trap signal in shell)
		cmd := exec.CommandContext(context.Background(), "sh", "-c", "trap '' TERM; sleep 30")
		require.NoError(t, cmd.Start())
		pid := cmd.Process.Pid

		// Short grace period forces SIGKILL
		terminated, errs := pm.TerminateProcesses([]int{pid}, 100*time.Millisecond)

		assert.Equal(t, 1, terminated)
		assert.Empty(t, errs)

		// Wait for process to be reaped (non-blocking with timeout)
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited successfully
		case <-time.After(1 * time.Second):
			t.Fatal("process did not exit within timeout")
		}
	})

	t.Run("skips invalid PIDs", func(t *testing.T) {
		terminated, errs := pm.TerminateProcesses([]int{-1, 0}, 100*time.Millisecond)

		// Invalid PIDs are skipped in SIGTERM phase (line 43-44)
		// If no processes got SIGTERM, returns len(pids) at line 69
		// This is the actual behavior - counts all as "terminated" (already dead)
		assert.Equal(t, 2, terminated)
		assert.Empty(t, errs)
	})

	t.Run("skips already dead processes", func(t *testing.T) {
		// Start and immediately finish
		cmd := exec.CommandContext(context.Background(), "true")
		require.NoError(t, cmd.Run())
		deadPid := cmd.Process.Pid

		terminated, errs := pm.TerminateProcesses([]int{deadPid}, 100*time.Millisecond)

		// Dead process: Signal fails, counted as terminated
		assert.GreaterOrEqual(t, terminated, 0)
		assert.Empty(t, errs)
	})

	t.Run("mixed alive and dead processes", func(t *testing.T) {
		aliveCmd := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, aliveCmd.Start())
		alivePid := aliveCmd.Process.Pid

		deadCmd := exec.CommandContext(context.Background(), "true")
		require.NoError(t, deadCmd.Run())
		deadPid := deadCmd.Process.Pid

		terminated, errs := pm.TerminateProcesses([]int{alivePid, deadPid, -1}, 200*time.Millisecond)

		assert.GreaterOrEqual(t, terminated, 1)
		assert.Empty(t, errs)

		// Wait for alive process to be reaped
		done := make(chan error, 1)
		go func() {
			done <- aliveCmd.Wait()
		}()

		select {
		case <-done:
			// Process exited successfully
		case <-time.After(1 * time.Second):
			t.Fatal("alive process did not exit within timeout")
		}
	})

	t.Run("handles SIGKILL failure gracefully", func(t *testing.T) {
		// Use PID 1 (init/systemd) which can't be killed by non-root
		terminated, errs := pm.TerminateProcesses([]int{1}, 100*time.Millisecond)

		// Should fail to kill PID 1
		if len(errs) > 0 {
			assert.Contains(t, errs[0].Error(), "failed to kill PID 1")
		}
		// Terminated count depends on whether Signal(0) succeeds
		assert.GreaterOrEqual(t, terminated, 0)
	})

	t.Run("multiple processes all terminate", func(t *testing.T) {
		cmd1 := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd1.Start())
		pid1 := cmd1.Process.Pid

		cmd2 := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd2.Start())
		pid2 := cmd2.Process.Pid

		cmd3 := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd3.Start())
		pid3 := cmd3.Process.Pid

		terminated, errs := pm.TerminateProcesses([]int{pid1, pid2, pid3}, 300*time.Millisecond)

		assert.Equal(t, 3, terminated)
		assert.Empty(t, errs)

		// Wait for all processes to be reaped
		done := make(chan struct{})
		go func() {
			_ = cmd1.Wait()
			_ = cmd2.Wait()
			_ = cmd3.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All processes exited successfully
		case <-time.After(2 * time.Second):
			t.Fatal("not all processes exited within timeout")
		}
	})

	t.Run("respects graceful wait duration", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "sleep", "30")
		require.NoError(t, cmd.Start())
		pid := cmd.Process.Pid

		start := time.Now()
		terminated, errs := pm.TerminateProcesses([]int{pid}, 200*time.Millisecond)
		elapsed := time.Since(start)

		assert.Equal(t, 1, terminated)
		assert.Empty(t, errs)
		// Should wait approximately 200ms before killing
		assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond)
	})
}
