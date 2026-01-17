// Package hd provides hierarchical deterministic key derivation and signing.
// It uses BIP32/BIP44 standards for key derivation.
package hd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	bip32 "github.com/bsv-blockchain/go-sdk/compat/bip32"
	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	chaincfg "github.com/bsv-blockchain/go-sdk/transaction/chaincfg"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

const (
	// BIP44Purpose is the standard BIP44 purpose for HD wallets.
	BIP44Purpose = 44

	// ATLASCoinType is the coin type for ATLAS (arbitrary, from spec).
	ATLASCoinType = 236

	// ReceiptAccount is the account index for receipt signing.
	ReceiptAccount = 0
)

// FileKeyManager manages master key storage and loading from a file.
type FileKeyManager struct {
	keyPath   string
	masterKey *bip32.ExtendedKey
	mu        sync.RWMutex
}

// NewFileKeyManager creates a new FileKeyManager.
func NewFileKeyManager(keyPath string) *FileKeyManager {
	return &FileKeyManager{
		keyPath: keyPath,
	}
}

// Load retrieves the master key, generating one if it doesn't exist.
func (km *FileKeyManager) Load(_ context.Context) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Check if key file exists
	if _, err := os.Stat(km.keyPath); os.IsNotExist(err) {
		// Generate new key
		return km.generate()
	}

	// Load existing key
	data, err := os.ReadFile(km.keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	// Deserialize the extended key
	key, err := bip32.NewKeyFromString(string(data))
	if err != nil {
		return fmt.Errorf("failed to parse key: %w", err)
	}

	km.masterKey = key
	return nil
}

// Exists checks if a master key is already configured.
func (km *FileKeyManager) Exists() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.masterKey != nil {
		return true
	}

	_, err := os.Stat(km.keyPath)
	return err == nil
}

// NewSigner creates a Signer using the loaded master key.
func (km *FileKeyManager) NewSigner() (*Signer, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.masterKey == nil {
		return nil, fmt.Errorf("%w: call Load() first", atlaserrors.ErrKeyNotLoaded)
	}

	return &Signer{
		masterKey: km.masterKey,
	}, nil
}

// generate creates a new master key and saves it to the file.
func (km *FileKeyManager) generate() error {
	// Generate a cryptographically secure seed
	seed, err := bip32.GenerateSeed(bip32.RecommendedSeedLen)
	if err != nil {
		return fmt.Errorf("failed to generate seed: %w", err)
	}

	// Create master key from seed
	masterKey, err := bip32.NewMaster(seed, &chaincfg.MainNet)
	if err != nil {
		return fmt.Errorf("failed to create master key: %w", err)
	}

	// Serialize the key
	keyStr := masterKey.String()

	// Ensure directory exists
	dir := filepath.Dir(km.keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Write to file with secure permissions (0600)
	if err := os.WriteFile(km.keyPath, []byte(keyStr), 0o600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	km.masterKey = masterKey
	return nil
}

// Signer provides ECDSA signing using HD key derivation.
type Signer struct {
	masterKey *bip32.ExtendedKey
}

// KeyPath returns the derivation path for a given task index.
// Format: m/44'/236'/0'/{task_index}
func (s *Signer) KeyPath(taskIndex uint32) string {
	return fmt.Sprintf("m/%d'/%d'/%d'/%d", BIP44Purpose, ATLASCoinType, ReceiptAccount, taskIndex)
}

// Sign signs the given message using the master key.
// The message is hashed with SHA256 before signing.
func (s *Signer) Sign(_ context.Context, message []byte) ([]byte, error) {
	// Derive signing key (use purpose/coin/account path for general signing)
	key, err := s.deriveAccountKey()
	if err != nil {
		return nil, fmt.Errorf("failed to derive signing key: %w", err)
	}

	// Get the private key
	privKey, err := key.ECPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	// Hash the message
	hash := sha256.Sum256(message)

	// Sign using the ec.PrivateKey.Sign method (secp256k1, RFC6979)
	sig, err := privKey.Sign(hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Serialize signature to DER format
	return sig.Serialize(), nil
}

// Verify checks that a signature is valid for the given message.
func (s *Signer) Verify(_ context.Context, message, signature []byte) error {
	if len(signature) == 0 {
		return atlaserrors.ErrSignatureEmpty
	}

	// Derive signing key
	key, err := s.deriveAccountKey()
	if err != nil {
		return fmt.Errorf("failed to derive signing key: %w", err)
	}

	// Get the public key
	pubKey, err := key.ECPubKey()
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Parse the signature
	sig, err := ec.ParseSignature(signature)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
	}

	// Hash the message
	hash := sha256.Sum256(message)

	// Verify using sig.Verify(hash, pubKey) - this takes a hash, not a message.
	// The sig.Verify method expects a pre-hashed message, matching Sign's behavior.
	valid := sig.Verify(hash[:], pubKey)
	if !valid {
		return atlaserrors.ErrSignatureVerificationFailed
	}

	return nil
}

// DeriveForTask derives a signing key for a specific task index.
// This is used for task-specific receipts.
func (s *Signer) DeriveForTask(taskIndex uint32) (*Signer, error) {
	// Derive: m/44'/236'/0'/{task_index}
	key, err := s.deriveAccountKey()
	if err != nil {
		return nil, err
	}

	// Derive task-specific key (non-hardened for performance)
	taskKey, err := key.Child(taskIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to derive task key: %w", err)
	}

	return &Signer{masterKey: taskKey}, nil
}

// PublicKeyHex returns the public key as a hex string.
func (s *Signer) PublicKeyHex() (string, error) {
	key, err := s.deriveAccountKey()
	if err != nil {
		return "", err
	}

	pubKey, err := key.ECPubKey()
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(pubKey.Compressed()), nil
}

// deriveAccountKey derives the account-level key: m/44'/236'/0'
func (s *Signer) deriveAccountKey() (*bip32.ExtendedKey, error) {
	// Derive purpose: m/44'
	purpose, err := s.masterKey.Child(bip32.HardenedKeyStart + BIP44Purpose)
	if err != nil {
		return nil, fmt.Errorf("failed to derive purpose: %w", err)
	}

	// Derive coin type: m/44'/236'
	coinType, err := purpose.Child(bip32.HardenedKeyStart + ATLASCoinType)
	if err != nil {
		return nil, fmt.Errorf("failed to derive coin type: %w", err)
	}

	// Derive account: m/44'/236'/0'
	account, err := coinType.Child(bip32.HardenedKeyStart + ReceiptAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to derive account: %w", err)
	}

	return account, nil
}
