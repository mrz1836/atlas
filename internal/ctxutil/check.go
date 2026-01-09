// Package ctxutil provides context utility functions.
package ctxutil

import "context"

// Canceled checks if the context has been canceled or exceeded its deadline.
// Returns the context error if done (Canceled or DeadlineExceeded), nil otherwise.
// This is a common pattern used throughout the codebase to check
// for cancellation at function entry points.
//
// The implementation directly returns ctx.Err() because it already returns nil
// if Done is not yet closed - no select with default case is needed.
func Canceled(ctx context.Context) error {
	return ctx.Err()
}
