package hook

import (
	"context"
	"fmt"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// MockSigner is a test implementation of ReceiptSigner.
// It provides deterministic signatures for testing hook logic without crypto overhead.
type MockSigner struct {
	// SignFunc allows customizing sign behavior in tests.
	SignFunc func(ctx context.Context, message []byte) ([]byte, error)

	// VerifyFunc allows customizing verify behavior in tests.
	// If nil, verification always passes.
	VerifyFunc func(ctx context.Context, message, signature []byte) error

	// FailVerification when true causes Verify to return an error.
	FailVerification bool

	// SignedReceipts tracks all receipts that were signed.
	SignedReceipts []*domain.ValidationReceipt
}

// NewMockSigner creates a new MockSigner with default behavior.
func NewMockSigner() *MockSigner {
	return &MockSigner{
		SignedReceipts: make([]*domain.ValidationReceipt, 0),
	}
}

// Sign returns a deterministic test signature.
func (m *MockSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if m.SignFunc != nil {
		return m.SignFunc(ctx, message)
	}
	// Return deterministic mock signature
	return []byte("mock-signature-" + string(message[:min(8, len(message))])), nil
}

// Verify checks the signature (configurable for testing).
func (m *MockSigner) Verify(ctx context.Context, message, signature []byte) error {
	if m.VerifyFunc != nil {
		return m.VerifyFunc(ctx, message, signature)
	}
	if m.FailVerification {
		return atlaserrors.ErrMockVerificationFailed
	}
	return nil
}

// SignReceipt signs a validation receipt with mock values.
func (m *MockSigner) SignReceipt(ctx context.Context, receipt *domain.ValidationReceipt, taskIndex uint32) error {
	// Build signature message
	msg := m.buildSignatureMessage(receipt)

	sig, err := m.Sign(ctx, []byte(msg))
	if err != nil {
		return err
	}

	receipt.Signature = fmt.Sprintf("%x", sig)
	receipt.KeyPath = m.KeyPath(taskIndex)

	// Track signed receipts for assertions
	m.SignedReceipts = append(m.SignedReceipts, receipt)

	return nil
}

// VerifyReceipt verifies a receipt's signature.
func (m *MockSigner) VerifyReceipt(ctx context.Context, receipt *domain.ValidationReceipt) error {
	if m.FailVerification {
		return fmt.Errorf("%w for receipt %s", atlaserrors.ErrMockVerificationFailed, receipt.ReceiptID)
	}

	msg := m.buildSignatureMessage(receipt)
	return m.Verify(ctx, []byte(msg), []byte(receipt.Signature))
}

// KeyPath returns a mock key path.
func (m *MockSigner) KeyPath(taskIndex uint32) string {
	return fmt.Sprintf("m/44'/236'/0'/%d/0", taskIndex)
}

// buildSignatureMessage creates the message to sign for a receipt.
func (m *MockSigner) buildSignatureMessage(receipt *domain.ValidationReceipt) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%d",
		receipt.ReceiptID,
		receipt.Command,
		receipt.ExitCode,
		receipt.StdoutHash,
		receipt.StderrHash,
		receipt.CompletedAt.Unix(),
	)
}
