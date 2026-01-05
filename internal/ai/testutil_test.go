package ai

import (
	"testing"
)

// EnsureNoRealAPIKeys verifies that no production API keys are present in the test environment.
// This is a safety guard to prevent tests from accidentally making real API calls.
//
// All tests in this package use mocks (MockExecutor) to simulate AI CLI subprocess execution.
// Tests should NEVER make actual API requests or use production API keys.
//
// This function will automatically unset any API keys for the duration of the test using t.Setenv,
// ensuring tests can run safely even when API keys are present in the developer's environment.
func EnsureNoRealAPIKeys(t *testing.T) {
	t.Helper()

	// Unset all API keys for the duration of this test to prevent accidental API calls
	// This allows tests to run even when developers have API keys in their environment
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
}
