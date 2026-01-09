// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// SDDCommand defines the supported Speckit SDD commands.
type SDDCommand string

// SDD command constants.
const (
	SDDCmdSpecify   SDDCommand = "specify"
	SDDCmdPlan      SDDCommand = "plan"
	SDDCmdTasks     SDDCommand = "tasks"
	SDDCmdImplement SDDCommand = "implement"
	SDDCmdChecklist SDDCommand = "checklist"
)

// speckitInstallInstructions is the install instructions appended to the Speckit not installed error.
const speckitInstallInstructions = "Install with: uv tool install specify-cli --from git+https://github.com/github/spec-kit.git"

// speckitChecker manages the cached Speckit installation check.
//
//nolint:gochecknoglobals // Package-level state for caching Speckit installation status
var speckitChecker = struct {
	checked bool
	mu      sync.Mutex
}{}

// SDDExecutor handles Speckit SDD steps.
// It invokes Speckit via the AI runner to generate specification artifacts.
type SDDExecutor struct {
	runner         ai.Runner
	artifactSaver  ArtifactSaver
	workingDir     string
	artifactHelper *ArtifactHelper
	logger         zerolog.Logger
}

// NewSDDExecutor creates a new SDD executor.
// Deprecated: Use NewSDDExecutorWithArtifactSaver instead.
func NewSDDExecutor(runner ai.Runner, _ string) *SDDExecutor {
	return &SDDExecutor{
		runner: runner,
		logger: zerolog.Nop(),
	}
}

// NewSDDExecutorWithWorkingDir creates a new SDD executor with a custom working directory.
// Deprecated: Use NewSDDExecutorWithArtifactSaver instead.
func NewSDDExecutorWithWorkingDir(runner ai.Runner, _, workingDir string) *SDDExecutor {
	return &SDDExecutor{
		runner:     runner,
		workingDir: workingDir,
		logger:     zerolog.Nop(),
	}
}

// NewSDDExecutorWithArtifactSaver creates a new SDD executor with artifact saving support.
func NewSDDExecutorWithArtifactSaver(runner ai.Runner, saver ArtifactSaver, workingDir string, logger zerolog.Logger) *SDDExecutor {
	return &SDDExecutor{
		runner:         runner,
		artifactSaver:  saver,
		workingDir:     workingDir,
		artifactHelper: NewArtifactHelper(saver, logger),
		logger:         logger,
	}
}

// SetWorkingDir sets the working directory for Speckit execution.
// This is typically set to the worktree path.
func (e *SDDExecutor) SetWorkingDir(dir string) {
	e.workingDir = dir
}

// Execute runs an SDD command via the AI runner.
// The sdd_command is read from step.Config["sdd_command"].
// Supported commands: specify, plan, tasks, implement, checklist
// Generated artifacts are saved to the task artifacts directory.
func (e *SDDExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log := zerolog.Ctx(ctx)
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing sdd step")

	startTime := time.Now()

	// Get SDD command from step config
	sddCmd := SDDCmdSpecify // default
	if step.Config != nil {
		if cmd, ok := step.Config["sdd_command"].(string); ok {
			sddCmd = SDDCommand(cmd)
		}
	}

	// Check if Speckit is installed (cached check)
	if err := checkSpeckitInstalled(); err != nil {
		log.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Msg("speckit not installed")

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      constants.StepStatusFailed,
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  time.Since(startTime).Milliseconds(),
			Error:       err.Error(),
		}, fmt.Errorf("%w: %w", atlaserrors.ErrClaudeInvocation, err)
	}

	log.Debug().
		Str("sdd_command", string(sddCmd)).
		Bool("has_artifact_saver", e.artifactSaver != nil).
		Str("working_dir", e.workingDir).
		Msg("executing sdd command")

	// Build prompt for Speckit invocation using slash command format
	prompt := e.buildPrompt(task, sddCmd)

	// Check context again before making AI request
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Create AI request with working directory support
	req := &domain.AIRequest{
		Prompt:     prompt,
		Model:      task.Config.Model,
		MaxTurns:   task.Config.MaxTurns,
		Timeout:    task.Config.Timeout,
		WorkingDir: e.workingDir,
	}

	// Apply step timeout if set
	execCtx := ctx
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// Execute via AI runner
	result, err := e.runner.Run(execCtx, req)
	elapsed := time.Since(startTime)

	// Save AI artifact for audit trail (non-blocking, errors logged but don't fail task)
	e.saveSDDArtifact(ctx, task, step, req, result, startTime, elapsed, err)

	if err != nil {
		log.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Str("sdd_command", string(sddCmd)).
			Dur("duration_ms", elapsed).
			Msg("sdd step failed")

		// Wrap with ErrClaudeInvocation and include SDD command context
		wrappedErr := fmt.Errorf("%w: sdd command '%s' failed: %w", atlaserrors.ErrClaudeInvocation, sddCmd, err)

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      constants.StepStatusFailed,
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  elapsed.Milliseconds(),
			Error:       wrappedErr.Error(),
		}, wrappedErr
	}

	// Check for empty output - treat as error
	if result.Output == "" {
		log.Warn().
			Str("task_id", task.ID).
			Str("sdd_command", string(sddCmd)).
			Msg("speckit returned empty output")

		wrappedErr := fmt.Errorf("%w: sdd command '%s' returned empty output", atlaserrors.ErrClaudeInvocation, sddCmd)

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      constants.StepStatusFailed,
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  time.Since(startTime).Milliseconds(),
			Error:       wrappedErr.Error(),
		}, wrappedErr
	}

	// Save artifact to file (implement command doesn't produce a single artifact)
	var artifactPath string
	if sddCmd != SDDCmdImplement {
		artifactPath, err = e.saveArtifact(ctx, task, sddCmd, result.Output)
		if err != nil {
			log.Warn().
				Err(err).
				Str("task_id", task.ID).
				Str("sdd_command", string(sddCmd)).
				Msg("failed to save sdd artifact")
			// Continue even if saving fails - the output is still in the result
		}
	}

	// elapsed is already calculated earlier after runner.Run()
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("sdd_command", string(sddCmd)).
		Str("artifact_path", artifactPath).
		Dur("duration_ms", elapsed).
		Msg("sdd step completed")

	return &domain.StepResult{
		StepIndex:    task.CurrentStep,
		StepName:     step.Name,
		Status:       constants.StepStatusSuccess,
		StartedAt:    startTime,
		CompletedAt:  time.Now(),
		DurationMs:   elapsed.Milliseconds(),
		Output:       result.Output,
		ArtifactPath: artifactPath,
	}, nil
}

// Type returns the step type this executor handles.
func (e *SDDExecutor) Type() domain.StepType {
	return domain.StepTypeSDD
}

// buildPrompt constructs the prompt for invoking Speckit using slash command format.
// Uses the /speckit.<command> format as required by Claude Code slash commands.
func (e *SDDExecutor) buildPrompt(task *domain.Task, cmd SDDCommand) string {
	switch cmd {
	case SDDCmdSpecify:
		// For specify, include the task description as context
		return fmt.Sprintf("/speckit.specify %s", task.Description)
	case SDDCmdPlan:
		// Plan uses existing spec.md
		return "/speckit.plan"
	case SDDCmdTasks:
		// Tasks uses existing spec.md and plan.md
		return "/speckit.tasks"
	case SDDCmdImplement:
		// Implement executes from tasks
		return "/speckit.implement"
	case SDDCmdChecklist:
		// Checklist generates review checklist
		return "/speckit.checklist"
	default:
		// Fallback for any unknown command
		return fmt.Sprintf("/speckit.%s", cmd)
	}
}

// getArtifactFilename returns the semantic filename for an SDD command.
// Returns the filename and true if a mapping exists, empty string and false otherwise.
func getArtifactFilename(cmd SDDCommand) (string, bool) {
	switch cmd {
	case SDDCmdSpecify:
		return "spec.md", true
	case SDDCmdPlan:
		return "plan.md", true
	case SDDCmdTasks:
		return "tasks.md", true
	case SDDCmdChecklist:
		return "checklist.md", true
	case SDDCmdImplement:
		// Implement doesn't produce a single artifact file
		return "", false
	default:
		return "", false
	}
}

// saveArtifact saves the SDD output using the artifact saver.
// Uses semantic naming (spec.md, plan.md, etc.) with versioning for retries.
// Returns the artifact filename if saved successfully, empty string otherwise.
func (e *SDDExecutor) saveArtifact(ctx context.Context, task *domain.Task, cmd SDDCommand, content string) (string, error) {
	if e.artifactSaver == nil {
		return "", nil
	}

	// Get semantic filename for this command
	baseFilename, hasMapping := getArtifactFilename(cmd)
	if !hasMapping {
		// Fallback to timestamp-based naming for unknown commands
		baseFilename = fmt.Sprintf("sdd-%s-%d.md", cmd, time.Now().Unix())
		filename := filepath.Join("sdd", baseFilename)
		if err := e.artifactSaver.SaveArtifact(ctx, task.WorkspaceID, task.ID, filename, []byte(content)); err != nil {
			return "", fmt.Errorf("failed to save artifact: %w", err)
		}
		return filename, nil
	}

	// Use semantic filename with versioning via artifact saver
	// The saver's SaveVersionedArtifact handles version numbering automatically
	baseName := filepath.Join("sdd", baseFilename)
	filename, err := e.artifactSaver.SaveVersionedArtifact(ctx, task.WorkspaceID, task.ID, baseName, []byte(content))
	if err != nil {
		return "", fmt.Errorf("failed to save versioned artifact: %w", err)
	}

	return filename, nil
}

// checkSpeckitInstalled verifies that the Speckit CLI (specify) is installed.
// The result is cached after the first successful check.
func checkSpeckitInstalled() error {
	speckitChecker.mu.Lock()
	defer speckitChecker.mu.Unlock()

	// Return immediately if already verified
	if speckitChecker.checked {
		return nil
	}

	// Check if 'specify' command is available in PATH
	_, err := exec.LookPath("specify")
	if err != nil {
		return fmt.Errorf("%w: Speckit not installed. %s", atlaserrors.ErrUnknownTool, speckitInstallInstructions)
	}

	// Cache the successful check
	speckitChecker.checked = true
	return nil
}

// ResetSpeckitCheck resets the cached Speckit installation check.
// This is primarily used for testing.
func ResetSpeckitCheck() {
	speckitChecker.mu.Lock()
	defer speckitChecker.mu.Unlock()
	speckitChecker.checked = false
}

// SetSpeckitChecked sets the Speckit installation check result.
// This is primarily used for testing.
func SetSpeckitChecked(checked bool) {
	speckitChecker.mu.Lock()
	defer speckitChecker.mu.Unlock()
	speckitChecker.checked = checked
}

// saveSDDArtifact saves the SDD AI request/response as an artifact for audit trail.
// This is non-blocking - artifact save failures are logged but don't fail the task.
func (e *SDDExecutor) saveSDDArtifact(ctx context.Context, task *domain.Task, step *domain.StepDefinition,
	req *domain.AIRequest, result *domain.AIResult, startTime time.Time, elapsed time.Duration, runErr error,
) {
	if e.artifactHelper == nil {
		return
	}

	artifact := &ai.Artifact{
		Timestamp:       startTime,
		StepName:        step.Name,
		StepIndex:       task.CurrentStep,
		Agent:           string(req.Agent),
		Model:           req.Model,
		Request:         req,
		Response:        result,
		ExecutionTimeMs: elapsed.Milliseconds(),
		Success:         runErr == nil,
	}

	if runErr != nil {
		artifact.ErrorMessage = runErr.Error()
	}

	path := e.artifactHelper.SaveAIInteraction(ctx, task, "sdd_step", artifact)
	if path != "" {
		e.logger.Debug().
			Str("artifact_path", path).
			Msg("saved SDD interaction artifact")
	}
}
