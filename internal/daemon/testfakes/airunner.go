package testfakes

import (
	"context"
	"sync"
	"time"

	"github.com/mrz1836/atlas/internal/daemon"
)

// FakeExecutor is a test double for daemon.TaskExecutor that records calls and
// returns configurable results. Configure fields before use; safe for concurrent
// access via the exported mu-protected slices.
//
// Usage:
//
//	exec := &testfakes.FakeExecutor{FinalStatus: "completed"}
//	runner := daemon.NewRunner(cfg, client, queue, events, logger, exec)
type FakeExecutor struct {
	mu sync.Mutex

	// FinalStatus is the status returned by Execute. Defaults to "completed" when empty.
	FinalStatus string

	// EngineTaskID is the engine-assigned task ID returned by Execute.
	EngineTaskID string

	// ExecErr, when non-nil, is returned as the error from Execute.
	ExecErr error

	// BlockUntilCtxDone causes Execute to block until ctx is done, then return ctx.Err().
	// Useful for testing cancellation and abandonment during execution.
	BlockUntilCtxDone bool

	// ExecDelay introduces a fixed delay before Execute returns. Respects ctx cancellation.
	ExecDelay time.Duration

	// AbandonErr is the error returned by Abandon.
	AbandonErr error

	// ExecuteCalls records each Execute invocation (by value) for post-test assertion.
	ExecuteCalls []daemon.TaskJob

	// AbandonCalls records each Abandon invocation (by value).
	AbandonCalls []daemon.TaskJob
}

// Execute implements daemon.TaskExecutor.
func (f *FakeExecutor) Execute(ctx context.Context, job daemon.TaskJob) (string, string, error) {
	f.mu.Lock()
	f.ExecuteCalls = append(f.ExecuteCalls, job)
	f.mu.Unlock()

	if f.BlockUntilCtxDone {
		<-ctx.Done()
		return "", "", ctx.Err()
	}
	if f.ExecDelay > 0 {
		select {
		case <-time.After(f.ExecDelay):
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	if f.ExecErr != nil {
		return "", "", f.ExecErr
	}
	status := f.FinalStatus
	if status == "" {
		status = "completed"
	}
	return f.EngineTaskID, status, nil
}

// Abandon implements daemon.TaskExecutor.
func (f *FakeExecutor) Abandon(_ context.Context, job daemon.TaskJob, _ string) error {
	f.mu.Lock()
	f.AbandonCalls = append(f.AbandonCalls, job)
	f.mu.Unlock()
	return f.AbandonErr
}

// CallCount returns the number of Execute invocations recorded so far.
func (f *FakeExecutor) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ExecuteCalls)
}
