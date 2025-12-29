package validation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ArtifactSaver abstracts artifact persistence.
// This interface allows the handler to save validation results without
// depending on the task package, maintaining proper package boundaries.
type ArtifactSaver interface {
	SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
}

// Notifier abstracts notification delivery.
type Notifier interface {
	Bell() // Emit terminal bell
}

// ResultHandler handles validation pipeline results.
// It saves results as artifacts, emits notifications on failure,
// and returns appropriate errors for task engine state transitions.
type ResultHandler struct {
	saver    ArtifactSaver
	notifier Notifier
	logger   zerolog.Logger
}

// NewResultHandler creates a result handler.
func NewResultHandler(saver ArtifactSaver, notifier Notifier, logger zerolog.Logger) *ResultHandler {
	return &ResultHandler{
		saver:    saver,
		notifier: notifier,
		logger:   logger,
	}
}

// HandleResult processes a validation pipeline result.
// It always saves the result as a versioned artifact (validation.json).
//
// Returns nil if validation passed (task should auto-proceed).
// Returns ErrValidationFailed if validation failed (task should pause).
func (h *ResultHandler) HandleResult(ctx context.Context, workspaceName, taskID string, result *PipelineResult) error {
	// Marshal result to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal validation result: %w", err)
	}

	// Save result as versioned artifact
	filename, err := h.saver.SaveVersionedArtifact(ctx, workspaceName, taskID, "validation.json", data)
	if err != nil {
		return fmt.Errorf("failed to save validation artifact: %w", err)
	}

	h.logger.Info().
		Str("task_id", taskID).
		Str("workspace", workspaceName).
		Str("artifact", filename).
		Bool("success", result.Success).
		Int64("duration_ms", result.DurationMs).
		Msg("saved validation result")

	if !result.Success {
		// Emit bell notification on failure
		if h.notifier != nil {
			h.notifier.Bell()
		}

		h.logger.Warn().
			Str("task_id", taskID).
			Str("failed_step", result.FailedStepName).
			Msg("validation failed")

		return fmt.Errorf("%w: %s", atlaserrors.ErrValidationFailed, result.FailedStepName)
	}

	return nil
}
