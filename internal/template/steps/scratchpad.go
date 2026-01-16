// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// ScratchpadWriter is the interface for cross-iteration memory.
// This interface enables mocking in tests.
type ScratchpadWriter interface {
	// Read returns the current scratchpad data, or empty data if none exists.
	Read() (*ScratchpadData, error)

	// Write saves the scratchpad data, overwriting any existing content.
	Write(data *ScratchpadData) error

	// AppendIteration adds an iteration summary to the scratchpad.
	AppendIteration(result *IterationSummary) error
}

// ScratchpadData is the JSON structure for scratchpad files.
// This provides cross-iteration memory for loop steps.
type ScratchpadData struct {
	// TaskID identifies the task that owns this scratchpad.
	TaskID string `json:"task_id"`

	// LoopName identifies the loop step using this scratchpad.
	LoopName string `json:"loop_name"`

	// StartedAt is when the loop started.
	StartedAt time.Time `json:"started_at"`

	// Iterations contains summaries of completed iterations.
	Iterations []IterationSummary `json:"iterations"`

	// Metadata stores arbitrary key-value data for AI use.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// IterationSummary captures key information from a completed iteration.
type IterationSummary struct {
	// Number is the 1-indexed iteration number.
	Number int `json:"number"`

	// CompletedAt is when the iteration finished.
	CompletedAt time.Time `json:"completed_at"`

	// FilesChanged lists files modified during this iteration.
	FilesChanged []string `json:"files_changed"`

	// Summary is a human/AI-readable description of what happened.
	Summary string `json:"summary"`

	// ExitSignal indicates if the AI signaled completion.
	ExitSignal bool `json:"exit_signal"`

	// Success indicates if all inner steps succeeded.
	Success bool `json:"success"`

	// Error contains any error message from the iteration.
	Error string `json:"error,omitempty"`
}

// FileScratchpad implements ScratchpadWriter using the filesystem.
type FileScratchpad struct {
	path   string
	logger zerolog.Logger
}

// NewFileScratchpad creates a new file-based scratchpad.
func NewFileScratchpad(path string, logger zerolog.Logger) *FileScratchpad {
	return &FileScratchpad{
		path:   path,
		logger: logger,
	}
}

// Path returns the file path for this scratchpad.
func (s *FileScratchpad) Path() string {
	return s.path
}

// Read returns the current scratchpad data from the file.
// Returns empty data if the file doesn't exist.
func (s *FileScratchpad) Read() (*ScratchpadData, error) {
	content, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.logger.Debug().Str("path", s.path).Msg("scratchpad file does not exist, returning empty data")
		return &ScratchpadData{}, nil
	}
	if err != nil {
		s.logger.Error().Err(err).Str("path", s.path).Msg("failed to read scratchpad file")
		return nil, err
	}

	var data ScratchpadData
	if err := json.Unmarshal(content, &data); err != nil {
		s.logger.Error().Err(err).Str("path", s.path).Msg("failed to parse scratchpad JSON")
		return nil, err
	}

	s.logger.Debug().
		Str("path", s.path).
		Int("iterations", len(data.Iterations)).
		Msg("read scratchpad data")
	return &data, nil
}

// Write saves the scratchpad data to the file.
func (s *FileScratchpad) Write(data *ScratchpadData) error {
	// Ensure parent directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.logger.Error().Err(err).Str("dir", dir).Msg("failed to create scratchpad directory")
		return err
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to marshal scratchpad data")
		return err
	}

	if err := os.WriteFile(s.path, content, 0o600); err != nil {
		s.logger.Error().Err(err).Str("path", s.path).Msg("failed to write scratchpad file")
		return err
	}

	s.logger.Debug().
		Str("path", s.path).
		Int("iterations", len(data.Iterations)).
		Msg("wrote scratchpad data")
	return nil
}

// AppendIteration adds an iteration summary to the scratchpad.
func (s *FileScratchpad) AppendIteration(result *IterationSummary) error {
	data, err := s.Read()
	if err != nil {
		return err
	}

	data.Iterations = append(data.Iterations, *result)
	return s.Write(data)
}

// Initialize sets up the scratchpad with initial metadata.
func (s *FileScratchpad) Initialize(taskID, loopName string) error {
	data := &ScratchpadData{
		TaskID:     taskID,
		LoopName:   loopName,
		StartedAt:  time.Now(),
		Iterations: []IterationSummary{},
		Metadata:   make(map[string]any),
	}
	return s.Write(data)
}
