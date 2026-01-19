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
// Uses crypto/rand for secure random generation with rejection sampling
// to ensure uniform distribution (avoiding modulo bias).
func GenerateID() (string, error) {
	// maxValid is the largest value that maps uniformly to idChars.
	// 256 / 36 = 7 remainder 4, so we accept values 0-251 (36*7=252).
	const maxValid = byte(len(idChars) * (256 / len(idChars)))

	result := make([]byte, idLength)
	buf := make([]byte, 1)

	for i := 0; i < idLength; {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}
		// Rejection sampling: only accept values that map uniformly
		if buf[0] < maxValid {
			result[i] = idChars[buf[0]%byte(len(idChars))] //nolint:gosec // index is guaranteed safe by maxValid check
			i++
		}
	}
	return idPrefix + string(result), nil
}
