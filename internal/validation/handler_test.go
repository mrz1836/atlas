package validation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// MockArtifactSaver implements ArtifactSaver for testing.
type MockArtifactSaver struct {
	SaveVersionedArtifactFn func(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
	Calls                   []SaveVersionedArtifactCall
}

type SaveVersionedArtifactCall struct {
	WorkspaceName string
	TaskID        string
	BaseName      string
	Data          []byte
}

func (m *MockArtifactSaver) SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error) {
	m.Calls = append(m.Calls, SaveVersionedArtifactCall{
		WorkspaceName: workspaceName,
		TaskID:        taskID,
		BaseName:      baseName,
		Data:          data,
	})
	if m.SaveVersionedArtifactFn != nil {
		return m.SaveVersionedArtifactFn(ctx, workspaceName, taskID, baseName, data)
	}
	return "validation.1.json", nil
}

// MockNotifier implements Notifier for testing.
type MockNotifier struct {
	BellCalled bool
}

func (m *MockNotifier) Bell() {
	m.BellCalled = true
}

func TestNewResultHandler(t *testing.T) {
	t.Parallel()

	saver := &MockArtifactSaver{}
	notifier := &MockNotifier{}
	logger := zerolog.Nop()

	handler := NewResultHandler(saver, notifier, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, saver, handler.saver)
	assert.Equal(t, notifier, handler.notifier)
}

func TestResultHandler_HandleResult_FailureSavesArtifactAndNotifies(t *testing.T) {
	t.Parallel()

	mockSaver := &MockArtifactSaver{
		SaveVersionedArtifactFn: func(_ context.Context, ws, task, base string, _ []byte) (string, error) {
			assert.Equal(t, "test-ws", ws)
			assert.Equal(t, "task-abc", task)
			assert.Equal(t, "validation.json", base)
			return "validation.1.json", nil
		},
	}
	mockNotifier := &MockNotifier{}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, mockNotifier, logger)

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		LintResults:    []Result{{Command: "magex lint", Success: false, ExitCode: 1}},
	}

	err := handler.HandleResult(context.Background(), "test-ws", "task-abc", result)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	assert.Contains(t, err.Error(), "lint")
	assert.True(t, mockNotifier.BellCalled)
	assert.Len(t, mockSaver.Calls, 1)
}

func TestResultHandler_HandleResult_SuccessAutoProceeds(t *testing.T) {
	t.Parallel()

	mockSaver := &MockArtifactSaver{
		SaveVersionedArtifactFn: func(_ context.Context, _, _, _ string, _ []byte) (string, error) {
			return "validation.1.json", nil
		},
	}
	mockNotifier := &MockNotifier{}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, mockNotifier, logger)

	result := &PipelineResult{Success: true}

	err := handler.HandleResult(context.Background(), "test-ws", "task-abc", result)

	require.NoError(t, err)
	assert.False(t, mockNotifier.BellCalled)
	assert.Len(t, mockSaver.Calls, 1)
}

func TestResultHandler_HandleResult_NilNotifier(t *testing.T) {
	t.Parallel()

	mockSaver := &MockArtifactSaver{}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, nil, logger)

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "test",
	}

	err := handler.HandleResult(context.Background(), "ws", "task", result)

	// Should not panic with nil notifier
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
}

func TestResultHandler_HandleResult_SaveError(t *testing.T) {
	t.Parallel()

	mockSaver := &MockArtifactSaver{
		SaveVersionedArtifactFn: func(_ context.Context, _, _, _ string, _ []byte) (string, error) {
			return "", atlaserrors.ErrArtifactNotFound // Use sentinel error
		},
	}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, nil, logger)

	result := &PipelineResult{Success: true}

	err := handler.HandleResult(context.Background(), "ws", "task", result)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save validation artifact")
}

func TestResultHandler_HandleResult_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockSaver := &MockArtifactSaver{
		SaveVersionedArtifactFn: func(ctx context.Context, _, _, _ string, _ []byte) (string, error) {
			return "", ctx.Err()
		},
	}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, nil, logger)

	result := &PipelineResult{Success: true}

	err := handler.HandleResult(ctx, "ws", "task", result)

	assert.Error(t, err)
}

func TestResultHandler_HandleResult_PreservesHistory(t *testing.T) {
	t.Parallel()

	var callCount int
	mockSaver := &MockArtifactSaver{
		SaveVersionedArtifactFn: func(_ context.Context, _, _, baseName string, _ []byte) (string, error) {
			callCount++
			assert.Equal(t, "validation.json", baseName)
			// Simulate versioned save
			return "validation." + string(rune('0'+callCount)) + ".json", nil
		},
	}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, nil, logger)

	// First result
	result1 := &PipelineResult{Success: true}
	err := handler.HandleResult(context.Background(), "ws", "task", result1)
	require.NoError(t, err)

	// Second result
	result2 := &PipelineResult{Success: true}
	err = handler.HandleResult(context.Background(), "ws", "task", result2)
	require.NoError(t, err)

	assert.Equal(t, 2, callCount)
}

func TestResultHandler_HandleResult_DataMarshaledCorrectly(t *testing.T) {
	t.Parallel()

	var savedData []byte
	mockSaver := &MockArtifactSaver{
		SaveVersionedArtifactFn: func(_ context.Context, _, _, _ string, data []byte) (string, error) {
			savedData = data
			return "validation.1.json", nil
		},
	}
	logger := zerolog.Nop()

	handler := NewResultHandler(mockSaver, nil, logger)

	result := &PipelineResult{
		Success:    true,
		DurationMs: 1234,
		LintResults: []Result{
			{Command: "golangci-lint run", Success: true, ExitCode: 0},
		},
	}

	err := handler.HandleResult(context.Background(), "ws", "task", result)
	require.NoError(t, err)

	// Verify JSON is valid and has expected structure
	var parsed PipelineResult
	err = json.Unmarshal(savedData, &parsed)
	require.NoError(t, err)
	assert.True(t, parsed.Success)
	assert.Equal(t, int64(1234), parsed.DurationMs)
	assert.Len(t, parsed.LintResults, 1)
}
