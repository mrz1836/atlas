package ai

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStreamingExecutor_Execute_CapturesStdout(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityHigh,
	})

	// Execute a simple echo command
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "hello world")

	stdout, stderr, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify stdout was captured
	stdoutStr := strings.TrimSpace(string(stdout))
	if stdoutStr != "hello world" {
		t.Errorf("stdout = %q, want %q", stdoutStr, "hello world")
	}

	// stderr should be empty for echo
	if len(stderr) != 0 {
		t.Errorf("stderr = %q, want empty", string(stderr))
	}
}

func TestStreamingExecutor_Execute_CapturesStderr(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityHigh,
	})

	// Execute a command that writes to stderr
	ctx := context.Background()
	// sh -c allows us to write to stderr
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'reading test.go' >&2")

	stdout, stderr, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// stdout should be empty
	if len(stdout) != 0 {
		t.Errorf("stdout = %q, want empty", string(stdout))
	}

	// stderr should be captured
	stderrStr := strings.TrimSpace(string(stderr))
	if stderrStr != "reading test.go" {
		t.Errorf("stderr = %q, want %q", stderrStr, "reading test.go")
	}

	// Should have received an activity event
	mu.Lock()
	defer mu.Unlock()
	if len(receivedEvents) == 0 {
		t.Error("No activity events received")
	}
}

func TestStreamingExecutor_Execute_ParsesActivityEvents(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityHigh,
	})

	// Execute a command that outputs recognizable patterns
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo "Reading file: main.go" >&2
		echo "Writing to: output.txt" >&2
		echo "Thinking..." >&2
	`)

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	events := make([]ActivityEvent, len(receivedEvents))
	copy(events, receivedEvents)
	mu.Unlock()

	// Should have parsed at least the recognizable patterns
	if len(events) < 2 {
		t.Errorf("Received %d events, want >= 2", len(events))
	}

	// Check that we got reading and writing events
	var hasReading, hasWriting bool
	for _, e := range events {
		if e.Type == ActivityReading {
			hasReading = true
		}
		if e.Type == ActivityWriting {
			hasWriting = true
		}
	}

	if !hasReading {
		t.Error("Did not receive Reading event")
	}
	if !hasWriting {
		t.Error("Did not receive Writing event")
	}
}

func TestStreamingExecutor_Execute_RespectsVerbosity(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	// Use low verbosity - should filter out most events
	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityLow, // Only phase changes
	})

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo "Reading file: main.go" >&2
		echo "Planning implementation" >&2
	`)

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Low verbosity should filter out "Reading" but allow "Planning"
	hasReading := false
	hasPlanning := false
	for _, e := range receivedEvents {
		if e.Type == ActivityReading {
			hasReading = true
		}
		if e.Type == ActivityPlanning {
			hasPlanning = true
		}
	}

	if hasReading {
		t.Error("Low verbosity should filter out Reading events")
	}
	if !hasPlanning {
		t.Error("Low verbosity should allow Planning events")
	}
}

func TestStreamingExecutor_Execute_ContextCancellation(t *testing.T) {
	t.Parallel()

	executor := NewStreamingExecutor(ActivityOptions{
		Callback:  func(_ ActivityEvent) {},
		Verbosity: VerbosityHigh,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running command
	cmd := exec.CommandContext(ctx, "sleep", "10")

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, err := executor.Execute(ctx, cmd)
	elapsed := time.Since(start)

	// Should return quickly due to cancellation
	if elapsed > 2*time.Second {
		t.Errorf("Execute took %v, expected < 2s due to cancellation", elapsed)
	}

	// Should have an error due to cancellation
	if err == nil {
		t.Error("Execute should return error when context is canceled")
	}
}

func TestStreamingExecutor_Execute_NoCallback(t *testing.T) {
	t.Parallel()

	// Test with nil callback - should not panic
	executor := NewStreamingExecutor(ActivityOptions{
		Callback:  nil,
		Verbosity: VerbosityHigh,
	})

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "test")

	stdout, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(string(stdout), "test") {
		t.Error("stdout should contain 'test'")
	}
}

func TestStreamingExecutor_SyntheticProgress(t *testing.T) {
	// Skip in short mode as this test takes time
	if testing.Short() {
		t.Skip("Skipping synthetic progress test in short mode")
	}

	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityHigh,
	})

	// Run a command that produces no stderr for 6+ seconds
	// This should trigger synthetic progress
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sleep", "6")

	start := time.Now()
	_, _, err := executor.Execute(ctx, cmd)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should have taken about 6 seconds
	if elapsed < 5*time.Second {
		t.Errorf("Execute took %v, expected ~6s", elapsed)
	}

	mu.Lock()
	events := receivedEvents
	mu.Unlock()

	// Should have received synthetic progress events
	// After 5 seconds of no activity, synthetic events should start
	if len(events) == 0 {
		t.Error("Expected synthetic progress events, got none")
	}
}

func TestStreamingExecutor_CommandError(t *testing.T) {
	t.Parallel()

	executor := NewStreamingExecutor(ActivityOptions{
		Callback:  func(_ ActivityEvent) {},
		Verbosity: VerbosityHigh,
	})

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")

	_, _, err := executor.Execute(ctx, cmd)
	if err == nil {
		t.Error("Execute should return error for failed command")
	}
}

func TestStreamingExecutor_ImplementsCommandExecutor(t *testing.T) {
	t.Parallel()

	// Verify interface implementation at compile time
	var _ CommandExecutor = (*StreamingExecutor)(nil)

	// Also verify we can use it where CommandExecutor is expected
	var executor CommandExecutor = NewStreamingExecutor(ActivityOptions{
		Callback:  func(_ ActivityEvent) {},
		Verbosity: VerbosityMedium,
	})

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "interface test")

	stdout, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(string(stdout), "interface test") {
		t.Error("stdout should contain 'interface test'")
	}
}
