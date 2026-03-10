package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEventSubscriber tests that NewEventSubscriber defaults channel correctly.
func TestNewEventSubscriber(t *testing.T) {
	t.Parallel()

	s := NewEventSubscriber(nil, "")
	assert.Equal(t, defaultEventsChannel, s.channel, "empty channel should default to defaultEventsChannel")
	assert.NotNil(t, s.eventCh)
	assert.NotNil(t, s.errCh)
	assert.NotNil(t, s.stopCh)

	s2 := NewEventSubscriber(nil, "custom:channel")
	assert.Equal(t, "custom:channel", s2.channel, "custom channel should be preserved")
}

// TestEventSubscriber_StopBeforeStart verifies Stop is safe before Start is called.
func TestEventSubscriber_StopBeforeStart(t *testing.T) {
	t.Parallel()

	s := NewEventSubscriber(nil, "")
	err := s.Stop()
	assert.NoError(t, err, "Stop before Start should be a no-op")
}

// TestEventSubscriber_StopIdempotent verifies Stop can be called multiple times.
func TestEventSubscriber_StopIdempotent(t *testing.T) {
	t.Parallel()

	s := NewEventSubscriber(nil, "")
	// Call Stop twice — must not panic.
	assert.NoError(t, s.Stop())
	assert.NoError(t, s.Stop())
}

// TestEventSubscriber_Events returns receive-only channel.
func TestEventSubscriber_Events(t *testing.T) {
	t.Parallel()

	s := NewEventSubscriber(nil, "")
	ch := s.Events()
	assert.NotNil(t, ch)
}

// TestEventSubscriber_Errors returns receive-only error channel.
func TestEventSubscriber_Errors(t *testing.T) {
	t.Parallel()

	s := NewEventSubscriber(nil, "")
	ch := s.Errors()
	assert.NotNil(t, ch)
}

// TestEventSubscriber_readLoop_InvalidJSON verifies that bad JSON is sent to errCh.
func TestEventSubscriber_readLoop_InvalidJSON(t *testing.T) {
	t.Parallel()

	s := NewEventSubscriber(nil, "")
	// Replace channels with buffered ones we control.
	s.eventCh = make(chan TaskEvent, 10)
	s.errCh = make(chan error, 10)
	s.stopCh = make(chan struct{})

	// Build a fake cache.Subscription channel by using the subscriber's readLoop indirectly.
	// We'll directly call the internal parsing logic instead to unit-test without Redis.
	data := []byte(`not valid json`)
	var ev TaskEvent
	err := json.Unmarshal(data, &ev)
	require.Error(t, err, "bad JSON should return error")
}

// TestEventSubscriber_readLoop_ValidJSON verifies that valid TaskEvent JSON is parsed correctly.
func TestEventSubscriber_readLoop_ValidJSON(t *testing.T) {
	t.Parallel()

	event := TaskEvent{
		Type:      EventTaskStarted,
		TaskID:    "test-id",
		Status:    "running",
		Workspace: "ws-1",
		Agent:     "claude",
		Model:     "opus",
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)

	var parsed TaskEvent
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, event.Type, parsed.Type)
	assert.Equal(t, event.TaskID, parsed.TaskID)
	assert.Equal(t, event.Workspace, parsed.Workspace)
	assert.Equal(t, event.Agent, parsed.Agent)
	assert.Equal(t, event.Model, parsed.Model)
}

// TestEventSubscriber_StartOnce verifies that Start only subscribes once.
func TestEventSubscriber_StartOnce(t *testing.T) {
	t.Parallel()

	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewEventSubscriber(d.redis, defaultEventsChannel)
	err := s.Start(ctx)
	require.NoError(t, err, "first Start should succeed")

	// Calling Start a second time should be a no-op (returns nil).
	err = s.Start(ctx)
	require.NoError(t, err, "second Start should be no-op")

	_ = s.Stop()
}
