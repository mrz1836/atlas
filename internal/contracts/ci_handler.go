// Package contracts provides shared interfaces and utilities to avoid circular dependencies.
package contracts

// CIFailureHandler abstracts CI failure handling to avoid import cycles.
// The concrete implementation is task.CIFailureHandler.
type CIFailureHandler interface {
	// HasHandler returns true if a failure handler is available.
	HasHandler() bool
}
