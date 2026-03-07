// Package validation provides command execution and result handling for validation pipelines.
//
// This package defines the core types and interfaces for running validation commands
// (format, lint, test, etc.) with proper timeout handling, output capture, and logging.
package validation

import (
	"time"
)

// VacuousThresholdMs is the duration threshold (in milliseconds) below which
// a command with empty output is considered vacuous (i.e., it didn't actually
// find anything to run, like a test runner in a non-Go project).
const VacuousThresholdMs = 1000

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
	EmptyOutput bool      `json:"empty_output,omitempty"` // True when stdout is empty and command completed quickly
}

// IsVacuous returns true if this result represents a vacuous success —
// the command succeeded but produced no output and ran very quickly,
// suggesting it found nothing to do (e.g., test runner with no tests).
func (r *Result) IsVacuous() bool {
	return r.Success && r.EmptyOutput
}

// PipelineResult aggregates results from all validation pipeline steps.
type PipelineResult struct {
	Success          bool              `json:"success"`
	FormatResults    []Result          `json:"format_results"`
	LintResults      []Result          `json:"lint_results"`
	TestResults      []Result          `json:"test_results"`
	PreCommitResults []Result          `json:"pre_commit_results"`
	DurationMs       int64             `json:"duration_ms"`
	FailedStepName   string            `json:"failed_step,omitempty"`
	SkippedSteps     []string          `json:"skipped_steps,omitempty"`
	SkipReasons      map[string]string `json:"skip_reasons,omitempty"`
	VacuousTests     bool              `json:"vacuous_tests,omitempty"` // True when all test commands produced empty output quickly
}

// AllResults returns a flat list of all results from all steps.
func (p *PipelineResult) AllResults() []Result {
	all := make([]Result, 0, len(p.FormatResults)+len(p.LintResults)+len(p.TestResults)+len(p.PreCommitResults))
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

// Check represents a single validation check result for display.
type Check struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Skipped bool   `json:"skipped,omitempty"`
}

// BuildChecks creates validation check metadata from pipeline results.
// Returns a slice of Check for each validation category (Format, Lint, Test, Pre-commit).
// This is the single source of truth for building validation check summaries.
func (p *PipelineResult) BuildChecks() []Check {
	if p == nil {
		return nil
	}

	checks := make([]Check, 0, 4)

	// Format check
	checks = append(checks, Check{
		Name:   "Format",
		Passed: len(p.FormatResults) == 0 || !hasFailedResult(p.FormatResults),
	})

	// Lint check
	checks = append(checks, Check{
		Name:   "Lint",
		Passed: len(p.LintResults) == 0 || !hasFailedResult(p.LintResults),
	})

	// Test check
	checks = append(checks, Check{
		Name:   "Test",
		Passed: len(p.TestResults) == 0 || !hasFailedResult(p.TestResults),
	})

	// Pre-commit check (check if skipped)
	preCommitSkipped := false
	for _, skipped := range p.SkippedSteps {
		if skipped == "pre-commit" {
			preCommitSkipped = true
			break
		}
	}
	preCommitPassed := true
	if !preCommitSkipped {
		preCommitPassed = len(p.PreCommitResults) == 0 || !hasFailedResult(p.PreCommitResults)
	}
	checks = append(checks, Check{
		Name:    "Pre-commit",
		Passed:  preCommitPassed,
		Skipped: preCommitSkipped,
	})

	return checks
}

// BuildChecksAsMap creates validation check metadata as a slice of maps.
// This is a convenience method for use with step result metadata.
func (p *PipelineResult) BuildChecksAsMap() []map[string]any {
	checks := p.BuildChecks()
	if checks == nil {
		return nil
	}

	result := make([]map[string]any, len(checks))
	for i, c := range checks {
		m := map[string]any{
			"name":   c.Name,
			"passed": c.Passed,
		}
		if c.Skipped {
			m["skipped"] = c.Skipped
		}
		result[i] = m
	}
	return result
}

// hasFailedResult checks if any result in the slice indicates failure.
func hasFailedResult(results []Result) bool {
	for _, r := range results {
		if !r.Success {
			return true
		}
	}
	return false
}
