package hook

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/crypto/hd"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestHDReceiptSigner_SignReceipt(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := hd.NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	hdSigner, err := km.NewSigner()
	require.NoError(t, err)

	signer := NewHDReceiptSigner(hdSigner)

	t.Run("signs receipt and populates fields", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-12345678",
			StepName:    "validate",
			Command:     "magex lint",
			ExitCode:    0,
			StartedAt:   time.Now().Add(-10 * time.Second),
			CompletedAt: time.Now(),
			Duration:    "10.0s",
			StdoutHash:  "abc123def456",
			StderrHash:  "000000000000",
		}

		err := signer.SignReceipt(ctx, receipt, 0)
		require.NoError(t, err)

		// Should populate KeyPath
		assert.NotEmpty(t, receipt.KeyPath)
		assert.Contains(t, receipt.KeyPath, "m/44'/236'/0'")

		// Should populate Signature
		assert.NotEmpty(t, receipt.Signature)
	})

	t.Run("different task indices produce different key paths", func(t *testing.T) {
		receipt1 := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-1",
			Command:     "test",
			CompletedAt: time.Now(),
		}
		receipt2 := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-2",
			Command:     "test",
			CompletedAt: time.Now(),
		}

		err := signer.SignReceipt(ctx, receipt1, 0)
		require.NoError(t, err)

		err = signer.SignReceipt(ctx, receipt2, 5)
		require.NoError(t, err)

		assert.NotEqual(t, receipt1.KeyPath, receipt2.KeyPath)
	})
}

func TestHDReceiptSigner_VerifyReceipt(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := hd.NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	hdSigner, err := km.NewSigner()
	require.NoError(t, err)

	signer := NewHDReceiptSigner(hdSigner)

	t.Run("valid receipt verifies", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-verify-test",
			StepName:    "test",
			Command:     "go test ./...",
			ExitCode:    0,
			CompletedAt: time.Now(),
			StdoutHash:  "abc123",
			StderrHash:  "def456",
		}

		err := signer.SignReceipt(ctx, receipt, 0)
		require.NoError(t, err)

		// Verify should pass
		err = signer.VerifyReceipt(ctx, receipt)
		assert.NoError(t, err)
	})

	t.Run("tampered receipt fails verification", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-tamper-test",
			StepName:    "test",
			Command:     "go test ./...",
			ExitCode:    0,
			CompletedAt: time.Now(),
			StdoutHash:  "original",
			StderrHash:  "original",
		}

		err := signer.SignReceipt(ctx, receipt, 0)
		require.NoError(t, err)

		// Tamper with the receipt
		receipt.ExitCode = 1

		// Verify should fail
		err = signer.VerifyReceipt(ctx, receipt)
		assert.Error(t, err)
	})

	t.Run("missing signature fails", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-no-sig",
			Command:     "test",
			CompletedAt: time.Now(),
			// No signature
		}

		err := signer.VerifyReceipt(ctx, receipt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing signature")
	})

	t.Run("invalid signature fails", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-invalid-sig",
			Command:     "test",
			CompletedAt: time.Now(),
			Signature:   "invalid-hex-signature",
		}

		err := signer.VerifyReceipt(ctx, receipt)
		assert.Error(t, err)
	})
}

func TestHDReceiptSigner_KeyPath(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := hd.NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	hdSigner, err := km.NewSigner()
	require.NoError(t, err)

	signer := NewHDReceiptSigner(hdSigner)

	t.Run("returns BIP44 path format", func(t *testing.T) {
		path := signer.KeyPath(0)
		assert.Equal(t, "m/44'/236'/0'/0", path)

		path = signer.KeyPath(42)
		assert.Equal(t, "m/44'/236'/0'/42", path)
	})
}

func TestFormatReceiptMessage(t *testing.T) {
	t.Run("produces deterministic format", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-001",
			Command:     "magex lint",
			ExitCode:    0,
			StdoutHash:  "abc123",
			StderrHash:  "def456",
			CompletedAt: time.Unix(1737129933, 0),
		}

		msg := FormatReceiptMessage(receipt)
		expected := "rcpt-001|magex lint|0|abc123|def456|1737129933"
		assert.Equal(t, expected, msg)
	})

	t.Run("same receipt produces same message", func(t *testing.T) {
		receipt := &domain.ValidationReceipt{
			ReceiptID:   "rcpt-test",
			Command:     "test",
			ExitCode:    0,
			StdoutHash:  "hash1",
			StderrHash:  "hash2",
			CompletedAt: time.Unix(1000000000, 0),
		}

		msg1 := FormatReceiptMessage(receipt)
		msg2 := FormatReceiptMessage(receipt)

		assert.Equal(t, msg1, msg2)
	})
}

func TestHDReceiptSigner_Sign(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := hd.NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	hdSigner, err := km.NewSigner()
	require.NoError(t, err)

	signer := NewHDReceiptSigner(hdSigner)

	t.Run("implements crypto.Signer interface", func(t *testing.T) {
		message := []byte("test message")

		sig, err := signer.Sign(ctx, message)
		require.NoError(t, err)
		assert.NotEmpty(t, sig)

		err = signer.Verify(ctx, message, sig)
		assert.NoError(t, err)
	})
}
