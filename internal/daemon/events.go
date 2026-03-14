package daemon

import (
	"context"
	"encoding/json"
	"time"

	cache "github.com/mrz1836/go-cache"
)

// defaultEventsChannel is the Redis pub/sub channel used for Atlas task events.
const defaultEventsChannel = "atlas:events"

// EventPublisher publishes TaskEvents to a Redis pub/sub channel.
type EventPublisher struct {
	client  *cache.Client
	channel string
}

// NewEventPublisher creates a new EventPublisher.
// If channel is empty, defaultEventsChannel ("atlas:events") is used.
func NewEventPublisher(client *cache.Client, channel string) *EventPublisher {
	if channel == "" {
		channel = defaultEventsChannel
	}
	return &EventPublisher{client: client, channel: channel}
}

// Publish marshals event to JSON and sends it to the configured Redis channel.
// The event's Time field is set to the current UTC time before marshaling.
func (p *EventPublisher) Publish(ctx context.Context, event TaskEvent) error {
	event.Time = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = cache.Publish(ctx, p.client, p.channel, string(data))
	return err
}
