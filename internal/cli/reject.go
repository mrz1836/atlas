// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// AddRejectCommand adds the reject command to the root command.
func AddRejectCommand(root *cobra.Command) {
	root.AddCommand(newRejectCmd())
}

// rejectOptions contains all options for the reject command.
type rejectOptions struct {
	workspace string // Required workspace name
	retry     bool   // For JSON mode: reject and retry
	done      bool   // For JSON mode: reject done
	feedback  string // For JSON mode with retry: feedback text
	step      int    // For JSON mode with retry: step to resume from
}

// newRejectCmd creates the reject command.
func newRejectCmd() *cobra.Command {
	opts := &rejectOptions{}

	cmd := &cobra.Command{
		Use:   "reject [workspace]",
		Short: "Reject work with feedback",
		Long: `Reject a task that is awaiting approval.

You can either reject and retry (AI will retry with your feedback) or
reject and be done (preserve branch for manual work).

Interactive mode:
  atlas reject my-feature

  Presents a decision flow with options:
  - "Reject and retry": Provide feedback and choose a step to resume from
  - "Reject (done)": End task, preserve branch for manual work

JSON mode (requires --output json):
  # Reject with retry (step is 1-indexed, 0 = auto-select)
  atlas reject my-feature --output json --retry --feedback "Fix auth flow" --step 3

  # Reject done
  atlas reject my-feature --output json --done

Examples:
  atlas reject                    # Interactive selection if multiple tasks
  atlas reject my-feature         # Reject task in my-feature workspace
  atlas reject -o json my-feature --done  # JSON output, reject done`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.workspace = args[0]
			}
			return runReject(cmd.Context(), cmd, os.Stdout, opts)
		},
	}

	// Add JSON mode flags
	cmd.Flags().BoolVar(&opts.retry, "retry", false, "Reject and retry (JSON mode)")
	cmd.Flags().BoolVar(&opts.done, "done", false, "Reject and be done (JSON mode)")
	cmd.Flags().StringVar(&opts.feedback, "feedback", "", "Feedback for retry (JSON mode)")
	cmd.Flags().IntVar(&opts.step, "step", 0, "Step to resume from, 1-indexed (0 = auto-select, JSON mode)")

	return cmd
}

// rejectResponse represents the JSON output for reject operations.
type rejectResponse struct {
	Success       bool   `json:"success"`
	Action        string `json:"action"` // "retry" or "done"
	WorkspaceName string `json:"workspace_name"`
	TaskID        string `json:"task_id"`
	Feedback      string `json:"feedback,omitempty"`
	ResumeStep    int    `json:"resume_step,omitempty"`
	BranchName    string `json:"branch_name,omitempty"`
	WorktreePath  string `json:"worktree_path,omitempty"`
	Error         string `json:"error,omitempty"`
}

// rejectAction represents the decision flow options.
type rejectAction string

const (
	rejectActionRetry rejectAction = "retry"
	rejectActionDone  rejectAction = "done"
)

// runReject executes the reject command.
func runReject(ctx context.Context, cmd *cobra.Command, w io.Writer, opts *rejectOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := Logger()
	outputFormat := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// JSON mode requires workspace argument
	if outputFormat == OutputJSON && opts.workspace == "" {
		return handleRejectError(outputFormat, w, "", fmt.Errorf("workspace argument required with --output json: %w", atlaserrors.ErrInvalidArgument))
	}

	// JSON mode requires --retry or --done flag
	if outputFormat == OutputJSON && !opts.retry && !opts.done {
		return handleRejectError(outputFormat, w, opts.workspace, fmt.Errorf("--retry or --done flag required with --output json: %w", atlaserrors.ErrInvalidArgument))
	}

	// Cannot specify both --retry and --done
	if opts.retry && opts.done {
		return handleRejectError(outputFormat, w, opts.workspace, fmt.Errorf("cannot use both --retry and --done: %w", atlaserrors.ErrInvalidArgument))
	}

	// Create stores
	wsStore, taskStore, err := CreateStores("")
	if err != nil {
		return handleRejectError(outputFormat, w, "", err)
	}

	// Find and select task
	selectedWS, selectedTask, err := findAndSelectTaskForReject(ctx, outputFormat, w, out, opts, wsStore, taskStore)
	if err != nil {
		return err
	}
	if selectedWS == nil {
		// No tasks awaiting approval (message already shown)
		return nil
	}

	logger.Debug().
		Str("workspace_name", selectedWS.Name).
		Str("task_id", selectedTask.ID).
		Str("status", string(selectedTask.Status)).
		Msg("selected task for rejection")

	// Load config for notification settings
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	// Create notifier
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)

	// JSON mode: process directly without interactive menu
	if outputFormat == OutputJSON {
		return processJSONReject(ctx, w, taskStore, selectedWS, selectedTask, opts)
	}

	// Interactive rejection flow
	return runInteractiveReject(ctx, out, taskStore, selectedWS, selectedTask, notifier)
}

// findAndSelectTaskForReject finds awaiting tasks and selects one based on options.
func findAndSelectTaskForReject(ctx context.Context, outputFormat string, w io.Writer, out tui.Output, opts *rejectOptions, wsStore workspace.Store, taskStore task.Store) (*domain.Workspace, *domain.Task, error) {
	// Find tasks awaiting approval
	awaitingTasks, err := findAwaitingApprovalTasks(ctx, wsStore, taskStore)
	if err != nil {
		return nil, nil, handleRejectError(outputFormat, w, "", err)
	}

	// Handle case where no tasks are awaiting approval
	if len(awaitingTasks) == 0 {
		if outputFormat == OutputJSON {
			return nil, nil, handleRejectError(outputFormat, w, "", atlaserrors.ErrNoTasksFound)
		}
		out.Info("No tasks awaiting approval.")
		out.Info("Run 'atlas status' to see all workspace statuses.")
		return nil, nil, nil
	}

	// If workspace provided, find it directly
	if opts.workspace != "" {
		for _, at := range awaitingTasks {
			if at.workspace.Name == opts.workspace {
				return at.workspace, at.task, nil
			}
		}
		return nil, nil, handleRejectError(outputFormat, w, opts.workspace, fmt.Errorf("workspace '%s' not found or not awaiting approval: %w", opts.workspace, atlaserrors.ErrWorkspaceNotFound))
	}

	// Auto-select if only one task
	if len(awaitingTasks) == 1 {
		return awaitingTasks[0].workspace, awaitingTasks[0].task, nil
	}

	// Present selection menu
	selected, err := selectWorkspaceForReject(awaitingTasks)
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Rejection canceled.")
			return nil, nil, nil
		}
		return nil, nil, handleRejectError(outputFormat, w, "", err)
	}

	return selected.workspace, selected.task, nil
}

// selectWorkspaceForReject presents a selection menu for multiple awaiting tasks.
func selectWorkspaceForReject(tasks []awaitingTask) (*awaitingTask, error) {
	idx, err := SelectWorkspaceTask("Select a workspace to reject:", tasks)
	if err != nil {
		return nil, err
	}
	return &tasks[idx], nil
}

// runInteractiveReject runs the interactive rejection flow.
func runInteractiveReject(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	// Display task summary (AC: #1)
	displayTaskSummary(out, ws, t)

	// Present decision flow (AC: #2)
	action, err := selectRejectAction()
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Rejection canceled.")
			return nil
		}
		return err
	}

	switch action {
	case rejectActionRetry:
		return handleRejectAndRetry(ctx, out, taskStore, ws, t, notifier)
	case rejectActionDone:
		return handleRejectDone(ctx, out, taskStore, ws, t, notifier)
	}

	return nil
}

// displayTaskSummary shows a brief summary of the task before the decision flow.
func displayTaskSummary(out tui.Output, ws *domain.Workspace, t *domain.Task) {
	out.Info(fmt.Sprintf("Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("Task: %s", t.Description))
	out.Info(fmt.Sprintf("Status: %s", t.Status))
	out.Info("")
}

// selectRejectAction presents the decision flow menu (AC: #2).
func selectRejectAction() (rejectAction, error) {
	options := []tui.Option{
		{
			Label:       "Reject and retry",
			Description: "AI will retry with your feedback",
			Value:       string(rejectActionRetry),
		},
		{
			Label:       "Reject (done)",
			Description: "End task, preserve branch for manual work",
			Value:       string(rejectActionDone),
		},
	}

	selected, err := tui.Select("How would you like to proceed?", options)
	if err != nil {
		return "", err
	}

	return rejectAction(selected), nil
}

// handleRejectAndRetry processes the "Reject and retry" flow (AC: #3, #4, #5, #6, #7).
func handleRejectAndRetry(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	// Get feedback with validation loop (AC: #3)
	var feedback string
	for {
		var err error
		feedback, err = tui.TextArea("What should be changed or fixed?", "Describe what needs to be improved...")
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Rejection canceled.")
				return nil
			}
			return err
		}

		// Validate feedback is not empty (AC: #3)
		feedback = strings.TrimSpace(feedback)
		if feedback != "" {
			break
		}
		out.Warning("Feedback is required for retry. Please provide details about what should be fixed.")
	}

	// Get step to resume from (AC: #4)
	resumeStep, err := selectResumeStep(t)
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Rejection canceled.")
			return nil
		}
		return err
	}

	// Save feedback as artifact (AC: #5)
	if err := saveRejectionFeedback(ctx, taskStore, ws.Name, t.ID, feedback, resumeStep); err != nil {
		return fmt.Errorf("failed to save rejection feedback: %w", err)
	}

	// Update task metadata
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	t.Metadata["rejection_feedback"] = feedback
	t.Metadata["resume_from_step"] = resumeStep

	// Reset current step to selected step (AC: #6)
	t.CurrentStep = resumeStep

	// Transition task status to running (AC: #6)
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User rejected with feedback, retrying"); err != nil {
		return fmt.Errorf("failed to transition task: %w", err)
	}

	// Save updated task (AC: #6)
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	stepName := ""
	if resumeStep >= 0 && resumeStep < len(t.Steps) {
		stepName = t.Steps[resumeStep].Name
	}

	out.Success(fmt.Sprintf("Task rejected with feedback. Resuming from step %d: %s", resumeStep+1, stepName))
	notifier.Bell()

	return nil
}

// selectResumeStep presents the step selection menu (AC: #4).
func selectResumeStep(t *domain.Task) (int, error) {
	if len(t.Steps) == 0 {
		return 0, nil
	}

	options := make([]tui.Option, len(t.Steps))
	defaultStep := findDefaultResumeStep(t)

	for i, step := range t.Steps {
		options[i] = tui.Option{
			Label:       fmt.Sprintf("Step %d: %s", i+1, step.Name),
			Description: fmt.Sprintf("Type: %s", step.Type),
			Value:       strconv.Itoa(i),
		}
	}

	// Reorder to put default step first
	if defaultStep > 0 && defaultStep < len(options) {
		defaultOpt := options[defaultStep]
		remaining := make([]tui.Option, 0, len(options)-1)
		for i, opt := range options {
			if i != defaultStep {
				remaining = append(remaining, opt)
			}
		}
		options = append([]tui.Option{defaultOpt}, remaining...)
	}

	selected, err := tui.Select("Resume from which step?", options)
	if err != nil {
		return 0, err
	}

	stepIdx, err := strconv.Atoi(selected)
	if err != nil {
		return 0, fmt.Errorf("invalid step selection: %w", err)
	}

	return stepIdx, nil
}

// findDefaultResumeStep finds the earliest relevant step (e.g., "implement" step) (AC: #4).
func findDefaultResumeStep(t *domain.Task) int {
	implementStepNames := []string{"implement", "implementation", "code", "develop"}

	for i, step := range t.Steps {
		stepNameLower := strings.ToLower(step.Name)
		for _, name := range implementStepNames {
			if strings.Contains(stepNameLower, name) {
				return i
			}
		}
	}

	// Default to step 0 if no "implement" step found
	return 0
}

// saveRejectionFeedback saves the feedback as an artifact (AC: #5).
func saveRejectionFeedback(ctx context.Context, taskStore task.Store, workspaceName, taskID, feedback string, resumeStep int) error {
	content := fmt.Sprintf(`# Rejection Feedback

Date: %s
Resume From: Step %d

## Feedback

%s
`, time.Now().UTC().Format(time.RFC3339), resumeStep+1, feedback)

	return taskStore.SaveArtifact(ctx, workspaceName, taskID, "rejection-feedback.md", []byte(content))
}

// handleRejectDone processes the "Reject (done)" flow (AC: #8, #9).
func handleRejectDone(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	// Transition task status to rejected (AC: #8)
	if err := task.Transition(ctx, t, constants.TaskStatusRejected, "User rejected task"); err != nil {
		return fmt.Errorf("failed to transition task: %w", err)
	}

	// Save updated task (AC: #8)
	// Workspace status is NOT changed - branch and worktree preserved (AC: #9)
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	out.Info(fmt.Sprintf("Task rejected. Branch '%s' preserved at '%s'", ws.Branch, ws.WorktreePath))
	out.Info("You can work on the code manually or destroy the workspace later.")
	notifier.Bell()

	return nil
}

// processJSONReject handles JSON mode rejection (AC: #10).
func processJSONReject(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task, opts *rejectOptions) error {
	if opts.retry {
		return processJSONRejectRetry(ctx, w, taskStore, ws, t, opts)
	}
	return processJSONRejectDone(ctx, w, taskStore, ws, t)
}

// processJSONRejectRetry handles JSON mode "reject and retry" (AC: #10).
func processJSONRejectRetry(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task, opts *rejectOptions) error {
	// Validate feedback for retry
	feedback := strings.TrimSpace(opts.feedback)
	if feedback == "" {
		return outputRejectErrorJSON(w, ws.Name, t.ID, "feedback required for retry (use --feedback)")
	}

	// Determine resume step (1-indexed input, 0 = auto-select)
	var resumeStep int
	if opts.step == 0 {
		// Auto-select default step (same logic as interactive mode)
		resumeStep = findDefaultResumeStep(t)
	} else {
		// Validate 1-indexed step input
		if opts.step < 1 || (len(t.Steps) > 0 && opts.step > len(t.Steps)) {
			return outputRejectErrorJSON(w, ws.Name, t.ID, fmt.Sprintf("invalid step %d (valid range: 1-%d, or 0 for auto)", opts.step, len(t.Steps)))
		}
		// Convert 1-indexed input to 0-indexed internal
		resumeStep = opts.step - 1
	}

	// Save feedback as artifact
	if err := saveRejectionFeedback(ctx, taskStore, ws.Name, t.ID, feedback, resumeStep); err != nil {
		return outputRejectErrorJSON(w, ws.Name, t.ID, fmt.Sprintf("failed to save feedback: %v", err))
	}

	// Update task metadata
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	t.Metadata["rejection_feedback"] = feedback
	t.Metadata["resume_from_step"] = resumeStep

	// Reset current step
	t.CurrentStep = resumeStep

	// Transition to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User rejected with feedback (JSON mode)"); err != nil {
		return outputRejectErrorJSON(w, ws.Name, t.ID, fmt.Sprintf("failed to transition task: %v", err))
	}

	// Save task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return outputRejectErrorJSON(w, ws.Name, t.ID, fmt.Sprintf("failed to save task: %v", err))
	}

	// Output success (1-indexed step for consistency with UI)
	resp := rejectResponse{
		Success:       true,
		Action:        "retry",
		WorkspaceName: ws.Name,
		TaskID:        t.ID,
		Feedback:      feedback,
		ResumeStep:    resumeStep + 1, // Output 1-indexed to match UI display
		BranchName:    ws.Branch,
		WorktreePath:  ws.WorktreePath,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// processJSONRejectDone handles JSON mode "reject done" (AC: #10).
func processJSONRejectDone(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task) error {
	// Transition to rejected
	if err := task.Transition(ctx, t, constants.TaskStatusRejected, "User rejected task (JSON mode)"); err != nil {
		return outputRejectErrorJSON(w, ws.Name, t.ID, fmt.Sprintf("failed to transition task: %v", err))
	}

	// Save task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return outputRejectErrorJSON(w, ws.Name, t.ID, fmt.Sprintf("failed to save task: %v", err))
	}

	// Output success
	resp := rejectResponse{
		Success:       true,
		Action:        "done",
		WorkspaceName: ws.Name,
		TaskID:        t.ID,
		BranchName:    ws.Branch,
		WorktreePath:  ws.WorktreePath,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// handleRejectError handles errors based on output format.
func handleRejectError(format string, w io.Writer, workspaceName string, err error) error {
	return HandleCommandError(format, w, rejectResponse{
		Success:       false,
		WorkspaceName: workspaceName,
		Error:         err.Error(),
	}, err)
}

// outputRejectErrorJSON outputs an error result as JSON.
func outputRejectErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	return HandleCommandError(OutputJSON, w, rejectResponse{
		Success:       false,
		WorkspaceName: workspaceName,
		TaskID:        taskID,
		Error:         errMsg,
	}, atlaserrors.ErrJSONErrorOutput)
}
