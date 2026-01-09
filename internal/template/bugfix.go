package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// NewBugfixTemplate creates the bugfix template for fixing bugs.
// Steps: analyze → implement → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewBugfixTemplate() *domain.Template {
	return &domain.Template{
		Name:         "bugfix",
		Description:  "Fix a reported bug with analysis, implementation, and validation",
		BranchPrefix: "fix",
		DefaultAgent: domain.AgentClaude, // Default to Claude for backwards compatibility
		DefaultModel: "sonnet",
		Verify:       false,  // Verification OFF by default for bugfix (enable with --verify)
		VerifyModel:  "opus", // Uses different model family automatically
		Steps: []domain.StepDefinition{
			{
				Name:        "analyze",
				Type:        domain.StepTypeAI,
				Description: "Analyze the bug report and identify root cause",
				Required:    true,
				Timeout:     15 * time.Minute,
				RetryCount:  2,
				Config: map[string]any{
					"permission_mode": "plan",
					"prompt_template": "analyze_bug",
				},
			},
			{
				Name:        "implement",
				Type:        domain.StepTypeAI,
				Description: "Implement the fix for the identified issue",
				Required:    true,
				Timeout:     constants.DefaultAITimeout,
				RetryCount:  3,
				Config: map[string]any{
					"permission_mode": "default",
					"prompt_template": "implement_fix",
				},
			},
			{
				Name:        "verify",
				Type:        domain.StepTypeVerify,
				Description: "Optional AI verification of implementation",
				Required:    false, // Optional for bugfix
				Timeout:     5 * time.Minute,
				Config: map[string]any{
					"agent":  "gemini",                     // Use Gemini for verification
					"model":  "",                           // Will use Gemini default (flash)
					"checks": []string{"code_correctness"}, // Add test_coverage, garbage_files, security for deeper checks
				},
			},
			{
				Name:        "validate",
				Type:        domain.StepTypeValidation,
				Description: "Run format, lint, and test commands",
				Required:    true,
				Timeout:     10 * time.Minute,
				RetryCount:  1,
			},
			newGitCommitStep("Create commit with fix changes"),
			newGitPushStep(),
			newGitPRStep(),
			newCIWaitStep(),
			newReviewStep("Human review of completed fix", "Review the fix and approve or reject"),
		},
		ValidationCommands: defaultValidationCommands,
	}
}
