package ai

import (
	"encoding/json"
	"fmt"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// CodexResponse represents the JSON response from Codex CLI.
// This struct matches the JSON output format when using codex exec --json.
//
// The actual Codex CLI response format may differ. This structure
// is based on expected common patterns and may need adjustment once we
// have actual Codex CLI output to test against.
type CodexResponse struct {
	// Success indicates whether the request completed successfully.
	Success bool `json:"success"`

	// Content contains the AI's text response or output.
	Content string `json:"content"`

	// Result is an alternative field for the response content.
	// Some versions may use "result" instead of "content".
	Result string `json:"result"`

	// SessionID identifies the AI session for debugging.
	SessionID string `json:"session_id"`

	// DurationMs is how long the AI session took in milliseconds.
	DurationMs int `json:"duration_ms"`

	// NumTurns is how many conversation turns occurred.
	NumTurns int `json:"num_turns"`

	// TotalCostUSD is the estimated cost of the AI session in USD.
	// May be 0 for free tier usage.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

// parseCodexResponse parses the JSON output from Codex CLI.
// Returns an error wrapped with ErrCodexInvocation on parse failure.
func parseCodexResponse(data []byte) (*CodexResponse, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty response", atlaserrors.ErrCodexInvocation)
	}

	var resp CodexResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse json response: %s", atlaserrors.ErrCodexInvocation, err.Error())
	}

	return &resp, nil
}

// toAIResult converts a CodexResponse to a domain.AIResult.
// Maps Codex-specific fields to the domain-agnostic AIResult structure.
func (r *CodexResponse) toAIResult(stderr string) *domain.AIResult {
	// Use Content or Result, whichever is populated
	output := r.Content
	if output == "" {
		output = r.Result
	}

	result := &domain.AIResult{
		Success:      r.Success,
		Output:       output,
		SessionID:    r.SessionID,
		DurationMs:   r.DurationMs,
		NumTurns:     r.NumTurns,
		TotalCostUSD: r.TotalCostUSD,
	}

	// Include error information if this is an error response
	if !r.Success {
		if r.Error != "" {
			result.Error = r.Error
		} else if stderr != "" {
			result.Error = stderr
		}
	}

	return result
}
