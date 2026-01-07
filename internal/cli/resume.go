// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
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

	logger := GetLogger()
	outputFormat := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// Setup workspace manager and get workspace
	_, ws, err := setupWorkspace(ctx, workspaceName, "", outputFormat, w, logger)
	if err != nil {
		return err
	}

	// Get task store and latest task
	taskStore, currentTask, err := getLatestTask(ctx, workspaceName, "", outputFormat, w, logger)
	if err != nil {
		return err
	}

	// Validate task is in resumable state
	if !isResumableStatus(currentTask.Status) {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("%w: task status %s is not resumable", atlaserrors.ErrInvalidTransition, currentTask.Status))
	}

	// Check if worktree exists and recreate if needed
	ws, err = ensureWorktreeExists(ctx, ws, out, logger)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("failed to ensure worktree exists: %w", err))
	}

	// Get template
	registry := template.NewDefaultRegistry()
	tmpl, err := registry.Get(currentTask.TemplateID)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, fmt.Errorf("failed to get template: %w", err))
	}

	// If AI fix requested, show not yet implemented
	if opts.aiFix {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("--ai-fix not yet implemented: %w", atlaserrors.ErrResumeNotImplemented))
	}

	// Create engine with all dependencies
	engine, err := createResumeEngine(ctx, ws, taskStore, logger)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, err)
	}

	// Display resume information
	displayResumeInfo(out, workspaceName, currentTask)

	// Ensure worktree_dir is set in task metadata for validation retry
	if currentTask.Metadata == nil {
		currentTask.Metadata = make(map[string]any)
	}
	currentTask.Metadata["worktree_dir"] = ws.WorktreePath

	// Resume task execution
	if err := engine.Resume(ctx, currentTask, tmpl); err != nil {
		if currentTask.Status == constants.TaskStatusValidationFailed {
			tui.DisplayManualFixInstructions(out, currentTask, ws)
		}
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, err)
	}

	// Handle JSON output format
	if outputFormat == OutputJSON {
		return outputResumeSuccessJSON(out, ws, currentTask)
	}

	return displayResumeResult(out, ws, currentTask, nil)
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
	resp := resumeResponse{
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
	if err := encoder.Encode(resp); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return atlaserrors.ErrJSONErrorOutput
}

// displayResumeResult outputs the resume result in the appropriate format.
func displayResumeResult(out tui.Output, ws *domain.Workspace, t *domain.Task, execErr error) error {
	// TTY output
	out.Success(fmt.Sprintf("Task resumed successfully. Status: %s", t.Status))
	out.Info(fmt.Sprintf("  Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("  Task ID:   %s", t.ID))
	out.Info(fmt.Sprintf("  Progress:  Step %d/%d", t.CurrentStep+1, len(t.Steps)))

	if execErr != nil {
		out.Warning(fmt.Sprintf("Execution paused: %s", execErr.Error()))
	}

	return nil
}

// createResumeEngine creates the task engine with all required dependencies.
func createResumeEngine(ctx context.Context, ws *domain.Workspace, taskStore *task.FileStore, logger zerolog.Logger) (*task.Engine, error) {
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

	// Resolve commit agent/model with fallback to global AI config
	commitAgent := cfg.SmartCommit.Agent
	if commitAgent == "" {
		commitAgent = cfg.AI.Agent
	}
	commitModel := cfg.SmartCommit.Model
	if commitModel == "" {
		commitModel = cfg.AI.Model
	}

	// Resolve PR description agent/model with fallback to global AI config
	prDescAgent := cfg.PRDescription.Agent
	if prDescAgent == "" {
		prDescAgent = cfg.AI.Agent
	}
	prDescModel := cfg.PRDescription.Model
	if prDescModel == "" {
		prDescModel = cfg.AI.Model
	}

	// Resolve smart commit timeout/retry settings with defaults
	commitTimeout := cfg.SmartCommit.Timeout
	if commitTimeout == 0 {
		commitTimeout = 30 * time.Second
	}
	commitMaxRetries := cfg.SmartCommit.MaxRetries
	if commitMaxRetries == 0 {
		commitMaxRetries = 2
	}
	commitRetryBackoffFactor := cfg.SmartCommit.RetryBackoffFactor
	if commitRetryBackoffFactor == 0 {
		commitRetryBackoffFactor = 1.5
	}

	smartCommitter := git.NewSmartCommitRunner(gitRunner, ws.WorktreePath, aiRunner,
		git.WithAgent(commitAgent),
		git.WithModel(commitModel),
		git.WithTimeout(commitTimeout),
		git.WithMaxRetries(commitMaxRetries),
		git.WithRetryBackoffFactor(commitRetryBackoffFactor),
		git.WithLogger(logger),
	)
	pusher := git.NewPushRunner(gitRunner)
	hubRunner := git.NewCLIGitHubRunner(ws.WorktreePath)
	prDescGen := git.NewAIDescriptionGenerator(aiRunner,
		git.WithAIDescAgent(prDescAgent),
		git.WithAIDescModel(prDescModel),
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
	engineOpts := []task.EngineOption{task.WithNotifier(stateNotifier)}
	if validationRetryHandler != nil {
		engineOpts = append(engineOpts, task.WithValidationRetryHandler(validationRetryHandler))
	}

	return task.NewEngine(taskStore, execRegistry, engineCfg, logger, engineOpts...), nil
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
