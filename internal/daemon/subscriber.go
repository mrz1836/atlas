package daemon

import (
	"context"
	"encoding/json"
	"sync"

	cache "github.com/mrz1836/go-cache"
)

// EventSubscriber receives real-time events from the daemon via Redis pub/sub.
// It wraps a cache.Subscription and deserializes TaskEvent payloads from the
// raw message bytes, delivering them on a typed channel.
type EventSubscriber struct {
	client  *cache.Client
	channel string

	mu      sync.Mutex
	sub     *cache.Subscription
	eventCh chan TaskEvent
	errCh   chan error
	stopCh  chan struct{}
	once    sync.Once
}

// NewEventSubscriber creates a subscriber for the given Redis channel.
// Use defaultEventsChannel ("atlas:events") as the channel for normal usage.
func NewEventSubscriber(client *cache.Client, channel string) *EventSubscriber {
	if channel == "" {
		channel = defaultEventsChannel
	}
	return &EventSubscriber{
		client:  client,
		channel: channel,
		eventCh: make(chan TaskEvent, 256),
		errCh:   make(chan error, 16),
		stopCh:  make(chan struct{}),
	}
}

// Start begins listening on the configured channel.
// Events arrive on the Events() channel; errors on Errors().
// Start may only be called once; subsequent calls are no-ops.
func (s *EventSubscriber) Start(ctx context.Context) error {
	var startErr error
	s.once.Do(func() {
		sub, err := cache.Subscribe(ctx, s.client, s.channel)
		if err != nil {
			startErr = err
			return
		}
		s.mu.Lock()
		s.sub = sub
		s.mu.Unlock()

		go s.readLoop(ctx, sub)
	})
	return startErr
}

// Events returns a receive-only channel of decoded TaskEvents.
// The channel is closed when the subscriber stops.
func (s *EventSubscriber) Events() <-chan TaskEvent {
	return s.eventCh
}

// Errors returns a receive-only channel of non-fatal errors (e.g. JSON parse failures).
func (s *EventSubscriber) Errors() <-chan error {
	return s.errCh
}

// Stop closes the subscription and signals the read loop to exit.
// It is safe to call Stop multiple times.
func (s *EventSubscriber) Stop() error {
	select {
	case <-s.stopCh:
		// Already stopped.
	default:
		close(s.stopCh)
	}
	s.mu.Lock()
	sub := s.sub
	s.mu.Unlock()
	if sub != nil {
		return sub.Close()
	}
	return nil
}

// readLoop forwards messages from the cache.Subscription to the typed eventCh.
func (s *EventSubscriber) readLoop(ctx context.Context, sub *cache.Subscription) {
	defer close(s.eventCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case msg, ok := <-sub.Messages:
			if !ok {
				// Subscription closed.
				return
			}
			var ev TaskEvent
			if err := json.Unmarshal(msg.Data, &ev); err != nil {
				select {
				case s.errCh <- err:
				default:
				}
				continue
			}
			select {
			case s.eventCh <- ev:
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			}
		}
	}
}
