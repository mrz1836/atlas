//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setDaemonSysProcAttr creates a new session so the daemon is detached from
// the parent process's controlling terminal.
func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
