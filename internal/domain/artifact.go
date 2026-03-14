// Package domain provides shared data types for ATLAS.
//
// This file defines artifact structures for CI results that are saved to JSON files.
// These types are shared across packages to avoid duplication.
package domain

// CIResultArtifact is the structure saved to ci-result.json.
// Used by both template/steps and task packages.
type CIResultArtifact struct {
	// Status is the final CI status (success, failure, timeout).
	Status string `json:"status"`
	// ElapsedTime is how long CI was monitored.
	ElapsedTime string `json:"elapsed_time"`
	// FailedChecks is the list of checks that failed.
	FailedChecks []CICheckArtifact `json:"failed_checks"`
	// AllChecks is the complete list of checks.
	AllChecks []CICheckArtifact `json:"all_checks"`
	// ErrorMessage is the error description.
	ErrorMessage string `json:"error_message,omitempty"`
	// Timestamp is when the artifact was created.
	Timestamp string `json:"timestamp"`
}

// CICheckArtifact represents a single CI check in the artifact.
type CICheckArtifact struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Bucket   string `json:"bucket"`
	URL      string `json:"url,omitempty"`
	Duration string `json:"duration,omitempty"`
	Workflow string `json:"workflow,omitempty"`
}
