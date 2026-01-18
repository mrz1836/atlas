// Package crypto provides cryptographic interfaces and utilities for ATLAS.
// This package defines interfaces that can be implemented by different crypto backends.
package crypto

import "context"

// Signer provides signing capabilities.
// Implementations must be deterministic: signing the same message twice produces the same signature.
type Signer interface {
	// Sign signs the given message and returns the signature.
	// Returns error if signing fails.
	Sign(ctx context.Context, message []byte) ([]byte, error)

	// Verify checks that a signature is valid for the given message.
	// Returns nil if valid, error if invalid or verification fails.
	Verify(ctx context.Context, message, signature []byte) error
}

// Verifier provides signature verification capabilities.
// This is a read-only subset of Signer for consumers that only need to verify.
type Verifier interface {
	// Verify checks that a signature is valid for the given message.
	// Returns nil if valid, error if invalid or verification fails.
	Verify(ctx context.Context, message, signature []byte) error
}
