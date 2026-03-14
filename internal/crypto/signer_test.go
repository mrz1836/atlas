package crypto

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignerImplementsVerifier verifies that Signer interface embeds Verifier behavior.
func TestSignerImplementsVerifier(_ *testing.T) {
	// This is a compile-time check that any Signer can be used as a Verifier
	var _ Verifier = (Signer)(nil)
}

// mockSigner is a simple mock implementation for testing the interface contract.
type mockSigner struct {
	signFunc   func(ctx context.Context, message []byte) ([]byte, error)
	verifyFunc func(ctx context.Context, message, signature []byte) error
}

func (m *mockSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if m.signFunc != nil {
		return m.signFunc(ctx, message)
	}
	return []byte("mock-signature"), nil
}

func (m *mockSigner) Verify(ctx context.Context, message, signature []byte) error {
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, message, signature)
	}
	return nil
}

// TestSignerInterface verifies the interface contract using a mock implementation.
func TestSignerInterface(t *testing.T) {
	t.Run("Sign returns signature", func(t *testing.T) {
		signer := &mockSigner{}
		ctx := context.Background()

		sig, err := signer.Sign(ctx, []byte("test message"))
		require.NoError(t, err)
		assert.NotEmpty(t, sig)
	})

	t.Run("Verify accepts valid signature", func(t *testing.T) {
		signer := &mockSigner{}
		ctx := context.Background()

		err := signer.Verify(ctx, []byte("test message"), []byte("valid-sig"))
		assert.NoError(t, err)
	})

	t.Run("Signer can be used as Verifier", func(t *testing.T) {
		signer := &mockSigner{}
		ctx := context.Background()

		// Use signer as verifier
		var verifier Verifier = signer
		err := verifier.Verify(ctx, []byte("message"), []byte("sig"))
		assert.NoError(t, err)
	})
}
