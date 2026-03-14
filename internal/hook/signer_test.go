package hook

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestMockSigner_Sign(t *testing.T) {
	signer := NewMockSigner()
	ctx := context.Background()

	sig, err := signer.Sign(ctx, []byte("test message"))
	require.NoError(t, err)
	assert.NotEmpty(t, sig)

	// Verify determinism - same input produces same output
	sig2, err := signer.Sign(ctx, []byte("test message"))
	require.NoError(t, err)
	assert.Equal(t, sig, sig2)
}

func TestMockSigner_Verify(t *testing.T) {
	t.Run("default passes", func(t *testing.T) {
		signer := NewMockSigner()
		ctx := context.Background()

		err := signer.Verify(ctx, []byte("message"), []byte("signature"))
		assert.NoError(t, err)
	})

	t.Run("fails when configured", func(t *testing.T) {
		signer := NewMockSigner()
		signer.FailVerification = true
		ctx := context.Background()

		err := signer.Verify(ctx, []byte("message"), []byte("signature"))
		assert.Error(t, err)
	})
}

func TestMockSigner_SignReceipt(t *testing.T) {
	signer := NewMockSigner()
	ctx := context.Background()

	receipt := &domain.ValidationReceipt{
		ReceiptID:   "rcpt-00000001",
		StepName:    "analyze",
		Command:     "magex lint",
		ExitCode:    0,
		StartedAt:   time.Now(),
		CompletedAt: time.Now().Add(12 * time.Second),
		Duration:    "12.3s",
		StdoutHash:  "a1b2c3d4",
		StderrHash:  "00000000",
	}

	err := signer.SignReceipt(ctx, receipt, 0)
	require.NoError(t, err)

	// Verify signature is populated
	assert.NotEmpty(t, receipt.Signature)

	// Verify receipt was tracked
	assert.Len(t, signer.SignedReceipts, 1)
	assert.Equal(t, receipt.ReceiptID, signer.SignedReceipts[0].ReceiptID)
}

func TestMockSigner_VerifyReceipt(t *testing.T) {
	t.Run("valid receipt", func(t *testing.T) {
		signer := NewMockSigner()
		ctx := context.Background()

		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-00000001",
			Command:     "magex lint",
			ExitCode:    0,
			CompletedAt: time.Now(),
			StdoutHash:  "a1b2c3d4",
			StderrHash:  "00000000",
		}

		// Sign it first
		err := signer.SignReceipt(ctx, receipt, 0)
		require.NoError(t, err)

		// Verify it
		err = signer.VerifyReceipt(ctx, receipt)
		assert.NoError(t, err)
	})

	t.Run("tampered receipt fails when configured", func(t *testing.T) {
		signer := NewMockSigner()
		signer.FailVerification = true
		ctx := context.Background()

		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-00000001",
			Command:     "magex lint",
			ExitCode:    0,
			CompletedAt: time.Now(),
			Signature:   "tampered",
		}

		err := signer.VerifyReceipt(ctx, receipt)
		assert.Error(t, err)
	})
}

func TestMockSigner_KeyPath(t *testing.T) {
	signer := NewMockSigner()

	tests := []struct {
		taskIndex uint32
		expected  string
	}{
		{0, "native-ed25519-v1"},
		{1, "native-ed25519-v1"},
		{5, "native-ed25519-v1"},
		{100, "native-ed25519-v1"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			path := signer.KeyPath(tt.taskIndex)
			assert.Equal(t, tt.expected, path)
		})
	}
}

// TestReceiptSignerInterface verifies that MockSigner implements ReceiptSigner.
func TestReceiptSignerInterface(_ *testing.T) {
	var _ ReceiptSigner = (*MockSigner)(nil)
}
