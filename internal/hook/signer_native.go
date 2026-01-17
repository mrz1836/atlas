package hook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/mrz1836/atlas/internal/crypto/native"
	"github.com/mrz1836/atlas/internal/domain"
)

// NativeReceiptSigner implements ReceiptSigner using standard Ed25519 keys.
type NativeReceiptSigner struct {
	km *native.KeyManager
}

// NewNativeReceiptSigner creates a new receipt signer backed by native crypto.
func NewNativeReceiptSigner(km *native.KeyManager) (*NativeReceiptSigner, error) {
	return &NativeReceiptSigner{km: km}, nil
}

// Sign implements crypto.Signer.
func (s *NativeReceiptSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	// Delegate to the key manager's internal signer mechanism
	// We need to access the private key, which is simpler to do by exposing a Signer method on KeyManager
	// or creating a transient signer.
	// For now, let's create a transient signer from the KeyManager.
	signer, err := s.km.NewSigner()
	if err != nil {
		return nil, err
	}
	return signer.Sign(ctx, message)
}

// Verify implements crypto.Signer (and Verifier).
func (s *NativeReceiptSigner) Verify(ctx context.Context, message, signature []byte) error {
	signer, err := s.km.NewSigner()
	if err != nil {
		return err
	}
	return signer.Verify(ctx, message, signature)
}

// SignReceipt signs the receipt and populates the Signature field.
func (s *NativeReceiptSigner) SignReceipt(ctx context.Context, receipt *domain.ValidationReceipt, _ uint32) error {
	msg := formatReceiptMessage(receipt)

	// Create hash of the message to sign (standard practice, though Ed25519 hashes internally too)
	// We sign the SHA256 of the pipe-delimited string to keep payload small and uniform
	hash := sha256.Sum256([]byte(msg))

	sig, err := s.Sign(ctx, hash[:])
	if err != nil {
		return err
	}

	receipt.Signature = hex.EncodeToString(sig)
	return nil
}

// VerifyReceipt verifies the receipt's signature.
func (s *NativeReceiptSigner) VerifyReceipt(ctx context.Context, receipt *domain.ValidationReceipt) error {
	msg := formatReceiptMessage(receipt)
	hash := sha256.Sum256([]byte(msg))

	sigBytes, err := hex.DecodeString(receipt.Signature)
	if err != nil {
		return fmt.Errorf("decoding signature: %w", err)
	}

	return s.Verify(ctx, hash[:], sigBytes)
}

// KeyPath returns a static identifier for native keys since we don't use HD paths.
func (s *NativeReceiptSigner) KeyPath(_ uint32) string {
	return "native-ed25519-v1"
}

// formatReceiptMessage creates the deterministic string to sign.
// Format: {receipt_id}|{command}|{exit_code}|{stdout_hash}|{stderr_hash}|{completed_at_unix}
func formatReceiptMessage(receipt *domain.ValidationReceipt) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%d",
		receipt.ReceiptID,
		receipt.Command,
		receipt.ExitCode,
		receipt.StdoutHash,
		receipt.StderrHash,
		receipt.CompletedAt.Unix(),
	)
}
