package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	cache "github.com/mrz1836/go-cache"
)

// ErrQueueFull is returned by Submit when the queue has reached its MaxSize limit.
var ErrQueueFull = errors.New("queue is full")

// Priority levels for the task queue.
type Priority string

const (
	// PriorityUrgent is the highest-priority level.
	PriorityUrgent Priority = "urgent"
	// PriorityNormal is the default priority level.
	PriorityNormal Priority = "normal"
	// PriorityLow is the lowest-priority level.
	PriorityLow Priority = "low"
)

// QueueEntry represents a task in the queue.
type QueueEntry struct {
	TaskID   string
	Priority Priority
	Score    float64 // Unix nanosecond timestamp — lower score = submitted earlier (FIFO within same priority)
	// Score is time.Now().UnixNano() cast to float64. Redis sorted-set scores are
	// float64 (IEEE 754 double), which gives exact integer representation up to 2^53.
	// UnixNano values in 2024 are ~1.7×10^18 < 2^53 (~9×10^15), so collisions are
	// possible for concurrent submits on the same nanosecond tick. In practice this
	// is acceptable: the sort order within the same nanosecond is arbitrary but stable.
}

// QueueStats holds queue depth per priority level.
type QueueStats struct {
	Urgent int64
	Normal int64
	Low    int64
	Total  int64
}

// Queue defines the interface for the Atlas task queue.
type Queue interface {
	// Submit enqueues a task at the given priority.
	Submit(ctx context.Context, taskID string, priority Priority) error
	// Pop removes and returns the taskID of the highest-priority task.
	// Returns ("", "", nil) when all queues are empty.
	Pop(ctx context.Context) (string, Priority, error)
	// Remove removes a specific task from whichever priority queue it is in.
	Remove(ctx context.Context, taskID string) error
	// List returns all queued tasks, optionally filtered by priority.
	List(ctx context.Context, priority *Priority) ([]QueueEntry, error)
	// Stats returns the count of tasks at each priority level.
	Stats(ctx context.Context) (QueueStats, error)
	// Clear removes all tasks from the queue, optionally limited to one priority.
	Clear(ctx context.Context, priority *Priority) error
}

// RedisQueue implements Queue using go-cache sorted sets.
// Tasks within the same priority level are ordered FIFO by submission time
// (nanosecond Unix timestamp as the score).
type RedisQueue struct {
	client    *cache.Client
	keyPrefix string
	maxSize   int // 0 = unlimited; positive = ErrQueueFull when total >= maxSize
}

// NewRedisQueue creates a new RedisQueue.
// keyPrefix is prepended to every Redis key (e.g. "atlas:").
func NewRedisQueue(client *cache.Client, keyPrefix string) *RedisQueue {
	return &RedisQueue{client: client, keyPrefix: keyPrefix}
}

// NewRedisQueueWithMaxSize creates a RedisQueue with a maximum size constraint.
// When maxSize > 0, Submit returns ErrQueueFull once the total queue depth reaches maxSize.
// maxSize 0 means unlimited.
func NewRedisQueueWithMaxSize(client *cache.Client, keyPrefix string, maxSize int) *RedisQueue {
	return &RedisQueue{client: client, keyPrefix: keyPrefix, maxSize: maxSize}
}

// Submit adds a task to the priority queue.
//
// Score: nanosecond Unix timestamp (float64). IEEE 754 doubles represent integers
// exactly up to 2^53 ≈ 9×10^15; UnixNano values (~1.7×10^18) exceed this, so the
// effective resolution is ~256 ns. Two submits within the same ~256 ns window receive
// the same score and are ordered arbitrarily by Redis. In practice Redis + Go
// goroutine scheduling overhead is measured in microseconds, so this is not a concern.
//
// Returns ErrQueueFull when maxSize > 0 and the total queue depth >= maxSize.
func (q *RedisQueue) Submit(ctx context.Context, taskID string, priority Priority) error {
	if q.maxSize > 0 {
		stats, err := q.Stats(ctx)
		if err != nil {
			return fmt.Errorf("check queue size: %w", err)
		}
		if stats.Total >= int64(q.maxSize) {
			return fmt.Errorf("%w: limit is %d", ErrQueueFull, q.maxSize)
		}
	}
	score := float64(time.Now().UnixNano())
	if err := cache.SortedSetAdd(ctx, q.client, q.queueKey(priority), score, taskID); err != nil {
		return err
	}
	// Publish notification to unblock waiting dispatchLoops
	_, _ = cache.Publish(ctx, q.client, q.keyPrefix+"queue:notify", "1")
	return nil
}

// Pop removes and returns the highest-priority task (urgent > normal > low).
// Returns ("", "", nil) if all queues are empty.
func (q *RedisQueue) Pop(ctx context.Context) (string, Priority, error) {
	for _, p := range allPriorities() {
		members, err := cache.SortedSetPopMin(ctx, q.client, q.queueKey(p), 1)
		if err != nil {
			return "", "", err
		}
		if len(members) > 0 {
			return members[0].Member.(string), p, nil
		}
	}
	return "", "", nil
}

// Remove deletes a specific task from every priority queue it might appear in.
func (q *RedisQueue) Remove(ctx context.Context, taskID string) error {
	for _, p := range allPriorities() {
		if err := cache.SortedSetRemove(ctx, q.client, q.queueKey(p), taskID); err != nil {
			return err
		}
	}
	return nil
}

// List returns all tasks currently in the queue.
// When priority is non-nil, only that priority level is returned.
func (q *RedisQueue) List(ctx context.Context, priority *Priority) ([]QueueEntry, error) {
	var priorities []Priority
	if priority != nil {
		priorities = []Priority{*priority}
	} else {
		priorities = allPriorities()
	}

	var entries []QueueEntry
	for _, p := range priorities {
		members, err := cache.SortedSetRangeWithScores(ctx, q.client, q.queueKey(p), 0, -1)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			entries = append(entries, QueueEntry{
				TaskID:   m.Member.(string),
				Priority: p,
				Score:    m.Score,
			})
		}
	}
	return entries, nil
}

// Stats returns the number of tasks queued at each priority level.
func (q *RedisQueue) Stats(ctx context.Context) (QueueStats, error) {
	var s QueueStats
	urgent, err := cache.SortedSetCard(ctx, q.client, q.queueKey(PriorityUrgent))
	if err != nil {
		return s, err
	}
	normal, err := cache.SortedSetCard(ctx, q.client, q.queueKey(PriorityNormal))
	if err != nil {
		return s, err
	}
	low, err := cache.SortedSetCard(ctx, q.client, q.queueKey(PriorityLow))
	if err != nil {
		return s, err
	}
	s.Urgent = urgent
	s.Normal = normal
	s.Low = low
	s.Total = urgent + normal + low
	return s, nil
}

// Clear removes all tasks from the queue.
// When priority is non-nil, only that priority level is cleared.
func (q *RedisQueue) Clear(ctx context.Context, priority *Priority) error {
	var priorities []Priority
	if priority != nil {
		priorities = []Priority{*priority}
	} else {
		priorities = allPriorities()
	}

	keys := make([]string, len(priorities))
	for i, p := range priorities {
		keys[i] = q.queueKey(p)
	}
	_, err := cache.DeleteWithoutDependency(ctx, q.client, keys...)
	return err
}

// queueKey returns the sorted-set key for the given priority.
func (q *RedisQueue) queueKey(p Priority) string {
	return q.keyPrefix + "queue:" + string(p)
}

// allPriorities returns the pop order from highest to lowest.
// Defined as a function to avoid a package-level variable.
func allPriorities() []Priority {
	return []Priority{PriorityUrgent, PriorityNormal, PriorityLow}
}
