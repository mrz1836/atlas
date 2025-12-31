// Package git provides Git operations for ATLAS.
// This file implements PR description artifact saving.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

const (
	// PRDescriptionFilename is the standard filename for PR descriptions.
	PRDescriptionFilename = "pr-description.md"
)

// PRArtifactSaver saves PR descriptions as artifacts.
type PRArtifactSaver interface {
	// Save saves the PR description as an artifact and returns the file path.
	Save(ctx context.Context, desc *PRDescription, opts PRDescOptions) (string, error)
}

// ArtifactStore is the interface for task artifact storage (from internal/task).
// This is defined here to avoid circular imports.
type ArtifactStore interface {
	SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error
}

// TaskStoreArtifactSaver saves PR descriptions using the task store.
type TaskStoreArtifactSaver struct {
	store  ArtifactStore
	logger zerolog.Logger
}

// TaskStoreArtifactOption configures a TaskStoreArtifactSaver.
type TaskStoreArtifactOption func(*TaskStoreArtifactSaver)

// NewTaskStoreArtifactSaver creates a saver that uses the task store.
func NewTaskStoreArtifactSaver(store ArtifactStore, opts ...TaskStoreArtifactOption) *TaskStoreArtifactSaver {
	s := &TaskStoreArtifactSaver{
		store:  store,
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithTaskStoreLogger sets the logger.
func WithTaskStoreLogger(logger zerolog.Logger) TaskStoreArtifactOption {
	return func(s *TaskStoreArtifactSaver) {
		s.logger = logger
	}
}

// Save saves the PR description as an artifact using the task store.
func (s *TaskStoreArtifactSaver) Save(ctx context.Context, desc *PRDescription, opts PRDescOptions) (string, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if opts.WorkspaceName == "" || opts.TaskID == "" {
		return "", fmt.Errorf("workspace name and task ID required for artifact saving: %w", atlaserrors.ErrEmptyValue)
	}

	content := formatPRArtifact(desc, opts)

	s.logger.Info().
		Str("workspace", opts.WorkspaceName).
		Str("task_id", opts.TaskID).
		Str("filename", PRDescriptionFilename).
		Msg("saving PR description artifact")

	if err := s.store.SaveArtifact(ctx, opts.WorkspaceName, opts.TaskID, PRDescriptionFilename, []byte(content)); err != nil {
		return "", fmt.Errorf("failed to save PR description artifact: %w", err)
	}

	return PRDescriptionFilename, nil
}

// FileArtifactSaver saves PR descriptions directly to files.
type FileArtifactSaver struct {
	baseDir string
	logger  zerolog.Logger
}

// FileArtifactOption configures a FileArtifactSaver.
type FileArtifactOption func(*FileArtifactSaver)

// NewFileArtifactSaver creates a saver that writes directly to files.
func NewFileArtifactSaver(baseDir string, opts ...FileArtifactOption) *FileArtifactSaver {
	s := &FileArtifactSaver{
		baseDir: baseDir,
		logger:  zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithFileArtifactLogger sets the logger.
func WithFileArtifactLogger(logger zerolog.Logger) FileArtifactOption {
	return func(s *FileArtifactSaver) {
		s.logger = logger
	}
}

// Save saves the PR description directly to a file.
func (s *FileArtifactSaver) Save(ctx context.Context, desc *PRDescription, opts PRDescOptions) (string, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Determine output path
	outputPath := filepath.Join(s.baseDir, PRDescriptionFilename)

	content := formatPRArtifact(desc, opts)

	s.logger.Info().
		Str("path", outputPath).
		Msg("saving PR description to file")

	// Ensure directory exists (0700 for security)
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file (0600 for security)
	if err := os.WriteFile(outputPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write PR description file: %w", err)
	}

	return outputPath, nil
}

// formatPRArtifact formats the PR description as a markdown file with metadata header.
func formatPRArtifact(desc *PRDescription, opts PRDescOptions) string {
	var content string

	// Add metadata header
	content += "---\n"
	content += fmt.Sprintf("title: %q\n", desc.Title)
	content += fmt.Sprintf("type: %s\n", desc.ConventionalType)
	if desc.Scope != "" {
		content += fmt.Sprintf("scope: %s\n", desc.Scope)
	}
	if opts.BaseBranch != "" {
		content += fmt.Sprintf("base: %s\n", opts.BaseBranch)
	}
	if opts.HeadBranch != "" {
		content += fmt.Sprintf("head: %s\n", opts.HeadBranch)
	}
	if opts.TaskID != "" {
		content += fmt.Sprintf("task_id: %s\n", opts.TaskID)
	}
	if opts.WorkspaceName != "" {
		content += fmt.Sprintf("workspace: %s\n", opts.WorkspaceName)
	}
	content += fmt.Sprintf("generated: %s\n", time.Now().UTC().Format(time.RFC3339))
	content += "---\n\n"

	// Add title as H1
	content += fmt.Sprintf("# %s\n\n", desc.Title)

	// Add body
	content += desc.Body

	return content
}

// Compile-time interface checks.
var (
	_ PRArtifactSaver = (*TaskStoreArtifactSaver)(nil)
	_ PRArtifactSaver = (*FileArtifactSaver)(nil)
)
