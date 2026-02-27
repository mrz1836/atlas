// Package workflow provides workflow orchestration for ATLAS task execution.
package workflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
)

// Workspace name generation constants.
const (
	maxWorkspaceNameLen = 50
	// MaxWorkspaceNameLen is exported for testing purposes.
	MaxWorkspaceNameLen = maxWorkspaceNameLen
)

// Regex patterns for workspace name generation.
var (
	// nonAlphanumericRegex matches any character that is not a lowercase letter, digit, or hyphen.
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9-]+`)
	// multipleHyphensRegex matches consecutive hyphens.
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

// Orchestrator coordinates task execution workflow.
type Orchestrator struct {
	services    *ServiceFactory
	initializer *Initializer
	prompter    *Prompter
	logger      zerolog.Logger
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(logger zerolog.Logger, out tui.Output) *Orchestrator {
	return &Orchestrator{
		services:    NewServiceFactory(logger),
		initializer: NewInitializer(logger),
		prompter:    NewPrompter(out),
		logger:      logger,
	}
}

// Services returns the service factory for external access.
func (o *Orchestrator) Services() *ServiceFactory {
	return o.services
}

// Initializer returns the initializer for external access.
func (o *Orchestrator) Initializer() *Initializer {
	return o.initializer
}

// Prompter returns the prompter for external access.
func (o *Orchestrator) Prompter() *Prompter {
	return o.prompter
}

// StartTask starts the task execution and handles errors.
func (o *Orchestrator) StartTask(ctx context.Context, engine *task.Engine, ws *domain.Workspace, tmpl *domain.Template, description, fromBacklogID string) (*domain.Task, error) {
	t, err := engine.Start(ctx, ws.Name, ws.Branch, ws.WorktreePath, tmpl, description, fromBacklogID)
	if err != nil {
		o.logger.Error().Err(err).
			Str("workspace_name", ws.Name).
			Msg("task start failed")
		return t, err
	}
	return t, nil
}

// GenerateWorkspaceName creates a sanitized workspace name from description.
func GenerateWorkspaceName(description string) string {
	name := sanitizeWorkspaceName(description)

	// Handle empty result
	if name == "" {
		name = fmt.Sprintf("task-%s", time.Now().Format(constants.TimeFormatCompact))
	}

	return name
}

// sanitizeWorkspaceName sanitizes a string for use as a workspace name.
func sanitizeWorkspaceName(input string) string {
	// Lowercase and replace spaces with hyphens
	name := strings.ToLower(input)
	name = strings.ReplaceAll(name, " ", "-")

	// Remove special characters
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Collapse multiple hyphens
	name = multipleHyphensRegex.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Truncate to max length
	if len(name) > maxWorkspaceNameLen {
		name = name[:maxWorkspaceNameLen]
		// Don't end with a hyphen
		name = strings.TrimRight(name, "-")
	}

	return name
}

// ApplyAgentModelOverrides applies agent and model overrides to the template.
func ApplyAgentModelOverrides(tmpl *domain.Template, agent, model string) {
	if agent != "" {
		tmpl.DefaultAgent = domain.Agent(agent)
	}
	if model != "" {
		tmpl.DefaultModel = model
	}
}

// ApplyVerifyOverrides applies --verify or --no-verify flag overrides to the template.
// If neither flag is set, the template's default Verify setting is used.
// Also propagates VerifyModel from template to the verify step config, but only if
// the step doesn't have a different agent override (since VerifyModel may not be
// compatible with other agents).
func ApplyVerifyOverrides(tmpl *domain.Template, verify, noVerify bool) {
	// CLI flags override template defaults
	if verify {
		tmpl.Verify = true
	} else if noVerify {
		tmpl.Verify = false
	}

	// Update the verify step's Required field and model based on the template settings
	for i := range tmpl.Steps {
		if tmpl.Steps[i].Type == domain.StepTypeVerify {
			applyVerifyToStep(tmpl, &tmpl.Steps[i])
		}
	}
}

// applyVerifyToStep applies verify settings to a single verify step.
func applyVerifyToStep(tmpl *domain.Template, step *domain.StepDefinition) {
	step.Required = tmpl.Verify

	// Check if step has a different agent override
	stepHasDifferentAgent := stepHasDifferentAgent(step, tmpl.DefaultAgent)

	// Propagate VerifyModel from template to step config if applicable
	if shouldPropagateVerifyModel(tmpl.VerifyModel, stepHasDifferentAgent) {
		propagateVerifyModel(step, tmpl.VerifyModel)
	}
}

// stepHasDifferentAgent checks if the step has an agent override different from the default.
func stepHasDifferentAgent(step *domain.StepDefinition, defaultAgent domain.Agent) bool {
	if step.Config == nil {
		return false
	}

	stepAgent, ok := step.Config["agent"].(string)
	if !ok || stepAgent == "" {
		return false
	}

	return domain.Agent(stepAgent) != defaultAgent
}

// shouldPropagateVerifyModel determines if VerifyModel should be propagated to the step.
func shouldPropagateVerifyModel(verifyModel string, stepHasDifferentAgent bool) bool {
	return verifyModel != "" && !stepHasDifferentAgent
}

// propagateVerifyModel sets the VerifyModel on a step's config if not already set.
func propagateVerifyModel(step *domain.StepDefinition, verifyModel string) {
	if step.Config == nil {
		step.Config = make(map[string]any)
	}

	// Only set if not already configured in step
	if model, ok := step.Config["model"].(string); !ok || model == "" {
		step.Config["model"] = verifyModel
	}
}

// StoreCLIOverrides saves CLI flag overrides to task metadata for resume.
// Only stores flags that were explicitly set (non-zero values).
// This allows `atlas resume` to restore the original execution context.
func StoreCLIOverrides(task *domain.Task, verify, noVerify bool, agent, model string) {
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}
	// Only store if flags were explicitly set
	if verify {
		task.Metadata[constants.MetaKeyVerifyOverride] = true
	}
	if noVerify {
		task.Metadata[constants.MetaKeyNoVerifyOverride] = true
	}
	if agent != "" {
		task.Metadata[constants.MetaKeyAgentOverride] = agent
	}
	if model != "" {
		task.Metadata[constants.MetaKeyModelOverride] = model
	}
}

// ApplyCLIOverridesFromTask re-applies CLI overrides from task metadata to template.
// This is called during `atlas resume` to restore the original execution context
// from the flags used during `atlas start`.
func ApplyCLIOverridesFromTask(task *domain.Task, tmpl *domain.Template) {
	if task.Metadata == nil {
		return
	}

	verify, _ := task.Metadata[constants.MetaKeyVerifyOverride].(bool)
	noVerify, _ := task.Metadata[constants.MetaKeyNoVerifyOverride].(bool)
	agent, _ := task.Metadata[constants.MetaKeyAgentOverride].(string)
	model, _ := task.Metadata[constants.MetaKeyModelOverride].(string)

	ApplyVerifyOverrides(tmpl, verify, noVerify)
	ApplyAgentModelOverrides(tmpl, agent, model)
}
