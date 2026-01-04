package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
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
					"model":  "", // Will use different model family
					"checks": []string{"code_correctness", "test_coverage", "garbage_files", "security"},
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
			{
				Name:        "git_commit",
				Type:        domain.StepTypeGit,
				Description: "Create commit with feature changes",
				Required:    true,
				Timeout:     1 * time.Minute,
				Config: map[string]any{
					"operation": "commit",
				},
			},
			{
				Name:        "git_push",
				Type:        domain.StepTypeGit,
				Description: "Push branch to remote",
				Required:    true,
				Timeout:     2 * time.Minute,
				RetryCount:  3,
				Config: map[string]any{
					"operation": "push",
				},
			},
			{
				Name:        "git_pr",
				Type:        domain.StepTypeGit,
				Description: "Create pull request",
				Required:    true,
				Timeout:     2 * time.Minute,
				RetryCount:  2,
				Config: map[string]any{
					"operation": "create_pr",
				},
			},
			{
				Name:        "ci_wait",
				Type:        domain.StepTypeCI,
				Description: "Wait for CI pipeline to complete",
				Required:    true,
				Timeout:     constants.DefaultCITimeout,
				Config: map[string]any{
					"poll_interval": constants.CIPollInterval,
				},
			},
			{
				Name:        "review",
				Type:        domain.StepTypeHuman,
				Description: "Human review of completed feature",
				Required:    true,
				Config: map[string]any{
					"prompt": "Review the feature implementation and approve or reject",
				},
			},
		},
		ValidationCommands: []string{
			"magex format:fix",
			"magex lint",
			"magex test:race",
			"go-pre-commit run --all-files",
		},
	}
}
