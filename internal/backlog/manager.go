package backlog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

const (
	// backlogDir is the directory name under .atlas for storing discoveries.
	backlogDir = ".atlas/backlog"
	// gitkeepFile is the name of the gitkeep file.
	gitkeepFile = ".gitkeep"
	// fileExtension is the file extension for discovery files.
	fileExtension = ".yaml"
	// maxConcurrent is the maximum number of concurrent file reads.
	maxConcurrent = 50
	// filePerm is the permission for discovery files.
	filePerm = 0o644
	// dirPerm is the permission for the backlog directory.
	dirPerm = 0o755
)

// Manager handles discovery storage operations.
// It provides CRUD operations for discoveries stored as individual YAML files.
type Manager struct {
	// dir is the absolute path to the backlog directory.
	dir string
	// projectRoot is the absolute path to the project root.
	projectRoot string
}

// NewManager creates a new Manager for the given project root.
// If projectRoot is empty, it uses the current working directory.
func NewManager(projectRoot string) (*Manager, error) {
	if projectRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		projectRoot = cwd
	}

	// Convert to absolute path
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project root: %w", err)
	}

	return &Manager{
		dir:         filepath.Join(absRoot, backlogDir),
		projectRoot: absRoot,
	}, nil
}

// Dir returns the absolute path to the backlog directory.
func (m *Manager) Dir() string {
	return m.dir
}

// ProjectRoot returns the absolute path to the project root.
func (m *Manager) ProjectRoot() string {
	return m.projectRoot
}

// EnsureDir creates the backlog directory if it doesn't exist.
// It also creates a .gitkeep file to ensure the directory is tracked by git.
func (m *Manager) EnsureDir() error {
	if err := os.MkdirAll(m.dir, dirPerm); err != nil {
		return fmt.Errorf("failed to create backlog directory: %w", err)
	}

	// Create .gitkeep file
	gitkeepPath := filepath.Join(m.dir, gitkeepFile)
	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitkeepPath, []byte{}, filePerm); err != nil {
			return fmt.Errorf("failed to create .gitkeep: %w", err)
		}
	}

	return nil
}

// Add creates a new discovery and returns it.
// It generates a unique ID, captures git context, and writes the discovery to disk.
func (m *Manager) Add(ctx context.Context, d *Discovery) error {
	// Ensure directory exists
	if err := m.EnsureDir(); err != nil {
		return err
	}

	// Generate ID if not set
	if d.ID == "" {
		id, err := GenerateID()
		if err != nil {
			return fmt.Errorf("failed to generate ID: %w", err)
		}
		d.ID = id
	}

	// Set defaults
	d.SchemaVersion = SchemaVersion
	if d.Status == "" {
		d.Status = StatusPending
	}
	if d.Context.DiscoveredAt.IsZero() {
		d.Context.DiscoveredAt = time.Now().UTC()
	}

	// Capture git context if not already set
	if d.Context.Git == nil {
		gitCtx := m.captureGitContext(ctx)
		if gitCtx != nil {
			d.Context.Git = gitCtx
		}
	}

	// Validate the discovery
	if err := d.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("failed to marshal discovery: %w", err)
	}

	// Write to disk with collision protection
	path := filepath.Join(m.dir, d.ID+fileExtension)
	if err := createSafe(path, data); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", atlaserrors.ErrDuplicateDiscoveryID, d.ID)
		}
		return fmt.Errorf("failed to write discovery: %w", err)
	}

	return nil
}

// Get retrieves a discovery by ID.
func (m *Manager) Get(_ context.Context, id string) (*Discovery, error) {
	path := filepath.Join(m.dir, id+fileExtension)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", atlaserrors.ErrDiscoveryNotFound, id)
	}

	d, err := m.loadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load discovery %s: %w", id, err)
	}

	return d, nil
}

// List returns all discoveries matching the filter.
// It reads files in parallel with bounded concurrency for performance.
func (m *Manager) List(ctx context.Context, filter Filter) ([]*Discovery, error) {
	// Check if directory exists
	if _, err := os.Stat(m.dir); os.IsNotExist(err) {
		return []*Discovery{}, nil
	}

	// Find all discovery files
	pattern := filepath.Join(m.dir, "disc-*"+fileExtension)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	if len(files) == 0 {
		return []*Discovery{}, nil
	}

	// Use worker pool for parallel reads
	type result struct {
		discovery *Discovery
		err       error
	}

	results := make(chan result, len(files))
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for _, file := range files {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()

			// Check for context cancellation
			select {
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			default:
			}

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			d, loadErr := m.loadFile(f)
			if loadErr != nil {
				// Log warning but don't fail the entire list
				results <- result{err: loadErr}
				return
			}

			// Apply filter
			if filter.Match(d) {
				results <- result{discovery: d}
			} else {
				results <- result{} // Filtered out
			}
		}(file)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var discoveries []*Discovery
	for r := range results {
		if r.discovery != nil {
			discoveries = append(discoveries, r.discovery)
		}
		// Silently skip errors (malformed files) - don't break the list
	}

	// Sort by discovered_at descending (newest first)
	sort.Slice(discoveries, func(i, j int) bool {
		return discoveries[i].Context.DiscoveredAt.After(discoveries[j].Context.DiscoveredAt)
	})

	// Apply limit
	if filter.Limit > 0 && len(discoveries) > filter.Limit {
		discoveries = discoveries[:filter.Limit]
	}

	return discoveries, nil
}

// Update saves changes to an existing discovery.
func (m *Manager) Update(_ context.Context, d *Discovery) error {
	path := filepath.Join(m.dir, d.ID+fileExtension)

	// Check if discovery exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", atlaserrors.ErrDiscoveryNotFound, d.ID)
	}

	// Validate the discovery
	if err := d.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("failed to marshal discovery: %w", err)
	}

	// Write to disk (overwrite existing)
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("failed to write discovery: %w", err)
	}

	return nil
}

// Promote changes a discovery's status to promoted with the given task ID.
// Only pending discoveries can be promoted.
func (m *Manager) Promote(ctx context.Context, id, taskID string) (*Discovery, error) {
	d, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate status transition
	if d.Status != StatusPending {
		return nil, fmt.Errorf("%w: can only promote pending discoveries, current status is %q",
			atlaserrors.ErrInvalidStatusTransition, d.Status)
	}

	// Update status and lifecycle
	d.Status = StatusPromoted
	d.Lifecycle.PromotedToTask = taskID

	if err := m.Update(ctx, d); err != nil {
		return nil, err
	}

	return d, nil
}

// Dismiss changes a discovery's status to dismissed with the given reason.
// Only pending discoveries can be dismissed.
func (m *Manager) Dismiss(ctx context.Context, id, reason string) (*Discovery, error) {
	d, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate status transition
	if d.Status != StatusPending {
		return nil, fmt.Errorf("%w: can only dismiss pending discoveries, current status is %q",
			atlaserrors.ErrInvalidStatusTransition, d.Status)
	}

	// Update status and lifecycle
	d.Status = StatusDismissed
	d.Lifecycle.DismissedReason = reason

	if err := m.Update(ctx, d); err != nil {
		return nil, err
	}

	return d, nil
}

// createSafe creates a new file with O_EXCL to prevent overwriting existing files.
// This is used to prevent ID collisions.
func createSafe(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm) //nolint:gosec // path is constructed internally
	if err != nil {
		return err
	}

	writeErr := func() error {
		if _, err := f.Write(data); err != nil {
			return err
		}
		return f.Sync()
	}()

	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// loadFile reads and parses a single discovery YAML file.
func (m *Manager) loadFile(path string) (*Discovery, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from trusted directory
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var d Discovery
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%w: %w", atlaserrors.ErrMalformedDiscovery, err)
	}

	return &d, nil
}

// captureGitContext attempts to capture the current git branch and commit.
// Returns nil if git context cannot be captured (not in a git repo, etc.).
func (m *Manager) captureGitContext(ctx context.Context) *GitContext {
	// Get current branch
	branch, err := git.RunCommand(ctx, m.projectRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil
	}

	// Get current commit (short form)
	commit, err := git.RunCommand(ctx, m.projectRoot, "rev-parse", "--short", "HEAD")
	if err != nil {
		return nil
	}

	return &GitContext{
		Branch: branch,
		Commit: commit,
	}
}
