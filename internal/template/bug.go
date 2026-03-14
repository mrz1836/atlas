package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// NewBugTemplate creates the consolidated bug template for fixing bugs.
//
// This template intelligently selects between two modes based on description length:
//
// With description (>20 chars): analyze → implement → verify → validate → git workflow
//
//	When the user provides a bug description, we analyze the bug report first.
//
// Without description (≤20 chars): detect → implement → verify → validate → git workflow
//
//	When the user provides just "fix lint" or similar, we detect issues first.
//
// Steps use skip_condition to automatically select the appropriate path:
//   - "detect" step: skipped when has_description (user described the bug)
//   - "analyze" step: skipped when no_description (detect mode)
//
// The implement step receives context from whichever analysis step ran:
//   - From analyze: the bug analysis and root cause
//   - From detect: the validation errors to fix
func NewBugTemplate() *domain.Template {
	return &domain.Template{
		Name:         "bug",
		Description:  "Fix bugs with smart detection: analyzes described bugs or auto-detects validation issues",
		BranchPrefix: "fix",
		DefaultAgent: domain.AgentClaude,
		DefaultModel: "sonnet",
		Verify:       false,  // Verification OFF by default (enable with --verify)
		VerifyModel:  "opus", // Uses different model family automatically
		Steps: []domain.StepDefinition{
			{
				Name:        "detect",
				Type:        domain.StepTypeValidation,
				Description: "Run validation commands to detect issues (skipped if bug description provided)",
				Required:    true, // Required when active, but skip_condition controls activation
				Timeout:     10 * time.Minute,
				Config: map[string]any{
					"detect_only":    true,              // Don't fail, just capture errors
					"skip_condition": "has_description", // Skip when user described the bug
				},
			},
			{
				Name:        "analyze",
				Type:        domain.StepTypeAI,
				Description: "Analyze the bug report and identify root cause (skipped if no description)",
				Required:    true, // Required when active, but skip_condition controls activation
				Timeout:     15 * time.Minute,
				RetryCount:  2,
				Config: map[string]any{
					"permission_mode": "plan",
					"prompt_template": "analyze_bug",
					"skip_condition":  "no_description", // Skip when using detect mode
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
					"permission_mode":         "default",
					"prompt_template":         "implement_fix",
					"include_previous_errors": true, // Receives detect output if available
				},
			},
			{
				Name:        "verify",
				Type:        domain.StepTypeVerify,
				Description: "Optional AI verification of implementation",
				Required:    false, // Optional - enable with --verify flag
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
