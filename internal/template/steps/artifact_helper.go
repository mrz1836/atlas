// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
)

// ArtifactHelper provides common artifact saving operations with consistent
// nil-checking, JSON marshaling, and path construction.
//
// This helper eliminates duplicated boilerplate code across executors by providing:
//   - Automatic nil-check for optional artifact savers
//   - Consistent JSON marshaling with pretty-printing
//   - Standardized error handling (warn but don't fail)
//   - Unified path construction patterns
//
// Example usage:
//
//	helper := NewArtifactHelper(saver, logger)
//	path := helper.SaveJSON(ctx, task, "commit_step", "commit-result.json", result)
type ArtifactHelper struct {
	saver  ArtifactSaver
	logger zerolog.Logger
}

// NewArtifactHelper creates a new artifact helper.
// Returns nil if saver is nil, allowing callers to safely call methods
// which will gracefully no-op.
func NewArtifactHelper(saver ArtifactSaver, logger zerolog.Logger) *ArtifactHelper {
	if saver == nil {
		return nil
	}
	return &ArtifactHelper{
		saver:  saver,
		logger: logger,
	}
}

// SaveJSON marshals data to JSON and saves it as an artifact.
// Returns the artifact path on success, or empty string if:
//   - helper is nil (graceful no-op for optional artifacts)
//   - JSON marshaling fails
//   - artifact saving fails
//
// The filename is constructed as: <stepName>/<filename>
// Example: SaveJSON(ctx, task, "commit_step", "commit-result.json", result)
// produces: "commit_step/commit-result.json"
func (h *ArtifactHelper) SaveJSON(ctx context.Context, task *domain.Task,
	stepName, filename string, data any,
) string {
	if h == nil || h.saver == nil {
		return ""
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		h.logger.Warn().Err(err).
			Str("filename", filename).
			Msg("failed to marshal artifact data to JSON")
		return ""
	}

	artifactPath := filepath.Join(stepName, filename)
	if err := h.saver.SaveArtifact(ctx, task.WorkspaceID, task.ID, artifactPath, jsonData); err != nil {
		h.logger.Warn().Err(err).
			Str("artifact_path", artifactPath).
			Msg("failed to save JSON artifact")
		return ""
	}

	h.logger.Debug().
		Str("artifact_path", artifactPath).
		Int("size_bytes", len(jsonData)).
		Msg("saved JSON artifact")

	return artifactPath
}

// SaveText saves text content as an artifact.
// Returns the artifact path on success, or empty string if:
//   - helper is nil (graceful no-op for optional artifacts)
//   - artifact saving fails
//
// The filename is constructed as: <stepName>/<filename>
// Example: SaveText(ctx, task, "git_pr", "pr-description.md", content)
// produces: "git_pr/pr-description.md"
func (h *ArtifactHelper) SaveText(ctx context.Context, task *domain.Task,
	stepName, filename, content string,
) string {
	if h == nil || h.saver == nil {
		return ""
	}

	artifactPath := filepath.Join(stepName, filename)
	if err := h.saver.SaveArtifact(ctx, task.WorkspaceID, task.ID, artifactPath, []byte(content)); err != nil {
		h.logger.Warn().Err(err).
			Str("artifact_path", artifactPath).
			Msg("failed to save text artifact")
		return ""
	}

	h.logger.Debug().
		Str("artifact_path", artifactPath).
		Int("size_bytes", len(content)).
		Msg("saved text artifact")

	return artifactPath
}

// SaveVersionedJSON saves JSON data with automatic version numbering.
// Returns the actual filename used (with version suffix) on success,
// or empty string on failure.
//
// Example: If "validation.json" exists, saves as "validation.1.json"
func (h *ArtifactHelper) SaveVersionedJSON(ctx context.Context, task *domain.Task,
	baseName string, data any,
) (string, error) {
	if h == nil || h.saver == nil {
		return "", nil
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		h.logger.Warn().Err(err).
			Str("base_name", baseName).
			Msg("failed to marshal versioned artifact data to JSON")
		return "", err
	}

	filename, err := h.saver.SaveVersionedArtifact(ctx, task.WorkspaceID, task.ID, baseName, jsonData)
	if err != nil {
		h.logger.Warn().Err(err).
			Str("base_name", baseName).
			Msg("failed to save versioned JSON artifact")
		return "", err
	}

	h.logger.Debug().
		Str("filename", filename).
		Str("base_name", baseName).
		Int("size_bytes", len(jsonData)).
		Msg("saved versioned JSON artifact")

	return filename, nil
}

// SaveVersionedText saves text content with automatic version numbering.
// Returns the actual filename used (with version suffix) on success,
// or empty string on failure.
func (h *ArtifactHelper) SaveVersionedText(ctx context.Context, task *domain.Task,
	baseName, content string,
) (string, error) {
	if h == nil || h.saver == nil {
		return "", nil
	}

	filename, err := h.saver.SaveVersionedArtifact(ctx, task.WorkspaceID, task.ID, baseName, []byte(content))
	if err != nil {
		h.logger.Warn().Err(err).
			Str("base_name", baseName).
			Msg("failed to save versioned text artifact")
		return "", err
	}

	h.logger.Debug().
		Str("filename", filename).
		Str("base_name", baseName).
		Int("size_bytes", len(content)).
		Msg("saved versioned text artifact")

	return filename, nil
}

// IsEnabled returns true if the helper has a configured artifact saver.
// Useful for conditional logic when artifact saving is optional.
func (h *ArtifactHelper) IsEnabled() bool {
	return h != nil && h.saver != nil
}
