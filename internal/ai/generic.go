// Package ai provides AI runner implementations for different providers.
package ai

import (
	"encoding/json"
	"fmt"
)

// parseResponse is a generic JSON response parser that handles the common
// pattern of parsing AI CLI responses. It handles:
// - Empty response detection
// - JSON unmarshaling with proper error wrapping
//
// The errSentinel is the provider-specific error (e.g., ErrClaudeInvocation)
// used to wrap parsing errors.
//
// Usage:
//
//	resp, err := parseResponse[ClaudeResponse](stdout, atlaserrors.ErrClaudeInvocation)
func parseResponse[T any](data []byte, errSentinel error) (*T, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty response", errSentinel)
	}

	var resp T
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse json response (%d bytes): %w", errSentinel, len(data), err)
	}

	return &resp, nil
}
