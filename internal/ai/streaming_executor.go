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

// StreamingExecutor executes commands while streaming stderr for activity parsing.
// It implements CommandExecutor for use with AI runners.
type StreamingExecutor struct {
	parser      *ActivityParser
	onActivity  ActivityCallback
	verbosity   VerbosityLevel
	mu          sync.Mutex
	lastEmit    time.Time
	syntheticMu sync.Mutex
	stopSynth   chan struct{}
}

// NewStreamingExecutor creates a new StreamingExecutor with the given options.
func NewStreamingExecutor(opts ActivityOptions) *StreamingExecutor {
	return &StreamingExecutor{
		parser:     NewActivityParser(),
		onActivity: opts.Callback,
		verbosity:  opts.Verbosity,
	}
}

// Execute runs the command while streaming stderr for activity events.
// It captures stdout completely for JSON parsing, while streaming stderr
// line-by-line to the activity parser.
func (e *StreamingExecutor) Execute(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
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

	// Read stdout completely
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdout, stdoutPipe)
	}()

	// Stream stderr line by line
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

// streamStderr reads stderr line by line, parsing activity events
// and collecting the full output.
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

		// Parse and emit activity event
		if event := e.parser.ParseLine(line); event != nil {
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
