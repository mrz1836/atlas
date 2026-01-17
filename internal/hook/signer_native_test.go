package hook

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/crypto/native"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNativeReceiptSigner_SignAndVerify(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	km := native.NewKeyManager(tmpDir)
	require.NoError(t, km.Load(context.Background()))

	signer, err := NewNativeReceiptSigner(km)
	require.NoError(t, err)

	// Create a receipt
	receipt := &domain.ValidationReceipt{
		ReceiptID:   "rcpt-123",
		StepName:    "test-step",
		Command:     "echo hello",
		ExitCode:    0,
		CompletedAt: time.Now().UTC(),
		StdoutHash:  "hash1",
		StderrHash:  "hash2",
	}

	// Sign it
	err = signer.SignReceipt(context.Background(), receipt, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, receipt.Signature)

	// Verify it
	err = signer.VerifyReceipt(context.Background(), receipt)
	require.NoError(t, err)

	// Verify tampering fails
	receipt.ExitCode = 1
	err = signer.VerifyReceipt(context.Background(), receipt)
	assert.Error(t, err)
}

func TestNativeReceiptSigner_KeyPath(t *testing.T) {
	signer := &NativeReceiptSigner{}
	assert.Equal(t, "native-ed25519-v1", signer.KeyPath(0))
}
