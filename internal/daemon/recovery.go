package daemon

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	cache "github.com/mrz1836/go-cache"
)

const (
	// maxRetryCount is the maximum number of times a task is re-queued after a crash.
	maxRetryCount = 3
)

// RecoverOrphanedTasks scans the active-tasks set and applies the full recovery
// contract from todo §174–181:
//
//   - queued           → skip (will be picked up normally by workers)
//   - running, no lock → requeue up to maxRetryCount, then fail
//   - running, lock OK → skip (live worker holds it)
//   - awaiting_approval → skip (preserve; worker removes from active set on resume)
//   - completed/failed/canceled/abandoned → remove from active set (cleanup stale entries)
//
// Every recovery decision is emitted as a structured RecoveryEvent (Q4 verbose).
func (d *Daemon) RecoverOrphanedTasks(ctx context.Context) error {
	keyPrefix := d.cfg.Redis.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "atlas:"
	}
	activeSetKey := keyPrefix + "active"

	members, err := cache.SetMembers(ctx, d.redis, activeSetKey)
	if err != nil {
		return fmt.Errorf("recover: get active set: %w", err)
	}

	if len(members) == 0 {
		d.logger.Info().Msg("recovery: no active tasks found")
		return nil
	}

	d.logger.Info().Int("count", len(members)).Msg("recovery: scanning active tasks")

	var events []RecoveryEvent

	for _, taskID := range members {
		// Reject malformed IDs to prevent unexpected Redis key construction.
		if _, parseErr := uuid.Parse(taskID); parseErr != nil {
			d.logger.Warn().Str("task_id", taskID).Msg("recovery: skipping non-UUID task ID")
			continue
		}
		ev, recoverErr := d.recoverTask(ctx, taskID, keyPrefix)
		if recoverErr != nil {
			d.logger.Error().
				Err(recoverErr).
				Str("task_id", taskID).
				Msg("recovery: error processing task")
			// Continue — one bad task should not abort the whole recovery scan.
			continue
		}
		if ev != nil {
			events = append(events, *ev)
			// Emit verbose structured log event (Q4).
			d.logger.Info().
				Str("task_id", ev.TaskID).
				Str("decision", ev.Decision).
				Str("prior_state", ev.PriorState).
				Str("reason", ev.Reason).
				Msg("recovery: task recovery decision")
		}
	}

	// Store recovery events for doctor/status output.
	d.storeRecoveryEvents(events)

	if len(events) > 0 {
		d.logger.Info().Int("decisions", len(events)).Msg("recovery: completed")
	}

	return nil
}

// recoverTask inspects a single task and applies the recovery contract.
// Returns a RecoveryEvent describing the decision, or nil if no action was taken.
func (d *Daemon) recoverTask(ctx context.Context, taskID, keyPrefix string) (*RecoveryEvent, error) {
	fields, err := d.getTaskFields(ctx, taskID, keyPrefix)
	if err != nil {
		return nil, fmt.Errorf("get task fields: %w", err)
	}

	status := fields["status"]
	retryStr := fields["retry_count"]
	priority := fields["priority"]

	ev := &RecoveryEvent{
		TaskID:     taskID,
		PriorState: status,
	}

	switch status {
	case "":
		// Hash missing or empty — task in active set but no metadata.
		// Remove from active set to clean up stale state.
		if rmErr := d.removeFromActiveSet(ctx, taskID, keyPrefix); rmErr != nil {
			return nil, fmt.Errorf("remove stale task from active set: %w", rmErr)
		}
		ev.Decision = "remove_terminal"
		ev.Reason = "task hash missing or empty; stale active-set entry removed"
		return ev, nil

	case "completed", "failed", "canceled", "abandoned":
		// Terminal states must never be requeued. Remove from active set if still present.
		if rmErr := d.removeFromActiveSet(ctx, taskID, keyPrefix); rmErr != nil {
			return nil, fmt.Errorf("remove terminal task from active set: %w", rmErr)
		}
		ev.Decision = "remove_terminal"
		ev.Reason = fmt.Sprintf("task is in terminal state %q; removed from active set", status)
		return ev, nil

	case "awaiting_approval":
		// Preserve: the task is waiting for human input, not orphaned.
		ev.Decision = "preserve_approval"
		ev.Reason = "task is awaiting approval; preserved without re-queuing"
		return ev, nil

	case "queued":
		// Already in queue; will be picked up by a worker naturally.
		ev.Decision = "skip"
		ev.Reason = "task is already queued; no action needed"
		return ev, nil

	case "running":
		// Fall through to orphan recovery logic below.

	default:
		// Unknown/intermediate state (paused, interrupted, etc.) — skip.
		ev.Decision = "skip"
		ev.Reason = fmt.Sprintf("task in non-recoverable state %q; skipping", status)
		return ev, nil
	}

	// Running task: check if the worker heartbeat lock is still live.
	lockKey := keyPrefix + "lock:task:" + taskID
	hasLock, err := cache.Exists(ctx, d.redis, lockKey)
	if err != nil {
		return nil, fmt.Errorf("check lock key %q: %w", lockKey, err)
	}

	if hasLock {
		// Live lock — task is still being worked on by a worker.
		ev.Decision = "skip"
		ev.Reason = "task lock is live; worker is still active"
		return ev, nil
	}

	// Orphaned running task (no lock). Apply retry/fail logic.
	retryCount, _ := strconv.Atoi(retryStr)

	if retryCount >= maxRetryCount {
		d.logger.Warn().
			Str("task_id", taskID).
			Int("retry_count", retryCount).
			Msg("recovery: max retries exceeded; marking task failed")

		if err := d.setTaskField(ctx, taskID, keyPrefix, "status", "failed"); err != nil {
			return nil, fmt.Errorf("set failed status: %w", err)
		}
		if err := d.setTaskField(ctx, taskID, keyPrefix, "error", "max retries exceeded"); err != nil {
			return nil, fmt.Errorf("set error field: %w", err)
		}
		if err := d.setTaskField(ctx, taskID, keyPrefix, "completed_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
			return nil, fmt.Errorf("set completed_at field: %w", err)
		}
		if rmErr := d.removeFromActiveSet(ctx, taskID, keyPrefix); rmErr != nil {
			d.logger.Warn().Err(rmErr).Str("task_id", taskID).Msg("recovery: failed to remove exhausted task from active set")
		}

		ev.Decision = "fail"
		ev.Reason = fmt.Sprintf("max retries (%d) exceeded", maxRetryCount)
		return ev, nil
	}

	// Re-queue: increment retry_count, reset status, push back onto queue.
	newRetry := strconv.Itoa(retryCount + 1)
	if err := d.setTaskField(ctx, taskID, keyPrefix, "retry_count", newRetry); err != nil {
		return nil, fmt.Errorf("increment retry_count: %w", err)
	}
	if err := d.setTaskField(ctx, taskID, keyPrefix, "status", "queued"); err != nil {
		return nil, fmt.Errorf("reset status to queued: %w", err)
	}

	p := Priority(priority)
	if p != PriorityUrgent && p != PriorityNormal && p != PriorityLow {
		p = PriorityNormal
	}
	if err := d.queue.Submit(ctx, taskID, p); err != nil {
		return nil, fmt.Errorf("re-submit to queue: %w", err)
	}

	ev.Decision = "requeue"
	ev.Reason = fmt.Sprintf("orphaned running task re-queued (retry %d/%d)", retryCount+1, maxRetryCount)
	return ev, nil
}

// removeFromActiveSet removes taskID from the active set. Errors are returned.
func (d *Daemon) removeFromActiveSet(ctx context.Context, taskID, keyPrefix string) error {
	activeSetKey := keyPrefix + "active"
	return cache.SetRemoveMember(ctx, d.redis, activeSetKey, taskID)
}

// getTaskFields reads the status, retry_count, and priority fields for a task.
func (d *Daemon) getTaskFields(ctx context.Context, taskID, keyPrefix string) (map[string]string, error) {
	hashKey := keyPrefix + "task:" + taskID
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

// getTaskStatus is kept for backward compatibility with existing tests.
// New code should use getTaskFields.
func (d *Daemon) getTaskStatus(ctx context.Context, taskID, keyPrefix string) (map[string]string, error) {
	return d.getTaskFields(ctx, taskID, keyPrefix)
}

// setTaskField updates a single field in the task hash {prefix}task:{taskID}.
func (d *Daemon) setTaskField(ctx context.Context, taskID, keyPrefix, field, value string) error {
	hashKey := keyPrefix + "task:" + taskID
	if err := cache.HashSet(ctx, d.redis, hashKey, field, value); err != nil {
		return fmt.Errorf("hash set %q[%q]: %w", hashKey, field, err)
	}
	return nil
}
