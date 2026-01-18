package hook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

// maxFileSizeForHash is the maximum file size for computing SHA256 hash.
// Files larger than this limit will have an empty SHA256 field to prevent
// memory exhaustion and long blocking operations during checkpointing.
const maxFileSizeForHash = 10 * 1024 * 1024 // 10MB

// DefaultMaxCheckpoints is the default maximum number of checkpoints to retain.
// Used when config.HookConfig.MaxCheckpoints is not set or <= 0.
const DefaultMaxCheckpoints = 50

// Checkpointer manages checkpoint creation and retrieval.
type Checkpointer struct {
	cfg   *config.HookConfig
	store Store
}

// NewCheckpointer creates a new checkpointer.
func NewCheckpointer(cfg *config.HookConfig, store Store) *Checkpointer {
	return &Checkpointer{
		cfg:   cfg,
		store: store,
	}
}

// CreateCheckpoint creates a new checkpoint with the given trigger and description.
// Automatically captures git state and file snapshots.
// Prunes oldest checkpoints if limit exceeded.
func (c *Checkpointer) CreateCheckpoint(ctx context.Context, hook *domain.Hook, trigger domain.CheckpointTrigger, description string) error {
	checkpoint := domain.StepCheckpoint{
		CheckpointID: GenerateCheckpointID(),
		CreatedAt:    time.Now().UTC(),
		Description:  description,
		Trigger:      trigger,
	}

	// Capture step context if available
	if hook.CurrentStep != nil {
		checkpoint.StepName = hook.CurrentStep.StepName
		checkpoint.StepIndex = hook.CurrentStep.StepIndex

		// Capture file snapshots for touched files
		if len(hook.CurrentStep.FilesTouched) > 0 {
			snapshots := c.captureFileSnapshots(hook.CurrentStep.FilesTouched)
			checkpoint.FilesSnapshot = snapshots
		}

		// Update current checkpoint reference
		hook.CurrentStep.CurrentCheckpointID = checkpoint.CheckpointID
	}

	// Capture git state
	gitBranch, gitCommit, gitDirty := c.captureGitState(ctx)
	checkpoint.GitBranch = gitBranch
	checkpoint.GitCommit = gitCommit
	checkpoint.GitDirty = gitDirty

	// Append checkpoint
	hook.Checkpoints = append(hook.Checkpoints, checkpoint)

	// Prune old checkpoints if over limit
	c.pruneCheckpoints(hook)

	return nil
}

// GetLatestCheckpoint returns the most recent checkpoint, or nil if none.
func (c *Checkpointer) GetLatestCheckpoint(hook *domain.Hook) *domain.StepCheckpoint {
	if len(hook.Checkpoints) == 0 {
		return nil
	}
	return &hook.Checkpoints[len(hook.Checkpoints)-1]
}

// GetCheckpointByID returns a specific checkpoint, or nil if not found.
func (c *Checkpointer) GetCheckpointByID(hook *domain.Hook, checkpointID string) *domain.StepCheckpoint {
	for i := range hook.Checkpoints {
		if hook.Checkpoints[i].CheckpointID == checkpointID {
			return &hook.Checkpoints[i]
		}
	}
	return nil
}

// GenerateCheckpointID creates a unique checkpoint ID.
// Format: ckpt-{uuid8} (e.g., ckpt-a1b2c3d4)
func GenerateCheckpointID() string {
	return "ckpt-" + uuid.New().String()[:8]
}

// gitCommandTimeout is the maximum time allowed for git commands to complete.
// This prevents the interval checkpointer from hanging indefinitely if git
// operations are slow (e.g., large repo, network issues, corrupted index).
const gitCommandTimeout = 5 * time.Second

// captureGitState gets the current git branch, commit, and dirty status.
// Uses a dedicated timeout to prevent blocking the checkpointer goroutine
// if git operations hang.
func (c *Checkpointer) captureGitState(ctx context.Context) (branch, commit string, dirty bool) {
	// Use a dedicated timeout for git operations to prevent hangs
	gitCtx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()

	// Get current branch
	branchCmd := exec.CommandContext(gitCtx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if out, err := branchCmd.Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}

	// Get current commit
	commitCmd := exec.CommandContext(gitCtx, "git", "rev-parse", "--short", "HEAD")
	if out, err := commitCmd.Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}

	// Check if dirty
	statusCmd := exec.CommandContext(gitCtx, "git", "status", "--porcelain")
	if out, err := statusCmd.Output(); err == nil {
		dirty = len(strings.TrimSpace(string(out))) > 0
	}

	return branch, commit, dirty
}

// captureFileSnapshots creates snapshots for the given files.
// Uses streaming hash with size limit to prevent memory exhaustion on large files.
func (c *Checkpointer) captureFileSnapshots(files []string) []domain.FileSnapshot {
	snapshots := make([]domain.FileSnapshot, 0, len(files))

	for _, filePath := range files {
		snapshot := domain.FileSnapshot{
			Path: filePath,
		}

		info, err := os.Stat(filePath)
		if err != nil {
			snapshot.Exists = false
			snapshots = append(snapshots, snapshot)
			continue
		}

		snapshot.Exists = true
		snapshot.Size = info.Size()
		snapshot.ModTime = info.ModTime().UTC().Format(time.RFC3339)

		// Only hash files under the size limit to prevent memory exhaustion
		if info.Size() <= maxFileSizeForHash {
			if hash, err := hashFileStreaming(filePath); err == nil {
				snapshot.SHA256 = hash[:16] // First 16 chars
			}
		}
		// For large files, SHA256 remains empty (intentional - indicates "too large")

		snapshots = append(snapshots, snapshot)
	}

	return snapshots
}

// hashFileStreaming computes SHA256 without loading entire file into memory.
// Returns the full hex-encoded hash string.
func hashFileStreaming(filePath string) (string, error) {
	//nolint:gosec // G304: filePath is from task execution context, validated by caller
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// pruneCheckpoints removes oldest checkpoints if over the configured limit.
func (c *Checkpointer) pruneCheckpoints(hook *domain.Hook) {
	maxCheckpoints := c.cfg.MaxCheckpoints
	if maxCheckpoints <= 0 {
		maxCheckpoints = DefaultMaxCheckpoints
	}

	if len(hook.Checkpoints) > maxCheckpoints {
		hook.Checkpoints = hook.Checkpoints[len(hook.Checkpoints)-maxCheckpoints:]
	}
}

// PruneCheckpoints removes oldest checkpoints if over the default limit.
// This is a package-level function for use in manager.go where there's no Checkpointer instance.
func PruneCheckpoints(hook *domain.Hook, maxCheckpoints int) {
	if maxCheckpoints <= 0 {
		maxCheckpoints = DefaultMaxCheckpoints
	}

	if len(hook.Checkpoints) > maxCheckpoints {
		hook.Checkpoints = hook.Checkpoints[len(hook.Checkpoints)-maxCheckpoints:]
	}
}

// IntervalCheckpointer manages periodic checkpoints during long-running steps.
// It fetches the hook fresh from the store on each interval tick to avoid data races.
// This design ensures thread-safety when other goroutines modify the hook state.
type IntervalCheckpointer struct {
	checkpointer *Checkpointer
	taskID       string // Store taskID instead of hook pointer to avoid data races
	store        Store
	interval     time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewIntervalCheckpointer creates a new interval checkpointer.
// It stores the taskID rather than a hook pointer to ensure thread-safe access.
func NewIntervalCheckpointer(checkpointer *Checkpointer, taskID string, store Store, interval time.Duration) *IntervalCheckpointer {
	return &IntervalCheckpointer{
		checkpointer: checkpointer,
		taskID:       taskID,
		store:        store,
		interval:     interval,
	}
}

// Start begins periodic checkpoint creation.
// Checkpoints are created at the configured interval only when the hook is in step_running state.
func (ic *IntervalCheckpointer) Start(ctx context.Context) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.cancel != nil {
		return // Already running
	}

	ctx, ic.cancel = context.WithCancel(ctx)
	ic.done = make(chan struct{})

	go func() {
		defer close(ic.done)
		defer func() {
			if r := recover(); r != nil {
				// Panic recovered - checkpointing has stopped.
				// The done channel is closed by the outer defer, allowing Stop() to complete.
				// In production, this would be logged: log.Error("interval checkpointer panic", "recover", r)
				_ = r // Explicitly ignored - recovery is sufficient
			}
		}()

		ticker := time.NewTicker(ic.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ic.createIntervalCheckpoint(ctx)
			}
		}
	}()
}

// Stop cancels the interval checkpointer and waits for the goroutine to finish.
// The lock is released before waiting on the done channel to prevent deadlock.
func (ic *IntervalCheckpointer) Stop() {
	ic.mu.Lock()
	if ic.cancel == nil {
		ic.mu.Unlock()
		return
	}
	cancel := ic.cancel
	ic.cancel = nil
	done := ic.done
	ic.mu.Unlock() // Release lock BEFORE blocking wait

	cancel() // Signal cancellation
	<-done   // Wait for goroutine WITHOUT holding lock
}

// createIntervalCheckpoint creates a checkpoint if the hook is in step_running state.
// It uses Update to perform an atomic read-modify-write operation, ensuring thread-safety
// against concurrent modifications from other goroutines.
func (ic *IntervalCheckpointer) createIntervalCheckpoint(ctx context.Context) {
	err := ic.store.Update(ctx, ic.taskID, func(hook *domain.Hook) error {
		// Only create checkpoints when running
		if hook.State != domain.HookStateStepRunning {
			return nil // No-op, not an error
		}

		// Create checkpoint on the hook instance
		// This modifies the hook in-place
		return ic.checkpointer.CreateCheckpoint(ctx, hook, domain.CheckpointTriggerInterval, "Periodic checkpoint")
	})
	if err != nil {
		// Log error in production
		// fmt.Printf("Error creating interval checkpoint: %v\n", err)
		return
	}
}

// CaptureFileSnapshot creates a snapshot for a single file.
// Uses streaming hash with size limit to prevent memory exhaustion on large files.
func CaptureFileSnapshot(filePath string) domain.FileSnapshot {
	snapshot := domain.FileSnapshot{
		Path: filePath,
	}

	info, err := os.Stat(filePath)
	if err != nil {
		snapshot.Exists = false
		return snapshot
	}

	snapshot.Exists = true
	snapshot.Size = info.Size()
	snapshot.ModTime = info.ModTime().UTC().Format(time.RFC3339)

	// Only hash files under the size limit to prevent memory exhaustion
	if info.Size() <= maxFileSizeForHash {
		if hash, err := hashFileStreaming(filePath); err == nil {
			snapshot.SHA256 = hash[:16] // First 16 chars
		}
	}
	// For large files, SHA256 remains empty (intentional - indicates "too large")

	return snapshot
}

// GetCheckpointsForStep returns all checkpoints for a specific step.
func GetCheckpointsForStep(hook *domain.Hook, stepName string) []domain.StepCheckpoint {
	var checkpoints []domain.StepCheckpoint
	for _, cp := range hook.Checkpoints {
		if cp.StepName == stepName {
			checkpoints = append(checkpoints, cp)
		}
	}
	return checkpoints
}

// GetCheckpointsSince returns checkpoints created after the given time.
func GetCheckpointsSince(hook *domain.Hook, since time.Time) []domain.StepCheckpoint {
	var checkpoints []domain.StepCheckpoint
	for _, cp := range hook.Checkpoints {
		if cp.CreatedAt.After(since) {
			checkpoints = append(checkpoints, cp)
		}
	}
	return checkpoints
}

// CountCheckpointsByTrigger returns the count of checkpoints by trigger type.
func CountCheckpointsByTrigger(hook *domain.Hook) map[domain.CheckpointTrigger]int {
	counts := make(map[domain.CheckpointTrigger]int)
	for _, cp := range hook.Checkpoints {
		counts[cp.Trigger]++
	}
	return counts
}

// FindFilesInCheckpoints returns all unique files across all checkpoints.
func FindFilesInCheckpoints(hook *domain.Hook) []string {
	fileSet := make(map[string]bool)
	for _, cp := range hook.Checkpoints {
		for _, snapshot := range cp.FilesSnapshot {
			fileSet[snapshot.Path] = true
		}
	}

	files := make([]string, 0, len(fileSet))
	for f := range fileSet {
		files = append(files, f)
	}
	return files
}

// ResolveAbsolutePath resolves a path relative to a base directory.
func ResolveAbsolutePath(basePath, relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(basePath, relativePath)
}
