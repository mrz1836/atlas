package config

// CryptoConfig holds configuration for cryptographic operations.
type CryptoConfig struct {
	// Provider selects the crypto implementation (e.g. "native").
	// Defaults to "native".
	Provider string `yaml:"provider" env:"ATLAS_CRYPTO_PROVIDER" default:"native"`
}
