package hook

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/mrz1836/atlas/internal/crypto/hd"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// HDReceiptSigner implements ReceiptSigner using HD key derivation.
// It wraps a Signer and provides receipt-specific signing operations.
type HDReceiptSigner struct {
	signer *hd.Signer
}

// NewHDReceiptSigner creates a new HDReceiptSigner.
func NewHDReceiptSigner(signer *hd.Signer) *HDReceiptSigner {
	return &HDReceiptSigner{
		signer: signer,
	}
}

// Sign signs the given message and returns the signature.
// Implements crypto.Signer interface.
func (s *HDReceiptSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	return s.signer.Sign(ctx, message)
}

// Verify checks that a signature is valid for the given message.
// Implements crypto.Signer interface.
func (s *HDReceiptSigner) Verify(ctx context.Context, message, signature []byte) error {
	return s.signer.Verify(ctx, message, signature)
}

// SignReceipt signs a validation receipt and populates its Signature and KeyPath fields.
// The taskIndex is used for key derivation.
func (s *HDReceiptSigner) SignReceipt(ctx context.Context, receipt *domain.ValidationReceipt, taskIndex uint32) error {
	// Build the message to sign
	message := FormatReceiptMessage(receipt)

	// Derive task-specific signer
	taskSigner, err := s.signer.DeriveForTask(taskIndex)
	if err != nil {
		return fmt.Errorf("failed to derive task key: %w", err)
	}

	// Sign the message
	sig, err := taskSigner.Sign(ctx, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to sign receipt: %w", err)
	}

	// Populate receipt fields
	receipt.KeyPath = s.KeyPath(taskIndex)
	receipt.Signature = hex.EncodeToString(sig)

	return nil
}

// VerifyReceipt checks that a receipt's signature is valid.
func (s *HDReceiptSigner) VerifyReceipt(ctx context.Context, receipt *domain.ValidationReceipt) error {
	if receipt.Signature == "" {
		return atlaserrors.ErrReceiptMissingSignature
	}

	// Decode hex signature
	sig, err := hex.DecodeString(receipt.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Parse task index from key path
	taskIndex, err := parseTaskIndexFromKeyPath(receipt.KeyPath)
	if err != nil {
		return fmt.Errorf("failed to parse key path: %w", err)
	}

	// Derive task-specific signer
	taskSigner, err := s.signer.DeriveForTask(taskIndex)
	if err != nil {
		return fmt.Errorf("failed to derive task key: %w", err)
	}

	// Build the message
	message := FormatReceiptMessage(receipt)

	// Verify
	return taskSigner.Verify(ctx, []byte(message), sig)
}

// KeyPath returns the derivation path used for a given task index.
func (s *HDReceiptSigner) KeyPath(taskIndex uint32) string {
	return s.signer.KeyPath(taskIndex)
}

// FormatReceiptMessage creates a deterministic message string from a receipt.
// Format: {receipt_id}|{command}|{exit_code}|{stdout_hash}|{stderr_hash}|{completed_at_unix}
func FormatReceiptMessage(receipt *domain.ValidationReceipt) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%d",
		receipt.ReceiptID,
		receipt.Command,
		receipt.ExitCode,
		receipt.StdoutHash,
		receipt.StderrHash,
		receipt.CompletedAt.Unix(),
	)
}

// parseTaskIndexFromKeyPath extracts the task index from a key path.
// Path format: m/44'/236'/0'/{task_index}
func parseTaskIndexFromKeyPath(keyPath string) (uint32, error) {
	var purpose, coinType, account, taskIndex uint32
	n, err := fmt.Sscanf(keyPath, "m/%d'/%d'/%d'/%d", &purpose, &coinType, &account, &taskIndex)
	if err != nil || n != 4 {
		return 0, fmt.Errorf("%w: %s", atlaserrors.ErrInvalidKeyPath, keyPath)
	}
	return taskIndex, nil
}
