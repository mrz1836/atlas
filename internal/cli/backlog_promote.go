package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/backlog"
	"github.com/mrz1836/atlas/internal/cli/workflow"
	"github.com/mrz1836/atlas/internal/config"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
)

// promoteOptions holds the flags for the promote command.
type promoteOptions struct {
	// template overrides the auto-detected template.
	template string

	// ai enables AI-assisted analysis.
	ai bool

	// agent overrides the AI agent.
	agent string

	// model overrides the AI model.
	model string

	// dryRun shows what would happen without executing.
	dryRun bool

	// jsonOutput enables JSON output.
	jsonOutput bool
}

// newBacklogPromoteCmd creates the backlog promote command.
func newBacklogPromoteCmd() *cobra.Command {
	var opts promoteOptions

	cmd := &cobra.Command{
		Use:   "promote <id>",
		Short: "Promote a discovery to a task",
		Long: `Promote a discovery to a task, creating the task configuration automatically.

Generates task configuration from the discovery based on category and severity.
Critical security issues use the hotfix template; bugs use bugfix; other
categories use the task template.

The --ai flag enables AI-assisted analysis to determine the optimal task
configuration (template, description, workspace name).

The --dry-run flag shows what would happen without making any changes.

Examples:
  # Auto-create task from discovery (deterministic mapping)
  atlas backlog promote disc-abc123

  # Preview what would happen
  atlas backlog promote disc-abc123 --dry-run

  # Use AI to determine optimal task configuration
  atlas backlog promote disc-abc123 --ai

  # Override template selection
  atlas backlog promote disc-abc123 --template feature

Exit codes:
  0: Success
  1: Discovery not found or error
  2: Invalid input (discovery not pending, conflicting flags)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogPromote(cmd.Context(), cmd, cmd.OutOrStdout(), args[0], opts)
		},
	}

	// Flags
	cmd.Flags().StringVarP(&opts.template, "template", "t", "", "Override template selection (bugfix, feature, task, hotfix)")
	cmd.Flags().BoolVar(&opts.ai, "ai", false, "Use AI to determine optimal task configuration")
	cmd.Flags().StringVar(&opts.agent, "agent", "", "Override AI agent (claude, gemini, codex)")
	cmd.Flags().StringVar(&opts.model, "model", "", "Override AI model")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show what would happen without executing")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "Output as JSON")

	return cmd
}

// runBacklogPromote executes the backlog promote command.
func runBacklogPromote(ctx context.Context, cmd *cobra.Command, w io.Writer, id string, opts promoteOptions) error {
	outputFormat := getOutputFormat(cmd, opts.jsonOutput)
	out := tui.NewOutput(w, outputFormat)

	// Validate flags
	if opts.template != "" && !backlog.IsValidTemplateName(opts.template) {
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: invalid template %q, valid templates are: %s",
				atlaserrors.ErrInvalidArgument, opts.template, strings.Join(backlog.ValidTemplateNames(), ", ")))
	}

	// Create manager
	mgr, err := backlog.NewManager("")
	if err != nil {
		return outputBacklogError(w, outputFormat, "promote", err)
	}

	// Build promote options
	promoteOpts := backlog.PromoteOptions{
		Template: opts.template,
		Agent:    opts.agent,
		Model:    opts.model,
		UseAI:    opts.ai,
		DryRun:   opts.dryRun,
	}

	// Create AI promoter if AI mode is enabled
	var aiPromoter *backlog.AIPromoter
	if opts.ai {
		// Load config to get AI settings
		cfg, cfgErr := config.Load(ctx)
		if cfgErr != nil {
			// Use defaults if config loading fails
			cfg = config.DefaultConfig()
		}

		// Create AI runner
		aiRunner := workflow.CreateAIRunner(cfg)
		aiPromoter = backlog.NewAIPromoter(aiRunner, &cfg.AI)

		// Show progress for AI analysis (only in non-JSON mode)
		if outputFormat != OutputJSON {
			aiCfg := &backlog.AIPromoterConfig{
				Agent: opts.agent,
				Model: opts.model,
			}
			agent, model := aiPromoter.ResolvedConfig(aiCfg)
			out.Info(fmt.Sprintf("AI Analysis (%s/%s)...", agent, model))
		}
	}

	// Promote with options
	result, err := mgr.PromoteWithOptions(ctx, id, promoteOpts, aiPromoter)
	if err != nil {
		// Check if this is an invalid transition error
		if atlaserrors.IsExitCode2Error(err) {
			return err
		}
		return outputBacklogError(w, outputFormat, "promote", err)
	}

	// Show completion for AI analysis
	if opts.ai && outputFormat != OutputJSON {
		out.Success("AI Analysis complete")
	}

	// Output results
	if outputFormat == OutputJSON {
		return outputPromoteResultJSON(out, result)
	}

	displayPromoteResult(out, result)

	// Add informational note
	if !result.DryRun {
		out.Info("")
		out.Info("Note: Discovery status will change to 'promoted' when you run the start command.")
	}

	return nil
}

// outputPromoteResultJSON outputs the promote result as JSON.
func outputPromoteResultJSON(out tui.Output, result *backlog.PromoteResult) error {
	response := map[string]any{
		"success":        true,
		"id":             result.Discovery.ID,
		"status":         result.Discovery.Status,
		"dry_run":        result.DryRun,
		"template":       result.TemplateName,
		"workspace_name": result.WorkspaceName,
		"branch_name":    result.BranchName,
		"description":    result.Description,
		"start_command":  buildStartCommand(result),
	}

	if result.AIAnalysis != nil {
		aiMap := map[string]any{
			"template":       result.AIAnalysis.Template,
			"description":    result.AIAnalysis.Description,
			"reasoning":      result.AIAnalysis.Reasoning,
			"workspace_name": result.AIAnalysis.WorkspaceName,
			"priority":       result.AIAnalysis.Priority,
		}
		if result.AIAnalysis.BaseBranch != "" {
			aiMap["base_branch"] = result.AIAnalysis.BaseBranch
		}
		if result.AIAnalysis.UseVerify != nil {
			aiMap["use_verify"] = *result.AIAnalysis.UseVerify
		}
		response["ai_analysis"] = aiMap
	}

	response["discovery"] = result.Discovery

	return out.JSON(response)
}

// displayPromoteResult displays the promote result in text format.
func displayPromoteResult(out tui.Output, result *backlog.PromoteResult) {
	if result.DryRun {
		out.Info("Dry-run mode: showing what would happen\n")
	}

	out.Info(fmt.Sprintf("Promoting discovery: %s", result.Discovery.ID))
	out.Info(fmt.Sprintf("  Title:    %s", result.Discovery.Title))
	out.Info(fmt.Sprintf("  Category: %s", result.Discovery.Content.Category))
	out.Info(fmt.Sprintf("  Severity: %s", result.Discovery.Content.Severity))

	// Show generated configuration
	out.Info("\nTask configuration:")
	out.Info(fmt.Sprintf("  Template:  %s", result.TemplateName))
	out.Info(fmt.Sprintf("  Workspace: %s", result.WorkspaceName))
	out.Info(fmt.Sprintf("  Branch:    %s", result.BranchName))

	if result.AIAnalysis != nil {
		out.Info("\nAI Analysis:")
		out.Info(fmt.Sprintf("  Reasoning: %s", result.AIAnalysis.Reasoning))
		if result.AIAnalysis.Priority > 0 {
			out.Info(fmt.Sprintf("  Priority:  %d/5", result.AIAnalysis.Priority))
		}
	}

	// Build the suggested command with all flags
	startCmd := buildStartCommand(result)

	if result.DryRun {
		out.Text("\nTo create the task, run without --dry-run:")
		out.Text(fmt.Sprintf("  atlas backlog promote %s", result.Discovery.ID))
		if result.TemplateName != "" {
			out.Text("\nOr start the task directly with:")
			out.Text(fmt.Sprintf("  %s \\", startCmd))
			out.Text(fmt.Sprintf("    %q", result.Discovery.Title))
		}
	} else {
		// Not dry-run: show instructions for next steps
		out.Success(fmt.Sprintf("\nDiscovery %s ready for task creation", result.Discovery.ID))
		out.Text("\nTo create and start the task, run:")
		out.Text(fmt.Sprintf("  %s \\", startCmd))
		out.Text(fmt.Sprintf("    %q", result.Discovery.Title))
		out.Text(fmt.Sprintf("\nDiscovery file: .atlas/backlog/%s.yaml", result.Discovery.ID))
	}
}

// buildStartCommand constructs the atlas start command with all recommended flags.
func buildStartCommand(result *backlog.PromoteResult) string {
	// Start with base command (template and workspace)
	cmd := fmt.Sprintf("atlas start -t %s -w %s",
		result.TemplateName, result.WorkspaceName)

	// Use the branch where the discovery was made as the base branch (explicit, no assumptions)
	if result.Discovery.Context.Git != nil && result.Discovery.Context.Git.Branch != "" {
		cmd += fmt.Sprintf(" -b %s", result.Discovery.Context.Git.Branch)
	}

	// Add AI-recommended flags
	if result.AIAnalysis != nil && result.AIAnalysis.UseVerify != nil {
		if *result.AIAnalysis.UseVerify {
			cmd += " --verify"
		} else {
			cmd += " --no-verify"
		}
	}

	// Add backlog link
	cmd += fmt.Sprintf(" --from-backlog %s", result.Discovery.ID)

	return cmd
}

// truncateDescription truncates a description to a maximum length.
func truncateDescription(desc string, maxLen int) string {
	// Take only the first line
	if idx := strings.Index(desc, "\n"); idx != -1 {
		desc = desc[:idx]
	}

	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
}
