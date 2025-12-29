// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/domain"
)

// SDDCommand defines the supported Speckit SDD commands.
type SDDCommand string

// SDD command constants.
const (
	SDDCmdSpecify   SDDCommand = "specify"
	SDDCmdPlan      SDDCommand = "plan"
	SDDCmdTasks     SDDCommand = "tasks"
	SDDCmdChecklist SDDCommand = "checklist"
)

// SDDExecutor handles Speckit SDD steps.
// It invokes Speckit via the AI runner to generate specification artifacts.
type SDDExecutor struct {
	runner       ai.Runner
	artifactsDir string
}

// NewSDDExecutor creates a new SDD executor.
func NewSDDExecutor(runner ai.Runner, artifactsDir string) *SDDExecutor {
	return &SDDExecutor{
		runner:       runner,
		artifactsDir: artifactsDir,
	}
}

// Execute runs an SDD command via the AI runner.
// The sdd_command is read from step.Config["sdd_command"].
// Supported commands: specify, plan, tasks, checklist
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

	log.Debug().
		Str("sdd_command", string(sddCmd)).
		Str("artifacts_dir", e.artifactsDir).
		Msg("executing sdd command")

	// Build prompt for Speckit invocation
	prompt := e.buildPrompt(task, sddCmd)

	// Create AI request
	req := &domain.AIRequest{
		Prompt:   prompt,
		Model:    task.Config.Model,
		MaxTurns: task.Config.MaxTurns,
		Timeout:  task.Config.Timeout,
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
	if err != nil {
		elapsed := time.Since(startTime)
		log.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Str("sdd_command", string(sddCmd)).
			Dur("duration_ms", elapsed).
			Msg("sdd step failed")

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      "failed",
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  elapsed.Milliseconds(),
			Error:       err.Error(),
		}, err
	}

	// Save artifact to file
	artifactPath, err := e.saveArtifact(task.ID, sddCmd, result.Output)
	if err != nil {
		log.Warn().
			Err(err).
			Str("task_id", task.ID).
			Str("sdd_command", string(sddCmd)).
			Msg("failed to save sdd artifact")
		// Continue even if saving fails - the output is still in the result
	}

	elapsed := time.Since(startTime)
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
		Status:       "success",
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

// buildPrompt constructs the prompt for invoking Speckit.
func (e *SDDExecutor) buildPrompt(task *domain.Task, cmd SDDCommand) string {
	switch cmd {
	case SDDCmdSpecify:
		return fmt.Sprintf("Using Speckit SDD, generate a specification for: %s", task.Description)
	case SDDCmdPlan:
		return fmt.Sprintf("Using Speckit SDD, create an implementation plan for: %s", task.Description)
	case SDDCmdTasks:
		return fmt.Sprintf("Using Speckit SDD, break down into tasks: %s", task.Description)
	case SDDCmdChecklist:
		return fmt.Sprintf("Using Speckit SDD, create a review checklist for: %s", task.Description)
	default:
		return fmt.Sprintf("Using Speckit SDD (%s): %s", cmd, task.Description)
	}
}

// saveArtifact saves the SDD output to a file in the artifacts directory.
func (e *SDDExecutor) saveArtifact(taskID string, cmd SDDCommand, content string) (string, error) {
	if e.artifactsDir == "" {
		return "", nil
	}

	// Ensure artifacts directory exists
	taskDir := filepath.Join(e.artifactsDir, taskID)
	if err := os.MkdirAll(taskDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	// Generate filename based on command
	filename := fmt.Sprintf("sdd-%s-%d.md", cmd, time.Now().Unix())
	artifactPath := filepath.Join(taskDir, filename)

	// Write content
	if err := os.WriteFile(artifactPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	return artifactPath, nil
}
