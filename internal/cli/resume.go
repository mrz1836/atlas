// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/hook"
	"github.com/mrz1836/atlas/internal/signal"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/template/steps"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
	"github.com/mrz1836/atlas/internal/workspace"
)

// AddResumeCommand adds the resume command to the root command.
func AddResumeCommand(root *cobra.Command) {
	root.AddCommand(newResumeCmd())
}

// resumeOptions contains all options for the resume command.
type resumeOptions struct {
	aiFix bool
}

// newResumeCmd creates the resume command.
func newResumeCmd() *cobra.Command {
	var aiFix bool

	cmd := &cobra.Command{
		Use:   "resume <workspace>",
		Short: "Resume a paused or failed task",
		Long: `Resume execution of a task that was paused or failed.

Use this command after manually fixing validation errors in the worktree.
The task will re-run validation from the current step.

Examples:
  atlas resume auth-fix           # Resume task in auth-fix workspace
  atlas resume auth-fix --ai-fix  # Resume with AI attempting to fix errors`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(cmd.Context(), cmd, os.Stdout, args[0], resumeOptions{
				aiFix: aiFix,
			})
		},
	}

	cmd.Flags().BoolVar(&aiFix, "ai-fix", false, "Retry with AI attempting to fix errors")

	return cmd
}

// runResume executes the resume command.
func runResume(ctx context.Context, cmd *cobra.Command, w io.Writer, workspaceName string, opts resumeOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := Logger()
	outputFormat := cmd.Flag("output").Value.String()
	tui.CheckNoColor()
	out := tui.NewOutput(w, outputFormat)

	// Setup signal handler
	sigHandler := signal.NewHandler(ctx)
	defer sigHandler.Stop()
	ctx = sigHandler.Context()

	// Setup workspace and task
	ws, currentTask, taskStore, wsStore, err := setupResumeWorkspaceAndTask(ctx, workspaceName, outputFormat, w, out, logger) //nolint:contextcheck // ctx inherits from parent via signal.NewHandler
	if err != nil {
		return err
	}

	// Get template and check AI fix option
	tmpl, err := prepareResumeTemplate(currentTask, opts, outputFormat, w, workspaceName)
	if err != nil {
		return err
	}

	// Create engine and execute resume
	engine, err := createResumeEngine(ctx, ws, taskStore, logger, out) //nolint:contextcheck // ctx inherits from parent via signal.NewHandler
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, err)
	}

	// Display info and prepare task
	displayResumeInfo(out, workspaceName, currentTask)
	if currentTask.Metadata == nil {
		currentTask.Metadata = make(map[string]any)
	}
	currentTask.Metadata["worktree_dir"] = ws.WorktreePath

	// Execute resume and handle result
	return executeResumeAndHandleResult(ctx, engine, currentTask, tmpl, sigHandler, out, ws, wsStore, outputFormat, w, workspaceName, logger) //nolint:contextcheck // ctx inherits from parent via signal.NewHandler
}

// setupResumeWorkspaceAndTask sets up the workspace, task, and stores for resume.
func setupResumeWorkspaceAndTask(ctx context.Context, workspaceName, outputFormat string, w io.Writer, out tui.Output, logger zerolog.Logger) (*domain.Workspace, *domain.Task, *task.FileStore, workspace.Store, error) {
	// Setup workspace
	_, ws, err := setupWorkspace(ctx, workspaceName, "", outputFormat, w, logger)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("setup workspace: %w", err)
	}

	// Get task store and latest task
	taskStore, currentTask, err := getLatestTask(ctx, workspaceName, "", outputFormat, w, logger)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("get latest task: %w", err)
	}

	// Validate task is in resumable state
	if !isResumableStatus(currentTask.Status) {
		return nil, nil, nil, nil, handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("%w: task status %s is not resumable", atlaserrors.ErrInvalidTransition, currentTask.Status))
	}

	// Check for hook-based recovery context
	if shouldShowRecoveryContext(ctx, currentTask, out, outputFormat, logger) {
		proceed, recoveryErr := showRecoveryContextAndPrompt(ctx, currentTask, out, logger)
		if recoveryErr != nil {
			logger.Warn().Err(recoveryErr).Msg("failed to show recovery context")
		}
		if !proceed {
			return nil, nil, nil, nil, atlaserrors.ErrOperationCanceled
		}
	}

	// Ensure worktree exists
	ws, err = ensureWorktreeExists(ctx, ws, out, logger)
	if err != nil {
		return nil, nil, nil, nil, handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("failed to ensure worktree exists: %w", err))
	}

	// Create workspace store and update status
	wsStore, wsStoreErr := workspace.NewFileStore("")
	if wsStoreErr != nil {
		logger.Warn().Err(wsStoreErr).Msg("failed to create workspace store for status updates")
	}

	ws.Status = constants.WorkspaceStatusActive
	if wsStore != nil {
		if updateErr := wsStore.Update(ctx, ws); updateErr != nil {
			logger.Warn().Err(updateErr).
				Str("workspace_name", ws.Name).
				Msg("failed to update workspace status to active")
		}
	}

	return ws, currentTask, taskStore, wsStore, nil
}

// prepareResumeTemplate gets the template and checks AI fix option.
func prepareResumeTemplate(currentTask *domain.Task, opts resumeOptions, outputFormat string, w io.Writer, workspaceName string) (*domain.Template, error) {
	registry := template.NewDefaultRegistry()
	tmpl, err := registry.Get(currentTask.TemplateID)
	if err != nil {
		return nil, handleResumeError(outputFormat, w, workspaceName, currentTask.ID, fmt.Errorf("failed to get template: %w", err))
	}

	if opts.aiFix {
		return nil, handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("--ai-fix not yet implemented: %w", atlaserrors.ErrResumeNotImplemented))
	}

	return tmpl, nil
}

// executeResumeAndHandleResult executes the resume and handles the result/interruption.
func executeResumeAndHandleResult(ctx context.Context, engine *task.Engine, currentTask *domain.Task, tmpl *domain.Template, sigHandler *signal.Handler, out tui.Output, ws *domain.Workspace, wsStore workspace.Store, outputFormat string, w io.Writer, workspaceName string, logger zerolog.Logger) error {
	// Resume task execution
	if err := engine.Resume(ctx, currentTask, tmpl); err != nil {
		// Check if we were interrupted by Ctrl+C
		select {
		case <-sigHandler.Interrupted():
			return handleResumeInterruption(ctx, out, ws, currentTask, wsStore, logger)
		default:
		}

		if currentTask.Status == constants.TaskStatusValidationFailed {
			tui.DisplayManualFixInstructions(out, currentTask, ws)
		}
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, err)
	}

	// Check if we were interrupted by Ctrl+C (even if no error)
	select {
	case <-sigHandler.Interrupted():
		return handleResumeInterruption(ctx, out, ws, currentTask, wsStore, logger)
	default:
	}

	// Handle JSON output format
	if outputFormat == OutputJSON {
		return outputResumeSuccessJSON(out, ws, currentTask)
	}

	displayResumeResult(out, ws, currentTask, nil)
	return nil
}

// isResumableStatus returns true if the task status allows resuming.
func isResumableStatus(status constants.TaskStatus) bool {
	return task.IsErrorStatus(status) || status == constants.TaskStatusAwaitingApproval
}

// resumeResponse represents the JSON output for resume operations.
type resumeResponse struct {
	Success   bool          `json:"success"`
	Workspace workspaceInfo `json:"workspace"`
	Task      taskInfo      `json:"task"`
	Error     string        `json:"error,omitempty"`
}

// handleResumeError handles errors based on output format.
func handleResumeError(format string, w io.Writer, workspaceName, taskID string, err error) error {
	if format == OutputJSON {
		return outputResumeErrorJSON(w, workspaceName, taskID, err.Error())
	}
	return err
}

// outputResumeErrorJSON outputs an error result as JSON.
func outputResumeErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	if err := encodeJSONIndented(w, resumeResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: workspaceName,
		},
		Task: taskInfo{
			ID: taskID,
		},
		Error: errMsg,
	}); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return atlaserrors.ErrJSONErrorOutput
}

// displayResumeResult outputs the resume result in the appropriate format.
func displayResumeResult(out tui.Output, ws *domain.Workspace, t *domain.Task, execErr error) {
	// TTY output
	out.Success(fmt.Sprintf("Task resumed successfully. Status: %s", t.Status))
	out.Info(fmt.Sprintf("  Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("  Task ID:   %s", t.ID))
	out.Info(fmt.Sprintf("  Progress:  Step %d/%d", t.CurrentStep+1, len(t.Steps)))

	if execErr != nil {
		out.Warning(fmt.Sprintf("Execution paused: %s", execErr.Error()))
	}
}

// createResumeEngine creates the task engine with all required dependencies.
func createResumeEngine(ctx context.Context, ws *domain.Workspace, taskStore *task.FileStore, logger zerolog.Logger, out tui.Output) (*task.Engine, error) {
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)
	stateNotifier := task.NewStateChangeNotifier(task.NotificationConfig{
		BellEnabled: cfg.Notifications.Bell,
		Quiet:       false,
		Events:      cfg.Notifications.Events,
	})

	aiRunner := ai.NewClaudeCodeRunner(&cfg.AI, nil)

	gitRunner, err := git.NewRunner(ctx, ws.WorktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create git runner: %w", err)
	}

	// Resolve git config settings with fallbacks
	gitCfg := ResolveGitConfig(cfg)

	smartCommitter := git.NewSmartCommitRunner(gitRunner, ws.WorktreePath, aiRunner,
		git.WithAgent(gitCfg.CommitAgent),
		git.WithModel(gitCfg.CommitModel),
		git.WithTimeout(gitCfg.CommitTimeout),
		git.WithMaxRetries(gitCfg.CommitMaxRetries),
		git.WithRetryBackoffFactor(gitCfg.CommitBackoffFactor),
		git.WithLogger(logger),
	)
	pusher := git.NewPushRunner(gitRunner)
	hubRunner := git.NewCLIGitHubRunner(ws.WorktreePath)
	prDescGen := git.NewAIDescriptionGenerator(aiRunner,
		git.WithAIDescAgent(gitCfg.PRDescAgent),
		git.WithAIDescModel(gitCfg.PRDescModel),
		git.WithAIDescLogger(logger),
	)
	ciFailureHandler := task.NewCIFailureHandler(hubRunner)

	execRegistry := steps.NewDefaultRegistry(steps.ExecutorDeps{
		WorkDir:                ws.WorktreePath,
		ArtifactSaver:          taskStore,
		Notifier:               notifier,
		AIRunner:               aiRunner,
		Logger:                 logger,
		SmartCommitter:         smartCommitter,
		Pusher:                 pusher,
		HubRunner:              hubRunner,
		PRDescriptionGenerator: prDescGen,
		GitRunner:              gitRunner,
		CIFailureHandler:       ciFailureHandler,
		BaseBranch:             cfg.Git.BaseBranch,
		CIConfig:               &cfg.CI,
		FormatCommands:         cfg.Validation.Commands.Format,
		LintCommands:           cfg.Validation.Commands.Lint,
		TestCommands:           cfg.Validation.Commands.Test,
		PreCommitCommands:      cfg.Validation.Commands.PreCommit,
	})

	validationRetryHandler := createResumeValidationRetryHandler(aiRunner, cfg, logger)

	engineCfg := task.DefaultEngineConfig()
	engineCfg.ProgressCallback = createProgressCallback(ctx, out, ws.Name)
	engineOpts := []task.EngineOption{task.WithNotifier(stateNotifier)}
	if validationRetryHandler != nil {
		engineOpts = append(engineOpts, task.WithValidationRetryHandler(validationRetryHandler))
	}

	return task.NewEngine(taskStore, execRegistry, engineCfg, logger, engineOpts...), nil
}

// handleResumeInterruption handles graceful shutdown when user presses Ctrl+C during resume.
// It saves the task and workspace state so the user can resume later.
// The wsStore parameter allows dependency injection for testing - pass nil to skip persistence.
func handleResumeInterruption(ctx context.Context, out tui.Output, ws *domain.Workspace, t *domain.Task, wsStore workspace.Store, logger zerolog.Logger) error {
	logger.Info().
		Str("workspace_name", ws.Name).
		Str("task_id", t.ID).
		Msg("received interrupt signal during resume, initiating graceful shutdown")

	out.Warning("\n⚠ Interrupt received - saving state...")

	// Use a context without cancellation for cleanup since the original is canceled
	cleanupCtx := context.WithoutCancel(ctx)

	// Save interrupted task state (reuse the function from start.go)
	saveInterruptedTaskState(cleanupCtx, ws, t, logger)

	// Update workspace to paused
	ws.Status = constants.WorkspaceStatusPaused

	// Persist workspace state if store is provided (allows nil for testing)
	if wsStore != nil {
		if updateErr := wsStore.Update(cleanupCtx, ws); updateErr != nil {
			logger.Warn().Err(updateErr).
				Str("workspace_name", ws.Name).
				Msg("failed to persist workspace pause status")
		}
	}

	// Display summary (reuse the function from start.go)
	displayInterruptionSummary(out, ws, t)

	return atlaserrors.ErrTaskInterrupted
}

// displayResumeInfo displays information about the task being resumed.
func displayResumeInfo(out tui.Output, workspaceName string, currentTask *domain.Task) {
	out.Info(fmt.Sprintf("Resuming task in workspace '%s'...", workspaceName))
	out.Info(fmt.Sprintf("  Task ID: %s", currentTask.ID))
	out.Info(fmt.Sprintf("  Status: %s → running", currentTask.Status))
	out.Info(fmt.Sprintf("  Current Step: %d/%d", currentTask.CurrentStep+1, len(currentTask.Steps)))

	// Show specific message for interrupted tasks
	if currentTask.Status == constants.TaskStatusInterrupted {
		stepName := "unknown"
		if currentTask.CurrentStep < len(currentTask.Steps) {
			stepName = currentTask.Steps[currentTask.CurrentStep].Name
		}
		out.Info(fmt.Sprintf("  Note: Task was interrupted by user, resuming from step %d (%s)", currentTask.CurrentStep+1, stepName))
	}
}

// outputResumeSuccessJSON outputs a successful resume result as JSON.
func outputResumeSuccessJSON(out tui.Output, ws *domain.Workspace, currentTask *domain.Task) error {
	resp := resumeResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name:         ws.Name,
			Branch:       ws.Branch,
			WorktreePath: ws.WorktreePath,
			Status:       string(ws.Status),
		},
		Task: taskInfo{
			ID:           currentTask.ID,
			TemplateName: currentTask.TemplateID,
			Description:  currentTask.Description,
			Status:       string(currentTask.Status),
			CurrentStep:  currentTask.CurrentStep,
			TotalSteps:   len(currentTask.Steps),
		},
	}
	return out.JSON(resp)
}

// createResumeValidationRetryHandler creates the validation retry handler for automatic AI-assisted fixes.
func createResumeValidationRetryHandler(aiRunner ai.Runner, cfg *config.Config, logger zerolog.Logger) *validation.RetryHandler {
	if !cfg.Validation.AIRetryEnabled {
		return nil
	}

	executor := validation.NewExecutorWithRunner(validation.DefaultTimeout, &validation.DefaultCommandRunner{})

	return validation.NewRetryHandlerFromConfig(
		aiRunner,
		executor,
		cfg.Validation.AIRetryEnabled,
		cfg.Validation.MaxAIRetryAttempts,
		logger,
	)
}

// ensureWorktreeExists checks if the workspace worktree exists and recreates it if missing.
// This handles the case where a task failed and the worktree was removed (e.g., due to a bug),
// but the branch still exists and can be recovered.
func ensureWorktreeExists(ctx context.Context, ws *domain.Workspace, out tui.Output, logger zerolog.Logger) (*domain.Workspace, error) {
	// Check if worktree path is set and exists
	if ws.WorktreePath != "" {
		if _, err := os.Stat(ws.WorktreePath); err == nil {
			// Worktree exists, nothing to do
			return ws, nil
		}
	}

	// Worktree is missing - try to recreate it
	logger.Info().
		Str("workspace_name", ws.Name).
		Str("branch", ws.Branch).
		Msg("worktree missing, attempting to recreate")

	out.Warning("Worktree is missing. Attempting to recreate from branch...")

	// Get the main repo path (parent directory of expected worktree)
	repoPath, err := detectMainRepoPath(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect main repository: %w", err)
	}

	// Check if branch exists
	branchExists := checkBranchExists(ctx, repoPath, ws.Branch)
	if !branchExists {
		return nil, fmt.Errorf("%w: %s", atlaserrors.ErrBranchNotFound, ws.Branch)
	}

	// Calculate worktree path (sibling to main repo)
	worktreePath := calculateWorktreePath(repoPath, ws.Name)

	// Create worktree for existing branch
	if createErr := createWorktreeForBranch(ctx, repoPath, worktreePath, ws.Branch); createErr != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", createErr)
	}

	// Update workspace with new worktree path
	ws.WorktreePath = worktreePath
	ws.Status = constants.WorkspaceStatusActive

	// Save updated workspace
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		logger.Warn().Err(err).Msg("failed to create workspace store for update")
	} else if updateErr := wsStore.Update(ctx, ws); updateErr != nil {
		logger.Warn().Err(updateErr).Msg("failed to update workspace with new worktree path")
	}

	out.Success(fmt.Sprintf("Worktree recreated at '%s'", worktreePath))
	logger.Info().
		Str("workspace_name", ws.Name).
		Str("worktree_path", worktreePath).
		Msg("worktree recreated successfully")

	return ws, nil
}

// detectMainRepoPath finds the main repository path.
func detectMainRepoPath(ctx context.Context) (string, error) {
	// Start from current directory and find git repo root
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Use git rev-parse to find repo root
	output, err := git.RunCommand(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	return output, nil
}

// checkBranchExists checks if a branch exists locally or on remote.
func checkBranchExists(ctx context.Context, repoPath, branchName string) bool {
	// Check local branch
	_, localErr := git.RunCommand(ctx, repoPath, "rev-parse", "--verify", branchName)
	if localErr == nil {
		return true
	}

	// Check remote branch (try to fetch first)
	_, _ = git.RunCommand(ctx, repoPath, "fetch", "origin", branchName)
	_, remoteErr := git.RunCommand(ctx, repoPath, "rev-parse", "--verify", "origin/"+branchName)
	return remoteErr == nil
}

// calculateWorktreePath calculates the worktree path as a sibling to the main repo.
func calculateWorktreePath(repoPath, workspaceName string) string {
	parentDir := filepath.Dir(repoPath)
	repoBaseName := filepath.Base(repoPath)
	return filepath.Join(parentDir, repoBaseName+"-"+workspaceName)
}

// createWorktreeForBranch creates a worktree for an existing branch.
func createWorktreeForBranch(ctx context.Context, repoPath, worktreePath, branchName string) error {
	// Remove any existing directory at the worktree path
	if _, err := os.Stat(worktreePath); err == nil {
		if removeErr := os.RemoveAll(worktreePath); removeErr != nil {
			return fmt.Errorf("failed to remove existing directory at worktree path: %w", removeErr)
		}
	}

	// Create worktree for existing branch (no -b flag)
	_, err := git.RunCommand(ctx, repoPath, "worktree", "add", worktreePath, branchName)
	if err != nil {
		return err
	}

	return nil
}

// shouldShowRecoveryContext checks if hook-based recovery context should be displayed.
func shouldShowRecoveryContext(ctx context.Context, t *domain.Task, _ tui.Output, outputFormat string, _ zerolog.Logger) bool {
	// Skip for JSON output
	if outputFormat == OutputJSON {
		return false
	}

	// Get base path for hook store
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	baseDir := filepath.Join(homeDir, constants.AtlasHome)

	// Try to get the hook
	hookStore := hook.NewFileStore(baseDir)
	exists, _ := hookStore.Exists(ctx, t.ID)
	return exists
}

// showRecoveryContextAndPrompt displays recovery context from hook and prompts for confirmation.
func showRecoveryContextAndPrompt(ctx context.Context, t *domain.Task, out tui.Output, _ zerolog.Logger) (bool, error) {
	// Get base path for hook store
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return true, err
	}
	baseDir := filepath.Join(homeDir, constants.AtlasHome)

	// Get the hook
	hookStore := hook.NewFileStore(baseDir)
	h, err := hookStore.Get(ctx, t.ID)
	if err != nil {
		return true, err // Continue without hook context
	}

	// Load config for stale threshold
	cfg, err := config.Load(ctx)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Create recovery detector and get recommendation
	detector := hook.NewRecoveryDetector(&cfg.Hooks)
	_ = detector.DiagnoseAndRecommend(ctx, h)

	// Display recovery context
	out.Info("")
	out.Info("Recovery Context (from HOOK.md):")
	out.Info(fmt.Sprintf("  State: %s", h.State))
	if h.CurrentStep != nil {
		out.Info(fmt.Sprintf("  Step: %s (index %d)", h.CurrentStep.StepName, h.CurrentStep.StepIndex+1))
		out.Info(fmt.Sprintf("  Attempt: %d/%d", h.CurrentStep.Attempt, h.CurrentStep.MaxAttempts))
	}
	out.Info(fmt.Sprintf("  Checkpoints: %d", len(h.Checkpoints)))
	if h.Recovery != nil {
		out.Info(fmt.Sprintf("  Recommendation: %s", h.Recovery.RecommendedAction))
		out.Info(fmt.Sprintf("  Reason: %s", h.Recovery.Reason))
	}
	out.Info("")

	// Prompt for confirmation
	var proceed bool
	err = huh.NewConfirm().
		Title("Continue with resume?").
		Affirmative("Yes").
		Negative("No").
		Value(&proceed).
		Run()
	if err != nil {
		return true, err // On error (e.g., non-interactive), return error
	}

	return proceed, nil
}
