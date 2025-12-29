package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// NewBugfixTemplate creates the bugfix template for fixing bugs.
// Steps: analyze → implement → validate → git_commit → git_push → git_pr → ci_wait → review
func NewBugfixTemplate() *domain.Template {
	return &domain.Template{
		Name:         "bugfix",
		Description:  "Fix a reported bug with analysis, implementation, and validation",
		BranchPrefix: "fix/",
		DefaultModel: "sonnet",
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
				Name:        "validate",
				Type:        domain.StepTypeValidation,
				Description: "Run format, lint, and test commands",
				Required:    true,
				Timeout:     10 * time.Minute,
				RetryCount:  1,
			},
			{
				Name:        "git_commit",
				Type:        domain.StepTypeGit,
				Description: "Create commit with fix changes",
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
				Description: "Human review of completed fix",
				Required:    true,
				Config: map[string]any{
					"prompt": "Review the fix and approve or reject",
				},
			},
		},
		ValidationCommands: []string{
			"magex format:fix",
			"magex lint",
			"magex test",
		},
	}
}
