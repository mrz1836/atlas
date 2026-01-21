package tui_test

import (
	"bytes"
	"context"
	"fmt"
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
	spinner := tui.NewTerminalSpinner(&buf)
	require.NotNil(t, spinner)
}

func TestSpinner_Start_Stop(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Initial")

	// Wait longer than throttle interval (200ms) to ensure update is not throttled
	time.Sleep(250 * time.Millisecond)
	spinner.UpdateMessage("Updated message")
	time.Sleep(150 * time.Millisecond)

	spinner.Stop()

	// Should contain the updated message
	output := buf.String()
	assert.Contains(t, output, "Updated message")
}

func TestSpinner_StopWithSuccess(t *testing.T) {
	buf := &safeSpinnerBuffer{}
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(&buf)

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
	assert.Equal(t, 200*time.Millisecond, tui.SpinnerMessageThrottle)
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
	if testing.Short() {
		t.Skip("skipping slow animation test in short mode")
	}

	buf := &safeSpinnerBuffer{}
	spinner := tui.NewTerminalSpinner(buf)

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
	spinner := tui.NewTerminalSpinner(buf)

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
	if testing.Short() {
		t.Skip("skipping slow rate test in short mode")
	}

	buf := &safeSpinnerBuffer{}
	spinner := tui.NewTerminalSpinner(buf)

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

func TestSpinner_RaceCondition_StartStopConcurrent(t *testing.T) {
	// This test verifies that concurrent Start/Stop calls don't cause races
	// Run with -race flag to detect race conditions
	t.Parallel()

	for i := 0; i < 10; i++ {
		buf := &safeSpinnerBuffer{}
		spinner := tui.NewTerminalSpinner(buf)

		ctx, cancel := context.WithCancel(context.Background())

		// Start spinner
		spinner.Start(ctx, "Test message")

		// Concurrently call Stop and cancel context
		done := make(chan struct{})
		go func() {
			time.Sleep(10 * time.Millisecond)
			spinner.Stop()
			close(done)
		}()

		// Also cancel context to trigger the other exit path
		go func() {
			time.Sleep(5 * time.Millisecond)
			cancel()
		}()

		// Wait for stop to complete
		<-done

		// Try starting again (should work after proper cleanup)
		spinner.Start(ctx, "New message")
		time.Sleep(50 * time.Millisecond)
		spinner.Stop()
	}
}

func TestSpinner_RaceCondition_MultipleStops(t *testing.T) {
	// Test that multiple Stop calls don't panic
	t.Parallel()

	buf := &safeSpinnerBuffer{}
	spinner := tui.NewTerminalSpinner(buf)

	ctx := context.Background()
	spinner.Start(ctx, "Test message")

	time.Sleep(50 * time.Millisecond)

	// Multiple concurrent stops should be safe
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spinner.Stop()
		}()
	}

	wg.Wait()

	// Should not panic and buffer should have content
	assert.NotEmpty(t, buf.String())
}

func TestTerminalSpinner_UpdateMessageThrottling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throttling test in short mode")
	}

	buf := &safeSpinnerBuffer{}
	s := tui.NewTerminalSpinner(buf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx, "Initial message")
	time.Sleep(50 * time.Millisecond) // Let animation start

	// Attempt rapid updates (should be throttled)
	// We'll try to update every 50ms, but with 200ms throttle,
	// we should only see updates roughly every 200ms
	for i := 0; i < 10; i++ {
		msg := fmt.Sprintf("Message %d", i)
		s.UpdateMessage(msg)

		time.Sleep(50 * time.Millisecond) // 50ms between attempts
	}

	s.Stop()

	// With 50ms attempts and 200ms throttle, in 500ms total time,
	// we should see approximately 2-3 actual updates (plus the initial one)
	// The exact number can vary due to timing, but it should be significantly
	// less than the 10 attempted updates

	// To verify throttling is working, we can check that not all 10 messages
	// appear in the output. Due to throttling, many should be skipped.
	output := buf.String()

	// Count how many of our test messages appear in the output
	foundCount := 0
	for i := 0; i < 10; i++ {
		if strings.Contains(output, fmt.Sprintf("Message %d", i)) {
			foundCount++
		}
	}

	// With throttling, we should see fewer messages than attempted
	// Allow some flexibility, but it should be significantly throttled
	require.Less(t, foundCount, 10, "updates should be throttled (not all messages should appear)")
	require.Positive(t, foundCount, "at least some messages should appear")

	// Additional verification: the actual number of frame updates should be
	// consistent with the throttling interval
	t.Logf("Found %d out of 10 messages in output (throttling working)", foundCount)
}
