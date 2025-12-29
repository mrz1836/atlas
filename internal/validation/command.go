package validation

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// CommandRunner defines the interface for executing shell commands.
// This allows for testing by injecting mock implementations.
type CommandRunner interface {
	// Run executes a shell command and returns its output.
	Run(ctx context.Context, workDir, command string) (stdout, stderr string, exitCode int, err error)
}

// DefaultCommandRunner implements CommandRunner using os/exec.
type DefaultCommandRunner struct{}

// Run executes a shell command using sh -c.
func (r *DefaultCommandRunner) Run(ctx context.Context, workDir, command string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return stdout, stderr, exitCode, err
}

// Ensure DefaultCommandRunner implements CommandRunner.
var _ CommandRunner = (*DefaultCommandRunner)(nil)
