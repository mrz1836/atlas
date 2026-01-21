package ai

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"sync"
	"time"
)

// StreamProvider identifies the AI provider for stream parsing.
type StreamProvider string

const (
	// StreamProviderClaude uses Claude Code's stream-json format.
	StreamProviderClaude StreamProvider = "claude"

	// StreamProviderGemini uses Gemini CLI's stream-json format.
	StreamProviderGemini StreamProvider = "gemini"
)

// StreamingExecutor executes commands while streaming output for activity parsing.
// It implements CommandExecutor for use with AI runners.
// When using stream-json output format, it parses stdout for tool events.
// It also parses stderr as a fallback for legacy activity patterns.
type StreamingExecutor struct {
	provider           StreamProvider           // AI provider for format selection
	stderrParser       *ActivityParser          // Parses stderr for legacy activity patterns
	claudeStreamParser *StreamEventParser       // Parses stdout for Claude stream-json events
	geminiStreamParser *GeminiStreamEventParser // Parses stdout for Gemini stream-json events
	onActivity         ActivityCallback
	verbosity          VerbosityLevel
	mu                 sync.Mutex
	lastEmit           time.Time
	syntheticMu        sync.Mutex
	stopSynth          chan struct{}
	lastClaudeResult   *ClaudeResponse     // Stores the final result from Claude stream events
	lastGeminiResult   *GeminiStreamResult // Stores the final result from Gemini stream events
	lastResultMu       sync.Mutex
}

// StreamingExecutorOption configures a StreamingExecutor.
type StreamingExecutorOption func(*StreamingExecutor)

// WithStreamProvider sets the AI provider for stream parsing.
func WithStreamProvider(provider StreamProvider) StreamingExecutorOption {
	return func(e *StreamingExecutor) {
		e.provider = provider
	}
}

// NewStreamingExecutor creates a new StreamingExecutor with the given options.
// By default, uses Claude provider for backwards compatibility.
func NewStreamingExecutor(opts ActivityOptions, execOpts ...StreamingExecutorOption) *StreamingExecutor {
	e := &StreamingExecutor{
		provider:           StreamProviderClaude, // Default to Claude for backwards compatibility
		stderrParser:       NewActivityParser(),
		claudeStreamParser: NewStreamEventParser(),
		geminiStreamParser: NewGeminiStreamEventParser(),
		onActivity:         opts.Callback,
		verbosity:          opts.Verbosity,
	}

	for _, opt := range execOpts {
		opt(e)
	}

	return e
}

// Execute runs the command while streaming output for activity events.
// For stream-json output format, it parses stdout line-by-line for tool events.
// It also streams stderr for legacy activity patterns as a fallback.
func (e *StreamingExecutor) Execute(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	// Reset last results
	e.lastResultMu.Lock()
	e.lastClaudeResult = nil
	e.lastGeminiResult = nil
	e.lastResultMu.Unlock()

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	// Start the command
	if startErr := cmd.Start(); startErr != nil {
		return nil, nil, startErr
	}

	// Start synthetic progress in background if no real activity
	e.startSyntheticProgress(ctx)
	defer e.stopSyntheticProgress()

	// Capture stdout and stderr concurrently
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout line by line for NDJSON parsing
	go func() {
		defer wg.Done()
		e.streamStdout(stdoutPipe, &stdout)
	}()

	// Stream stderr line by line for legacy activity patterns
	go func() {
		defer wg.Done()
		e.streamStderr(stderrPipe, &stderr)
	}()

	// Wait for all I/O to complete
	wg.Wait()

	// Wait for the command to finish
	err = cmd.Wait()

	return stdout.Bytes(), stderr.Bytes(), err
}

// LastClaudeResult returns the last Claude result parsed from stream-json output.
// Returns nil if no result event was received or if non-streaming output was used.
func (e *StreamingExecutor) LastClaudeResult() *ClaudeResponse {
	e.lastResultMu.Lock()
	defer e.lastResultMu.Unlock()
	return e.lastClaudeResult
}

// LastGeminiResult returns the last Gemini result parsed from stream-json output.
// Returns nil if no result event was received or if non-streaming output was used.
func (e *StreamingExecutor) LastGeminiResult() *GeminiStreamResult {
	e.lastResultMu.Lock()
	defer e.lastResultMu.Unlock()
	return e.lastGeminiResult
}

// streamStdout reads stdout line by line, parsing stream-json events
// and collecting the full output.
func (e *StreamingExecutor) streamStdout(r io.Reader, buf *bytes.Buffer) {
	scanner := bufio.NewScanner(r)

	// Increase buffer size for long lines (tool outputs can be large)
	const maxLineSize = 10 * 1024 * 1024 // 10MB
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()

		// Write to buffer for complete stdout capture
		buf.WriteString(line)
		buf.WriteByte('\n')

		// Parse based on provider
		switch e.provider {
		case StreamProviderGemini:
			e.parseGeminiLine(line)
		case StreamProviderClaude:
			e.parseClaudeLine(line)
		default:
			e.parseClaudeLine(line)
		}
	}
}

// parseClaudeLine parses a single line as Claude stream-json format.
func (e *StreamingExecutor) parseClaudeLine(line string) {
	event := e.claudeStreamParser.ParseLine(line)
	if event == nil {
		return
	}

	// Check if this is the final result
	if e.claudeStreamParser.IsResultEvent(event) {
		e.lastResultMu.Lock()
		e.lastClaudeResult = e.claudeStreamParser.ToClaudeResponse(event)
		e.lastResultMu.Unlock()
		return
	}

	// Convert to activity event and emit
	if activity := e.claudeStreamParser.ToActivityEvent(event); activity != nil {
		e.emitEvent(*activity)
	}
}

// parseGeminiLine parses a single line as Gemini stream-json format.
func (e *StreamingExecutor) parseGeminiLine(line string) {
	event := e.geminiStreamParser.ParseLine(line)
	if event == nil {
		return
	}

	// Check if this is the final result
	if e.geminiStreamParser.IsResultEvent(event) {
		e.lastResultMu.Lock()
		e.lastGeminiResult = e.geminiStreamParser.ToGeminiResult(event)
		e.lastResultMu.Unlock()
		return
	}

	// Convert to activity event and emit
	if activity := e.geminiStreamParser.ToActivityEvent(event); activity != nil {
		e.emitEvent(*activity)
	}
}

// streamStderr reads stderr line by line, parsing activity events
// and collecting the full output. This serves as a fallback for
// legacy activity patterns when stream-json doesn't provide events.
func (e *StreamingExecutor) streamStderr(r io.Reader, buf *bytes.Buffer) {
	scanner := bufio.NewScanner(r)

	// Increase buffer size for long lines
	const maxLineSize = 1024 * 1024 // 1MB
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()

		// Write to buffer for complete stderr capture
		buf.WriteString(line)
		buf.WriteByte('\n')

		// Parse and emit activity event from legacy patterns
		if event := e.stderrParser.ParseLine(line); event != nil {
			e.emitEvent(*event)
		}
	}
}

// emitEvent emits an activity event if it passes the verbosity filter.
func (e *StreamingExecutor) emitEvent(event ActivityEvent) {
	if e.onActivity == nil {
		return
	}

	// Check verbosity filter
	if !e.verbosity.ShouldShow(event.Type) {
		return
	}

	e.mu.Lock()
	e.lastEmit = time.Now()
	e.mu.Unlock()

	e.onActivity(event)
}

// startSyntheticProgress starts a goroutine that emits synthetic progress
// events if no real activity has occurred for a period of time.
func (e *StreamingExecutor) startSyntheticProgress(ctx context.Context) {
	e.syntheticMu.Lock()
	e.stopSynth = make(chan struct{})
	stopCh := e.stopSynth
	e.syntheticMu.Unlock()

	go func() {
		syntheticPhases := []struct {
			msg     string
			actType ActivityType
		}{
			{"Analyzing...", ActivityAnalyzing},
			{"Planning...", ActivityPlanning},
			{"Implementing...", ActivityImplementing},
		}
		phaseIdx := 0
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				e.mu.Lock()
				sinceLastEmit := time.Since(e.lastEmit)
				e.mu.Unlock()

				// If no real activity for 5+ seconds, emit synthetic event
				if sinceLastEmit >= 5*time.Second {
					phase := syntheticPhases[phaseIdx%len(syntheticPhases)] //nolint:gosec // Index is safely bounded by modulo
					e.emitEvent(ActivityEvent{
						Timestamp: time.Now(),
						Type:      phase.actType,
						Message:   phase.msg,
					})
					phaseIdx++
				}
			}
		}
	}()
}

// stopSyntheticProgress stops the synthetic progress goroutine.
func (e *StreamingExecutor) stopSyntheticProgress() {
	e.syntheticMu.Lock()
	defer e.syntheticMu.Unlock()
	if e.stopSynth != nil {
		close(e.stopSynth)
		e.stopSynth = nil
	}
}

// Compile-time check that StreamingExecutor implements CommandExecutor.
var _ CommandExecutor = (*StreamingExecutor)(nil)
