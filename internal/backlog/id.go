package backlog

import (
	"crypto/rand"
	"fmt"
)

const (
	// idChars is the character set for ID generation (lowercase alphanumeric).
	idChars = "abcdefghijklmnopqrstuvwxyz0123456789"
	// idLength is the number of random characters in the ID suffix.
	idLength = 6
	// idPrefix is the prefix for all discovery IDs.
	idPrefix = "disc-"
)

// GenerateID creates a new unique discovery ID.
// The ID format is "disc-" followed by 6 random alphanumeric characters.
// Uses crypto/rand for secure random generation.
func GenerateID() (string, error) {
	bytes := make([]byte, idLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	for i := range bytes {
		bytes[i] = idChars[bytes[i]%byte(len(idChars))]
	}
	return idPrefix + string(bytes), nil
}
