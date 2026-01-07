package ai

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestArtifact_JSONSerialization(t *testing.T) {
	now := time.Date(2025, 1, 7, 10, 30, 0, 0, time.UTC)
	artifact := &Artifact{
		Timestamp: now,
		StepName:  "ai_step",
		StepIndex: 1,
		Agent:     "claude",
		Model:     "sonnet",
		Request: &domain.AIRequest{
			Agent:  domain.AgentClaude,
			Prompt: "Test prompt",
			Model:  "sonnet",
		},
		Response: &domain.AIResult{
			Success:      true,
			Output:       "Test output",
			SessionID:    "session-123",
			DurationMs:   45000,
			NumTurns:     5,
			TotalCostUSD: 0.05,
		},
		ExecutionTimeMs: 45000,
		Success:         true,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal Artifact: %v", err)
	}

	// Verify JSON structure
	var unmarshaled map[string]any
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check top-level fields with snake_case
	if unmarshaled["step_name"] != "ai_step" {
		t.Errorf("Expected step_name=ai_step, got %v", unmarshaled["step_name"])
	}
	if unmarshaled["step_index"] != float64(1) {
		t.Errorf("Expected step_index=1, got %v", unmarshaled["step_index"])
	}
	if unmarshaled["agent"] != "claude" {
		t.Errorf("Expected agent=claude, got %v", unmarshaled["agent"])
	}
	if unmarshaled["model"] != "sonnet" {
		t.Errorf("Expected model=sonnet, got %v", unmarshaled["model"])
	}
	if unmarshaled["execution_time_ms"] != float64(45000) {
		t.Errorf("Expected execution_time_ms=45000, got %v", unmarshaled["execution_time_ms"])
	}
	if unmarshaled["success"] != true {
		t.Errorf("Expected success=true, got %v", unmarshaled["success"])
	}

	// Check request is nested
	if request, ok := unmarshaled["request"].(map[string]any); !ok {
		t.Error("Expected request to be an object")
	} else if request["prompt"] != "Test prompt" {
		t.Errorf("Expected request.prompt='Test prompt', got %v", request["prompt"])
	}

	// Check response is nested
	if response, ok := unmarshaled["response"].(map[string]any); !ok {
		t.Error("Expected response to be an object")
	} else {
		if response["session_id"] != "session-123" {
			t.Errorf("Expected response.session_id='session-123', got %v", response["session_id"])
		}
		if response["total_cost_usd"] != 0.05 {
			t.Errorf("Expected response.total_cost_usd=0.05, got %v", response["total_cost_usd"])
		}
	}

	// Unmarshal back to struct
	var roundTrip Artifact
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Failed to unmarshal back to Artifact: %v", err)
	}

	// Verify fields are preserved
	if roundTrip.StepName != artifact.StepName {
		t.Errorf("StepName not preserved: got %s, want %s", roundTrip.StepName, artifact.StepName)
	}
	if roundTrip.Agent != artifact.Agent {
		t.Errorf("Agent not preserved: got %s, want %s", roundTrip.Agent, artifact.Agent)
	}
	if roundTrip.Success != artifact.Success {
		t.Errorf("Success not preserved: got %v, want %v", roundTrip.Success, artifact.Success)
	}
}

func TestArtifact_WithError(t *testing.T) {
	artifact := &Artifact{
		Timestamp:       time.Now(),
		StepName:        "ai_step",
		Agent:           "claude",
		Model:           "sonnet",
		Request:         &domain.AIRequest{Prompt: "test"},
		Response:        nil, // No response when there's an error
		ExecutionTimeMs: 1000,
		Success:         false,
		ErrorMessage:    "AI request failed",
	}

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("Failed to marshal artifact with error: %v", err)
	}

	var unmarshaled map[string]any
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if unmarshaled["success"] != false {
		t.Errorf("Expected success=false, got %v", unmarshaled["success"])
	}
	if unmarshaled["error_message"] != "AI request failed" {
		t.Errorf("Expected error_message='AI request failed', got %v", unmarshaled["error_message"])
	}
}

func TestRetryAttempt_JSONSerialization(t *testing.T) {
	attempt := &RetryAttempt{
		AttemptNumber:   2,
		Timestamp:       time.Now(),
		FailureReason:   "Previous attempt timed out",
		Request:         &domain.AIRequest{Prompt: "retry prompt"},
		Response:        &domain.AIResult{Success: true, Output: "retry output"},
		ExecutionTimeMs: 30000,
		Success:         true,
	}

	data, err := json.Marshal(attempt)
	if err != nil {
		t.Fatalf("Failed to marshal RetryAttempt: %v", err)
	}

	var unmarshaled map[string]any
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if unmarshaled["attempt_number"] != float64(2) {
		t.Errorf("Expected attempt_number=2, got %v", unmarshaled["attempt_number"])
	}
	if unmarshaled["failure_reason"] != "Previous attempt timed out" {
		t.Errorf("Expected failure_reason, got %v", unmarshaled["failure_reason"])
	}
}

func TestRetrySummary_JSONSerialization(t *testing.T) {
	now := time.Now()
	summary := &RetrySummary{
		InitialFailure: "Validation failed",
		Attempts: []RetryAttempt{
			{AttemptNumber: 1, Success: false, ExecutionTimeMs: 10000},
			{AttemptNumber: 2, Success: true, ExecutionTimeMs: 12000},
		},
		FinalSuccess:  true,
		TotalAttempts: 2,
		TotalCostUSD:  0.10,
		TotalTimeMs:   22000,
		StartedAt:     now,
		CompletedAt:   now.Add(22 * time.Second),
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal RetrySummary: %v", err)
	}

	var unmarshaled map[string]any
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if unmarshaled["initial_failure"] != "Validation failed" {
		t.Errorf("Expected initial_failure, got %v", unmarshaled["initial_failure"])
	}
	if unmarshaled["total_attempts"] != float64(2) {
		t.Errorf("Expected total_attempts=2, got %v", unmarshaled["total_attempts"])
	}
	if unmarshaled["final_success"] != true {
		t.Errorf("Expected final_success=true, got %v", unmarshaled["final_success"])
	}
	if unmarshaled["total_cost_usd"] != 0.10 {
		t.Errorf("Expected total_cost_usd=0.10, got %v", unmarshaled["total_cost_usd"])
	}
	if unmarshaled["total_time_ms"] != float64(22000) {
		t.Errorf("Expected total_time_ms=22000, got %v", unmarshaled["total_time_ms"])
	}

	// Check attempts array
	if attempts, ok := unmarshaled["attempts"].([]any); !ok {
		t.Error("Expected attempts to be an array")
	} else if len(attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", len(attempts))
	}
}
