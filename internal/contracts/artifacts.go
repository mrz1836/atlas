// Package contracts provides shared interfaces and utilities to avoid circular dependencies.
package contracts

import "context"

// ArtifactStore is the interface for task artifact storage.
// This interface is defined here to avoid circular imports between
// internal/git and internal/task packages.
type ArtifactStore interface {
	SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error
}
