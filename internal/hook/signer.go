// Package hook provides crash recovery and context persistence for ATLAS tasks.
package hook

import (
	"context"

	"github.com/mrz1836/atlas/internal/crypto"
	"github.com/mrz1836/atlas/internal/domain"
)

// ReceiptSigner handles cryptographic signing of validation receipts.
// It embeds crypto.Signer and adds receipt-specific functionality.
// Implementations can use different key derivation schemes (HD, simple, etc.).
type ReceiptSigner interface {
	crypto.Signer

	// SignReceipt signs a validation receipt and populates its Signature and KeyPath fields.
	// The taskIndex is used for key derivation (e.g., HD path component).
	SignReceipt(ctx context.Context, receipt *domain.ValidationReceipt, taskIndex uint32) error

	// VerifyReceipt checks that a receipt's signature is valid.
	// Returns nil if valid, error if invalid or verification fails.
	VerifyReceipt(ctx context.Context, receipt *domain.ValidationReceipt) error

	// KeyPath returns the derivation path used for a given task index.
	// Format depends on implementation (e.g., "m/44'/236'/0'/5/0" for HD).
	KeyPath(taskIndex uint32) string
}

// KeyManager handles master key lifecycle (generation, loading, storage).
// Abstracted to allow different storage backends or key types.
type KeyManager interface {
	// Load retrieves the master key, generating one if it doesn't exist.
	// Returns error if key cannot be loaded or generated.
	Load(ctx context.Context) error

	// Exists checks if a master key is already configured.
	Exists() bool

	// NewSigner creates a ReceiptSigner using the loaded master key.
	// Must call Load() first.
	NewSigner() (ReceiptSigner, error)
}
