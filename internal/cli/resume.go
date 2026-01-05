// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

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
	"github.com/mrz1836/atlas/internal/workspace"
	"github.com/spf13/cobra"
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

	// Create workspace store and manager
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create workspace store: %w", err))
	}

	// Find git repository for worktree runner
	repoPath, err := findGitRepository(ctx)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("not in a git repository: %w", err))
	}

	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, repoPath)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create worktree runner: %w", err))
	}

	wsMgr := workspace.NewManager(wsStore, wtRunner)

	// Get workspace
	ws, err := wsMgr.Get(ctx, workspaceName)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to get workspace: %w", err))
	}

	// Create task store
	taskStore, err := task.NewFileStore("")
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create task store: %w", err))
	}

	// Get latest task for this workspace
	tasks, err := taskStore.List(ctx, workspaceName)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to list tasks: %w", err))
	}

	if len(tasks) == 0 {
		return handleResumeError(outputFormat, w, workspaceName, "", fmt.Errorf("no tasks found in workspace '%s': %w", workspaceName, atlaserrors.ErrNoTasksFound))
	}

	// Get the latest task (list returns newest first)
	currentTask := tasks[0]

	logger.Debug().
		Str("workspace_name", workspaceName).
		Str("task_id", currentTask.ID).
		Str("status", string(currentTask.Status)).
		Msg("found task to resume")

	// Validate task is in resumable state
	if !isResumableStatus(currentTask.Status) {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("%w: task status %s is not resumable", atlaserrors.ErrInvalidTransition, currentTask.Status))
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

	// Load config for notification settings
	cfg, err := config.Load(ctx)
	if err != nil {
		// Log warning but continue with defaults
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	// Create notifier from config
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)

	// Create state change notifier for engine-level notifications (Story 7.6).
	// This emits bell on task state transitions to attention-required states.
	stateNotifier := task.NewStateChangeNotifier(task.NotificationConfig{
		BellEnabled: cfg.Notifications.Bell,
		Quiet:       false, // TODO: Pass quiet flag through when available
		Events:      cfg.Notifications.Events,
	})

	// Create AI runner for AI-dependent services
	aiRunner := ai.NewClaudeCodeRunner(&cfg.AI, nil)

	// Create git services for commit, push, and PR operations
	gitRunner, err := git.NewRunner(ctx, ws.WorktreePath)
	if err != nil {
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, fmt.Errorf("failed to create git runner: %w", err))
	}

	// Determine model for smart commit: smart_commit.model > ai.model
	commitModel := cfg.SmartCommit.Model
	if commitModel == "" {
		commitModel = cfg.AI.Model
	}

	smartCommitter := git.NewSmartCommitRunner(gitRunner, ws.WorktreePath, aiRunner,
		git.WithModel(commitModel),
	)
	pusher := git.NewPushRunner(gitRunner)
	hubRunner := git.NewCLIGitHubRunner(ws.WorktreePath)
	prDescGen := git.NewAIDescriptionGenerator(aiRunner)
	ciFailureHandler := task.NewCIFailureHandler(hubRunner)

	// Create executor registry with full dependencies
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

	engineCfg := task.DefaultEngineConfig()
	engine := task.NewEngine(taskStore, execRegistry, engineCfg, logger,
		task.WithNotifier(stateNotifier),
	)

	// Display resume information
	out.Info(fmt.Sprintf("Resuming task in workspace '%s'...", workspaceName))
	out.Info(fmt.Sprintf("  Task ID: %s", currentTask.ID))
	out.Info(fmt.Sprintf("  Current Status: %s", currentTask.Status))
	out.Info(fmt.Sprintf("  Current Step: %d/%d", currentTask.CurrentStep+1, len(currentTask.Steps)))

	// Resume task execution
	if err := engine.Resume(ctx, currentTask, tmpl); err != nil {
		// If validation failed again, display manual fix info
		if currentTask.Status == constants.TaskStatusValidationFailed {
			tui.DisplayManualFixInstructions(out, currentTask, ws)
		}
		return handleResumeError(outputFormat, w, workspaceName, currentTask.ID, err)
	}

	// Handle JSON output format
	if outputFormat == OutputJSON {
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
