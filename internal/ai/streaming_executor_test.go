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

func TestStreamingExecutor_ParsesStreamJSON(t *testing.T) {
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

	// Execute a command that outputs realistic Claude stream-json format to stdout
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo '{"type":"system","subtype":"init","session_id":"test123","tools":["Read","Edit"]}'
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"main.go"}}]},"session_id":"test123"}'
		echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file content"}]}}'
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_2","name":"Edit","input":{"file_path":"config.go"}}]},"session_id":"test123"}'
		echo '{"type":"result","subtype":"success","is_error":false,"result":"Done","session_id":"test123","duration_ms":1000,"num_turns":2,"total_cost_usd":0.05}'
	`)

	stdout, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify stdout was captured
	if !strings.Contains(string(stdout), "assistant") {
		t.Error("stdout should contain stream-json events")
	}

	// Wait a bit for async processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	events := make([]ActivityEvent, len(receivedEvents))
	copy(events, receivedEvents)
	mu.Unlock()

	// Should have parsed the tool events
	if len(events) < 2 {
		t.Errorf("Received %d events, want >= 2", len(events))
	}

	// Check that we got reading and writing events from stream-json
	var hasReading, hasWriting bool
	for _, e := range events {
		if e.Type == ActivityReading && e.File == "main.go" {
			hasReading = true
		}
		if e.Type == ActivityWriting && e.File == "config.go" {
			hasWriting = true
		}
	}

	if !hasReading {
		t.Error("Did not receive Reading event from stream-json")
	}
	if !hasWriting {
		t.Error("Did not receive Writing event from stream-json")
	}

	// Check that last result was captured
	lastResult := executor.LastClaudeResult()
	if lastResult == nil {
		t.Fatal("LastResult should not be nil")
	}
	if lastResult.Result != "Done" {
		t.Errorf("LastResult.Result = %q, want %q", lastResult.Result, "Done")
	}
	if lastResult.SessionID != "test123" {
		t.Errorf("LastResult.SessionID = %q, want %q", lastResult.SessionID, "test123")
	}
}

func TestStreamingExecutor_MediumVerbosityShowsFileOps(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	// Use medium verbosity - should show file operations
	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityMedium,
	})

	// Use realistic Claude stream-json format
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"main.go"}}]}}'
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"config.go"}}]}}'
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"go test","description":"Run tests"}}]}}'
	`)

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Medium verbosity should show all file operations and commands
	var hasReading, hasWriting, hasExecuting bool
	for _, e := range receivedEvents {
		if e.Type == ActivityReading {
			hasReading = true
		}
		if e.Type == ActivityWriting {
			hasWriting = true
		}
		if e.Type == ActivityExecuting {
			hasExecuting = true
		}
	}

	if !hasReading {
		t.Error("Medium verbosity should show Reading events")
	}
	if !hasWriting {
		t.Error("Medium verbosity should show Writing events")
	}
	if !hasExecuting {
		t.Error("Medium verbosity should show Executing events")
	}
}

func TestStreamingExecutor_LowVerbosityFiltersFileOps(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	// Use low verbosity - should filter out file operations
	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityLow,
	})

	// Use realistic Claude stream-json format
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"main.go"}}]}}'
		echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Task","input":{"subagent_type":"Explore"}}]}}'
	`)

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Low verbosity should filter out Reading but allow Analyzing (Task/sub-agent)
	var hasReading, hasAnalyzing bool
	for _, e := range receivedEvents {
		if e.Type == ActivityReading {
			hasReading = true
		}
		if e.Type == ActivityAnalyzing {
			hasAnalyzing = true
		}
	}

	if hasReading {
		t.Error("Low verbosity should filter out Reading events")
	}
	if !hasAnalyzing {
		t.Error("Low verbosity should allow Analyzing events (from Task tool)")
	}
}

func TestStreamingExecutor_LastResultNilWithoutResult(t *testing.T) {
	t.Parallel()

	executor := NewStreamingExecutor(ActivityOptions{
		Callback:  func(_ ActivityEvent) {},
		Verbosity: VerbosityHigh,
	})

	// Execute a command that doesn't output a result event
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "hello")

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// LastResult should be nil when no result event was received
	if executor.LastClaudeResult() != nil {
		t.Error("LastResult should be nil when no result event was received")
	}
}

func TestStreamingExecutor_GeminiStreamJSON(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	// Use Gemini provider
	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityHigh,
	}, WithStreamProvider(StreamProviderGemini))

	// Execute a command that outputs realistic Gemini stream-json format to stdout
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo '{"type":"init","timestamp":"2026-01-21T14:44:36.452Z","session_id":"test-gemini-123","model":"auto-gemini-3"}'
		echo '{"type":"message","timestamp":"2026-01-21T14:44:36.517Z","role":"user","content":"What version?"}'
		echo '{"type":"tool_use","timestamp":"2026-01-21T14:44:55.963Z","tool_name":"read_file","tool_id":"read_file-1","parameters":{"file_path":"go.mod"}}'
		echo '{"type":"tool_result","timestamp":"2026-01-21T14:44:56.123Z","tool_id":"read_file-1","status":"success","output":"module example\n\ngo 1.24"}'
		echo '{"type":"tool_use","timestamp":"2026-01-21T14:44:57.000Z","tool_name":"edit_file","tool_id":"edit_file-1","parameters":{"file_path":"main.go"}}'
		echo '{"type":"result","timestamp":"2026-01-21T14:44:57.289Z","status":"success","stats":{"total_tokens":16166,"input_tokens":15783,"output_tokens":124,"duration_ms":5419,"tool_calls":2}}'
	`)

	stdout, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify stdout was captured
	if !strings.Contains(string(stdout), "tool_use") {
		t.Error("stdout should contain Gemini stream-json events")
	}

	// Wait a bit for async processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	events := make([]ActivityEvent, len(receivedEvents))
	copy(events, receivedEvents)
	mu.Unlock()

	// Should have parsed the tool events (2 tool_use events)
	if len(events) < 2 {
		t.Errorf("Received %d events, want >= 2", len(events))
	}

	// Check that we got reading and writing events from Gemini stream-json
	var hasReading, hasWriting bool
	for _, e := range events {
		if e.Type == ActivityReading && e.File == "go.mod" {
			hasReading = true
		}
		if e.Type == ActivityWriting && e.File == "main.go" {
			hasWriting = true
		}
	}

	if !hasReading {
		t.Error("Did not receive Reading event from Gemini stream-json")
	}
	if !hasWriting {
		t.Error("Did not receive Writing event from Gemini stream-json")
	}

	// Check that last Gemini result was captured
	lastResult := executor.LastGeminiResult()
	if lastResult == nil {
		t.Fatal("LastGeminiResult should not be nil")
	}
	if !lastResult.Success {
		t.Error("LastGeminiResult.Success should be true")
	}
	if lastResult.SessionID != "test-gemini-123" {
		t.Errorf("LastGeminiResult.SessionID = %q, want %q", lastResult.SessionID, "test-gemini-123")
	}
	if lastResult.DurationMs != 5419 {
		t.Errorf("LastGeminiResult.DurationMs = %d, want %d", lastResult.DurationMs, 5419)
	}
	if lastResult.ToolCalls != 2 {
		t.Errorf("LastGeminiResult.ToolCalls = %d, want %d", lastResult.ToolCalls, 2)
	}

	// Claude result should be nil since we're using Gemini provider
	if executor.LastClaudeResult() != nil {
		t.Error("LastClaudeResult should be nil when using Gemini provider")
	}
}

func TestStreamingExecutor_GeminiMediumVerbosity(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	// Use Gemini provider with medium verbosity
	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityMedium,
	}, WithStreamProvider(StreamProviderGemini))

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo '{"type":"init","session_id":"test-123"}'
		echo '{"type":"tool_use","tool_name":"read_file","tool_id":"t1","parameters":{"file_path":"config.go"}}'
		echo '{"type":"tool_use","tool_name":"shell","tool_id":"t2","parameters":{"command":"go test ./..."}}'
		echo '{"type":"tool_use","tool_name":"search_files","tool_id":"t3","parameters":{"pattern":"func Test"}}'
	`)

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Medium verbosity should show all file operations and commands
	var hasReading, hasExecuting, hasSearching bool
	for _, e := range receivedEvents {
		if e.Type == ActivityReading {
			hasReading = true
		}
		if e.Type == ActivityExecuting {
			hasExecuting = true
		}
		if e.Type == ActivitySearching {
			hasSearching = true
		}
	}

	if !hasReading {
		t.Error("Medium verbosity should show Reading events for Gemini")
	}
	if !hasExecuting {
		t.Error("Medium verbosity should show Executing events for Gemini")
	}
	if !hasSearching {
		t.Error("Medium verbosity should show Searching events for Gemini")
	}
}

func TestStreamingExecutor_GeminiLowVerbosityFiltersFileOps(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent
	var mu sync.Mutex

	// Use Gemini provider with low verbosity
	executor := NewStreamingExecutor(ActivityOptions{
		Callback: func(event ActivityEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		},
		Verbosity: VerbosityLow,
	}, WithStreamProvider(StreamProviderGemini))

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		echo '{"type":"init","session_id":"test-123"}'
		echo '{"type":"tool_use","tool_name":"read_file","tool_id":"t1","parameters":{"file_path":"main.go"}}'
		echo '{"type":"tool_use","tool_name":"UnknownAnalysisTool","tool_id":"t2","parameters":{}}'
	`)

	_, _, err := executor.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Low verbosity should filter out Reading but allow Analyzing (unknown tool)
	var hasReading, hasAnalyzing bool
	for _, e := range receivedEvents {
		if e.Type == ActivityReading {
			hasReading = true
		}
		if e.Type == ActivityAnalyzing {
			hasAnalyzing = true
		}
	}

	if hasReading {
		t.Error("Low verbosity should filter out Reading events for Gemini")
	}
	if !hasAnalyzing {
		t.Error("Low verbosity should allow Analyzing events for Gemini")
	}
}

func TestStreamingExecutor_WithStreamProvider(t *testing.T) {
	t.Parallel()

	t.Run("defaults to Claude provider", func(t *testing.T) {
		executor := NewStreamingExecutor(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityHigh,
		})

		if executor.provider != StreamProviderClaude {
			t.Errorf("Default provider = %q, want %q", executor.provider, StreamProviderClaude)
		}
	})

	t.Run("can set Gemini provider", func(t *testing.T) {
		executor := NewStreamingExecutor(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityHigh,
		}, WithStreamProvider(StreamProviderGemini))

		if executor.provider != StreamProviderGemini {
			t.Errorf("Provider = %q, want %q", executor.provider, StreamProviderGemini)
		}
	})
}
