//go:build stress

package steps

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// ====================
// Phase 7: Stress Tests
// ====================

// StressMockRunner is optimized for high-volume testing.
type StressMockRunner struct {
	callCount atomic.Int64
}

func (m *StressMockRunner) ExecuteStep(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	m.callCount.Add(1)
	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
		FilesChanged: []string{"file.go"},
	}, nil
}

// StressMockStateStore is optimized for high-volume state operations.
// It properly isolates state per (taskID, stepName) like the real implementation.
type StressMockStateStore struct {
	saveCount atomic.Int64
	loadCount atomic.Int64
	states    map[string]*domain.LoopState // key: taskID_stepName
	mu        sync.RWMutex
}

func (m *StressMockStateStore) SaveLoopState(_ context.Context, task *domain.Task, state *domain.LoopState) error {
	m.saveCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize map if needed
	if m.states == nil {
		m.states = make(map[string]*domain.LoopState)
	}

	// Make a copy to avoid race
	stateCopy := *state
	// Deep copy the slice
	if state.CompletedIterations != nil {
		stateCopy.CompletedIterations = make([]domain.IterationResult, len(state.CompletedIterations))
		copy(stateCopy.CompletedIterations, state.CompletedIterations)
	}

	// Store per task + step name (like FileStateStore)
	key := task.ID + "_" + state.StepName
	m.states[key] = &stateCopy
	return nil
}

func (m *StressMockStateStore) LoadLoopState(_ context.Context, task *domain.Task, stepName string) (*domain.LoopState, error) {
	m.loadCount.Add(1)
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Load per task + step name (like FileStateStore)
	key := task.ID + "_" + stepName
	loaded := m.states[key]
	if loaded == nil {
		return nil, nil //nolint:nilnil // returning nil state when no state exists is expected behavior
	}

	// Return a deep copy to avoid race conditions
	stateCopy := *loaded
	// Copy the slice to prevent shared slice mutations
	if loaded.CompletedIterations != nil {
		stateCopy.CompletedIterations = make([]domain.IterationResult, len(loaded.CompletedIterations))
		copy(stateCopy.CompletedIterations, loaded.CompletedIterations)
	}
	return &stateCopy, nil
}

func TestLoopStress_ManyIterations(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &StressMockRunner{}
	mockStore := &StressMockStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "stress-many-1", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "stress_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	start := time.Now()
	result, err := executor.Execute(ctx, task, step)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 100, result.Metadata["iterations_completed"])
	assert.Equal(t, int64(100), mockRunner.callCount.Load())
	assert.Equal(t, int64(100), mockStore.saveCount.Load())

	// Performance check: should complete reasonably fast
	t.Logf("100 iterations completed in %v", duration)
	assert.Less(t, duration, 10*time.Second, "100 iterations should complete in under 10 seconds")
}

func TestLoopStress_RapidCancel(t *testing.T) {
	logger := zerolog.Nop()

	const numCycles = 50
	var completedCycles atomic.Int32
	var cancelledCycles atomic.Int32

	for i := 0; i < numCycles; i++ {
		ctx, cancel := context.WithCancel(context.Background())

		// Create long-running mock
		mockRunner := &StressMockRunner{}
		mockStore := &StressMockStateStore{}

		executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

		task := &domain.Task{ID: "stress-cancel", CurrentStep: 0}
		step := &domain.StepDefinition{
			Name: "cancel_loop",
			Type: domain.StepTypeLoop,
			Config: map[string]any{
				"max_iterations": 1000,
				"steps": []any{
					map[string]any{"name": "inner", "type": "ai"},
				},
			},
		}

		// Start execution in goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, err := executor.Execute(ctx, task, step)
			if err != nil {
				cancelledCycles.Add(1)
			} else {
				completedCycles.Add(1)
			}
		}()

		// Cancel after random delay
		time.Sleep(time.Duration(i%5) * time.Millisecond)
		cancel()

		// Wait for completion with timeout
		select {
		case <-done:
			// Good
		case <-time.After(5 * time.Second):
			t.Fatalf("Cycle %d timed out", i)
		}
	}

	t.Logf("Completed: %d, Cancelled: %d", completedCycles.Load(), cancelledCycles.Load())
}

func TestLoopStress_ConcurrentLoops(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	const numLoops = 10
	const iterationsPerLoop = 50

	var wg sync.WaitGroup
	errors := make(chan error, numLoops)
	results := make(chan *domain.StepResult, numLoops)

	for i := 0; i < numLoops; i++ {
		wg.Add(1)
		go func(loopIdx int) {
			defer wg.Done()

			mockRunner := &StressMockRunner{}
			mockStore := &StressMockStateStore{}

			executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

			task := &domain.Task{ID: "stress-concurrent-" + string(rune('0'+loopIdx)), CurrentStep: 0}
			step := &domain.StepDefinition{
				Name: "concurrent_loop",
				Type: domain.StepTypeLoop,
				Config: map[string]any{
					"max_iterations": iterationsPerLoop,
					"steps": []any{
						map[string]any{"name": "inner", "type": "ai"},
					},
				},
			}

			result, err := executor.Execute(ctx, task, step)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}(i)
	}

	// Wait for all loops to complete
	wg.Wait()
	close(errors)
	close(results)

	// Check for errors
	for err := range errors {
		t.Errorf("Loop failed: %v", err)
	}

	// Verify all loops completed correctly
	resultCount := 0
	for result := range results {
		resultCount++
		assert.Equal(t, iterationsPerLoop, result.Metadata["iterations_completed"])
	}
	assert.Equal(t, numLoops, resultCount)
}

func TestLoopStress_MemoryUsage(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Force GC before measurement
	runtime.GC()

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	mockRunner := &StressMockRunner{}
	mockStore := &StressMockStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "stress-memory", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "memory_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 500, // Large number of iterations
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	_, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)

	// Force GC and measure after
	runtime.GC()

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate memory change using signed arithmetic to handle GC
	memChange := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	memChangeMB := float64(memChange) / (1024 * 1024)

	if memChange < 0 {
		t.Logf("Memory decreased after 500 iterations: %.2f MB (GC was effective)", -memChangeMB)
		// Memory decrease is good - test passes
		return
	}

	t.Logf("Memory increase after 500 iterations: %d bytes (%.2f MB)", memChange, memChangeMB)

	// Memory shouldn't grow excessively (less than 50MB for 500 iterations)
	// This checks that we're not leaking memory on each iteration
	maxExpectedMemory := int64(50 * 1024 * 1024) // 50MB
	assert.Less(t, memChange, maxExpectedMemory,
		"Memory increase should be less than 50MB for 500 iterations")
}

func TestLoopStress_CheckpointPerformance(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &StressMockRunner{}
	mockStore := &StressMockStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "stress-checkpoint", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "checkpoint_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	start := time.Now()
	_, err := executor.Execute(ctx, task, step)
	duration := time.Since(start)

	require.NoError(t, err)

	saveCount := mockStore.saveCount.Load()
	t.Logf("100 checkpoints in %v (%.2f ms per checkpoint)",
		duration, float64(duration.Milliseconds())/float64(saveCount))

	// Each iteration should checkpoint
	assert.Equal(t, int64(100), saveCount)

	// Checkpointing shouldn't add significant overhead
	avgCheckpointTime := duration.Milliseconds() / int64(saveCount)
	assert.Less(t, avgCheckpointTime, int64(100),
		"Average checkpoint time should be less than 100ms")
}

func TestLoopStress_ScratchpadPerformance(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &StressMockRunner{}
	mockStore := &StressMockStateStore{}
	scratchpad := NewFileScratchpad(tmpDir+"/scratchpad.json", logger)

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(scratchpad),
	)

	task := &domain.Task{ID: "stress-scratchpad", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "scratchpad_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  100,
			"scratchpad_file": "scratchpad.json",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	start := time.Now()
	_, err := executor.Execute(ctx, task, step)
	duration := time.Since(start)

	require.NoError(t, err)

	t.Logf("100 iterations with scratchpad in %v", duration)

	// Verify scratchpad has all iterations
	data, err := scratchpad.Read()
	require.NoError(t, err)
	assert.Len(t, data.Iterations, 100)

	// Scratchpad operations shouldn't add excessive overhead
	assert.Less(t, duration, 30*time.Second,
		"100 iterations with scratchpad should complete in under 30 seconds")
}

func TestLoopStress_StateStoreHighContention(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Shared state store for all goroutines
	sharedStore := &StressMockStateStore{}

	const numWorkers = 5
	const iterationsPerWorker = 50

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mockRunner := &StressMockRunner{}
			executor := NewLoopExecutor(mockRunner, sharedStore, WithLoopLogger(logger))

			task := &domain.Task{ID: "contention-worker-" + string(rune('a'+workerID)), CurrentStep: 0}
			step := &domain.StepDefinition{
				Name: "contention_loop",
				Type: domain.StepTypeLoop,
				Config: map[string]any{
					"max_iterations": iterationsPerWorker,
					"steps": []any{
						map[string]any{"name": "inner", "type": "ai"},
					},
				},
			}

			_, err := executor.Execute(ctx, task, step)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors (no races or deadlocks)
	errorCount := 0
	for err := range errors {
		t.Errorf("Worker error: %v", err)
		errorCount++
	}
	assert.Equal(t, 0, errorCount, "No errors should occur under high contention")

	// Verify store operations count
	totalSaves := sharedStore.saveCount.Load()
	expectedSaves := int64(numWorkers * iterationsPerWorker)
	t.Logf("Total saves under contention: %d (expected: %d)", totalSaves, expectedSaves)
	assert.Equal(t, expectedSaves, totalSaves)
}

func TestLoopStress_LargeStatePayload(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// Mock runner that generates large outputs
	mockRunner := &MockInnerStepRunner{}
	largeOutput := make([]byte, 10*1024) // 10KB per iteration
	for i := range largeOutput {
		largeOutput[i] = 'x'
	}

	for i := 0; i < 100; i++ {
		mockRunner.Results = append(mockRunner.Results, &domain.StepResult{
			Status:       constants.StepStatusSuccess,
			Output:       string(largeOutput),
			FilesChanged: []string{"file1.go", "file2.go", "file3.go"},
		})
	}

	stateStore := NewFileStateStore(tmpDir)

	executor := NewLoopExecutor(mockRunner, stateStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "stress-large-state", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "large_state_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	start := time.Now()
	result, err := executor.Execute(ctx, task, step)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 100, result.Metadata["iterations_completed"])
	t.Logf("100 iterations with large payloads completed in %v", duration)
}

// BenchmarkLoopExecution provides a microbenchmark for loop execution.
func BenchmarkLoopExecution(b *testing.B) {
	ctx := context.Background()
	logger := zerolog.Nop()

	for i := 0; i < b.N; i++ {
		mockRunner := &StressMockRunner{}
		mockStore := &StressMockStateStore{}

		executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

		task := &domain.Task{ID: "bench", CurrentStep: 0}
		step := &domain.StepDefinition{
			Name: "bench_loop",
			Type: domain.StepTypeLoop,
			Config: map[string]any{
				"max_iterations": 10,
				"steps": []any{
					map[string]any{"name": "inner", "type": "ai"},
				},
			},
		}

		_, _ = executor.Execute(ctx, task, step)
	}
}

// BenchmarkCheckpointSave benchmarks checkpoint save operations.
func BenchmarkCheckpointSave(b *testing.B) {
	ctx := context.Background()
	mockStore := &StressMockStateStore{}

	task := &domain.Task{ID: "bench"}
	state := &domain.LoopState{
		StepName:            "test",
		CurrentIteration:    50,
		MaxIterations:       100,
		CompletedIterations: make([]domain.IterationResult, 50),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mockStore.SaveLoopState(ctx, task, state)
	}
}
