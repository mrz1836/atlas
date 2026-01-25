package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// NewPatchTemplate creates the patch template for fixing issues on existing branches.
// This template is designed for scenarios where a PR already exists and needs quick fixes.
//
// Unlike other templates that create a new branch and PR, the patch template:
// - Expects to work on an existing branch (use --target flag)
// - Does NOT create a PR (the branch is already in a PR)
// - Pushes directly to the target branch
//
// Workflow:
// 1. detect: Runs validation in detect_only mode to find issues (optional)
// 2. fix: AI receives the actual validation errors and fixes them
// 3. validate: Confirms all fixes work
// 4. git_commit: Commits the changes
// 5. git_push: Pushes to the target branch (no PR creation)
//
// Usage:
//
//	atlas start "fix lint errors" --template patch --target feat/my-feature
func NewPatchTemplate() *domain.Template {
	return &domain.Template{
		Name:         "patch",
		Description:  "Fix issues on an existing branch (no PR creation)",
		BranchPrefix: "patch", // Fallback if --target not used
		DefaultAgent: domain.AgentClaude,
		DefaultModel: "sonnet",
		Verify:       false, // OFF by default (enable with --verify)
		VerifyModel:  "opus",
		Steps: []domain.StepDefinition{
			{
				Name:        "detect",
				Type:        domain.StepTypeValidation,
				Description: "Run validation commands to detect issues",
				Required:    false, // Optional - user may already know what to fix
				Timeout:     10 * time.Minute,
				Config: map[string]any{
					"detect_only": true, // Don't fail, just capture errors
				},
			},
			{
				Name:        "fix",
				Type:        domain.StepTypeAI,
				Description: "Fix detected validation issues",
				Required:    true,
				Timeout:     constants.DefaultAITimeout,
				RetryCount:  3,
				Config: map[string]any{
					"permission_mode":         "default",
					"include_previous_errors": true, // Inject validation errors into prompt
				},
			},
			{
				Name:        "verify",
				Type:        domain.StepTypeVerify,
				Description: "Optional AI verification of fixes",
				Required:    false, // Optional for patch
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
				Description: "Confirm all fixes pass validation",
				Required:    true,
				Timeout:     10 * time.Minute,
				RetryCount:  1,
			},
			newGitCommitStep("Commit patch changes"),
			newGitPushStep(),
			// NO git_pr step - the branch is already in a PR
			// NO ci_wait step - the existing PR will trigger CI
			// NO review step - changes go directly to the branch
		},
		ValidationCommands: defaultValidationCommands,
	}
}
