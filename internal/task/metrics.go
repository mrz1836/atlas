// Package task provides task lifecycle management for ATLAS.
package task

import (
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// Metrics collects metrics about task and step execution.
// Implementations can send these to monitoring systems like Prometheus,
// StatsD, or custom observability platforms.
type Metrics interface {
	// TaskStarted is called when a new task begins execution.
	TaskStarted(taskID, templateName string)

	// TaskCompleted is called when a task finishes (success or failure).
	TaskCompleted(taskID string, duration time.Duration, status string)

	// StepExecuted is called after each step completes.
	StepExecuted(taskID, stepName string, stepType domain.StepType, duration time.Duration, success bool)

	// LoopIteration is called after each loop iteration completes.
	LoopIteration(taskID, stepName string, iteration int, duration time.Duration)
}

// NoopMetrics is a no-op implementation of Metrics for default behavior.
// Use this when metrics collection is not needed.
type NoopMetrics struct{}

// Ensure NoopMetrics implements Metrics interface.
var _ Metrics = (*NoopMetrics)(nil)

// TaskStarted implements Metrics.
func (NoopMetrics) TaskStarted(string, string) {}

// TaskCompleted implements Metrics.
func (NoopMetrics) TaskCompleted(string, time.Duration, string) {}

// StepExecuted implements Metrics.
func (NoopMetrics) StepExecuted(string, string, domain.StepType, time.Duration, bool) {}

// LoopIteration implements Metrics.
func (NoopMetrics) LoopIteration(string, string, int, time.Duration) {}
