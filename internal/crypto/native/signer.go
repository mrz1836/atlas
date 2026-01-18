// Package native provides Ed25519 signing using standard crypto libraries.
package native

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	// ErrInvalidKeySize is returned when the loaded key has an invalid size.
	ErrInvalidKeySize = errors.New("invalid key size")

	// ErrMasterKeyNotLoaded is returned when attempting to create a signer without loading the key.
	ErrMasterKeyNotLoaded = errors.New("master key not loaded")

	// ErrInvalidSignature is returned when signature verification fails.
	ErrInvalidSignature = errors.New("invalid signature")
)

// KeyManager implements the hook.KeyManager interface using standard Ed25519 keys.
type KeyManager struct {
	keyPath string
	mu      sync.RWMutex
	privKey ed25519.PrivateKey // Cached private key
}

// NewKeyManager creates a new KeyManager that stores keys in the given directory.
func NewKeyManager(keyDir string) *KeyManager {
	return &KeyManager{
		keyPath: filepath.Join(keyDir, "master.key"),
	}
}

// Load loads the master key from disk, generating one if it doesn't exist.
func (km *KeyManager) Load(_ context.Context) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	if km.privKey != nil {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(km.keyPath), 0o700); err != nil {
		return fmt.Errorf("creating key directory: %w", err)
	}

	// Try to read existing key
	data, err := os.ReadFile(km.keyPath)
	if os.IsNotExist(err) {
		// Generate new key
		_, priv, genErr := ed25519.GenerateKey(rand.Reader)
		if genErr != nil {
			return fmt.Errorf("generating ed25519 key: %w", genErr)
		}

		// Save hex-encoded private key (seed + pubkey)
		// Ed25519 private keys are 64 bytes (32 byte seed + 32 byte pubkey)
		hexKey := hex.EncodeToString(priv)
		if writeErr := os.WriteFile(km.keyPath, []byte(hexKey), 0o600); writeErr != nil {
			return fmt.Errorf("saving master key: %w", writeErr)
		}
		km.privKey = priv
		return nil
	} else if err != nil {
		return fmt.Errorf("reading master key: %w", err)
	}

	// Decode existing key
	decoded, err := hex.DecodeString(string(data))
	if err != nil {
		return fmt.Errorf("decoding master key hex: %w", err)
	}

	if len(decoded) != ed25519.PrivateKeySize {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidKeySize, ed25519.PrivateKeySize, len(decoded))
	}

	km.privKey = ed25519.PrivateKey(decoded)
	return nil
}

// Exists checks if the master key exists on disk.
func (km *KeyManager) Exists() bool {
	_, err := os.Stat(km.keyPath)
	return err == nil
}

// NewSigner creates a new Signer using the loaded key.
func (km *KeyManager) NewSigner() (*Signer, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.privKey == nil {
		return nil, ErrMasterKeyNotLoaded
	}

	return &Signer{privKey: km.privKey}, nil
}

// Signer implements the crypto.Signer interface using specific Ed25519 key.
type Signer struct {
	privKey ed25519.PrivateKey
}

// Sign signs the message using Ed25519.
func (s *Signer) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(s.privKey, message), nil
}

// Verify checks the signature using Ed25519.
func (s *Signer) Verify(_ context.Context, message, signature []byte) error {
	pubKey := s.privKey.Public().(ed25519.PublicKey)
	if !ed25519.Verify(pubKey, message, signature) {
		return ErrInvalidSignature
	}
	return nil
}
