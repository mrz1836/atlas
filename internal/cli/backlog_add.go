package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/backlog"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/tui"
)

// backlogAddFlags holds the flags for the add command.
type backlogAddFlags struct {
	file        string
	line        int
	category    string
	severity    string
	description string
	tags        string
	json        bool
	projectRoot string // used for testing
}

// newBacklogAddCmd creates the backlog add command.
func newBacklogAddCmd() *cobra.Command {
	flags := &backlogAddFlags{}

	cmd := &cobra.Command{
		Use:   "add [title]",
		Short: "Add a new discovery to the backlog",
		Long: `Add a new discovery to the work backlog.

When called without a title argument, launches an interactive form (for humans).
When called with a title argument and flags, adds the discovery directly (for AI/scripts).

The discoverer is automatically detected:
- Interactive mode: human:<github-username>
- Flag mode with --ai: ai:<agent>:<model>
- Flag mode with env vars: ai:<ATLAS_AGENT>:<ATLAS_MODEL>

Examples:
  # Interactive mode (for humans)
  atlas backlog add

  # Flag mode (for AI/scripts)
  atlas backlog add "Missing error handling" --file main.go --line 47 \
    --category bug --severity high --description "Details here"

  # With tags
  atlas backlog add "Add unit tests" --category testing --severity medium \
    --tags "tests,coverage"

Exit codes:
  0: Success
  1: General error (IO, validation)
  2: Invalid input (missing required flags)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogAdd(cmd.Context(), cmd, cmd.OutOrStdout(), args, flags)
		},
	}

	cmd.Flags().StringVarP(&flags.file, "file", "f", "", "File path where issue was found")
	cmd.Flags().IntVarP(&flags.line, "line", "l", 0, "Line number in file")
	cmd.Flags().StringVarP(&flags.category, "category", "c", "", "Issue category (bug, security, performance, maintainability, testing, documentation)")
	cmd.Flags().StringVarP(&flags.severity, "severity", "s", "", "Priority level (low, medium, high, critical)")
	cmd.Flags().StringVarP(&flags.description, "description", "d", "", "Detailed explanation")
	cmd.Flags().StringVarP(&flags.tags, "tags", "t", "", "Comma-separated labels")
	cmd.Flags().BoolVar(&flags.json, "json", false, "Output created discovery as JSON")

	return cmd
}

// runBacklogAdd executes the backlog add command.
func runBacklogAdd(ctx context.Context, cmd *cobra.Command, w io.Writer, args []string, flags *backlogAddFlags) error {
	outputFormat := getOutputFormat(cmd, flags.json)
	out := tui.NewOutput(w, outputFormat)

	// Create manager
	mgr, err := backlog.NewManager(flags.projectRoot)
	if err != nil {
		return outputBacklogError(w, outputFormat, "add", err)
	}

	// Determine mode and get discovery
	discovery, err := resolveBacklogAddMode(ctx, mgr, args, flags)
	if err != nil {
		if atlaserrors.IsExitCode2Error(err) {
			return err
		}
		return outputBacklogError(w, outputFormat, "add", err)
	}

	// Add the discovery
	if err := mgr.Add(ctx, discovery); err != nil {
		return outputBacklogError(w, outputFormat, "add", err)
	}

	// Output result
	if outputFormat == OutputJSON {
		return out.JSON(discovery)
	}

	displayBacklogAddSuccess(out, discovery)
	return nil
}

// hasAnyBacklogAddFlags checks if any flags were provided.
func hasAnyBacklogAddFlags(flags *backlogAddFlags) bool {
	return flags.file != "" || flags.line != 0 || flags.category != "" ||
		flags.severity != "" || flags.description != "" || flags.tags != ""
}

// resolveBacklogAddMode determines whether to use interactive or flag mode and returns a discovery.
func resolveBacklogAddMode(ctx context.Context, mgr *backlog.Manager, args []string, flags *backlogAddFlags) (*backlog.Discovery, error) {
	// Interactive mode: no args and no flags
	if len(args) == 0 && !hasAnyBacklogAddFlags(flags) {
		return runBacklogAddInteractive(ctx, mgr)
	}

	// Flag mode: validate required inputs
	if len(args) == 0 {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: title argument is required in flag mode", atlaserrors.ErrUserInputRequired))
	}
	if flags.category == "" {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: --category flag is required", atlaserrors.ErrUserInputRequired))
	}
	if flags.severity == "" {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: --severity flag is required", atlaserrors.ErrUserInputRequired))
	}

	return buildDiscoveryFromFlags(ctx, mgr, args[0], flags)
}

// buildDiscoveryFromFlags creates a Discovery from command flags.
func buildDiscoveryFromFlags(ctx context.Context, mgr *backlog.Manager, title string, flags *backlogAddFlags) (*backlog.Discovery, error) {
	// Parse category
	category := backlog.Category(flags.category)
	if !category.IsValid() {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: invalid category %q, must be one of: %v",
				atlaserrors.ErrInvalidArgument, flags.category, backlog.ValidCategories()))
	}

	// Parse severity
	severity := backlog.Severity(flags.severity)
	if !severity.IsValid() {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: invalid severity %q, must be one of: %v",
				atlaserrors.ErrInvalidArgument, flags.severity, backlog.ValidSeverities()))
	}

	// Parse tags
	var tags []string
	if flags.tags != "" {
		for _, tag := range strings.Split(flags.tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	// Build discovery
	d := &backlog.Discovery{
		Title:  title,
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Description: flags.description,
			Category:    category,
			Severity:    severity,
			Tags:        tags,
		},
		Context: backlog.Context{
			DiscoveredBy: detectDiscoverer(ctx, mgr.ProjectRoot(), false),
		},
	}

	// Add location if provided
	if flags.file != "" || flags.line > 0 {
		d.Location = &backlog.Location{
			File: flags.file,
			Line: flags.line,
		}
	}

	return d, nil
}

// runBacklogAddInteractive runs the interactive form for adding a discovery.
func runBacklogAddInteractive(ctx context.Context, mgr *backlog.Manager) (*backlog.Discovery, error) {
	var (
		title       string
		description string
		category    string
		severity    string
		file        string
		line        int
		lineStr     string
	)

	// Build category options
	categoryOptions := make([]huh.Option[string], 0, len(backlog.ValidCategories()))
	for _, c := range backlog.ValidCategories() {
		categoryOptions = append(categoryOptions, huh.NewOption(string(c), string(c)))
	}

	// Build severity options
	severityOptions := make([]huh.Option[string], 0, len(backlog.ValidSeverities()))
	for _, s := range backlog.ValidSeverities() {
		severityOptions = append(severityOptions, huh.NewOption(string(s), string(s)))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Description("Brief title describing what you found (required)").
				Value(&title).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("%w: title", atlaserrors.ErrEmptyValue)
					}
					if len(s) > backlog.MaxTitleLength {
						return fmt.Errorf("%w: title exceeds %d characters", atlaserrors.ErrValueOutOfRange, backlog.MaxTitleLength)
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewText().
				Title("Description (optional)").
				Description("Provide additional details about the discovery").
				Value(&description).
				CharLimit(2000),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Category").
				Description("What type of issue is this?").
				Options(categoryOptions...).
				Value(&category),
			huh.NewSelect[string]().
				Title("Severity").
				Description("How important is this?").
				Options(severityOptions...).
				Value(&severity),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("File path (optional)").
				Description("Relative path to the file").
				Value(&file),
			huh.NewInput().
				Title("Line number (optional)").
				Description("Line number in the file").
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					var l int
					if _, err := fmt.Sscanf(s, "%d", &l); err != nil {
						return fmt.Errorf("%w: line number", atlaserrors.ErrInvalidArgument)
					}
					if l < 1 {
						return fmt.Errorf("%w: line must be positive", atlaserrors.ErrValueOutOfRange)
					}
					return nil
				}).
				Value(&lineStr),
		),
	).WithTheme(tui.AtlasTheme())

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("form canceled: %w", err)
	}

	// Parse line number if provided
	if lineStr != "" {
		_, _ = fmt.Sscanf(lineStr, "%d", &line) // validation already passed in form
	}

	// Build discovery
	d := &backlog.Discovery{
		Title:  title,
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Description: description,
			Category:    backlog.Category(category),
			Severity:    backlog.Severity(severity),
		},
		Context: backlog.Context{
			DiscoveredBy: detectDiscoverer(ctx, mgr.ProjectRoot(), true),
		},
	}

	// Add location if provided
	if file != "" {
		d.Location = &backlog.Location{
			File: file,
			Line: line,
		}
	}

	return d, nil
}

// detectDiscoverer determines the discoverer identifier.
// For interactive mode, uses "human:<github-username>".
// For flag mode, checks environment variables or defaults to AI detection.
func detectDiscoverer(ctx context.Context, projectRoot string, interactive bool) string {
	if interactive {
		username := getGitHubUsername(ctx, projectRoot)
		if username == "" {
			return "human:unknown"
		}
		return "human:" + strings.ToLower(username)
	}

	// Check environment variables for AI agent detection
	agent := os.Getenv("ATLAS_AGENT")
	model := os.Getenv("ATLAS_MODEL")

	if agent != "" && model != "" {
		return fmt.Sprintf("ai:%s:%s", agent, model)
	}

	// Check for Claude Code environment variables
	if os.Getenv("CLAUDE_CODE") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" {
		model = os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "unknown"
		}
		return "ai:claude-code:" + model
	}

	// Default to human with GitHub username
	username := getGitHubUsername(ctx, projectRoot)
	if username == "" {
		return "human:unknown"
	}
	return "human:" + strings.ToLower(username)
}

// getGitHubUsername attempts to detect the GitHub username using multiple methods.
// Priority: env vars > git config > gh CLI > OS username.
func getGitHubUsername(ctx context.Context, projectRoot string) string {
	// Method 1: Environment variables (fastest, no I/O)
	if username := os.Getenv("GITHUB_USER"); username != "" {
		return username
	}
	if username := os.Getenv("GITHUB_ACTOR"); username != "" {
		return username
	}

	// Method 2: Custom git config (fast, local file read)
	if username, err := git.RunCommand(ctx, projectRoot, "config", "user.github"); err == nil && username != "" {
		return username
	}

	// Method 3: GitHub CLI (authoritative but slower)
	if username := getGitHubUsernameViaCLI(ctx, projectRoot); username != "" {
		return username
	}

	// Method 4: OS username fallback
	if currentUser, err := user.Current(); err == nil && currentUser.Username != "" {
		return currentUser.Username
	}

	return ""
}

// getGitHubUsernameViaCLI attempts to get the GitHub username via gh CLI.
// Returns empty string if gh is not installed or not authenticated.
func getGitHubUsernameViaCLI(ctx context.Context, workDir string) string {
	cmd := exec.CommandContext(ctx, "gh", "api", "user", "--jq", ".login") //#nosec G204 -- args are constant
	cmd.Dir = workDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // Suppress stderr

	if err := cmd.Run(); err != nil {
		return ""
	}

	return strings.TrimSpace(stdout.String())
}

// displayBacklogAddSuccess displays the success message for add command.
func displayBacklogAddSuccess(out tui.Output, d *backlog.Discovery) {
	out.Success(fmt.Sprintf("Created discovery: %s", d.ID))
	out.Info(fmt.Sprintf("  Title: %s", d.Title))
	out.Info(fmt.Sprintf("  Category: %s | Severity: %s", d.Content.Category, d.Content.Severity))
	if d.Location != nil && d.Location.File != "" {
		if d.Location.Line > 0 {
			out.Info(fmt.Sprintf("  Location: %s:%d", d.Location.File, d.Location.Line))
		} else {
			out.Info(fmt.Sprintf("  Location: %s", d.Location.File))
		}
	}
}

// getOutputFormat safely retrieves the output format from the command flags.
// If the jsonFlag is true, it returns OutputJSON. Otherwise, it checks the
// "output" flag if defined, returning its value or empty string if not defined.
func getOutputFormat(cmd *cobra.Command, jsonFlag bool) string {
	if jsonFlag {
		return OutputJSON
	}
	if flag := cmd.Flag("output"); flag != nil {
		return flag.Value.String()
	}
	return ""
}

// outputBacklogError outputs an error in the appropriate format.
func outputBacklogError(w io.Writer, format, command string, err error) error {
	if format == OutputJSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if encErr := encoder.Encode(map[string]any{
			"success": false,
			"command": "backlog " + command,
			"error":   err.Error(),
		}); encErr != nil {
			return fmt.Errorf("failed to encode JSON: %w", encErr)
		}
		return atlaserrors.ErrJSONErrorOutput
	}
	return err
}
