// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/cli/workflow"
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
	retry bool // Skip recovery menu and directly retry
	menu  bool // Force recovery menu even for interrupted tasks
}

// newResumeCmd creates the resume command.
func newResumeCmd() *cobra.Command {
	var aiFix bool
	var retry bool
	var menu bool

	cmd := &cobra.Command{
		Use:   "resume <workspace>",
		Short: "Resume a paused or failed task",
		Long: `Resume execution of a task that was paused or failed.

This command intelligently handles all recovery scenarios:
  - Interrupted tasks: Directly resumes execution (fast path)
  - Error tasks: Shows interactive recovery menu with auto-execution
  - Awaiting approval: Continues with approval flow

Error states handled:
  - validation_failed: Validation checks failed
  - gh_failed: GitHub operations (push/PR) failed
  - ci_failed: CI pipeline checks failed
  - ci_timeout: CI pipeline exceeded timeout

Interactive mode (default):
  atlas resume auth-fix

  For interrupted tasks, directly resumes. For error tasks, shows menu with options:
  - Retry with AI fix - AI attempts to fix based on error context
  - Fix manually - Edit files in worktree, then resume
  - Rebase and retry - For non-fast-forward push failures
  - Continue waiting - For CI timeout, resume polling
  - View errors/logs - See detailed error output
  - Abandon task - End task, preserve branch for later

Power user flags:
  atlas resume auth-fix --retry   # Skip menu, directly retry
  atlas resume auth-fix --menu    # Force menu for interrupted tasks

Examples:
  atlas resume auth-fix           # Smart resume (menu for errors, direct for interrupted)
  atlas resume auth-fix --ai-fix  # Resume with AI attempting to fix errors
  atlas resume auth-fix --retry   # Skip menu and directly retry`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(cmd.Context(), cmd, os.Stdout, args[0], resumeOptions{
				aiFix: aiFix,
				retry: retry,
				menu:  menu,
			})
		},
	}

	cmd.Flags().BoolVar(&aiFix, "ai-fix", false, "Retry with AI attempting to fix errors")
	cmd.Flags().BoolVarP(&retry, "retry", "r", false, "Skip recovery menu and directly retry")
	cmd.Flags().BoolVar(&menu, "menu", false, "Show recovery menu even for interrupted tasks")

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

	// Get template
	tmpl, err := prepareResumeTemplate(currentTask, opts, outputFormat, w, workspaceName)
	if err != nil {
		return err
	}

	// Create engine
	engine, err := createResumeEngine(ctx, ws, taskStore, currentTask, logger, out) //nolint:contextcheck // ctx inherits from parent via signal.NewHandler
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, err)
	}

	// Intelligent status-based behavior routing
	//nolint:exhaustive // Only handling specific resumable states
	switch currentTask.Status {
	case constants.TaskStatusInterrupted:
		// Fast path: directly resume unless --menu flag is set
		if opts.menu {
			// User explicitly wants the recovery menu
			//nolint:contextcheck // context properly propagated through function calls
			return handleRecoveryMenu(ctx, cmd, out, taskStore, ws, currentTask, engine, tmpl, sigHandler, wsStore, outputFormat, w, workspaceName, logger)
		}
		// Direct resume for interrupted tasks
		displayResumeInfo(out, workspaceName, currentTask)
		if currentTask.Metadata == nil {
			currentTask.Metadata = make(map[string]any)
		}
		currentTask.Metadata["worktree_dir"] = ws.WorktreePath
		//nolint:contextcheck // context properly propagated through function calls
		return executeResumeAndHandleResult(ctx, engine, currentTask, tmpl, sigHandler, out, ws, wsStore, outputFormat, w, workspaceName, logger)

	case constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout:
		// Error states: show recovery menu unless --retry flag is set
		if opts.retry {
			// Skip menu and directly retry
			displayResumeInfo(out, workspaceName, currentTask)
			if currentTask.Metadata == nil {
				currentTask.Metadata = make(map[string]any)
			}
			currentTask.Metadata["worktree_dir"] = ws.WorktreePath
			//nolint:contextcheck // context properly propagated through function calls
			return executeResumeAndHandleResult(ctx, engine, currentTask, tmpl, sigHandler, out, ws, wsStore, outputFormat, w, workspaceName, logger)
		}
		// Show interactive recovery menu with auto-execution
		//nolint:contextcheck // context properly propagated through function calls
		return handleRecoveryMenu(ctx, cmd, out, taskStore, ws, currentTask, engine, tmpl, sigHandler, wsStore, outputFormat, w, workspaceName, logger)

	case constants.TaskStatusAwaitingApproval:
		// Existing approval flow
		displayResumeInfo(out, workspaceName, currentTask)
		if currentTask.Metadata == nil {
			currentTask.Metadata = make(map[string]any)
		}
		currentTask.Metadata["worktree_dir"] = ws.WorktreePath
		//nolint:contextcheck // context properly propagated through function calls
		return executeResumeAndHandleResult(ctx, engine, currentTask, tmpl, sigHandler, out, ws, wsStore, outputFormat, w, workspaceName, logger)

	default:
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("%w: %s", atlaserrors.ErrInvalidStatus, currentTask.Status))
	}
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

	// Re-apply CLI overrides from original start command
	workflow.ApplyCLIOverridesFromTask(currentTask, tmpl)

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
func createResumeEngine(ctx context.Context, ws *domain.Workspace, taskStore *task.FileStore, currentTask *domain.Task, logger zerolog.Logger, out tui.Output) (*task.Engine, error) {
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	// Create hook manager for resume (via service factory for consistency)
	services := workflow.NewServiceFactory(logger)
	hookManager := services.CreateHookManager(cfg, logger)

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

	// Create progress callback for both engine and executors
	progressCallback := createProgressCallback(ctx, out, ws.Name)

	// Create executor progress callback wrapper that handles both task.StepProgressEvent
	// and steps.AutoFixProgressEvent
	executorProgressCallback := func(event interface{}) {
		switch e := event.(type) {
		case task.StepProgressEvent:
			progressCallback(e)
		case steps.AutoFixProgressEvent:
			// Convert AutoFixProgressEvent to task.StepProgressEvent
			progressCallback(task.StepProgressEvent{
				Type:              e.Type,
				TaskID:            e.TaskID,
				WorkspaceName:     e.WorkspaceName,
				Agent:             e.Agent,
				Model:             e.Model,
				Status:            e.Status,
				DurationMs:        e.DurationMs,
				NumTurns:          e.NumTurns,
				FilesChangedCount: e.FilesChangedCount,
			})
		}
	}

	// Create validation progress callback for consistent step progress display (like start.go)
	validationProgressCallback := createValidationProgressAdapter(progressCallback, ws.Name, len(currentTask.Steps))

	// Create executor registry using service factory (same approach as start.go)
	execRegistry := services.CreateExecutorRegistry(workflow.RegistryDeps{
		WorkDir:   ws.WorktreePath,
		TaskStore: taskStore,
		Notifier:  notifier,
		AIRunner:  aiRunner,
		Logger:    logger,
		GitServices: &workflow.GitServices{
			Runner:           gitRunner,
			SmartCommitter:   smartCommitter,
			Pusher:           pusher,
			HubRunner:        hubRunner,
			PRDescGen:        prDescGen,
			CIFailureHandler: ciFailureHandler,
		},
		Config:                     cfg,
		ProgressCallback:           executorProgressCallback,
		ValidationProgressCallback: validationProgressCallback,
	})

	validationRetryHandler := createResumeValidationRetryHandler(aiRunner, cfg, logger)

	engineCfg := task.DefaultEngineConfig()
	engineCfg.ProgressCallback = progressCallback
	engineOpts := []task.EngineOption{task.WithNotifier(stateNotifier)}
	if validationRetryHandler != nil {
		engineOpts = append(engineOpts, task.WithValidationRetryHandler(validationRetryHandler))
	}
	if hookManager != nil {
		engineOpts = append(engineOpts, task.WithHookManager(hookManager))
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

// handleRecoveryMenu shows the interactive recovery menu and executes the chosen action with auto-resume.
func handleRecoveryMenu(ctx context.Context, _ *cobra.Command, out tui.Output, taskStore *task.FileStore, ws *domain.Workspace, t *domain.Task, engine *task.Engine, tmpl *domain.Template, sigHandler *signal.Handler, wsStore workspace.Store, outputFormat string, w io.Writer, workspaceName string, logger zerolog.Logger) error {
	// Load config for notification settings
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)

	// Display error context
	displayRecoveryErrorContext(out, ws, t)

	// Action menu loop - view actions return to menu
	for {
		action, err := selectRecoveryAction(t)
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Recovery canceled.")
				return nil
			}
			return err
		}

		done, autoResume, err := executeRecoveryActionWithResume(ctx, out, taskStore, ws, t, notifier, action)
		if err != nil {
			return err
		}
		if done {
			// Check if we should auto-resume
			if autoResume {
				// Display info and prepare task for execution
				displayResumeInfo(out, workspaceName, t)
				if t.Metadata == nil {
					t.Metadata = make(map[string]any)
				}
				t.Metadata["worktree_dir"] = ws.WorktreePath

				// Auto-resume execution
				out.Info("Auto-resuming task execution...")
				return executeResumeAndHandleResult(ctx, engine, t, tmpl, sigHandler, out, ws, wsStore, outputFormat, w, workspaceName, logger)
			}
			return nil
		}
		// Continue loop for view actions
	}
}

// displayRecoveryErrorContext shows the error state and relevant information before the recovery menu.
func displayRecoveryErrorContext(out tui.Output, ws *domain.Workspace, t *domain.Task) {
	out.Info("")
	out.Info(fmt.Sprintf("❌ Task failed: %s", t.Description))
	out.Info(fmt.Sprintf("   Workspace: %s", ws.Name))

	// Show which step failed
	if t.CurrentStep < len(t.Steps) {
		stepName := t.Steps[t.CurrentStep].Name
		out.Info(fmt.Sprintf("   Failed at step %d/%d: %s", t.CurrentStep+1, len(t.Steps), stepName))
	}

	out.Info(fmt.Sprintf("   Status: %s", tui.TaskStatusIcon(t.Status)+" "+string(t.Status)))

	displayStatusSpecificContext(out, t)

	out.Info("")
	out.Info("What would you like to do?")
}

// displayStatusSpecificContext shows context specific to the task's error status.
func displayStatusSpecificContext(out tui.Output, t *domain.Task) {
	// Collect error message from step or metadata
	errMsg := getTaskErrorMessage(t)

	// Show error-specific context
	//nolint:exhaustive // Only showing context for specific error states
	switch t.Status {
	case constants.TaskStatusValidationFailed:
		displayValidationContext(out, t)
	case constants.TaskStatusGHFailed:
		// Error details may also be in metadata (fallback if step error is empty)
		if errMsg == "" {
			errMsg = getMetadataError(t)
		}
	case constants.TaskStatusCIFailed, constants.TaskStatusCITimeout:
		displayCIContext(out, t)
	}

	// Display error message (truncate if too long)
	if errMsg != "" {
		if len(errMsg) > 150 {
			errMsg = errMsg[:150] + "..."
		}
		out.Info(fmt.Sprintf("   Error: %s", errMsg))
	}
}

// getTaskErrorMessage retrieves the error message from the current step.
func getTaskErrorMessage(t *domain.Task) string {
	if t.CurrentStep < len(t.Steps) && t.Steps[t.CurrentStep].Error != "" {
		return t.Steps[t.CurrentStep].Error
	}
	return ""
}

// getMetadataError retrieves the error message from task metadata.
func getMetadataError(t *domain.Task) string {
	if t.Metadata != nil {
		if metaErr, ok := t.Metadata["error"].(string); ok {
			return metaErr
		}
	}
	return ""
}

// displayValidationContext shows validation-specific error details.
func displayValidationContext(out tui.Output, t *domain.Task) {
	if t.Metadata != nil {
		if errCount, ok := t.Metadata["validation_error_count"].(int); ok {
			out.Info(fmt.Sprintf("   Errors: %d validation failures", errCount))
		}
	}
}

// displayCIContext shows CI-specific error details.
func displayCIContext(out tui.Output, t *domain.Task) {
	if t.Metadata != nil {
		if ciURL, ok := t.Metadata["ci_url"].(string); ok && ciURL != "" {
			out.Info(fmt.Sprintf("   CI Run: %s", ciURL))
		}
	}
}

// selectRecoveryAction selects the appropriate recovery menu based on task state.
func selectRecoveryAction(t *domain.Task) (tui.RecoveryAction, error) {
	// For GH failed state, use step-aware recovery
	if t.Status == constants.TaskStatusGHFailed {
		return selectGHFailedRecovery(t)
	}

	// Default: use standard recovery menu
	return tui.SelectErrorRecovery(t.Status)
}

// getTaskStepName returns the current step name from the task, or empty string if unavailable.
func getTaskStepName(t *domain.Task) string {
	if t.CurrentStep >= 0 && t.CurrentStep < len(t.Steps) {
		return t.Steps[t.CurrentStep].Name
	}
	return ""
}

// selectGHFailedRecovery shows step-aware recovery options for gh_failed status.
func selectGHFailedRecovery(t *domain.Task) (tui.RecoveryAction, error) {
	// Get step name for context-aware options
	stepName := getTaskStepName(t)

	// Check for specific push error type (existing logic for rebase option)
	action, handled, err := trySelectPushErrorRecovery(t, stepName)
	if handled {
		return action, err
	}

	// Use step-aware options and title
	options := tui.OptionsForGHFailedStep(stepName)
	baseOptions := make([]tui.Option, len(options))
	for i, opt := range options {
		baseOptions[i] = opt.Option
	}

	title := tui.MenuTitleForGHFailedStep(stepName)
	selected, err := tui.Select(title, baseOptions)
	if err != nil {
		return "", err
	}

	return tui.RecoveryAction(selected), nil
}

// trySelectPushErrorRecovery attempts to handle push-specific error recovery.
// Returns (action, handled, error) where handled indicates if push error was found.
func trySelectPushErrorRecovery(t *domain.Task, stepName string) (tui.RecoveryAction, bool, error) {
	if t.Metadata == nil {
		return "", false, nil
	}

	pushErrorType, ok := t.Metadata["push_error_type"].(string)
	if !ok || pushErrorType == "" {
		return "", false, nil
	}

	options := tui.GHFailedOptionsForPushError(pushErrorType)
	if len(options) == 0 {
		return "", false, nil
	}

	baseOptions := make([]tui.Option, len(options))
	for i, opt := range options {
		baseOptions[i] = opt.Option
	}

	// Use step-aware title even for push error type
	title := tui.MenuTitleForGHFailedStep(stepName)
	selected, err := tui.Select(title, baseOptions)
	if err != nil {
		return "", true, err
	}

	return tui.RecoveryAction(selected), true, nil
}

// executeRecoveryActionWithResume executes the selected recovery action.
// Returns (done, autoResume, error) where:
//   - done: true if action loop should exit
//   - autoResume: true if task should automatically resume execution after this action
//   - error: any error that occurred
func executeRecoveryActionWithResume(ctx context.Context, out tui.Output, taskStore *task.FileStore, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, action tui.RecoveryAction) (bool, bool, error) {
	switch action {
	case tui.RecoveryActionRetryAI, tui.RecoveryActionRetryGH, tui.RecoveryActionRetryCommit:
		err := handleRetryAction(ctx, out, taskStore, t, notifier)
		return true, err == nil, err

	case tui.RecoveryActionRebaseRetry:
		err := handleRebaseRetry(ctx, out, taskStore, ws, t, notifier)
		return true, err == nil, err

	case tui.RecoveryActionFixManually:
		err := handleFixManually(out, ws, notifier)
		return true, false, err // No auto-resume for manual fix

	case tui.RecoveryActionViewErrors:
		err := handleViewErrors(ctx, out, taskStore, ws.Name, t.ID)
		return false, false, err // Return to menu

	case tui.RecoveryActionViewLogs:
		err := handleViewLogs(ctx, out, ws, t)
		return false, false, err // Return to menu

	case tui.RecoveryActionContinueWaiting:
		err := handleContinueWaiting(ctx, out, taskStore, t, notifier)
		return true, err == nil, err

	case tui.RecoveryActionAbandon:
		err := handleAbandon(ctx, out, taskStore, ws, t, notifier)
		return true, false, err // No auto-resume for abandon
	}

	return false, false, nil
}

// handleRetryAction handles retry with AI fix actions.
func handleRetryAction(ctx context.Context, out tui.Output, taskStore *task.FileStore, t *domain.Task, notifier *tui.Notifier) error {
	// Transition task back to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User requested retry from recovery menu"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return err
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return err
	}

	out.Success("Retrying with AI fix...")
	notifier.Bell()
	return nil
}

// handleFixManually shows worktree path and resume instructions.
//
//nolint:unparam // error return maintained for consistent interface with other handlers
func handleFixManually(out tui.Output, ws *domain.Workspace, notifier *tui.Notifier) error {
	out.Info("Fix the issue in the worktree manually:")
	out.Info("")
	if ws.WorktreePath != "" {
		out.Info(fmt.Sprintf("  cd %s", ws.WorktreePath))
	} else {
		out.Info(fmt.Sprintf("  cd <worktree for %s>", ws.Name))
	}
	out.Info("  # Make your fixes")
	out.Info(fmt.Sprintf("  atlas resume %s", ws.Name))
	out.Info("")
	notifier.Bell()
	return nil
}

// handleRebaseRetry handles the "Rebase and retry" action for non-fast-forward push failures.
func handleRebaseRetry(ctx context.Context, out tui.Output, taskStore *task.FileStore, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	// Validate worktree path
	if ws.WorktreePath == "" {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("worktree path not available: %w", atlaserrors.ErrWorktreeNotFound)))
		return fmt.Errorf("worktree path not available: %w", atlaserrors.ErrWorktreeNotFound)
	}

	// Get branch name
	branch := ws.Branch
	if branch == "" {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("branch name not available: %w", atlaserrors.ErrEmptyValue)))
		return fmt.Errorf("branch name not available: %w", atlaserrors.ErrEmptyValue)
	}
	remote := "origin"

	// Create git runner for worktree
	runner, err := git.NewRunner(ctx, ws.WorktreePath)
	if err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to create git runner: %w", err)))
		return err
	}

	// Fetch latest from remote
	out.Info(fmt.Sprintf("Fetching latest from %s...", remote))
	if err := runner.Fetch(ctx, remote); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("fetch failed: %w", err)))
		return err
	}

	// Attempt rebase
	rebaseTarget := fmt.Sprintf("%s/%s", remote, branch)
	out.Info(fmt.Sprintf("Rebasing onto %s...", rebaseTarget))
	if err := runner.Rebase(ctx, rebaseTarget); err != nil {
		// Check for conflicts
		if errors.Is(err, atlaserrors.ErrRebaseConflict) {
			// Abort the failed rebase
			_ = runner.RebaseAbort(ctx)

			out.Warning("Rebase has conflicts that require manual resolution:")
			out.Info("")
			out.Info(fmt.Sprintf("  cd %s", ws.WorktreePath))
			out.Info(fmt.Sprintf("  git fetch %s", remote))
			out.Info(fmt.Sprintf("  git rebase %s", rebaseTarget))
			out.Info("  # Resolve conflicts in your editor")
			out.Info("  git add <resolved-files>")
			out.Info("  git rebase --continue")
			out.Info(fmt.Sprintf("  atlas resume %s", ws.Name))
			out.Info("")
			notifier.Bell()
			return nil // Don't auto-resume, user needs to fix conflicts
		}

		out.Error(tui.WrapWithSuggestion(fmt.Errorf("rebase failed: %w", err)))
		return err
	}

	// Rebase succeeded, transition task back to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "Rebased and retrying push"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return err
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return err
	}

	out.Success("Rebase successful. Auto-resuming execution...")
	notifier.Bell()
	return nil
}

// handleViewErrors displays the validation errors.
//
//nolint:unparam // error return maintained for consistent interface with other handlers
func handleViewErrors(ctx context.Context, out tui.Output, taskStore *task.FileStore, workspaceName, taskID string) error {
	// Try to get validation artifact
	data, err := taskStore.GetArtifact(ctx, workspaceName, taskID, "validation.json")
	if err != nil {
		// Try alternate filename
		data, err = taskStore.GetArtifact(ctx, workspaceName, taskID, "validation-result.json")
		if err != nil {
			out.Warning(fmt.Sprintf("Could not load validation results: %v", err))
			return nil
		}
	}

	if len(data) == 0 {
		out.Info("No validation errors recorded.")
		return nil
	}

	// Display the validation output
	out.Info("")
	out.Info("--- Validation Output ---")
	out.Info(string(data))
	out.Info("-------------------------")
	out.Info("")

	return nil
}

// handleViewLogs opens GitHub Actions in browser for CI states.
//
//nolint:unparam // error return maintained for consistent interface with other handlers
func handleViewLogs(ctx context.Context, out tui.Output, ws *domain.Workspace, t *domain.Task) error {
	// Extract GitHub Actions URL from task metadata or PR URL
	ghURL := extractGitHubActionsURL(t)
	if ghURL == "" {
		// Fall back to PR URL if available
		prURL := extractPRURL(t)
		if prURL != "" {
			ghURL = prURL + "/checks"
		}
	}

	if ghURL == "" {
		out.Warning("No GitHub Actions URL available.")
		out.Info(fmt.Sprintf("You can manually check: https://github.com/%s/actions", extractRepoInfo(ws)))
		return nil
	}

	// Open in browser
	if err := openInBrowser(ctx, ghURL); err != nil {
		out.Warning(fmt.Sprintf("Could not open browser: %v", err))
		out.Info(fmt.Sprintf("URL: %s", ghURL))
	} else {
		out.Info(fmt.Sprintf("Opened %s in browser.", ghURL))
	}

	return nil
}

// handleContinueWaiting resumes CI polling.
func handleContinueWaiting(ctx context.Context, out tui.Output, taskStore *task.FileStore, t *domain.Task, notifier *tui.Notifier) error {
	// Transition task back to running to continue CI polling
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User requested to continue waiting"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return err
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return err
	}

	out.Success("Continuing CI polling. Auto-resuming execution...")
	notifier.Bell()
	return nil
}

// handleAbandon transitions task to abandoned state.
func handleAbandon(ctx context.Context, out tui.Output, taskStore *task.FileStore, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	// Transition task to abandoned
	if err := task.Transition(ctx, t, constants.TaskStatusAbandoned, "User abandoned from recovery menu"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return err
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return err
	}

	out.Info(fmt.Sprintf("Task abandoned. Branch '%s' preserved at '%s'", ws.Branch, ws.WorktreePath))
	out.Info("You can work on the code manually or destroy the workspace later.")
	notifier.Bell()
	return nil
}

// extractGitHubActionsURL extracts the GitHub Actions URL from task metadata.
func extractGitHubActionsURL(t *domain.Task) string {
	if t == nil || t.Metadata == nil {
		return ""
	}
	if url, ok := t.Metadata["ci_url"].(string); ok {
		return url
	}
	if url, ok := t.Metadata["github_actions_url"].(string); ok {
		return url
	}
	return ""
}

// extractRepoInfo extracts repository info from workspace for manual URL construction.
func extractRepoInfo(ws *domain.Workspace) string {
	if ws == nil || ws.Metadata == nil {
		return ""
	}
	if repo, ok := ws.Metadata["repository"].(string); ok {
		return repo
	}
	return ""
}
