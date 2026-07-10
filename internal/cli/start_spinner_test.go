package cli

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
)

// spinnerRecorder tracks spinner lifecycle across a createProgressCallback run so
// tests can assert that at most one spinner is ever live at a time (no orphaned
// animate goroutines) and that every created spinner is eventually stopped.
type spinnerRecorder struct {
	mu      sync.Mutex
	created int
	live    int
	maxLive int
	events  []string // "create#<n>" / "stop#<n>" in occurrence order
}

func (r *spinnerRecorder) onCreate() *fakeSpinner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created++
	r.live++
	if r.live > r.maxLive {
		r.maxLive = r.live
	}
	id := r.created
	r.events = append(r.events, fmt.Sprintf("create#%d", id))
	return &fakeSpinner{rec: r, id: id}
}

func (r *spinnerRecorder) onStop(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live--
	r.events = append(r.events, fmt.Sprintf("stop#%d", id))
}

// counts returns the peak concurrent live spinners and the current live count.
func (r *spinnerRecorder) counts() (maxLive, live int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxLive, r.live
}

// eventLog returns a copy of the ordered create/stop event log.
func (r *spinnerRecorder) eventLog() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// fakeSpinner is a tui.Spinner whose Stop() is idempotent, mirroring
// TerminalSpinner.Stop()'s stopped-guard so a double Stop() cannot double-count.
type fakeSpinner struct {
	rec     *spinnerRecorder
	id      int
	mu      sync.Mutex
	stopped bool
	updates []string
}

func (s *fakeSpinner) Update(msg string) {
	s.mu.Lock()
	s.updates = append(s.updates, msg)
	s.mu.Unlock()
}

func (s *fakeSpinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	s.rec.onStop(s.id)
}

// fakeOutput implements tui.Output, routing spinner creation through the recorder
// and capturing Success lines. All other methods are no-ops.
type fakeOutput struct {
	rec *spinnerRecorder
}

func (o *fakeOutput) Spinner(_ context.Context, _ string) tui.Spinner { return o.rec.onCreate() }
func (o *fakeOutput) Success(string)                                  {}
func (o *fakeOutput) Error(error)                                     {}
func (o *fakeOutput) Warning(string)                                  {}
func (o *fakeOutput) Info(string)                                     {}
func (o *fakeOutput) Table([]string, [][]string)                      {}
func (o *fakeOutput) JSON(any) error                                  { return nil }
func (o *fakeOutput) URL(string, string)                              {}
func (o *fakeOutput) Text(string)                                     {}

// event constructors keep the test sequences readable.
func evStart(idx, total int, name, agent, model string) task.StepProgressEvent {
	return task.StepProgressEvent{Type: "start", StepIndex: idx, TotalSteps: total, StepName: name, Agent: agent, Model: model}
}

func evComplete(idx, total int, name, status string) task.StepProgressEvent {
	return task.StepProgressEvent{Type: "complete", StepIndex: idx, TotalSteps: total, StepName: name, Status: status}
}

func evType(t, status string) task.StepProgressEvent {
	return task.StepProgressEvent{Type: t, Status: status}
}

// TestProgressCallback_NoOrphanedSpinners verifies that across every progress
// event sequence at most one spinner is ever live (maxLive == 1) and that all
// spinners are stopped by the end (live == 0). Before the flicker fix, each
// handle*Start replaced state.activeSpinner without stopping the previous one, so
// the validate-step and per-attempt validation spinners were orphaned and maxLive
// climbed to 3+ with live > 0 at the end.
func TestProgressCallback_NoOrphanedSpinners(t *testing.T) {
	tests := []struct {
		name   string
		events []task.StepProgressEvent
	}{
		{
			name: "normal multi-step run",
			events: []task.StepProgressEvent{
				evStart(0, 3, "analyze", "claude", "opus"),
				evComplete(0, 3, "analyze", constants.StepStatusSuccess),
				evStart(1, 3, "implement", "claude", "sonnet"),
				evComplete(1, 3, "implement", constants.StepStatusSuccess),
				evStart(2, 3, "validate", "", ""),
				evComplete(2, 3, "validate", constants.StepStatusSuccess),
			},
		},
		{
			name: "validation retry succeeds on attempt 2",
			events: []task.StepProgressEvent{
				evStart(4, 10, "validate", "", ""),
				evType("retry_ai_start", "Retry 1/3: AI fix"),
				evType("retry_ai_complete", "success"),
				evType("retry_validation_start", "Retry 1/3: Validating..."),
				evType("retry_ai_start", "Retry 2/3: AI fix"),
				evType("retry_ai_complete", "success"),
				evType("retry_validation_start", "Retry 2/3: Validating..."),
				evComplete(4, 10, "validate", constants.StepStatusSuccess),
			},
		},
		{
			name: "validation retry exhausts and fails",
			events: []task.StepProgressEvent{
				evStart(4, 10, "validate", "", ""),
				evType("retry_ai_start", "Retry 1/3: AI fix"),
				evType("retry_ai_complete", "success"),
				evType("retry_validation_start", "Retry 1/3: Validating..."),
				evType("retry_ai_start", "Retry 2/3: AI fix"),
				evType("retry_ai_complete", "success"),
				evType("retry_validation_start", "Retry 2/3: Validating..."),
				evComplete(4, 10, "validate", constants.StepStatusFailed),
			},
		},
		{
			name: "auto-fix flow",
			events: []task.StepProgressEvent{
				evStart(5, 10, "verify", "claude", "sonnet"),
				evType("auto_fix_start", "Auto-fixing"),
				evType("auto_fix_complete", "success"),
				evComplete(5, 10, "verify", constants.StepStatusSuccess),
			},
		},
		{
			name: "back-to-back starts without intervening complete",
			events: []task.StepProgressEvent{
				evStart(0, 3, "analyze", "claude", "opus"),
				evStart(1, 3, "implement", "claude", "sonnet"),
				evComplete(1, 3, "implement", constants.StepStatusSuccess),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &spinnerRecorder{}
			out := &fakeOutput{rec: rec}
			state := &progressState{}
			cb := createProgressCallback(context.Background(), out, "", state)

			for _, ev := range tt.events {
				cb(ev)
			}

			maxLive, live := rec.counts()
			assert.Equal(t, 1, maxLive, "at most one spinner should ever be live")
			assert.Equal(t, 0, live, "every spinner should be stopped by the end")
		})
	}
}

// TestProgressCallback_StopBeforeCreate asserts the strict stop-then-create
// ordering: the previous spinner is stopped before the replacement is created.
// This ordering is what keeps TerminalSpinner.Stop()'s SpinnerManager.ClearActive()
// from clobbering the replacement's active pointer (which would break the logger's
// line-coordination).
func TestProgressCallback_StopBeforeCreate(t *testing.T) {
	rec := &spinnerRecorder{}
	out := &fakeOutput{rec: rec}
	state := &progressState{}
	cb := createProgressCallback(context.Background(), out, "", state)

	events := []task.StepProgressEvent{
		evStart(4, 10, "validate", "", ""),
		evType("retry_ai_start", "Retry 1/3: AI fix"),
		evType("retry_ai_complete", "success"),
		evType("retry_validation_start", "Retry 1/3: Validating..."),
		evType("retry_ai_start", "Retry 2/3: AI fix"),
		evType("retry_ai_complete", "success"),
		evType("retry_validation_start", "Retry 2/3: Validating..."),
		evComplete(4, 10, "validate", constants.StepStatusSuccess),
	}
	for _, ev := range events {
		cb(ev)
	}

	order := rec.eventLog()
	want := []string{
		"create#1", "stop#1",
		"create#2", "stop#2",
		"create#3", "stop#3",
		"create#4", "stop#4",
		"create#5", "stop#5",
	}
	assert.Equal(t, want, order, "each spinner must be stopped before the next is created")
}

// TestProgressState_ConcurrentActivityAndTransitions exercises the field race the
// mutex closes: background activity callbacks read the (spinner, baseMessage,
// showGitStats) snapshot while the main goroutine swaps the active spinner on step
// transitions. Run under `go test -race`; before the mutex was added this races on
// progressState.activeSpinner.
func TestProgressState_ConcurrentActivityAndTransitions(t *testing.T) {
	rec := &spinnerRecorder{}
	out := &fakeOutput{rec: rec}
	state := &progressState{}
	ctx := context.Background()

	activity := createActivityUICallback(state, ai.VerbosityHigh)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Reader goroutines: hammer the activity callback (simulating background
	// streaming / synthetic-progress events).
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := ai.ActivityEvent{Type: ai.ActivityAnalyzing, Message: "Analyzing..."}
			for {
				select {
				case <-stop:
					return
				default:
					activity(ev)
				}
			}
		}()
	}

	// Writer: main-goroutine step transitions swapping the active spinner.
	for i := 0; i < 2000; i++ {
		stopActiveSpinner(state)
		state.setActiveWithStats(out.Spinner(ctx, "msg"), "Step 1/2: implement", true)
	}

	close(stop)
	wg.Wait()

	stopActiveSpinner(state)
	_, live := rec.counts()
	assert.Equal(t, 0, live, "no spinner should remain live after cleanup")
}

// TestActivityCallback_SkipsWhenNoActiveSpinner confirms the activity callback does
// nothing once a transition has cleared the active spinner, so a background event
// arriving after a step completes cannot update a stale spinner.
func TestActivityCallback_SkipsWhenNoActiveSpinner(t *testing.T) {
	rec := &spinnerRecorder{}
	out := &fakeOutput{rec: rec}
	state := &progressState{}
	activity := createActivityUICallback(state, ai.VerbosityHigh)

	// Active spinner present with a base message: activity updates it.
	sp := out.Spinner(context.Background(), "Step 1/2: implement")
	state.setActive(sp, "Step 1/2: implement")
	activity(ai.ActivityEvent{Type: ai.ActivityAnalyzing, Message: "Analyzing..."})

	fake, ok := sp.(*fakeSpinner)
	require.True(t, ok)
	fake.mu.Lock()
	updatesWhileActive := len(fake.updates)
	fake.mu.Unlock()
	assert.Positive(t, updatesWhileActive, "activity should update the active spinner")

	// After the spinner is stopped/cleared, activity events are ignored.
	stopActiveSpinner(state)
	state.resetMessage()
	activity(ai.ActivityEvent{Type: ai.ActivityAnalyzing, Message: "Analyzing..."})

	fake.mu.Lock()
	updatesAfterClear := len(fake.updates)
	fake.mu.Unlock()
	assert.Equal(t, updatesWhileActive, updatesAfterClear, "no update after the spinner is cleared")
}
