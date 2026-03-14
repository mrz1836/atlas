package backlog

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// idChars is the unambiguous character set for ID generation.
	// Excludes 0/O/1/I/L to avoid visual confusion (30 chars total).
	idChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// idLength is the number of random characters in the ID suffix.
	idLength = 6
	// idPrefix is the prefix for all discovery IDs.
	idPrefix = "item-"
	// legacyIDPrefix is the legacy prefix for backward compatibility.
	legacyIDPrefix = "disc-"
)

// GenerateGUID creates a new UUID v4 string.
func GenerateGUID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}
	return id.String(), nil
}

// DeriveShortID deterministically derives a 6-character short ID from a GUID.
// It uses the first 8 bytes of the GUID (parsed as a UUID) to seed the derivation.
func DeriveShortID(guid string) (string, error) {
	id, err := uuid.Parse(guid)
	if err != nil {
		return "", fmt.Errorf("invalid GUID format: %w", err)
	}

	// Use the UUID bytes to derive a deterministic short ID
	bytes := id[:]
	result := make([]byte, idLength)
	charsetLen := uint64(len(idChars))

	// Use first 8 bytes as a seed number
	seed := binary.BigEndian.Uint64(bytes[:8])

	for i := 0; i < idLength; i++ {
		result[i] = idChars[seed%charsetLen]
		seed /= charsetLen
	}

	return idPrefix + string(result), nil
}

// GenerateID creates a new unique discovery ID with GUID.
// Returns (guid, shortID, error). The shortID format is "item-" followed by
// 6 uppercase unambiguous characters derived from the GUID.
func GenerateID() (string, string, error) {
	guid, err := GenerateGUID()
	if err != nil {
		return "", "", err
	}

	shortID, err := DeriveShortID(guid)
	if err != nil {
		return "", "", err
	}

	return guid, shortID, nil
}

// GenerateLegacyID creates an ID in the old format for testing purposes.
// The ID format is "disc-" followed by 6 random lowercase alphanumeric characters.
// Uses crypto/rand for secure random generation with rejection sampling
// to ensure uniform distribution (avoiding modulo bias).
func GenerateLegacyID() (string, error) {
	const legacyChars = "abcdefghijklmnopqrstuvwxyz0123456789"
	// maxValid is the largest value that maps uniformly to legacyChars.
	// 256 / 36 = 7 remainder 4, so we accept values 0-251 (36*7=252).
	const maxValid = byte(len(legacyChars) * (256 / len(legacyChars)))

	result := make([]byte, idLength)
	buf := make([]byte, 1)

	for i := 0; i < idLength; {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}
		// Rejection sampling: only accept values that map uniformly
		if buf[0] < maxValid {
			result[i] = legacyChars[buf[0]%byte(len(legacyChars))]
			i++
		}
	}
	return legacyIDPrefix + string(result), nil
}

// IsLegacyID returns true if the ID uses the legacy "disc-" prefix.
func IsLegacyID(id string) bool {
	return strings.HasPrefix(id, legacyIDPrefix)
}

// IsNewID returns true if the ID uses the new "item-" prefix.
func IsNewID(id string) bool {
	return strings.HasPrefix(id, idPrefix)
}
