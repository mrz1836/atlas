// Package git provides Git operations for ATLAS.
// This file provides shared git command execution utilities.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// RunCommand executes a git command in the specified directory and returns its output.
// All errors are wrapped with ErrGitOperation and include stderr for debugging.
// This function is exported for use by other packages (e.g., workspace).
func RunCommand(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //#nosec G204 -- args are constructed internally, not user input
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), nil
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return "", formatGitError(err, &stderr, &stdout, args[0])
}

func formatGitError(err error, stderr, stdout *bytes.Buffer, subcommand string) error {
	var exitErr *exec.ExitError
	exitCode := -1
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = strings.TrimSpace(stdout.String())
	}

	switch {
	case detail != "" && exitCode >= 0:
		return fmt.Errorf("git %s failed (exit %d): %s: %w", subcommand, exitCode, detail, atlaserrors.ErrGitOperation)
	case detail != "":
		return fmt.Errorf("git %s failed: %s: %w", subcommand, detail, atlaserrors.ErrGitOperation)
	case exitCode >= 0:
		return fmt.Errorf("git %s failed (exit %d): %w", subcommand, exitCode, atlaserrors.ErrGitOperation)
	default:
		return fmt.Errorf("git %s failed: %w", subcommand, atlaserrors.ErrGitOperation)
	}
}
