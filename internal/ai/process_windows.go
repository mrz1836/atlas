//go:build windows

package ai

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureProcessGroup is a no-op on Windows; Unix-style process groups are not
// available. Atlas's daemon mode targets Unix, so this platform runs without
// process-group isolation.
func configureProcessGroup(_ *exec.Cmd) {}

// signalProcessGroup terminates the given process on Windows, where Unix
// process-group signaling is unavailable. Termination signals map to Kill; other
// signals are forwarded best-effort.
func signalProcessGroup(proc *os.Process, sig syscall.Signal) error {
	if proc == nil {
		return nil
	}
	if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
		if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return nil
	}
	if err := proc.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
