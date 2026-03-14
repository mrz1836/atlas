package ai

import (
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// Artifact represents a complete AI request/response interaction with metadata.
// This type is used for artifact storage to provide a full audit trail of AI interactions.
//
// Example JSON representation:
//
//	{
//	  "timestamp": "2025-12-27T10:00:00Z",
//	  "step_name": "ai_step",
//	  "step_index": 2,
//	  "agent": "claude",
//	  "model": "sonnet",
//	  "request": {...},
//	  "response": {...},
//	  "execution_time_ms": 45000,
//	  "success": true
//	}
type Artifact struct {
	// Timestamp is when the AI request was initiated.
	Timestamp time.Time `json:"timestamp"`

	// StepName is the name of the step that triggered this AI interaction.
	StepName string `json:"step_name,omitempty"`

	// StepIndex is the position of the step in the task (zero-based).
	StepIndex int `json:"step_index,omitempty"`

	// Agent is the AI agent used (claude, gemini, codex).
	Agent string `json:"agent"`

	// Model is the AI model used (sonnet, opus, haiku, etc.).
	Model string `json:"model"`

	// Request contains the full AI request with all parameters.
	Request *domain.AIRequest `json:"request"`

	// Response contains the full AI response with all results.
	// Will be nil if the request failed before getting a response.
	Response *domain.AIResult `json:"response,omitempty"`

	// ExecutionTimeMs is the total time taken for the AI interaction in milliseconds.
	ExecutionTimeMs int64 `json:"execution_time_ms"`

	// Success indicates whether the AI interaction completed successfully.
	Success bool `json:"success"`

	// ErrorMessage contains the error message if Success is false.
	ErrorMessage string `json:"error_message,omitempty"`
}

// RetryAttempt represents a single retry attempt in an AI retry scenario.
// Used by RetryHandler to track individual retry attempts.
type RetryAttempt struct {
	// AttemptNumber is the 1-based attempt number (1, 2, 3, etc.).
	AttemptNumber int `json:"attempt_number"`

	// Timestamp is when this retry attempt was initiated.
	Timestamp time.Time `json:"timestamp"`

	// FailureReason explains why the previous attempt failed (if applicable).
	FailureReason string `json:"failure_reason,omitempty"`

	// Request contains the AI request for this attempt.
	Request *domain.AIRequest `json:"request"`

	// Response contains the AI response for this attempt.
	// Will be nil if the attempt failed before getting a response.
	Response *domain.AIResult `json:"response,omitempty"`

	// ExecutionTimeMs is the time taken for this attempt in milliseconds.
	ExecutionTimeMs int64 `json:"execution_time_ms"`

	// Success indicates whether this attempt succeeded.
	Success bool `json:"success"`

	// ErrorMessage contains the error message if this attempt failed.
	ErrorMessage string `json:"error_message,omitempty"`
}

// RetrySummary summarizes all retry attempts for an AI operation.
// This provides an aggregate view of the retry process.
type RetrySummary struct {
	// InitialFailure is the reason that triggered the retry process.
	InitialFailure string `json:"initial_failure"`

	// Attempts is the list of all retry attempts made.
	Attempts []RetryAttempt `json:"attempts"`

	// FinalSuccess indicates whether the retry process ultimately succeeded.
	FinalSuccess bool `json:"final_success"`

	// TotalAttempts is the total number of attempts made.
	TotalAttempts int `json:"total_attempts"`

	// TotalCostUSD is the sum of costs across all attempts.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// TotalTimeMs is the sum of execution times across all attempts.
	TotalTimeMs int64 `json:"total_time_ms"`

	// StartedAt is when the first attempt started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the final attempt completed.
	CompletedAt time.Time `json:"completed_at"`
}
