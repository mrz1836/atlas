//go:build !windows

package ai

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureProcessGroup places the command in its own process group so that the
// command and every process it spawns can be signaled together. Without this,
// terminating the CLI (on timeout, cancellation, or Ctrl+C) leaves any child
// processes it started orphaned — still running and still consuming resources.
func configureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalProcessGroup sends sig to the entire process group led by proc. Because
// the process was started with Setpgid, its PGID equals its PID, so a negative
// PID targets the whole group (leader included). If the group no longer exists it
// is treated as success; otherwise it falls back to signaling the leader directly.
func signalProcessGroup(proc *os.Process, sig syscall.Signal) error {
	if proc == nil {
		return nil
	}

	err := syscall.Kill(-proc.Pid, sig)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		// Delivered, or the group is already gone.
		return nil
	}

	// Group signaling failed for another reason (e.g. Setpgid did not take):
	// fall back to signaling just the leader.
	if ferr := proc.Signal(sig); ferr != nil && !errors.Is(ferr, os.ErrProcessDone) {
		return ferr
	}
	return nil
}
