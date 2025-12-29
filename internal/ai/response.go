package ai

import (
	"encoding/json"
	"fmt"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ClaudeResponse represents the JSON response from Claude Code CLI.
// This struct matches the JSON output format when using --output-format json.
type ClaudeResponse struct {
	// Type indicates the response type (e.g., "result").
	Type string `json:"type"`

	// Subtype provides additional type information.
	Subtype string `json:"subtype"`

	// IsError indicates whether the response represents an error.
	IsError bool `json:"is_error"`

	// Result contains the AI's text response or output.
	Result string `json:"result"`

	// SessionID identifies the AI session for debugging.
	SessionID string `json:"session_id"`

	// Duration is how long the AI session took in milliseconds.
	Duration int `json:"duration_ms"`

	// NumTurns is how many conversation turns occurred.
	NumTurns int `json:"num_turns"`

	// TotalCost is the estimated cost of the AI session in USD.
	TotalCost float64 `json:"total_cost_usd"`
}

// parseClaudeResponse parses the JSON output from Claude Code CLI.
// Returns an error wrapped with ErrClaudeInvocation on parse failure.
func parseClaudeResponse(data []byte) (*ClaudeResponse, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty response", atlaserrors.ErrClaudeInvocation)
	}

	var resp ClaudeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse json response: %s", atlaserrors.ErrClaudeInvocation, err.Error())
	}

	return &resp, nil
}

// toAIResult converts a ClaudeResponse to a domain.AIResult.
// Maps Claude-specific fields to the domain-agnostic AIResult structure.
func (r *ClaudeResponse) toAIResult(stderr string) *domain.AIResult {
	result := &domain.AIResult{
		Success:      !r.IsError,
		Output:       r.Result,
		SessionID:    r.SessionID,
		DurationMs:   r.Duration,
		NumTurns:     r.NumTurns,
		TotalCostUSD: r.TotalCost,
	}

	// Include stderr in error field if this is an error response
	if r.IsError && stderr != "" {
		result.Error = stderr
	}

	return result
}
