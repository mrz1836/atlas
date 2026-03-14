// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"regexp"
	"strings"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
)

// ExitEvaluator determines when a loop should terminate.
// This interface enables mocking exit logic in tests.
type ExitEvaluator interface {
	// Evaluate checks if the loop should exit based on iteration output.
	// Returns an ExitDecision indicating whether to exit and why.
	Evaluate(result *domain.IterationResult, output string) ExitDecision

	// ParseExitSignal extracts {"exit": true} from AI output.
	ParseExitSignal(output string) (bool, error)

	// CheckConditions verifies all configured exit conditions are met.
	CheckConditions(output string) bool
}

// ExitDecision represents the result of exit evaluation.
type ExitDecision struct {
	// ShouldExit indicates if the loop should terminate.
	ShouldExit bool

	// Reason explains why the decision was made.
	Reason string
}

// DefaultExitEvaluator implements ExitEvaluator with dual-gate logic.
// When configured with exit conditions, both the signal AND all conditions
// must be met to trigger exit (dual-gate pattern).
type DefaultExitEvaluator struct {
	conditions []string
	logger     zerolog.Logger
}

// NewExitEvaluator creates a new exit evaluator with the given conditions.
func NewExitEvaluator(conditions []string, logger zerolog.Logger) *DefaultExitEvaluator {
	return &DefaultExitEvaluator{
		conditions: conditions,
		logger:     logger,
	}
}

// Evaluate checks if the loop should exit based on the iteration result and output.
// Uses dual-gate logic: both exit signal AND all conditions must be met.
func (e *DefaultExitEvaluator) Evaluate(_ *domain.IterationResult, output string) ExitDecision {
	// Check for JSON exit signal: {"exit": true}
	hasSignal, _ := e.ParseExitSignal(output)
	if !hasSignal {
		e.logger.Debug().Msg("no exit signal found in output")
		return ExitDecision{ShouldExit: false, Reason: "no exit signal"}
	}

	// If no conditions configured, signal alone is sufficient
	if len(e.conditions) == 0 {
		e.logger.Info().Msg("exit signal received with no conditions configured - exiting")
		return ExitDecision{ShouldExit: true, Reason: "exit signal received"}
	}

	// Check all conditions are met (dual-gate)
	if !e.CheckConditions(output) {
		e.logger.Debug().
			Strs("conditions", e.conditions).
			Msg("exit signal received but not all conditions met")
		return ExitDecision{ShouldExit: false, Reason: "exit signal received but conditions not met"}
	}

	e.logger.Info().
		Strs("conditions", e.conditions).
		Msg("exit signal and all conditions met - exiting")
	return ExitDecision{ShouldExit: true, Reason: "all conditions met with exit signal"}
}

// exitSignalPattern matches {"exit": true} with flexible whitespace.
var exitSignalPattern = regexp.MustCompile(`\{\s*"exit"\s*:\s*true\s*\}`)

// ParseExitSignal extracts {"exit": true} from AI output.
func (e *DefaultExitEvaluator) ParseExitSignal(output string) (bool, error) {
	return exitSignalPattern.MatchString(output), nil
}

// CheckConditions verifies all configured exit conditions are present in the output.
func (e *DefaultExitEvaluator) CheckConditions(output string) bool {
	for _, cond := range e.conditions {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(cond)) {
			e.logger.Debug().
				Str("condition", cond).
				Msg("condition not found in output")
			return false
		}
	}
	return true
}

// ConditionFunc evaluates a named condition against task state.
type ConditionFunc func(task *domain.Task) bool

// builtinConditions returns the map of built-in condition names to their evaluators.
func builtinConditions() map[string]ConditionFunc {
	return map[string]ConditionFunc{
		"all_tests_pass":    checkTestsPassed,
		"validation_passed": checkValidationPassed,
		"no_changes":        checkNoRecentChanges,
	}
}

// checkTestsPassed checks if the last validation step had passing tests.
func checkTestsPassed(task *domain.Task) bool {
	for i := len(task.StepResults) - 1; i >= 0; i-- {
		result := task.StepResults[i]
		if result.StepName == "validate" || strings.Contains(result.StepName, "validation") {
			return result.Status == "success"
		}
	}
	return false
}

// checkValidationPassed checks if validation has passed.
func checkValidationPassed(task *domain.Task) bool {
	return checkTestsPassed(task)
}

// checkNoRecentChanges checks if no files were changed in the last iteration.
func checkNoRecentChanges(task *domain.Task) bool {
	if len(task.StepResults) == 0 {
		return true
	}
	lastResult := task.StepResults[len(task.StepResults)-1]
	return len(lastResult.FilesChanged) == 0
}

// EvaluateBuiltinCondition evaluates a named condition from the built-in set.
func EvaluateBuiltinCondition(conditionName string, task *domain.Task) bool {
	fn, exists := builtinConditions()[conditionName]
	if !exists {
		return false
	}
	return fn(task)
}
