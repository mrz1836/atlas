package validation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
)

// CommandRunner defines the interface for executing shell commands.
// This allows for testing by injecting mock implementations.
type CommandRunner interface {
	// Run executes a shell command and returns its output.
	Run(ctx context.Context, workDir, command string) (stdout, stderr string, exitCode int, err error)
}

// LiveOutputRunner defines a command runner that supports live output streaming.
type LiveOutputRunner interface {
	CommandRunner
	// RunWithLiveOutput executes a command and streams output to the writer while also capturing it.
	RunWithLiveOutput(ctx context.Context, workDir, command string, liveOut io.Writer) (stdout, stderr string, exitCode int, err error)
}

// DefaultCommandRunner implements CommandRunner and LiveOutputRunner using os/exec.
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

// RunWithLiveOutput executes a command and streams output to liveOut while also capturing it.
func (r *DefaultCommandRunner) RunWithLiveOutput(ctx context.Context, workDir, command string, liveOut io.Writer) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir

	var outBuf, errBuf bytes.Buffer

	// Use MultiWriter to both capture and stream output
	cmd.Stdout = io.MultiWriter(&outBuf, liveOut)
	cmd.Stderr = io.MultiWriter(&errBuf, liveOut)

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

// Ensure DefaultCommandRunner implements CommandRunner and LiveOutputRunner.
var (
	_ CommandRunner    = (*DefaultCommandRunner)(nil)
	_ LiveOutputRunner = (*DefaultCommandRunner)(nil)
)
