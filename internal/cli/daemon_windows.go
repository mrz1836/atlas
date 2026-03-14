//go:build windows

package cli

import "os/exec"

// setDaemonSysProcAttr is a no-op on Windows; Unix session IDs are not
// supported. The daemon process starts without session isolation.
func setDaemonSysProcAttr(_ *exec.Cmd) {}
