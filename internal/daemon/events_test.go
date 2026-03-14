package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEventPublisher_DefaultChannel(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())
	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	pub := NewEventPublisher(client, "")
	assert.Equal(t, defaultEventsChannel, pub.channel)
}

func TestNewEventPublisher_CustomChannel(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())
	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	pub := NewEventPublisher(client, "custom:events")
	assert.Equal(t, "custom:events", pub.channel)
}

func TestEventPublisher_Publish(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())
	ctx := context.Background()

	// Subscriber client
	subClient, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer subClient.Close()

	// Subscribe before publishing
	sub, err := cache.Subscribe(ctx, subClient, []string{defaultEventsChannel})
	require.NoError(t, err)
	defer sub.Close() //nolint:errcheck // subscription close may fail during miniredis teardown; error is non-actionable

	// Publisher client
	pubClient, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer pubClient.Close()

	publisher := NewEventPublisher(pubClient, defaultEventsChannel)

	event := TaskEvent{
		Type:   EventTaskSubmitted,
		TaskID: "t123",
		Status: "queued",
	}

	waitPubSubReady(ctx, t, subClient, defaultEventsChannel)

	err = publisher.Publish(ctx, event)
	require.NoError(t, err)

	// Receive the message with a short timeout
	select {
	case msg := <-sub.Messages:
		var got TaskEvent
		require.NoError(t, json.Unmarshal(msg.Data, &got))
		assert.Equal(t, EventTaskSubmitted, got.Type)
		assert.Equal(t, "t123", got.TaskID)
		assert.Equal(t, "queued", got.Status)
		// Time should be set by Publish
		assert.NotEmpty(t, got.Time)
		// Verify it's a valid RFC3339 time
		_, err = time.Parse(time.RFC3339, got.Time)
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for pub/sub message")
	}
}

func TestEventPublisher_PublishSetsTime(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())
	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	// Subscribe to capture the message
	sub, err := cache.Subscribe(ctx, client, []string{"test:events"})
	require.NoError(t, err)
	defer sub.Close() //nolint:errcheck // subscription close may fail during miniredis teardown; error is non-actionable

	pub2Client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer pub2Client.Close()

	publisher := NewEventPublisher(pub2Client, "test:events")

	waitPubSubReady(ctx, t, pub2Client, "test:events")

	before := time.Now().UTC().Truncate(time.Second)
	err = publisher.Publish(ctx, TaskEvent{Type: EventDaemonStarted, TaskID: "daemon"})
	require.NoError(t, err)
	after := time.Now().UTC().Add(time.Second)

	select {
	case msg := <-sub.Messages:
		var got TaskEvent
		require.NoError(t, json.Unmarshal(msg.Data, &got))
		parsed, err := time.Parse(time.RFC3339, got.Time)
		require.NoError(t, err)
		assert.False(t, parsed.Before(before), "event time should not be before publish start")
		assert.False(t, parsed.After(after), "event time should not be after publish end")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for pub/sub message")
	}
}

func TestEventPublisher_MultipleEvents(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())
	ctx := context.Background()

	subClient, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer subClient.Close()

	sub, err := cache.Subscribe(ctx, subClient, []string{defaultEventsChannel})
	require.NoError(t, err)
	defer sub.Close() //nolint:errcheck // subscription close may fail during miniredis teardown; error is non-actionable

	pubClient, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer pubClient.Close()

	publisher := NewEventPublisher(pubClient, defaultEventsChannel)

	events := []TaskEvent{
		{Type: EventTaskSubmitted, TaskID: "t1"},
		{Type: EventTaskStarted, TaskID: "t1"},
		{Type: EventTaskCompleted, TaskID: "t1"},
	}

	waitPubSubReady(ctx, t, subClient, defaultEventsChannel)

	for _, ev := range events {
		require.NoError(t, publisher.Publish(ctx, ev))
	}

	for i, expected := range events {
		select {
		case msg := <-sub.Messages:
			var got TaskEvent
			require.NoError(t, json.Unmarshal(msg.Data, &got))
			assert.Equal(t, expected.Type, got.Type, "event %d type mismatch", i)
			assert.Equal(t, expected.TaskID, got.TaskID, "event %d task_id mismatch", i)
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

// waitPubSubReady blocks until the given channel has ≥1 registered subscriber,
// confirming that cache.Subscribe has registered with the Redis server before
// any Publish call. Prevents SUBSCRIBE/PUBLISH ordering races under load.
func waitPubSubReady(ctx context.Context, t *testing.T, client *cache.Client, channel string) {
	t.Helper()
	conn, err := client.GetConnectionWithContext(ctx)
	require.NoError(t, err, "waitPubSubReady: get connection")
	defer client.CloseConnection(conn)
	require.Eventually(t, func() bool {
		result, err := conn.Do("PUBSUB", "NUMSUB", channel)
		if err != nil {
			return false
		}
		pairs, ok := result.([]interface{})
		if !ok || len(pairs) < 2 {
			return false
		}
		count, ok := pairs[1].(int64)
		return ok && count >= 1
	}, 2*time.Second, 2*time.Millisecond)
}
