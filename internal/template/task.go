package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// NewTaskTemplate creates the task template for generic simple tasks.
// Steps: implement → verify (optional) → validate → git_commit → git_push → git_pr → ci_wait → review
func NewTaskTemplate() *domain.Template {
	return &domain.Template{
		Name:         "task",
		Description:  "Complete a simple task with implementation and validation",
		BranchPrefix: "task",
		DefaultModel: "sonnet",
		Verify:       false,  // Verification OFF by default for task (enable with --verify)
		VerifyModel:  "opus", // Uses different model family automatically
		Steps: []domain.StepDefinition{
			{
				Name:        "implement",
				Type:        domain.StepTypeAI,
				Description: "Implement the requested task",
				Required:    true,
				Timeout:     constants.DefaultAITimeout,
				RetryCount:  3,
				Config: map[string]any{
					"permission_mode": "default",
					"prompt_template": "implement_task",
				},
			},
			{
				Name:        "verify",
				Type:        domain.StepTypeVerify,
				Description: "Optional AI verification of implementation",
				Required:    false, // Optional for task
				Timeout:     5 * time.Minute,
				Config: map[string]any{
					"model":  "", // Will use different model family
					"checks": []string{"code_correctness", "garbage_files"},
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
				Description: "Create commit with task changes",
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
				Description: "Human review of completed task",
				Required:    true,
				Config: map[string]any{
					"prompt": "Review the task and approve or reject",
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
