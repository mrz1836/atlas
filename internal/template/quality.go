package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// newQualityAIStep creates the analyze_and_fix AI step used by all quality templates.
func newQualityAIStep(description, promptTemplate string) domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "analyze_and_fix",
		Type:        domain.StepTypeAI,
		Description: description,
		Required:    true,
		Timeout:     constants.DefaultAITimeout,
		RetryCount:  3,
		Config: map[string]any{
			"permission_mode": "default",
			"prompt_template": promptTemplate,
		},
	}
}

// newQualityVerifyStep creates the optional verify step used by all quality templates.
func newQualityVerifyStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "verify",
		Type:        domain.StepTypeVerify,
		Description: "Optional AI verification of implementation",
		Required:    false,
		Timeout:     5 * time.Minute,
		Config: map[string]any{
			"agent":  "gemini",
			"model":  "",
			"checks": []string{"code_correctness"},
		},
	}
}

// newQualityValidateStep creates the validate step used by all quality templates.
func newQualityValidateStep() domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "validate",
		Type:        domain.StepTypeValidation,
		Description: "Run format, lint, and test commands",
		Required:    true,
		Timeout:     10 * time.Minute,
		RetryCount:  1,
	}
}

// newQualityTemplate builds a quality template with the standard step sequence.
func newQualityTemplate(name, description, aiDesc, promptTemplate, commitDesc, reviewDesc string) *domain.Template {
	return &domain.Template{
		Name:         name,
		Description:  description,
		BranchPrefix: "quality",
		DefaultAgent: domain.AgentClaude,
		DefaultModel: "sonnet",
		Verify:       false,
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			newQualityAIStep(aiDesc, promptTemplate),
			newQualityVerifyStep(),
			newQualityValidateStep(),
			newGitCommitStep(commitDesc),
			newGitPushStep(),
			newGitPRStep(),
			newCIWaitStep(),
			newReviewStep("Human review of quality improvements", reviewDesc),
		},
		ValidationCommands: defaultValidationCommands,
	}
}

// NewGoOptimizeTemplate creates the go-optimize template for modernizing Go code.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewGoOptimizeTemplate() *domain.Template {
	return newQualityTemplate(
		"go-optimize",
		"Modernize Go code using newer language features (version-aware)",
		"Identify and apply Go optimization improvements",
		"go-optimize",
		"Apply Go optimization improvements",
		"Review the Go optimizations and approve or reject",
	)
}

// NewDedupTemplate creates the dedup template for eliminating duplicated code.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewDedupTemplate() *domain.Template {
	return newQualityTemplate(
		"dedup",
		"Detect and eliminate duplicated code by extracting shared helpers or generics",
		"Identify and eliminate code duplication",
		"dedup",
		"Eliminate code duplication",
		"Review the deduplication changes and approve or reject",
	)
}

// NewGoroutineLeakTemplate creates the goroutine-leak template for detecting and fixing goroutine leaks.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewGoroutineLeakTemplate() *domain.Template {
	return newQualityTemplate(
		"goroutine-leak",
		"Detect and fix goroutine leaks caused by missing cancellation, context, or channel coordination",
		"Identify and fix goroutine leaks",
		"goroutine-leak",
		"Fix goroutine leaks",
		"Review the goroutine leak fixes and approve or reject",
	)
}

// NewJrToSrTemplate creates the jr-to-sr template for elevating junior patterns to senior-level code.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewJrToSrTemplate() *domain.Template {
	return newQualityTemplate(
		"jr-to-sr",
		"Elevate junior developer patterns to idiomatic senior-level Go code",
		"Identify and improve junior developer patterns",
		"jr-to-sr",
		"Elevate code quality from junior to senior patterns",
		"Review the code quality improvements and approve or reject",
	)
}

// NewConstantHunterTemplate creates the constant-hunter template for extracting magic values.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewConstantHunterTemplate() *domain.Template {
	return newQualityTemplate(
		"constant-hunter",
		"Extract magic numbers and hardcoded strings into named constants",
		"Identify and extract magic values into named constants",
		"constant-hunter",
		"Extract magic values into named constants",
		"Review the constant extraction changes and approve or reject",
	)
}

// NewConfigHunterTemplate creates the config-hunter template for centralizing scattered configuration.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewConfigHunterTemplate() *domain.Template {
	return newQualityTemplate(
		"config-hunter",
		"Detect and centralize scattered configuration values into a unified config struct",
		"Identify and centralize scattered configuration values",
		"config-hunter",
		"Centralize scattered configuration values",
		"Review the configuration centralization changes and approve or reject",
	)
}

// NewTestCreatorTemplate creates the test-creator template for generating missing test coverage.
// Steps: analyze_and_fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewTestCreatorTemplate() *domain.Template {
	return newQualityTemplate(
		"test-creator",
		"Generate missing tests for uncovered functions, error paths, and edge cases",
		"Identify gaps and create missing tests",
		"test-creator",
		"Add missing test coverage",
		"Review the new tests and approve or reject",
	)
}
