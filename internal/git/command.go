// Package git provides Git operations for ATLAS.
// This file provides shared git command execution utilities.
package git

import (
	"bytes"
	"context"
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
	if err != nil {
		// Check for context cancellation
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Include stderr in error for debugging, wrap with ErrGitOperation
		if stderr.Len() > 0 {
			return "", fmt.Errorf("git %s failed: %s: %w", args[0], strings.TrimSpace(stderr.String()), atlaserrors.ErrGitOperation)
		}
		return "", fmt.Errorf("git %s failed: %w", args[0], atlaserrors.ErrGitOperation)
	}

	return strings.TrimSpace(stdout.String()), nil
}
