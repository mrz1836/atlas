// Package ctxutil provides context utility functions.
package ctxutil

import "context"

// Canceled checks if the context has been canceled.
// Returns the context error if canceled, nil otherwise.
// This is a common pattern used throughout the codebase to check
// for cancellation at function entry points.
func Canceled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
