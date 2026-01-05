package template

import (
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// NewCommitTemplate creates the commit template for smart commits.
// Steps: analyze_changes → smart_commit → git_push
func NewCommitTemplate() *domain.Template {
	return &domain.Template{
		Name:         "commit",
		Description:  "Analyze changes and create smart commits with garbage detection",
		BranchPrefix: "chore",
		DefaultAgent: domain.AgentClaude, // Default to Claude for backwards compatibility
		DefaultModel: "sonnet",
		Steps: []domain.StepDefinition{
			{
				Name:        "analyze_changes",
				Type:        domain.StepTypeAI,
				Description: "Analyze working tree changes and detect garbage files",
				Required:    true,
				Timeout:     5 * time.Minute,
				Config: map[string]any{
					"permission_mode": "plan",
					"detect_garbage":  true,
					"garbage_patterns": []string{
						"*.tmp", "*.bak", "*.log",
						"node_modules/**", ".DS_Store",
						"*.exe", "*.dll",
						".env*", "*credentials*",
					},
				},
			},
			{
				Name:        "smart_commit",
				Type:        domain.StepTypeGit,
				Description: "Create logical commits with meaningful messages",
				Required:    true,
				Timeout:     2 * time.Minute,
				Config: map[string]any{
					"operation":        "smart_commit",
					"group_by_package": true,
					"conventional":     true,
				},
			},
			{
				Name:        "git_push",
				Type:        domain.StepTypeGit,
				Description: "Push commits to remote",
				Required:    true,
				Timeout:     2 * time.Minute,
				RetryCount:  3,
				Config: map[string]any{
					"operation": "push",
				},
			},
		},
	}
}
