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

// PipelineResult aggregates results from all validation pipeline steps.
type PipelineResult struct {
	Success          bool     `json:"success"`
	FormatResults    []Result `json:"format_results"`
	LintResults      []Result `json:"lint_results"`
	TestResults      []Result `json:"test_results"`
	PreCommitResults []Result `json:"pre_commit_results"`
	DurationMs       int64    `json:"duration_ms"`
	FailedStepName   string   `json:"failed_step,omitempty"`
}

// AllResults returns a flat list of all results from all steps.
func (p *PipelineResult) AllResults() []Result {
	var all []Result
	all = append(all, p.FormatResults...)
	all = append(all, p.LintResults...)
	all = append(all, p.TestResults...)
	all = append(all, p.PreCommitResults...)
	return all
}

// FailedStep returns the name of the first failed step, or empty string if all passed.
func (p *PipelineResult) FailedStep() string {
	return p.FailedStepName
}
