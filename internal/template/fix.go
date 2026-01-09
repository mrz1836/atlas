package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// NewFixTemplate creates the fix template for automated issue discovery and fixing.
// Steps: detect → fix → validate → git_commit → git_push → git_pr → ci_wait → review
//
// Unlike bugfix (which fixes a known/described bug), the fix template discovers
// issues by running validation commands (lint, format, test) and automatically
// fixes them. If no issues are found, the task completes without creating a PR.
//
// The workflow:
// 1. detect: Runs validation in detect_only mode to find issues without failing
// 2. fix: AI receives the actual validation errors and fixes them
// 3. validate: Confirms all fixes work
// 4. git_commit → git_push → git_pr → ci_wait → review: Standard PR workflow
func NewFixTemplate() *domain.Template {
	return &domain.Template{
		Name:         "fix",
		Description:  "Scan for validation issues and fix them automatically",
		BranchPrefix: "fix",
		DefaultAgent: domain.AgentClaude,
		DefaultModel: "sonnet",
		Verify:       false,
		Steps: []domain.StepDefinition{
			{
				Name:        "detect",
				Type:        domain.StepTypeValidation,
				Description: "Run validation commands to detect issues",
				Required:    true,
				Timeout:     10 * time.Minute,
				Config: map[string]any{
					"detect_only": true, // Don't fail, just capture errors
				},
			},
			{
				Name:        "fix",
				Type:        domain.StepTypeAI,
				Description: "Fix all detected validation issues",
				Required:    true,
				Timeout:     20 * time.Minute,
				RetryCount:  3,
				Config: map[string]any{
					"permission_mode":         "default",
					"include_previous_errors": true, // Inject validation errors into prompt
				},
			},
			{
				Name:        "validate",
				Type:        domain.StepTypeValidation,
				Description: "Confirm all fixes pass validation",
				Required:    true,
				Timeout:     10 * time.Minute,
				RetryCount:  1,
			},
			newGitCommitStep("Create commit with fix changes"),
			newGitPushStep(),
			newGitPRStep(),
			newCIWaitStep(),
			newReviewStep("Human review of automated fixes", "Review the automated fixes and approve or reject"),
		},
		ValidationCommands: defaultValidationCommands,
	}
}
