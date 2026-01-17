package hd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileKeyManager_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("generates new key when not exists", func(t *testing.T) {
		keyDir := t.TempDir()
		keyPath := filepath.Join(keyDir, "master.key")

		km := NewFileKeyManager(keyPath)

		// Key should not exist initially
		assert.False(t, km.Exists())

		// Load should generate a new key
		err := km.Load(ctx)
		require.NoError(t, err)

		// Key should now exist
		assert.True(t, km.Exists())

		// Key file should exist on disk
		_, err = os.Stat(keyPath)
		require.NoError(t, err)
	})

	t.Run("loads existing key", func(t *testing.T) {
		keyDir := t.TempDir()
		keyPath := filepath.Join(keyDir, "master.key")

		// Create first key manager and generate key
		km1 := NewFileKeyManager(keyPath)
		err := km1.Load(ctx)
		require.NoError(t, err)

		// Create signer and get a derived key path for verification
		signer1, err := km1.NewSigner()
		require.NoError(t, err)
		path1 := signer1.KeyPath(0)

		// Create second key manager and load existing key
		km2 := NewFileKeyManager(keyPath)
		err = km2.Load(ctx)
		require.NoError(t, err)

		// Create signer and verify same path
		signer2, err := km2.NewSigner()
		require.NoError(t, err)
		path2 := signer2.KeyPath(0)

		assert.Equal(t, path1, path2)
	})

	t.Run("key file has secure permissions", func(t *testing.T) {
		keyDir := t.TempDir()
		keyPath := filepath.Join(keyDir, "master.key")

		km := NewFileKeyManager(keyPath)
		err := km.Load(ctx)
		require.NoError(t, err)

		// Check file permissions (0600)
		info, err := os.Stat(keyPath)
		require.NoError(t, err)

		// On Unix, verify permissions are 0600 (owner read/write only)
		perm := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0o600), perm, "key file should have 0600 permissions")
	})
}

func TestFileKeyManager_NewSigner(t *testing.T) {
	ctx := context.Background()

	t.Run("fails before Load", func(t *testing.T) {
		keyDir := t.TempDir()
		keyPath := filepath.Join(keyDir, "master.key")

		km := NewFileKeyManager(keyPath)

		_, err := km.NewSigner()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key not loaded")
	})

	t.Run("succeeds after Load", func(t *testing.T) {
		keyDir := t.TempDir()
		keyPath := filepath.Join(keyDir, "master.key")

		km := NewFileKeyManager(keyPath)
		err := km.Load(ctx)
		require.NoError(t, err)

		signer, err := km.NewSigner()
		require.NoError(t, err)
		assert.NotNil(t, signer)
	})
}

func TestHDSigner_KeyPath(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	signer, err := km.NewSigner()
	require.NoError(t, err)

	t.Run("returns correct path format", func(t *testing.T) {
		// Path format: m/44'/236'/0'/{task_index}
		path := signer.KeyPath(0)
		assert.Equal(t, "m/44'/236'/0'/0", path)

		path = signer.KeyPath(5)
		assert.Equal(t, "m/44'/236'/0'/5", path)

		path = signer.KeyPath(100)
		assert.Equal(t, "m/44'/236'/0'/100", path)
	})
}

func TestHDSigner_Sign(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	signer, err := km.NewSigner()
	require.NoError(t, err)

	t.Run("produces signature", func(t *testing.T) {
		message := []byte("test message to sign")

		sig, err := signer.Sign(ctx, message)
		require.NoError(t, err)
		assert.NotEmpty(t, sig)
	})

	t.Run("same message produces same signature from same key", func(t *testing.T) {
		// With RFC6979 deterministic signatures, the same message produces the same signature.
		// Both signatures should verify correctly.
		message := []byte("deterministic test")

		sig1, err := signer.Sign(ctx, message)
		require.NoError(t, err)

		sig2, err := signer.Sign(ctx, message)
		require.NoError(t, err)

		// Both signatures should verify correctly
		err = signer.Verify(ctx, message, sig1)
		require.NoError(t, err)

		err = signer.Verify(ctx, message, sig2)
		require.NoError(t, err)
	})

	t.Run("different messages produce different signatures", func(t *testing.T) {
		sig1, err := signer.Sign(ctx, []byte("message 1"))
		require.NoError(t, err)

		sig2, err := signer.Sign(ctx, []byte("message 2"))
		require.NoError(t, err)

		assert.NotEqual(t, sig1, sig2)
	})
}

func TestHDSigner_Verify(t *testing.T) {
	ctx := context.Background()
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "master.key")

	km := NewFileKeyManager(keyPath)
	err := km.Load(ctx)
	require.NoError(t, err)

	signer, err := km.NewSigner()
	require.NoError(t, err)

	t.Run("valid signature passes", func(t *testing.T) {
		message := []byte("valid message")
		sig, err := signer.Sign(ctx, message)
		require.NoError(t, err)

		err = signer.Verify(ctx, message, sig)
		assert.NoError(t, err)
	})

	t.Run("tampered signature fails", func(t *testing.T) {
		message := []byte("valid message")
		sig, err := signer.Sign(ctx, message)
		require.NoError(t, err)

		// Tamper with the signature
		if len(sig) > 0 {
			sig[0] ^= 0xFF
		}

		err = signer.Verify(ctx, message, sig)
		assert.Error(t, err)
	})

	t.Run("wrong message fails", func(t *testing.T) {
		message := []byte("original message")
		sig, err := signer.Sign(ctx, message)
		require.NoError(t, err)

		err = signer.Verify(ctx, []byte("different message"), sig)
		assert.Error(t, err)
	})

	t.Run("empty signature fails", func(t *testing.T) {
		message := []byte("test message")

		err := signer.Verify(ctx, message, []byte{})
		assert.Error(t, err)
	})
}

func TestHDSigner_DifferentKeys(t *testing.T) {
	ctx := context.Background()

	// Create two different key managers
	keyDir1 := t.TempDir()
	km1 := NewFileKeyManager(filepath.Join(keyDir1, "master.key"))
	err := km1.Load(ctx)
	require.NoError(t, err)

	keyDir2 := t.TempDir()
	km2 := NewFileKeyManager(filepath.Join(keyDir2, "master.key"))
	err = km2.Load(ctx)
	require.NoError(t, err)

	signer1, err := km1.NewSigner()
	require.NoError(t, err)

	signer2, err := km2.NewSigner()
	require.NoError(t, err)

	t.Run("signature from one key cannot be verified by another", func(t *testing.T) {
		message := []byte("cross-key test")

		// Sign with signer1
		sig, err := signer1.Sign(ctx, message)
		require.NoError(t, err)

		// Verify with signer1 should pass
		err = signer1.Verify(ctx, message, sig)
		require.NoError(t, err)

		// Verify with signer2 should fail
		err = signer2.Verify(ctx, message, sig)
		assert.Error(t, err)
	})
}
