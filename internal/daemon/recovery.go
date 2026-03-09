package daemon

import (
	"context"
	"fmt"
	"strconv"

	cache "github.com/mrz1836/go-cache"
)

const (
	// maxRetryCount is the maximum number of times a task is re-queued after a crash.
	maxRetryCount = 3

	// activeSetKey is the Redis set that tracks all currently-running task IDs.
	activeSetKey = "atlas:active"
)

// RecoverOrphanedTasks scans the active-tasks set for tasks that were running when
// the daemon last crashed and re-queues them up to maxRetryCount times.
//
// A task is considered orphaned when its status is "running" but its worker
// heartbeat lock (atlas:lock:task:{id}) no longer exists in Redis.
func (d *Daemon) RecoverOrphanedTasks(ctx context.Context) error {
	// 1. Get all task IDs from the active set.
	members, err := cache.SetMembers(ctx, d.redis, activeSetKey)
	if err != nil {
		return fmt.Errorf("recover: get active set: %w", err)
	}

	if len(members) == 0 {
		d.logger.Info().Msg("recovery: no active tasks found")
		return nil
	}

	d.logger.Info().Int("count", len(members)).Msg("recovery: scanning active tasks")

	for _, taskID := range members {
		if recoverErr := d.recoverTask(ctx, taskID); recoverErr != nil {
			// Log and continue — one bad task should not abort the whole recovery scan.
			d.logger.Error().
				Err(recoverErr).
				Str("task_id", taskID).
				Msg("recovery: error processing task")
		}
	}

	return nil
}

// recoverTask inspects a single task and either re-queues it or marks it failed.
func (d *Daemon) recoverTask(ctx context.Context, taskID string) error {
	// a. Get task fields from hash atlas:task:{taskID}.
	fields, err := d.getTaskStatus(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task status: %w", err)
	}

	status := fields["status"]
	retryStr := fields["retry_count"]
	priority := fields["priority"]

	// b. Only recover tasks that were "running".
	if status != "running" {
		return nil
	}

	// c. Check if the worker heartbeat lock exists.
	lockKey := fmt.Sprintf("atlas:lock:task:%s", taskID)
	hasLock, err := cache.Exists(ctx, d.redis, lockKey)
	if err != nil {
		return fmt.Errorf("check lock key %q: %w", lockKey, err)
	}

	// d. If the lock is still live, the task is not orphaned.
	if hasLock {
		return nil
	}

	// e. Parse retry count.
	retryCount, _ := strconv.Atoi(retryStr)

	// f. Exceeded max retries — mark as failed.
	if retryCount >= maxRetryCount {
		d.logger.Warn().
			Str("task_id", taskID).
			Int("retry_count", retryCount).
			Msg("recovery: max retries exceeded; marking task failed")

		if err := d.setTaskField(ctx, taskID, "status", "failed"); err != nil {
			return fmt.Errorf("set failed status: %w", err)
		}
		if err := d.setTaskField(ctx, taskID, "error", "max retries exceeded"); err != nil {
			return fmt.Errorf("set error field: %w", err)
		}
		return nil
	}

	// g. Re-queue: increment retry_count, reset status, push back onto queue.
	newRetry := strconv.Itoa(retryCount + 1)
	if err := d.setTaskField(ctx, taskID, "retry_count", newRetry); err != nil {
		return fmt.Errorf("increment retry_count: %w", err)
	}
	if err := d.setTaskField(ctx, taskID, "status", "queued"); err != nil {
		return fmt.Errorf("reset status to queued: %w", err)
	}

	// Determine priority for re-queue; fall back to Normal.
	p := Priority(priority)
	if p != PriorityUrgent && p != PriorityNormal && p != PriorityLow {
		p = PriorityNormal
	}
	if err := d.queue.Submit(ctx, taskID, p); err != nil {
		return fmt.Errorf("re-submit to queue: %w", err)
	}

	// h. Log recovery action.
	d.logger.Info().
		Str("task_id", taskID).
		Int("retry_count", retryCount+1).
		Str("priority", string(p)).
		Msg("recovery: orphaned task re-queued")

	return nil
}

// getTaskStatus reads task fields from the Redis hash atlas:task:{taskID}.
// Returns a map of field names to values for the fields: status, retry_count, priority.
func (d *Daemon) getTaskStatus(ctx context.Context, taskID string) (map[string]string, error) {
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	keys := []interface{}{"status", "retry_count", "priority"}

	values, err := cache.HashMapGet(ctx, d.redis, hashKey, keys...)
	if err != nil {
		return nil, fmt.Errorf("hash map get %q: %w", hashKey, err)
	}

	result := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(values) {
			field, ok := k.(string)
			if !ok {
				d.logger.Warn().Interface("key", k).Msg("recovery: unexpected key type, skipping")
				continue
			}
			result[field] = values[i]
		}
	}
	return result, nil
}

// setTaskField updates a single field in the task hash atlas:task:{taskID}.
func (d *Daemon) setTaskField(ctx context.Context, taskID, field, value string) error {
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	if err := cache.HashSet(ctx, d.redis, hashKey, field, value); err != nil {
		return fmt.Errorf("hash set %q[%q]: %w", hashKey, field, err)
	}
	return nil
}
