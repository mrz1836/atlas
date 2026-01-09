package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// NewFeatureTemplate creates the feature template with Speckit SDD integration.
// Steps: specify → review_spec → plan → tasks → implement → verify → validate →
//
//	checklist → git_commit → git_push → git_pr → ci_wait → review
func NewFeatureTemplate() *domain.Template {
	return &domain.Template{
		Name:         "feature",
		Description:  "Develop a new feature with spec-driven development",
		BranchPrefix: "feat",
		DefaultAgent: domain.AgentClaude, // Default to Claude for backwards compatibility
		DefaultModel: "opus",
		Verify:       true, // Verification ON by default for feature (disable with --no-verify)
		VerifyModel:  "",   // Uses different model family automatically
		Steps: []domain.StepDefinition{
			{
				Name:        "specify",
				Type:        domain.StepTypeSDD,
				Description: "Generate specification using Speckit",
				Required:    true,
				Timeout:     20 * time.Minute,
				Config: map[string]any{
					"sdd_command": "specify",
				},
			},
			{
				Name:        "review_spec",
				Type:        domain.StepTypeHuman,
				Description: "Review generated specification",
				Required:    true,
				Config: map[string]any{
					"prompt": "Review the specification and approve or request changes",
				},
			},
			{
				Name:        "plan",
				Type:        domain.StepTypeSDD,
				Description: "Generate implementation plan using Speckit",
				Required:    true,
				Timeout:     15 * time.Minute,
				Config: map[string]any{
					"sdd_command": "plan",
				},
			},
			{
				Name:        "tasks",
				Type:        domain.StepTypeSDD,
				Description: "Generate task breakdown using Speckit",
				Required:    true,
				Timeout:     15 * time.Minute,
				Config: map[string]any{
					"sdd_command": "tasks",
				},
			},
			{
				Name:        "implement",
				Type:        domain.StepTypeSDD,
				Description: "Implement the feature using Speckit",
				Required:    true,
				Timeout:     45 * time.Minute,
				RetryCount:  3,
				Config: map[string]any{
					"sdd_command": "implement",
				},
			},
			{
				Name:        "verify",
				Type:        domain.StepTypeVerify,
				Description: "AI verification of implementation",
				Required:    true, // Required by default for feature
				Timeout:     10 * time.Minute,
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
			{
				Name:        "checklist",
				Type:        domain.StepTypeSDD,
				Description: "Verify implementation against checklist",
				Required:    true,
				Timeout:     10 * time.Minute,
				Config: map[string]any{
					"sdd_command": "checklist",
				},
			},
			newGitCommitStep("Create commit with feature changes"),
			newGitPushStep(),
			newGitPRStep(),
			newCIWaitStep(),
			newReviewStep("Human review of completed feature", "Review the feature implementation and approve or reject"),
		},
		ValidationCommands: defaultValidationCommands,
	}
}
