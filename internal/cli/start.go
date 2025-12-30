// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/template/steps"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// Workspace name generation constants.
const maxWorkspaceNameLen = 50

// Regex patterns for workspace name generation.
var (
	// nonAlphanumericRegex matches any character that is not a lowercase letter, digit, or hyphen.
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9-]+`)
	// multipleHyphensRegex matches consecutive hyphens.
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

// AddStartCommand adds the start command to the root command.
func AddStartCommand(root *cobra.Command) {
	root.AddCommand(newStartCmd())
}

// startOptions contains all options for the start command.
type startOptions struct {
	templateName  string
	workspaceName string
	model         string
	noInteractive bool
	verify        bool
	noVerify      bool
}

// newStartCmd creates the start command.
func newStartCmd() *cobra.Command {
	var (
		templateName  string
		workspaceName string
		model         string
		noInteractive bool
		verify        bool
		noVerify      bool
	)

	cmd := &cobra.Command{
		Use:   "start <description>",
		Short: "Start a new task with the given description",
		Long: `Start a new task by creating a workspace, selecting a template,
and beginning execution of the template steps.

Examples:
  atlas start "fix null pointer in parseConfig"
  atlas start "add retry logic to HTTP client" --template feature
  atlas start "update dependencies" --workspace deps-update --template commit
  atlas start "add new feature" --template feature --verify
  atlas start "quick fix" --template bugfix --no-verify`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), cmd, os.Stdout, args[0], startOptions{
				templateName:  templateName,
				workspaceName: workspaceName,
				model:         model,
				noInteractive: noInteractive,
				verify:        verify,
				noVerify:      noVerify,
			})
		},
	}

	cmd.Flags().StringVarP(&templateName, "template", "t", "",
		"Template to use (bugfix, feature, commit)")
	cmd.Flags().StringVarP(&workspaceName, "workspace", "w", "",
		"Custom workspace name")
	cmd.Flags().StringVarP(&model, "model", "m", "",
		"AI model to use (sonnet, opus, haiku)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false,
		"Disable interactive prompts")
	cmd.Flags().BoolVar(&verify, "verify", false,
		"Enable AI verification step (cross-model validation)")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false,
		"Disable AI verification step")

	return cmd
}

// startContext holds shared state for the start command execution.
type startContext struct {
	ctx          context.Context //nolint:containedctx // context needed for error handling
	outputFormat string
	out          tui.Output
	w            io.Writer
}

// runStart executes the start command.
func runStart(ctx context.Context, cmd *cobra.Command, w io.Writer, description string, opts startOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := GetLogger()
	outputFormat := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)
	sc := &startContext{
		ctx:          ctx,
		outputFormat: outputFormat,
		out:          out,
		w:            w,
	}

	// Validate model flag if provided
	if err := validateModel(opts.model); err != nil {
		return sc.handleError("", err)
	}

	// Validate verify flags - cannot use both
	if opts.verify && opts.noVerify {
		return sc.handleError("", errors.NewExitCode2Error(
			fmt.Errorf("%w: cannot use both --verify and --no-verify", errors.ErrConflictingFlags)))
	}

	// Validate we're in a git repository
	repoPath, err := findGitRepository()
	if err != nil {
		return sc.handleError("", fmt.Errorf("not in a git repository: %w", err))
	}

	logger.Debug().Str("repo_path", repoPath).Msg("found git repository")

	// Load template registry
	registry := template.NewDefaultRegistry()

	// Select template
	tmpl, err := selectTemplate(ctx, registry, opts.templateName, opts.noInteractive, outputFormat)
	if err != nil {
		return sc.handleError("", err)
	}

	logger.Debug().
		Str("template_name", tmpl.Name).
		Msg("template selected")

	// Determine workspace name
	wsName := opts.workspaceName
	if wsName == "" {
		wsName = generateWorkspaceName(description)
	} else {
		wsName = sanitizeWorkspaceName(wsName)
	}

	// Create and configure workspace
	ws, err := createWorkspace(ctx, sc, wsName, repoPath, tmpl.BranchPrefix, opts.noInteractive)
	if err != nil {
		return err
	}

	logger.Info().
		Str("workspace_name", ws.Name).
		Str("branch", ws.Branch).
		Str("worktree_path", ws.WorktreePath).
		Msg("workspace created")

	// Apply verify flag overrides to template
	applyVerifyOverrides(tmpl, opts.verify, opts.noVerify)

	// Start task execution
	t, err := startTaskExecution(ctx, ws, tmpl, description, opts.model, logger)
	if err != nil {
		// Clean up workspace on task start failure (AC#9 - graceful cleanup)
		if cleanupErr := cleanupWorkspace(ctx, ws.Name, repoPath); cleanupErr != nil {
			logger.Warn().Err(cleanupErr).
				Str("workspace_name", ws.Name).
				Msg("failed to cleanup workspace after task failure")
		}
		if t != nil {
			return displayTaskStatus(out, outputFormat, ws, t, err)
		}
		return sc.handleError(wsName, fmt.Errorf("failed to start task: %w", err))
	}

	logger.Info().
		Str("task_id", t.ID).
		Str("workspace_name", ws.Name).
		Str("template_name", tmpl.Name).
		Int("total_steps", len(t.Steps)).
		Msg("task started")

	return displayTaskStatus(out, outputFormat, ws, t, nil)
}

// handleError handles errors based on output format.
func (sc *startContext) handleError(wsName string, err error) error {
	if sc.outputFormat == OutputJSON {
		return outputStartErrorJSON(sc.w, wsName, "", err.Error())
	}
	return err
}

// createWorkspace creates a new workspace with all necessary components.
func createWorkspace(ctx context.Context, sc *startContext, wsName, repoPath, branchPrefix string, noInteractive bool) (*domain.Workspace, error) {
	// Create workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return nil, sc.handleError(wsName, fmt.Errorf("failed to create workspace store: %w", err))
	}

	// Create worktree runner
	wtRunner, err := workspace.NewGitWorktreeRunner(repoPath) //nolint:contextcheck // NewGitWorktreeRunner doesn't accept context
	if err != nil {
		return nil, sc.handleError(wsName, fmt.Errorf("failed to create worktree runner: %w", err))
	}

	// Create manager
	wsMgr := workspace.NewManager(wsStore, wtRunner)

	// Check for existing workspace
	wsName, err = handleWorkspaceConflict(ctx, wsMgr, wsName, noInteractive, sc.outputFormat, sc.out, sc.w)
	if err != nil {
		return nil, err
	}

	// Create workspace with branch type from template
	ws, err := wsMgr.Create(ctx, wsName, repoPath, branchPrefix)
	if err != nil {
		return nil, sc.handleError(wsName, fmt.Errorf("failed to create workspace: %w", err))
	}

	return ws, nil
}

// startTaskExecution creates and starts the task engine.
func startTaskExecution(ctx context.Context, ws *domain.Workspace, tmpl *domain.Template, description, model string, logger zerolog.Logger) (*domain.Task, error) {
	// Create task store
	taskStore, err := task.NewFileStore("")
	if err != nil {
		return nil, fmt.Errorf("failed to create task store: %w", err)
	}

	// Load config for notification settings
	cfg, err := config.Load(ctx)
	if err != nil {
		// Log warning but continue with defaults - don't fail task start for config issues
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	// Create notifier from config (bell enabled by config).
	// The quiet flag is not currently passed through to this function.
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)

	// Create executor registry with full dependencies for artifact saving and notifications
	execRegistry := steps.NewDefaultRegistry(steps.ExecutorDeps{
		WorkDir:       ws.WorktreePath,
		ArtifactSaver: taskStore,
		Notifier:      notifier,
	})

	engineCfg := task.DefaultEngineConfig()
	engine := task.NewEngine(taskStore, execRegistry, engineCfg, GetLogger())

	// Apply model override if specified
	if model != "" {
		tmpl.DefaultModel = model
	}

	// Start task
	t, err := engine.Start(ctx, ws.Name, tmpl, description)
	if err != nil {
		logger.Error().Err(err).
			Str("workspace_name", ws.Name).
			Msg("task start failed")
		return t, err
	}

	return t, nil
}

// generateWorkspaceName creates a sanitized workspace name from description.
func generateWorkspaceName(description string) string {
	name := sanitizeWorkspaceName(description)

	// Handle empty result
	if name == "" {
		name = fmt.Sprintf("task-%s", time.Now().Format("20060102-150405"))
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

// selectTemplate handles template selection based on flags and interactivity mode.
func selectTemplate(ctx context.Context, registry *template.Registry, templateName string, noInteractive bool, outputFormat string) (*domain.Template, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// If template specified via flag, use it directly
	if templateName != "" {
		tmpl, err := registry.Get(templateName)
		if err != nil {
			return nil, fmt.Errorf("template '%s' not found: %w", templateName, errors.ErrTemplateNotFound)
		}
		return tmpl, nil
	}

	// Non-interactive mode or JSON output requires template flag
	if noInteractive || outputFormat == OutputJSON || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.NewExitCode2Error(
			fmt.Errorf("use --template to specify template: %w", errors.ErrTemplateRequired))
	}

	return selectTemplateInteractive(registry)
}

// selectTemplateInteractive displays an interactive template selection menu.
func selectTemplateInteractive(registry *template.Registry) (*domain.Template, error) {
	templates := registry.List()
	options := make([]huh.Option[string], 0, len(templates))
	for _, t := range templates {
		label := fmt.Sprintf("%s - %s", t.Name, t.Description)
		options = append(options, huh.NewOption(label, t.Name))
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a template").
				Description("Choose the workflow template for this task").
				Options(options...).
				Value(&selected),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("template selection canceled: %w", err)
	}

	return registry.Get(selected)
}

// handleWorkspaceConflict checks for existing workspace and handles conflicts.
func handleWorkspaceConflict(ctx context.Context, mgr *workspace.DefaultManager, wsName string, noInteractive bool, outputFormat string, out tui.Output, w io.Writer) (string, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	exists, err := mgr.Exists(ctx, wsName)
	if err != nil {
		return "", fmt.Errorf("failed to check workspace existence: %w", err)
	}

	if !exists {
		return wsName, nil
	}

	// Workspace exists - handle conflict
	if noInteractive || outputFormat == OutputJSON {
		if outputFormat == OutputJSON {
			return "", outputStartErrorJSON(w, wsName, "", fmt.Sprintf("workspace '%s': %s", wsName, errors.ErrWorkspaceExists.Error()))
		}
		return "", errors.NewExitCode2Error(
			fmt.Errorf("workspace '%s': %w", wsName, errors.ErrWorkspaceExists))
	}

	// Check if we're in a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("workspace '%s': %w (use --workspace to specify a different name)", wsName, errors.ErrWorkspaceExists)
	}

	return resolveWorkspaceConflictInteractive(wsName, out)
}

// resolveWorkspaceConflictInteractive handles workspace conflict interactively.
func resolveWorkspaceConflictInteractive(wsName string, out tui.Output) (string, error) {
	action, err := promptWorkspaceConflict(wsName)
	if err != nil {
		return "", fmt.Errorf("failed to get user choice: %w", err)
	}

	switch action {
	case "resume":
		return "", errors.ErrResumeNotImplemented
	case "new":
		newName, err := promptNewWorkspaceName()
		if err != nil {
			return "", fmt.Errorf("failed to get new workspace name: %w", err)
		}
		return sanitizeWorkspaceName(newName), nil
	case "cancel":
		out.Info("Operation canceled")
		return "", errors.ErrOperationCanceled
	}

	return wsName, nil
}

// promptWorkspaceConflict prompts the user to resolve a workspace name conflict.
func promptWorkspaceConflict(name string) (string, error) {
	var action string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Workspace '%s' exists", name)).
				Description("What would you like to do?").
				Options(
					huh.NewOption("Resume existing workspace", "resume"),
					huh.NewOption("Use a different name", "new"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return "", err
	}

	return action, nil
}

// promptNewWorkspaceName prompts the user for a new workspace name.
func promptNewWorkspaceName() (string, error) {
	var name string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter new workspace name").
				Value(&name).
				Validate(validateWorkspaceName),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return "", err
	}

	return name, nil
}

// validateWorkspaceName validates a workspace name input.
func validateWorkspaceName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("name required: %w", errors.ErrEmptyValue)
	}
	return nil
}

// findGitRepository finds the git repository root from the current directory.
func findGitRepository() (string, error) {
	// Start from current directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up until we find .git
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return dir, nil
			}
			// .git file (worktree) - read the gitdir
			content, err := os.ReadFile(gitPath) //#nosec G304 -- path is constructed internally
			if err == nil && strings.HasPrefix(string(content), "gitdir:") {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.ErrNotGitRepo
		}
		dir = parent
	}
}

// startResponse represents the JSON output for start operations.
type startResponse struct {
	Success   bool          `json:"success"`
	Workspace workspaceInfo `json:"workspace"`
	Task      taskInfo      `json:"task"`
	Error     string        `json:"error,omitempty"`
}

// workspaceInfo contains workspace details for JSON output.
type workspaceInfo struct {
	Name         string `json:"name"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktree_path"`
	Status       string `json:"status"`
}

// taskInfo contains task details for JSON output.
type taskInfo struct {
	ID           string `json:"task_id"`
	TemplateName string `json:"template_name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	CurrentStep  int    `json:"current_step"`
	TotalSteps   int    `json:"total_steps"`
}

// cleanupWorkspace removes a workspace after a failed task start.
func cleanupWorkspace(ctx context.Context, wsName, repoPath string) error {
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	wtRunner, err := workspace.NewGitWorktreeRunner(repoPath) //nolint:contextcheck // NewGitWorktreeRunner doesn't accept context
	if err != nil {
		return fmt.Errorf("failed to create worktree runner: %w", err)
	}

	mgr := workspace.NewManager(wsStore, wtRunner)
	return mgr.Destroy(ctx, wsName)
}

// isValidModel checks if the model name is valid.
func isValidModel(model string) bool {
	switch model {
	case "sonnet", "opus", "haiku":
		return true
	default:
		return false
	}
}

// validateModel checks if the model name is valid.
func validateModel(model string) error {
	if model == "" {
		return nil // Empty is valid (use default)
	}
	if !isValidModel(model) {
		return errors.NewExitCode2Error(
			fmt.Errorf("%w: '%s' (must be one of sonnet, opus, haiku)", errors.ErrInvalidModel, model))
	}
	return nil
}

// applyVerifyOverrides applies --verify or --no-verify flag overrides to the template.
// If neither flag is set, the template's default Verify setting is used.
// Also propagates VerifyModel from template to the verify step config.
func applyVerifyOverrides(tmpl *domain.Template, verify, noVerify bool) {
	// CLI flags override template defaults
	if verify {
		tmpl.Verify = true
	} else if noVerify {
		tmpl.Verify = false
	}

	// Update the verify step's Required field and model based on the template settings
	//nolint:nestif // Configuration logic with nested validation checks
	for i := range tmpl.Steps {
		if tmpl.Steps[i].Type == domain.StepTypeVerify {
			tmpl.Steps[i].Required = tmpl.Verify

			// Propagate VerifyModel from template to step config if set
			if tmpl.VerifyModel != "" {
				if tmpl.Steps[i].Config == nil {
					tmpl.Steps[i].Config = make(map[string]any)
				}
				// Only set if not already configured in step
				if model, ok := tmpl.Steps[i].Config["model"].(string); !ok || model == "" {
					tmpl.Steps[i].Config["model"] = tmpl.VerifyModel
				}
			}
		}
	}
}

// displayTaskStatus outputs the task status in the appropriate format.
func displayTaskStatus(out tui.Output, format string, ws *domain.Workspace, t *domain.Task, execErr error) error {
	if format == OutputJSON {
		resp := startResponse{
			Success: execErr == nil,
			Workspace: workspaceInfo{
				Name:         ws.Name,
				Branch:       ws.Branch,
				WorktreePath: ws.WorktreePath,
				Status:       string(ws.Status),
			},
			Task: taskInfo{
				ID:           t.ID,
				TemplateName: t.TemplateID,
				Description:  t.Description,
				Status:       string(t.Status),
				CurrentStep:  t.CurrentStep,
				TotalSteps:   len(t.Steps),
			},
		}
		if execErr != nil {
			resp.Error = execErr.Error()
		}
		return out.JSON(resp)
	}

	// TTY output
	out.Success(fmt.Sprintf("Task started: %s", t.ID))
	out.Info(fmt.Sprintf("  Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("  Branch:    %s", ws.Branch))
	out.Info(fmt.Sprintf("  Template:  %s", t.TemplateID))
	out.Info(fmt.Sprintf("  Status:    %s", t.Status))
	out.Info(fmt.Sprintf("  Progress:  Step %d/%d", t.CurrentStep+1, len(t.Steps)))

	if execErr != nil {
		out.Warning(fmt.Sprintf("Execution paused: %s", execErr.Error()))

		// Display manual fix instructions for validation failures
		if t.Status == constants.TaskStatusValidationFailed {
			tui.DisplayManualFixInstructions(out, t, ws)
		}
	}

	return nil
}

// outputStartErrorJSON outputs an error result as JSON.
func outputStartErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	resp := startResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: workspaceName,
		},
		Task: taskInfo{
			ID: taskID,
		},
		Error: errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}
