package steps

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileScratchpad_ReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "scratchpad.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write data
	data := &ScratchpadData{
		TaskID:   "task-123",
		LoopName: "fix_loop",
		Iterations: []IterationSummary{
			{Number: 1, Summary: "Fixed lint errors"},
		},
	}
	err := s.Write(data)
	require.NoError(t, err)

	// Read it back
	read, err := s.Read()
	require.NoError(t, err)
	assert.Equal(t, "task-123", read.TaskID)
	assert.Equal(t, "fix_loop", read.LoopName)
	require.Len(t, read.Iterations, 1)
	assert.Equal(t, "Fixed lint errors", read.Iterations[0].Summary)
}

func TestFileScratchpad_ReadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Read should return empty data for non-existent file
	data, err := s.Read()
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.Empty(t, data.TaskID)
	assert.Empty(t, data.Iterations)
}

func TestFileScratchpad_AppendIteration(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "scratchpad.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Initialize with first iteration
	err := s.Write(&ScratchpadData{TaskID: "task-123"})
	require.NoError(t, err)

	// Append iterations
	err = s.AppendIteration(&IterationSummary{Number: 1, Summary: "First"})
	require.NoError(t, err)

	err = s.AppendIteration(&IterationSummary{Number: 2, Summary: "Second"})
	require.NoError(t, err)

	data, err := s.Read()
	require.NoError(t, err)
	require.Len(t, data.Iterations, 2)
	assert.Equal(t, "First", data.Iterations[0].Summary)
	assert.Equal(t, "Second", data.Iterations[1].Summary)
}

func TestFileScratchpad_Initialize(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "scratchpad.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	err := s.Initialize("task-456", "loop_step")
	require.NoError(t, err)

	// Verify file exists and has correct content
	data, err := s.Read()
	require.NoError(t, err)
	assert.Equal(t, "task-456", data.TaskID)
	assert.Equal(t, "loop_step", data.LoopName)
	assert.NotZero(t, data.StartedAt)
	assert.Empty(t, data.Iterations)
}

func TestFileScratchpad_Path(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)
	assert.Equal(t, path, s.Path())
}

func TestFileScratchpad_CreateParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "dir", "scratchpad.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write should create parent directories
	data := &ScratchpadData{TaskID: "task-789"}
	err := s.Write(data)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestIterationSummary_Fields(t *testing.T) {
	now := time.Now()
	summary := IterationSummary{
		Number:       3,
		CompletedAt:  now,
		FilesChanged: []string{"file1.go", "file2.go"},
		Summary:      "Fixed all issues",
		ExitSignal:   true,
		Success:      true,
		Error:        "",
	}

	assert.Equal(t, 3, summary.Number)
	assert.Equal(t, now, summary.CompletedAt)
	assert.Len(t, summary.FilesChanged, 2)
	assert.True(t, summary.ExitSignal)
	assert.True(t, summary.Success)
	assert.Empty(t, summary.Error)
}

// ====================
// Phase 4: Scratchpad Resilience Tests
// ====================

func TestFileScratchpad_CorruptedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupted.json")
	logger := zerolog.Nop()

	// Write invalid JSON
	err := os.WriteFile(path, []byte(`{"task_id": "test", "iterations": [`), 0o600)
	require.NoError(t, err)

	s := NewFileScratchpad(path, logger)

	// Read should return error on malformed JSON
	_, err = s.Read()
	require.Error(t, err)
}

func TestFileScratchpad_PartialWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "truncated.json")
	logger := zerolog.Nop()

	// Simulate truncated file (partial write)
	truncatedJSON := `{"task_id": "test-123", "loop_name": "fix_loop", "iterations": [{"number": 1, "summary": "first`
	err := os.WriteFile(path, []byte(truncatedJSON), 0o600)
	require.NoError(t, err)

	s := NewFileScratchpad(path, logger)

	// Read should fail on truncated JSON
	_, err = s.Read()
	require.Error(t, err)
}

func TestFileScratchpad_PermissionDenied(t *testing.T) {
	// Skip on systems without proper permission support
	if os.Getuid() == 0 {
		t.Skip("Test skipped when running as root")
	}

	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0o500) // #nosec G301 -- test directory needs read-only permissions
	require.NoError(t, err)
	defer func() {
		_ = os.Chmod(readOnlyDir, 0o750) // #nosec G302 -- test cleanup restores write permissions
	}()

	path := filepath.Join(readOnlyDir, "scratchpad.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write should fail due to permission
	data := &ScratchpadData{TaskID: "test-123"}
	err = s.Write(data)
	require.Error(t, err)
}

func TestFileScratchpad_ConcurrentRead(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "concurrent.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Initialize file
	data := &ScratchpadData{
		TaskID:   "task-concurrent",
		LoopName: "test_loop",
		Iterations: []IterationSummary{
			{Number: 1, Summary: "first"},
			{Number: 2, Summary: "second"},
		},
	}
	err := s.Write(data)
	require.NoError(t, err)

	// Concurrent reads should not race
	const numReaders = 10
	done := make(chan struct{}, numReaders)
	errors := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			readData, err := s.Read()
			if err != nil {
				errors <- err
				return
			}
			if readData.TaskID != "task-concurrent" {
				errors <- err
				return
			}
		}()
	}

	// Wait for all readers
	for i := 0; i < numReaders; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errors:
		t.Fatalf("Concurrent read failed: %v", err)
	default:
		// No errors
	}
}

func TestFileScratchpad_ConcurrentWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "concurrent_write.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Initialize file
	err := s.Write(&ScratchpadData{TaskID: "initial"})
	require.NoError(t, err)

	// Concurrent writes - this tests for race conditions
	// With -race flag, this will detect data races
	const numWriters = 10
	done := make(chan struct{}, numWriters)

	for i := 0; i < numWriters; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			data := &ScratchpadData{
				TaskID:   "task-123",
				LoopName: "test_loop",
				Iterations: []IterationSummary{
					{Number: idx, Summary: "iteration from goroutine"},
				},
			}
			_ = s.Write(data) // We don't check error - file may be locked
		}(i)
	}

	// Wait for all writers
	for i := 0; i < numWriters; i++ {
		<-done
	}

	// File should be readable after concurrent writes
	_, err = s.Read()
	assert.NoError(t, err)
}

func TestFileScratchpad_LargeData(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Create scratchpad with many iterations
	const numIterations = 1000
	iterations := make([]IterationSummary, numIterations)
	for i := 0; i < numIterations; i++ {
		iterations[i] = IterationSummary{
			Number:       i + 1,
			CompletedAt:  time.Now(),
			FilesChanged: []string{"file1.go", "file2.go", "file3.go"},
			Summary:      "This is a summary with some reasonable text describing what happened during this iteration. It includes details about the changes made and the outcome.",
			ExitSignal:   false,
			Success:      true,
		}
	}

	data := &ScratchpadData{
		TaskID:     "task-large",
		LoopName:   "large_loop",
		StartedAt:  time.Now().Add(-1 * time.Hour),
		Iterations: iterations,
		Metadata: map[string]any{
			"total_files_changed": 3000,
			"avg_duration_ms":     1500,
		},
	}

	// Write large data
	err := s.Write(data)
	require.NoError(t, err)

	// Read it back
	readData, err := s.Read()
	require.NoError(t, err)
	assert.Len(t, readData.Iterations, numIterations)
	assert.Equal(t, "task-large", readData.TaskID)
}

func TestFileScratchpad_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "special.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Data with special characters
	data := &ScratchpadData{
		TaskID:   "task-special",
		LoopName: "special_loop",
		Iterations: []IterationSummary{
			{
				Number:  1,
				Summary: "Unicode: 你好世界 🚀 émojis → arrows «quotes»", //nolint:gosmopolitan // testing Unicode support
				Success: true,
			},
			{
				Number:  2,
				Summary: "Newlines:\nline1\nline2\nline3\ttabbed",
				Success: true,
			},
			{
				Number:  3,
				Summary: `JSON chars: {"key": "value"} and "escaped quotes"`,
				Success: true,
			},
			{
				Number:  4,
				Summary: "Backslashes: C:\\path\\to\\file and \\n \\t literals",
				Success: true,
			},
			{
				Number:  5,
				Summary: "Control chars: \x00\x01\x02 (nulls)", // these may be filtered
				Success: true,
			},
		},
	}

	// Write data with special characters
	err := s.Write(data)
	require.NoError(t, err)

	// Read it back
	readData, err := s.Read()
	require.NoError(t, err)
	assert.Len(t, readData.Iterations, 5)

	// Verify special characters preserved
	assert.Contains(t, readData.Iterations[0].Summary, "你好世界") //nolint:gosmopolitan // testing Unicode support
	assert.Contains(t, readData.Iterations[0].Summary, "🚀")
	assert.Contains(t, readData.Iterations[1].Summary, "line1\nline2")
	assert.Contains(t, readData.Iterations[2].Summary, `"key"`)
	assert.Contains(t, readData.Iterations[3].Summary, `C:\path`)
}

func TestFileScratchpad_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.json")
	logger := zerolog.Nop()

	// Create empty file
	err := os.WriteFile(path, []byte{}, 0o600)
	require.NoError(t, err)

	s := NewFileScratchpad(path, logger)

	// Read empty file should return error (invalid JSON)
	_, err = s.Read()
	require.Error(t, err)
}

func TestFileScratchpad_AppendToCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupted_append.json")
	logger := zerolog.Nop()

	// Write corrupted JSON
	err := os.WriteFile(path, []byte(`not valid json at all`), 0o600)
	require.NoError(t, err)

	s := NewFileScratchpad(path, logger)

	// Append should fail because read fails
	summary := &IterationSummary{Number: 1, Summary: "test"}
	err = s.AppendIteration(summary)
	require.Error(t, err)
}

func TestFileScratchpad_DeepNestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "deep", "nested", "path", "to", "scratchpad.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write should create all parent directories
	data := &ScratchpadData{TaskID: "task-deep"}
	err := s.Write(data)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestFileScratchpad_AppendManyIterations(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "append_many.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Initialize
	err := s.Initialize("task-append", "append_loop")
	require.NoError(t, err)

	// Append many iterations
	const numIterations = 100
	for i := 1; i <= numIterations; i++ {
		summary := &IterationSummary{
			Number:       i,
			Summary:      "Iteration summary",
			CompletedAt:  time.Now(),
			FilesChanged: []string{"file.go"},
			Success:      true,
		}
		appendErr := s.AppendIteration(summary)
		require.NoError(t, appendErr)
	}

	// Verify all iterations present
	data, err := s.Read()
	require.NoError(t, err)
	assert.Len(t, data.Iterations, numIterations)
}

func TestFileScratchpad_OverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "overwrite.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write initial data with many iterations
	initialData := &ScratchpadData{
		TaskID: "initial",
		Iterations: []IterationSummary{
			{Number: 1}, {Number: 2}, {Number: 3},
		},
	}
	err := s.Write(initialData)
	require.NoError(t, err)

	// Overwrite with smaller data
	newData := &ScratchpadData{
		TaskID:     "new",
		Iterations: []IterationSummary{},
	}
	err = s.Write(newData)
	require.NoError(t, err)

	// Read should return new data
	readData, err := s.Read()
	require.NoError(t, err)
	assert.Equal(t, "new", readData.TaskID)
	assert.Empty(t, readData.Iterations)
}

func TestFileScratchpad_MetadataPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "metadata.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write data with complex metadata
	data := &ScratchpadData{
		TaskID:   "task-meta",
		LoopName: "meta_loop",
		Metadata: map[string]any{
			"string_val": "hello",
			"int_val":    float64(42), // JSON numbers are float64
			"float_val":  3.14,
			"bool_val":   true,
			"array_val":  []any{"a", "b", "c"},
			"nested_val": map[string]any{"key": "value"},
			"null_val":   nil,
		},
	}

	err := s.Write(data)
	require.NoError(t, err)

	readData, err := s.Read()
	require.NoError(t, err)

	assert.Equal(t, "hello", readData.Metadata["string_val"])
	assert.InDelta(t, float64(42), readData.Metadata["int_val"], 0.001)
	assert.InDelta(t, 3.14, readData.Metadata["float_val"], 0.001)
	assert.Equal(t, true, readData.Metadata["bool_val"])
	assert.Nil(t, readData.Metadata["null_val"])
}

func TestFileScratchpad_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "permissions.json")
	logger := zerolog.Nop()

	s := NewFileScratchpad(path, logger)

	// Write data
	data := &ScratchpadData{TaskID: "task-perms"}
	err := s.Write(data)
	require.NoError(t, err)

	// Check file permissions (should be 0600)
	info, err := os.Stat(path)
	require.NoError(t, err)
	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o600), mode)
}
