package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// newQualityAnalyzeStep creates the analyze AI step (opus, plan mode) used by all quality templates.
func newQualityAnalyzeStep(description, promptTemplate string) domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "analyze",
		Type:        domain.StepTypeAI,
		Description: description,
		Required:    true,
		Timeout:     constants.DefaultAITimeout,
		RetryCount:  2,
		Config: map[string]any{
			"permission_mode": "plan",
			"prompt_template": promptTemplate,
			"model":           "opus",
		},
	}
}

// newQualityFixStep creates the fix AI step (sonnet via DefaultModel, default mode) used by all quality templates.
func newQualityFixStep(description string) domain.StepDefinition {
	return domain.StepDefinition{
		Name:        "fix",
		Type:        domain.StepTypeAI,
		Description: description,
		Required:    true,
		Timeout:     constants.DefaultAITimeout,
		RetryCount:  3,
		Config: map[string]any{
			"permission_mode": "default",
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

// newQualityTemplate builds a quality template with the standard two-phase step sequence.
func newQualityTemplate(name, description, analyzeDesc, promptTemplate, fixDesc, commitDesc, reviewDesc string) *domain.Template {
	return &domain.Template{
		Name:         name,
		Description:  description,
		BranchPrefix: "quality",
		DefaultAgent: domain.AgentClaude,
		DefaultModel: "sonnet",
		Verify:       false,
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			newQualityAnalyzeStep(analyzeDesc, promptTemplate),
			newQualityFixStep(fixDesc),
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
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewGoOptimizeTemplate() *domain.Template {
	return newQualityTemplate(
		"go-optimize",
		"Modernize Go code using newer language features (version-aware)",
		"Identify Go modernization opportunities and produce a change plan",
		"quality/go_optimize",
		"Apply planned Go optimization improvements",
		"Apply Go optimization improvements",
		"Review the Go optimizations and approve or reject",
	)
}

// NewDedupTemplate creates the dedup template for eliminating duplicated code.
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewDedupTemplate() *domain.Template {
	return newQualityTemplate(
		"dedup",
		"Detect and eliminate duplicated code by extracting shared helpers or generics",
		"Identify duplicated code and produce a deduplication plan",
		"quality/dedup",
		"Eliminate code duplication per the analysis plan",
		"Eliminate code duplication",
		"Review the deduplication changes and approve or reject",
	)
}

// NewGoroutineLeakTemplate creates the goroutine-leak template for detecting and fixing goroutine leaks.
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewGoroutineLeakTemplate() *domain.Template {
	return newQualityTemplate(
		"goroutine-leak",
		"Detect and fix goroutine leaks caused by missing cancellation, context, or channel coordination",
		"Identify goroutine leaks and produce a fix plan",
		"quality/goroutine_leak",
		"Fix goroutine leaks per the analysis plan",
		"Fix goroutine leaks",
		"Review the goroutine leak fixes and approve or reject",
	)
}

// NewJrToSrTemplate creates the jr-to-sr template for elevating junior patterns to senior-level code.
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewJrToSrTemplate() *domain.Template {
	return newQualityTemplate(
		"jr-to-sr",
		"Elevate junior developer patterns to idiomatic senior-level Go code",
		"Identify junior patterns and produce an improvement plan",
		"quality/jr_to_sr",
		"Elevate junior patterns to senior-level code per the plan",
		"Elevate code quality from junior to senior patterns",
		"Review the code quality improvements and approve or reject",
	)
}

// NewConstantHunterTemplate creates the constant-hunter template for extracting magic values.
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewConstantHunterTemplate() *domain.Template {
	return newQualityTemplate(
		"constant-hunter",
		"Extract magic numbers and hardcoded strings into named constants",
		"Identify magic values and produce a constants extraction plan",
		"quality/constant_hunter",
		"Extract magic values into named constants per the plan",
		"Extract magic values into named constants",
		"Review the constant extraction changes and approve or reject",
	)
}

// NewConfigHunterTemplate creates the config-hunter template for centralizing scattered configuration.
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewConfigHunterTemplate() *domain.Template {
	return newQualityTemplate(
		"config-hunter",
		"Detect and centralize scattered configuration values into a unified config struct",
		"Identify scattered config and produce a centralization plan",
		"quality/config_hunter",
		"Centralize configuration per the analysis plan",
		"Centralize scattered configuration values",
		"Review the configuration centralization changes and approve or reject",
	)
}

// NewTestCreatorTemplate creates the test-creator template for generating missing test coverage.
// Steps: analyze (opus, plan) → fix → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewTestCreatorTemplate() *domain.Template {
	return newQualityTemplate(
		"test-creator",
		"Generate missing tests for uncovered functions, error paths, and edge cases",
		"Identify coverage gaps and produce a test creation plan",
		"quality/test_creator",
		"Create missing tests per the analysis plan",
		"Add missing test coverage",
		"Review the new tests and approve or reject",
	)
}
