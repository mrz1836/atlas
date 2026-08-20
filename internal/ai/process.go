package ai

import (
	"os/exec"
	"syscall"
	"time"
)

// processWaitDelay bounds how long os/exec waits to finish reading a command's
// stdout and stderr after the process exits or its context is canceled. Without
// a bound, a child process that inherited those pipes and outlives its parent can
// block cmd.Wait indefinitely. After the delay, os/exec closes the pipes and
// kills the process, so Execute always returns.
const processWaitDelay = 10 * time.Second

// prepareCommand configures an AI CLI command for robust lifecycle management.
// It MUST be called after the command is built and before it is started.
//
// It does three things that, together, prevent leaked subprocesses and hangs:
//   - isolates the command in its own process group, so all of its descendants
//     (these CLIs spawn workers) can be signaled as a unit;
//   - overrides context cancellation to kill the whole group rather than only the
//     group leader (the os/exec default), so no descendant is orphaned; and
//   - bounds post-exit / post-cancel I/O draining so an orphaned child holding
//     stdout/stderr cannot block Wait forever.
func prepareCommand(cmd *exec.Cmd) {
	configureProcessGroup(cmd)

	// On context cancellation/timeout, terminate the entire process group.
	cmd.Cancel = func() error {
		return signalProcessGroup(cmd.Process, syscall.SIGKILL)
	}

	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = processWaitDelay
	}
}
