package task

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

// ProcessManager handles termination of running processes.
type ProcessManager struct {
	logger zerolog.Logger
}

// NewProcessManager creates a new process manager.
func NewProcessManager(logger zerolog.Logger) *ProcessManager {
	return &ProcessManager{
		logger: logger,
	}
}

// TerminateProcesses attempts to gracefully terminate processes, then forcefully kills them if needed.
// It uses a two-phase approach:
//  1. Send SIGTERM to all processes and wait for gracefulWait duration
//  2. Send SIGKILL to any processes that didn't terminate
//
// Returns the number of processes successfully terminated and any errors encountered.
func (pm *ProcessManager) TerminateProcesses(pids []int, gracefulWait time.Duration) (terminated int, errs []error) {
	if len(pids) == 0 {
		return 0, nil
	}

	pm.logger.Info().
		Ints("pids", pids).
		Dur("graceful_wait", gracefulWait).
		Msg("terminating processes")

	// Phase 1: Send SIGTERM to all processes
	alivePIDs := make(map[int]bool)
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			// Process doesn't exist (already dead)
			pm.logger.Debug().Int("pid", pid).Msg("process not found (already terminated)")
			continue
		}

		// Try to send SIGTERM
		err = process.Signal(syscall.SIGTERM)
		if err != nil {
			pm.logger.Warn().
				Err(err).
				Int("pid", pid).
				Msg("failed to send SIGTERM, will try SIGKILL")
		} else {
			pm.logger.Debug().Int("pid", pid).Msg("sent SIGTERM")
			alivePIDs[pid] = true
		}
	}

	// If no processes to wait for, we're done
	if len(alivePIDs) == 0 {
		return len(pids), nil
	}

	// Phase 2: Wait for graceful termination
	pm.logger.Debug().Dur("wait", gracefulWait).Msg("waiting for graceful termination")
	time.Sleep(gracefulWait)

	// Phase 3: Check which processes are still alive and SIGKILL them
	for pid := range alivePIDs {
		process, err := os.FindProcess(pid)
		if err != nil {
			// Process is gone
			terminated++
			delete(alivePIDs, pid)
			continue
		}

		// Check if process is still running by sending signal 0
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			// Process is dead
			terminated++
			delete(alivePIDs, pid)
			pm.logger.Debug().Int("pid", pid).Msg("process terminated gracefully")
			continue
		}

		// Process is still alive, send SIGKILL
		pm.logger.Warn().Int("pid", pid).Msg("process did not terminate gracefully, sending SIGKILL")
		err = process.Signal(syscall.SIGKILL)
		if err != nil {
			pm.logger.Error().
				Err(err).
				Int("pid", pid).
				Msg("failed to send SIGKILL")
			errs = append(errs, fmt.Errorf("failed to kill PID %d: %w", pid, err))
		} else {
			terminated++
			pm.logger.Debug().Int("pid", pid).Msg("sent SIGKILL")
		}
	}

	pm.logger.Info().
		Int("total_pids", len(pids)).
		Int("terminated", terminated).
		Int("errors", len(errs)).
		Msg("process termination complete")

	return terminated, errs
}

// IsProcessAlive checks if a process with the given PID is currently running.
func (pm *ProcessManager) IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 doesn't actually send a signal, but checks if we can signal the process
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// CleanupDeadProcesses removes PIDs from the slice that are no longer running.
// Returns a new slice containing only PIDs of alive processes.
func (pm *ProcessManager) CleanupDeadProcesses(pids []int) []int {
	alive := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pm.IsProcessAlive(pid) {
			alive = append(alive, pid)
		} else {
			pm.logger.Debug().Int("pid", pid).Msg("removing dead process from tracking")
		}
	}
	return alive
}
