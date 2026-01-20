package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/backlog"
	"github.com/mrz1836/atlas/internal/cli/workflow"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
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
		Use:   "promote [id]",
		Short: "Promote a discovery to a task",
		Long: `Promote a discovery to a task, creating the task configuration automatically.

When called without arguments in a terminal, launches an interactive menu to
select a discovery and configure promotion options.

When called with an ID argument, promotes the discovery directly.

Generates task configuration from the discovery based on category and severity.
Critical security issues use the hotfix template; bugs use bugfix; other
categories use the task template.

The --ai flag enables AI-assisted analysis to determine the optimal task
configuration (template, description, workspace name).

The --dry-run flag shows what would happen without making any changes.

Examples:
  # Interactive mode (select discovery from menu)
  atlas backlog promote

  # Direct mode with discovery ID
  atlas backlog promote item-ABC123

  # Preview what would happen
  atlas backlog promote item-ABC123 --dry-run

  # Use AI to determine optimal task configuration
  atlas backlog promote item-ABC123 --ai

  # Override template selection
  atlas backlog promote item-ABC123 --template feature

Exit codes:
  0: Success
  1: Discovery not found or error
  2: Invalid input (discovery not pending, conflicting flags, ID required)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			return runBacklogPromote(cmd.Context(), cmd, cmd.OutOrStdout(), id, opts)
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

// isPromoteInteractiveMode determines if the promote command should run in interactive mode.
// Interactive mode is used when: no ID provided, not JSON output, and running in a terminal.
func isPromoteInteractiveMode(id string, opts promoteOptions) bool {
	if id != "" {
		return false // ID provided = direct mode
	}
	if opts.jsonOutput {
		return false // JSON = non-interactive
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// runBacklogPromote executes the backlog promote command.
func runBacklogPromote(ctx context.Context, cmd *cobra.Command, w io.Writer, id string, opts promoteOptions) error {
	outputFormat := getOutputFormat(cmd, opts.jsonOutput)
	out := tui.NewOutput(w, outputFormat)

	// Check for interactive mode
	if isPromoteInteractiveMode(id, opts) {
		return runBacklogPromoteInteractive(ctx, cmd, w, opts)
	}

	// Require ID in non-interactive mode
	if id == "" {
		if opts.jsonOutput {
			return atlaserrors.NewExitCode2Error(
				fmt.Errorf("%w: ID required with --json flag", atlaserrors.ErrUserInputRequired))
		}
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: ID required in non-interactive mode (not a terminal)", atlaserrors.ErrUserInputRequired))
	}

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

	// Detect available AI agents
	availableAgents := detectAvailableAgents(ctx)

	// Build promote options
	promoteOpts := backlog.PromoteOptions{
		Template:        opts.template,
		Agent:           opts.agent,
		Model:           opts.model,
		UseAI:           opts.ai,
		DryRun:          opts.dryRun,
		AvailableAgents: availableAgents,
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
				Agent:           opts.agent,
				Model:           opts.model,
				AvailableAgents: availableAgents,
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
		response["ai_analysis"] = buildAIAnalysisMap(result.AIAnalysis)
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
		displayAIAnalysis(out, result.AIAnalysis)
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

// buildAIAnalysisMap creates a map representation of AI analysis for JSON output.
func buildAIAnalysisMap(ai *backlog.AIAnalysis) map[string]any {
	aiMap := map[string]any{
		"template":       ai.Template,
		"description":    ai.Description,
		"reasoning":      ai.Reasoning,
		"workspace_name": ai.WorkspaceName,
		"priority":       ai.Priority,
	}
	if ai.BaseBranch != "" {
		aiMap["base_branch"] = ai.BaseBranch
	}
	if ai.UseVerify != nil {
		aiMap["use_verify"] = *ai.UseVerify
	}
	if ai.File != "" {
		aiMap["file"] = ai.File
	}
	if ai.Line > 0 {
		aiMap["line"] = ai.Line
	}
	return aiMap
}

// displayAIAnalysis displays AI analysis information in text format.
func displayAIAnalysis(out tui.Output, ai *backlog.AIAnalysis) {
	out.Info("\nAI Analysis:")
	out.Info(fmt.Sprintf("  Reasoning: %s", ai.Reasoning))
	if ai.File != "" {
		if ai.Line > 0 {
			out.Info(fmt.Sprintf("  Location:  %s:%d", ai.File, ai.Line))
		} else {
			out.Info(fmt.Sprintf("  Location:  %s", ai.File))
		}
	}
	if ai.Priority > 0 {
		out.Info(fmt.Sprintf("  Priority:  %d/5", ai.Priority))
	}
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

// detectAvailableAgents detects which AI agent CLIs are installed.
// Returns a slice of agent names (e.g., ["claude", "gemini"]).
func detectAvailableAgents(ctx context.Context) []string {
	detector := config.NewToolDetector()
	result, err := detector.Detect(ctx)
	if err != nil {
		return nil
	}

	var agents []string
	agentTools := []string{constants.ToolClaude, constants.ToolGemini, constants.ToolCodex}

	for _, tool := range result.Tools {
		for _, agentTool := range agentTools {
			if tool.Name == agentTool && tool.Status == config.ToolStatusInstalled {
				agents = append(agents, tool.Name)
			}
		}
	}

	return agents
}

// runBacklogPromoteInteractive runs the interactive mode for promoting a discovery.
func runBacklogPromoteInteractive(ctx context.Context, cmd *cobra.Command, w io.Writer, opts promoteOptions) error {
	out := tui.NewOutput(w, "")

	// Create manager
	mgr, err := backlog.NewManager("")
	if err != nil {
		return fmt.Errorf("failed to create backlog manager: %w", err)
	}

	// List pending discoveries
	pendingStatus := backlog.StatusPending
	discoveries, _, err := mgr.List(ctx, backlog.Filter{Status: &pendingStatus})
	if err != nil {
		return fmt.Errorf("failed to list discoveries: %w", err)
	}

	// Check if there are any pending discoveries
	if len(discoveries) == 0 {
		out.Info("No pending discoveries to promote.")
		return nil
	}

	// Build options from discoveries
	options := buildDiscoveryOptions(discoveries)

	// Select a discovery
	selectedID, err := tui.Select("Select a discovery to promote:", options)
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Promotion canceled.")
			return nil
		}
		return fmt.Errorf("selection failed: %w", err)
	}

	// Ask about AI mode (unless already specified via flag)
	useAI := opts.ai
	if !opts.ai {
		useAI, err = tui.Confirm("Use AI to determine optimal task configuration?", false)
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Promotion canceled.")
				return nil
			}
			return fmt.Errorf("confirmation failed: %w", err)
		}
	}

	// If not using AI, optionally select template override
	templateOverride := opts.template
	if !useAI && templateOverride == "" {
		templateOverride, err = selectTemplateOverride()
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Promotion canceled.")
				return nil
			}
			return fmt.Errorf("template selection failed: %w", err)
		}
	}

	// Build options for promotion
	promoteOpts := opts
	promoteOpts.ai = useAI
	if templateOverride != "" {
		promoteOpts.template = templateOverride
	}

	// Execute promotion with selected ID
	return runBacklogPromote(ctx, cmd, w, selectedID, promoteOpts)
}

// buildDiscoveryOptions builds TUI options from discoveries.
// Format: "[item-ABC123] Title truncated to 50 chars"
// Description: "bug/high | 2h ago"
func buildDiscoveryOptions(discoveries []*backlog.Discovery) []tui.Option {
	const maxTitleLen = 50
	options := make([]tui.Option, len(discoveries))

	for i, d := range discoveries {
		// Truncate title for display
		title := d.Title
		if len(title) > maxTitleLen {
			title = title[:maxTitleLen-3] + "..."
		}

		// Build label with ID and title
		label := fmt.Sprintf("[%s] %s", d.ID, title)

		// Build description with category/severity and relative time
		relTime := tui.RelativeTime(d.Context.DiscoveredAt)
		desc := fmt.Sprintf("%s/%s | %s", d.Content.Category, d.Content.Severity, relTime)

		options[i] = tui.Option{
			Label:       label,
			Description: desc,
			Value:       d.ID,
		}
	}

	return options
}

// selectTemplateOverride presents a menu to optionally select a template override.
// Returns empty string for auto-detect, or the selected template name.
func selectTemplateOverride() (string, error) {
	options := []tui.Option{
		{
			Label:       "Auto-detect",
			Description: "Based on category/severity",
			Value:       "",
		},
		{
			Label:       "bugfix",
			Description: "Fix bugs and regressions",
			Value:       "bugfix",
		},
		{
			Label:       "feature",
			Description: "New feature development",
			Value:       "feature",
		},
		{
			Label:       "hotfix",
			Description: "Critical production fixes",
			Value:       "hotfix",
		},
		{
			Label:       "task",
			Description: "General development tasks",
			Value:       "task",
		},
	}

	return tui.Select("Select task template:", options)
}
