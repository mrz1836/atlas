// Package git provides Git operations for ATLAS.
// This file provides error sentinel re-exports from internal/errors.
package git

import (
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ErrGitOperation is re-exported from internal/errors for convenience.
// Use errors.Is(err, ErrGitOperation) to check for git operation failures.
var ErrGitOperation = atlaserrors.ErrGitOperation

// ErrBranchExists is re-exported from internal/errors for convenience.
// Returned when attempting to create a branch that already exists.
var ErrBranchExists = atlaserrors.ErrBranchExists

// ErrNotGitRepo is re-exported from internal/errors for convenience.
// Returned when the path is not a git repository.
var ErrNotGitRepo = atlaserrors.ErrNotGitRepo
