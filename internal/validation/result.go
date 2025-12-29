// Package validation provides command execution and result handling for validation pipelines.
//
// This package defines the core types and interfaces for running validation commands
// (format, lint, test, etc.) with proper timeout handling, output capture, and logging.
package validation

import (
	"time"
)

// Result captures the outcome of a single validation command.
type Result struct {
	Command     string    `json:"command"`
	Success     bool      `json:"success"`
	ExitCode    int       `json:"exit_code"`
	Stdout      string    `json:"stdout"`
	Stderr      string    `json:"stderr"`
	DurationMs  int64     `json:"duration_ms"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
