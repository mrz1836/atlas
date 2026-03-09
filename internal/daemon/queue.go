package daemon

import (
	"context"
	"time"

	cache "github.com/mrz1836/go-cache"
)

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
	// Returns ("", nil) when all queues are empty.
	Pop(ctx context.Context) (string, error)
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
}

// NewRedisQueue creates a new RedisQueue.
// keyPrefix is prepended to every Redis key (e.g. "atlas:").
func NewRedisQueue(client *cache.Client, keyPrefix string) *RedisQueue {
	return &RedisQueue{client: client, keyPrefix: keyPrefix}
}

// Submit adds a task to the priority queue with a nanosecond timestamp score.
// Lower scores are popped first (FIFO within the same priority).
func (q *RedisQueue) Submit(ctx context.Context, taskID string, priority Priority) error {
	score := float64(time.Now().UnixNano())
	return cache.SortedSetAdd(ctx, q.client, q.queueKey(priority), score, taskID)
}

// Pop removes and returns the highest-priority task (urgent > normal > low).
// Returns ("", nil) if all queues are empty.
func (q *RedisQueue) Pop(ctx context.Context) (string, error) {
	for _, p := range allPriorities() {
		members, err := cache.SortedSetPopMin(ctx, q.client, q.queueKey(p), 1)
		if err != nil {
			return "", err
		}
		if len(members) > 0 {
			return members[0].Member.(string), nil
		}
	}
	return "", nil
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
