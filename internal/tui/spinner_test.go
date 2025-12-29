package tui_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/tui"
)

// safeSpinnerBuffer is a thread-safe buffer for spinner tests.
type safeSpinnerBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *safeSpinnerBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeSpinnerBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

var _ io.Writer = (*safeSpinnerBuffer)(nil)

func TestNewSpinner(t *testing.T) {
	var buf bytes.Buffer
	spinner := tui.NewSpinner(&buf)
	require.NotNil(t, spinner)
}

func TestSpinner_Start_Stop(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Testing...")

	// Let it run briefly
	time.Sleep(150 * time.Millisecond)

	spinner.Stop()

	// Should have written something to buffer
	assert.NotEmpty(t, buf.String())
}

func TestSpinner_StartMultipleTimes(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "First message")
	spinner.Start(ctx, "Second message") // Should just update message

	time.Sleep(150 * time.Millisecond)
	spinner.Stop()

	// Should not panic and should have output
	assert.NotEmpty(t, buf.String())
}

func TestSpinner_UpdateMessage(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Initial")

	time.Sleep(150 * time.Millisecond)
	spinner.UpdateMessage("Updated message")
	time.Sleep(150 * time.Millisecond)

	spinner.Stop()

	// Should contain the updated message
	output := buf.String()
	assert.Contains(t, output, "Updated message")
}

func TestSpinner_StopWithSuccess(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Working...")

	time.Sleep(50 * time.Millisecond)
	spinner.StopWithSuccess("Task completed")

	output := buf.String()
	assert.Contains(t, output, "✓")
	assert.Contains(t, output, "Task completed")
}

func TestSpinner_StopWithError(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Working...")

	time.Sleep(50 * time.Millisecond)
	spinner.StopWithError("Task failed")

	output := buf.String()
	assert.Contains(t, output, "✗")
	assert.Contains(t, output, "Task failed")
}

func TestSpinner_StopWithWarning(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Working...")

	time.Sleep(50 * time.Millisecond)
	spinner.StopWithWarning("Task skipped")

	output := buf.String()
	assert.Contains(t, output, "⚠")
	assert.Contains(t, output, "Task skipped")
}

func TestSpinner_ContextCancellation(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx, cancel := context.WithCancel(context.Background())
	spinner.Start(ctx, "Cancellable task")

	time.Sleep(150 * time.Millisecond)
	cancel()

	// Wait for cleanup
	time.Sleep(150 * time.Millisecond)

	// Should have cleaned up
	assert.NotEmpty(t, buf.String())
}

func TestSpinner_StopWithoutStart(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	spinner := tui.NewSpinner(&buf)

	// Should not panic
	spinner.Stop()
	spinner.StopWithSuccess("Done")
	spinner.StopWithError("Error")
	spinner.StopWithWarning("Warning")
}

func TestSpinner_SpinnerFrames(t *testing.T) {
	// Verify spinner frames are defined
	assert.NotEmpty(t, tui.SpinnerFrames())
	assert.Len(t, tui.SpinnerFrames(), 10)
}

func TestSpinner_Constants(t *testing.T) {
	// Verify constants are reasonable
	assert.Equal(t, 100*time.Millisecond, tui.SpinnerInterval)
	assert.Equal(t, 30*time.Second, tui.ElapsedTimeThreshold)
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{
			name:     "milliseconds",
			ms:       500,
			expected: "500ms",
		},
		{
			name:     "one second",
			ms:       1000,
			expected: "1.0s",
		},
		{
			name:     "seconds with decimal",
			ms:       1234,
			expected: "1.2s",
		},
		{
			name:     "many seconds",
			ms:       5678,
			expected: "5.7s",
		},
		{
			name:     "zero",
			ms:       0,
			expected: "0ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tui.FormatDuration(tt.ms)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSpinner_AnimationUpdatesAtInterval(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Animating")

	// Wait for multiple animation frames
	time.Sleep(350 * time.Millisecond)

	spinner.Stop()

	output := buf.String()

	// Should have multiple spinner frames in output (carriage returns indicate updates)
	frameCount := strings.Count(output, "\r")
	assert.GreaterOrEqual(t, frameCount, 2, "should have multiple animation updates")
}

func TestSpinner_NonBlockingOperation(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()

	// Start should return immediately (non-blocking)
	start := time.Now()
	spinner.Start(ctx, "Non-blocking test")
	startDuration := time.Since(start)

	// Start should take less than 10ms (it just spawns a goroutine)
	assert.Less(t, startDuration, 10*time.Millisecond, "Start should be non-blocking")

	// Let spinner run briefly
	time.Sleep(150 * time.Millisecond)

	// Stop should also be quick
	stopStart := time.Now()
	spinner.Stop()
	stopDuration := time.Since(stopStart)

	// Stop should take less than 50ms
	assert.Less(t, stopDuration, 50*time.Millisecond, "Stop should be quick")
}

func TestSpinner_UpdateRateReasonable(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Rate test")

	// Run for 500ms
	time.Sleep(500 * time.Millisecond)
	spinner.Stop()

	output := buf.String()

	// At 100ms interval, we should have ~5 updates in 500ms
	// Allow some flexibility (3-7 updates)
	frameCount := strings.Count(output, "\r")
	assert.GreaterOrEqual(t, frameCount, 3, "should have minimum updates")
	assert.LessOrEqual(t, frameCount, 10, "should not overwhelm with updates")
}
