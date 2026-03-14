package native

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKeyManager(t *testing.T) {
	t.Run("creates KeyManager with correct keyPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		require.NotNil(t, km)
		assert.Equal(t, filepath.Join(tmpDir, "master.key"), km.keyPath)
	})
}

func TestKeyManager_Load(t *testing.T) {
	t.Run("generates new key if none exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		err := km.Load(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, km.privKey)

		// Verify key was written to disk
		data, err := os.ReadFile(km.keyPath)
		require.NoError(t, err)
		assert.NotEmpty(t, data)

		// Verify it's valid hex-encoded ed25519 key
		decoded, err := hex.DecodeString(string(data))
		require.NoError(t, err)
		assert.Len(t, decoded, ed25519.PrivateKeySize)
	})

	t.Run("loads existing key from disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// First load - generates key
		err := km.Load(context.Background())
		require.NoError(t, err)
		firstKey := km.privKey

		// Create new KeyManager instance to simulate restart
		km2 := NewKeyManager(tmpDir)
		err = km2.Load(context.Background())
		require.NoError(t, err)

		// Keys should be identical
		assert.Equal(t, firstKey, km2.privKey)
	})

	t.Run("does not reload if key already loaded", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		err := km.Load(context.Background())
		require.NoError(t, err)
		firstKey := km.privKey

		// Load again
		err = km.Load(context.Background())
		require.NoError(t, err)

		// Should be the same instance
		assert.Equal(t, firstKey, km.privKey)
	})

	t.Run("returns error for invalid key size", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Create directory
		require.NoError(t, os.MkdirAll(filepath.Dir(km.keyPath), 0o700))

		// Write invalid key (wrong size)
		invalidKey := hex.EncodeToString([]byte("too-short"))
		err := os.WriteFile(km.keyPath, []byte(invalidKey), 0o600)
		require.NoError(t, err)

		// Try to load
		err = km.Load(context.Background())
		assert.ErrorIs(t, err, ErrInvalidKeySize)
	})

	t.Run("returns error for invalid hex encoding", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Create directory
		require.NoError(t, os.MkdirAll(filepath.Dir(km.keyPath), 0o700))

		// Write invalid hex
		err := os.WriteFile(km.keyPath, []byte("not-valid-hex!!!"), 0o600)
		require.NoError(t, err)

		// Try to load
		err = km.Load(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decoding master key hex")
	})

	t.Run("creates directory if it doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "keys")
		km := NewKeyManager(nestedDir)

		err := km.Load(context.Background())
		require.NoError(t, err)

		// Verify directory was created
		info, err := os.Stat(filepath.Dir(km.keyPath))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("handles read-only directory error", func(t *testing.T) {
		tmpDir := t.TempDir()
		readOnlyDir := filepath.Join(tmpDir, "readonly")

		// Create read-only directory
		require.NoError(t, os.Mkdir(readOnlyDir, 0o500))

		// Try to create key in read-only subdirectory
		km := NewKeyManager(filepath.Join(readOnlyDir, "subdir"))
		err := km.Load(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating key directory")
	})

	t.Run("handles write error when saving new key", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Make the directory read-only to prevent writing
		require.NoError(t, os.Chmod(tmpDir, 0o500)) //nolint:gosec // G302: intentionally testing error handling
		defer func() {
			_ = os.Chmod(tmpDir, 0o700) //nolint:gosec // G302: cleanup after test
		}()

		err := km.Load(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saving master key")
	})

	t.Run("handles file permission error when reading key", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Create the directory and a key file
		require.NoError(t, os.MkdirAll(filepath.Dir(km.keyPath), 0o700))
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		hexKey := hex.EncodeToString(priv)
		require.NoError(t, os.WriteFile(km.keyPath, []byte(hexKey), 0o600))

		// Make file unreadable
		require.NoError(t, os.Chmod(km.keyPath, 0o000))
		defer func() {
			_ = os.Chmod(km.keyPath, 0o600) // Cleanup - ignore error as file may not exist
		}()

		err = km.Load(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading master key")
	})
}

func TestKeyManager_Exists(t *testing.T) {
	t.Run("returns false when key doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		assert.False(t, km.Exists())
	})

	t.Run("returns true when key exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Generate key
		err := km.Load(context.Background())
		require.NoError(t, err)

		assert.True(t, km.Exists())
	})

	t.Run("returns true for existing key after restart", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Generate key
		err := km.Load(context.Background())
		require.NoError(t, err)

		// Create new KeyManager instance
		km2 := NewKeyManager(tmpDir)
		assert.True(t, km2.Exists())
	})
}

func TestKeyManager_NewSigner(t *testing.T) {
	t.Run("creates signer with loaded key", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		err := km.Load(context.Background())
		require.NoError(t, err)

		signer, err := km.NewSigner()
		require.NoError(t, err)
		assert.NotNil(t, signer)
		assert.Equal(t, km.privKey, signer.privKey)
	})

	t.Run("returns error when key not loaded", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		signer, err := km.NewSigner()
		require.ErrorIs(t, err, ErrMasterKeyNotLoaded)
		assert.Nil(t, signer)
	})

	t.Run("multiple signers share the same key", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		err := km.Load(context.Background())
		require.NoError(t, err)

		signer1, err := km.NewSigner()
		require.NoError(t, err)

		signer2, err := km.NewSigner()
		require.NoError(t, err)

		assert.Equal(t, signer1.privKey, signer2.privKey)
	})
}

func TestSigner_Sign(t *testing.T) {
	t.Run("signs message successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		message := []byte("test message")
		signature, err := signer.Sign(context.Background(), message)
		require.NoError(t, err)
		assert.NotEmpty(t, signature)
		assert.Len(t, signature, ed25519.SignatureSize)
	})

	t.Run("produces different signatures for different messages", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		sig1, err := signer.Sign(context.Background(), []byte("message 1"))
		require.NoError(t, err)

		sig2, err := signer.Sign(context.Background(), []byte("message 2"))
		require.NoError(t, err)

		assert.NotEqual(t, sig1, sig2)
	})

	t.Run("produces same signature for same message", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		message := []byte("consistent message")
		sig1, err := signer.Sign(context.Background(), message)
		require.NoError(t, err)

		sig2, err := signer.Sign(context.Background(), message)
		require.NoError(t, err)

		assert.Equal(t, sig1, sig2)
	})

	t.Run("signs empty message", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		signature, err := signer.Sign(context.Background(), []byte{})
		require.NoError(t, err)
		assert.NotEmpty(t, signature)
	})
}

func TestSigner_Verify(t *testing.T) {
	t.Run("verifies valid signature", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		message := []byte("test message")
		signature, err := signer.Sign(context.Background(), message)
		require.NoError(t, err)

		err = signer.Verify(context.Background(), message, signature)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid signature", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		message := []byte("test message")
		invalidSig := make([]byte, ed25519.SignatureSize)

		err = signer.Verify(context.Background(), message, invalidSig)
		assert.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("rejects signature for different message", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		originalMessage := []byte("original message")
		signature, err := signer.Sign(context.Background(), originalMessage)
		require.NoError(t, err)

		tamperedMessage := []byte("tampered message")
		err = signer.Verify(context.Background(), tamperedMessage, signature)
		assert.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("rejects signature from different key", func(t *testing.T) {
		tmpDir1 := t.TempDir()
		km1 := NewKeyManager(tmpDir1)
		require.NoError(t, km1.Load(context.Background()))

		tmpDir2 := t.TempDir()
		km2 := NewKeyManager(tmpDir2)
		require.NoError(t, km2.Load(context.Background()))

		signer1, err := km1.NewSigner()
		require.NoError(t, err)

		signer2, err := km2.NewSigner()
		require.NoError(t, err)

		message := []byte("test message")
		signature, err := signer1.Sign(context.Background(), message)
		require.NoError(t, err)

		// Try to verify with different signer's key
		err = signer2.Verify(context.Background(), message, signature)
		assert.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("verifies empty message signature", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		message := []byte{}
		signature, err := signer.Sign(context.Background(), message)
		require.NoError(t, err)

		err = signer.Verify(context.Background(), message, signature)
		assert.NoError(t, err)
	})
}

func TestKeyManager_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent Load calls", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := km.Load(context.Background())
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
		assert.NotNil(t, km.privKey)
	})

	t.Run("concurrent NewSigner calls", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		var wg sync.WaitGroup
		numGoroutines := 10
		signers := make([]*Signer, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				signer, err := km.NewSigner()
				assert.NoError(t, err)
				signers[idx] = signer
			}(i)
		}

		wg.Wait()

		// All signers should have the same key
		for i := 1; i < numGoroutines; i++ {
			assert.Equal(t, signers[0].privKey, signers[i].privKey)
		}
	})

	t.Run("concurrent Sign operations", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)
		require.NoError(t, km.Load(context.Background()))

		signer, err := km.NewSigner()
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				message := []byte("concurrent message")
				signature, err := signer.Sign(context.Background(), message)
				assert.NoError(t, err)
				assert.NotEmpty(t, signature)
			}()
		}

		wg.Wait()
	})
}

func TestSignAndVerifyIntegration(t *testing.T) {
	t.Run("full sign and verify workflow", func(t *testing.T) {
		tmpDir := t.TempDir()
		km := NewKeyManager(tmpDir)

		// Load key
		err := km.Load(context.Background())
		require.NoError(t, err)
		require.True(t, km.Exists())

		// Create signer
		signer, err := km.NewSigner()
		require.NoError(t, err)

		// Sign multiple messages
		messages := [][]byte{
			[]byte("message 1"),
			[]byte("message 2"),
			[]byte("message 3"),
		}

		signatures := make([][]byte, len(messages))
		for i, msg := range messages {
			sig, signErr := signer.Sign(context.Background(), msg)
			require.NoError(t, signErr)
			signatures[i] = sig
		}

		// Verify all signatures
		for i, msg := range messages {
			verifyErr := signer.Verify(context.Background(), msg, signatures[i])
			require.NoError(t, verifyErr)
		}

		// Cross-verify should fail
		err = signer.Verify(context.Background(), messages[0], signatures[1])
		require.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("persist and reload workflow", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Phase 1: Create and sign
		km1 := NewKeyManager(tmpDir)
		require.NoError(t, km1.Load(context.Background()))
		signer1, err := km1.NewSigner()
		require.NoError(t, err)

		message := []byte("persistent message")
		signature, err := signer1.Sign(context.Background(), message)
		require.NoError(t, err)

		// Phase 2: Reload and verify
		km2 := NewKeyManager(tmpDir)
		require.NoError(t, km2.Load(context.Background()))
		signer2, err := km2.NewSigner()
		require.NoError(t, err)

		err = signer2.Verify(context.Background(), message, signature)
		assert.NoError(t, err)
	})
}
