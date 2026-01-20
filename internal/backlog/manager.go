package backlog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	// maxDiscoveryFileSize is the maximum allowed size for a discovery file (1MB).
	maxDiscoveryFileSize = 1024 * 1024
	// newFilePrefix is the prefix for new discovery files.
	newFilePrefix = "item-"
	// legacyFilePrefix is the prefix for legacy discovery files.
	legacyFilePrefix = "disc-"
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
// It generates a unique ID and GUID, captures git context, and writes the discovery to disk.
func (m *Manager) Add(ctx context.Context, d *Discovery) error {
	// Ensure directory exists
	if err := m.EnsureDir(); err != nil {
		return err
	}

	// Generate ID and GUID if not set
	if d.ID == "" {
		guid, shortID, err := GenerateID()
		if err != nil {
			return fmt.Errorf("failed to generate ID: %w", err)
		}
		d.GUID = guid
		d.ID = shortID
	} else if d.GUID == "" && !IsLegacyID(d.ID) {
		// If ID is provided but no GUID, generate one for new format IDs
		guid, err := GenerateGUID()
		if err != nil {
			return fmt.Errorf("failed to generate GUID: %w", err)
		}
		d.GUID = guid
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

// List returns all discoveries matching the filter along with any warnings
// about malformed files that could not be loaded.
// It reads files in parallel with bounded concurrency for performance.
//
//nolint:gocognit // complexity justified by parallel file loading with proper error handling
func (m *Manager) List(ctx context.Context, filter Filter) ([]*Discovery, []string, error) {
	// Use ReadDir instead of Glob - single syscall, returns DirEntry without stat
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Discovery{}, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to list backlog directory: %w", err)
	}

	// Filter for discovery files matching item-*.yaml or disc-*.yaml pattern
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && (strings.HasPrefix(name, newFilePrefix) || strings.HasPrefix(name, legacyFilePrefix)) && strings.HasSuffix(name, fileExtension) {
			files = append(files, filepath.Join(m.dir, name))
		}
	}

	if len(files) == 0 {
		return []*Discovery{}, nil, nil
	}

	// Use worker pool for parallel reads
	type result struct {
		discovery *Discovery
		file      string
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
				results <- result{file: f, err: ctx.Err()}
				return
			default:
			}

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			d, loadErr := m.loadFile(f)
			if loadErr != nil {
				// Return error with file path for warning
				results <- result{file: f, err: loadErr}
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

	// Track when cleanup goroutine finishes to prevent goroutine leak
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		wg.Wait()
		close(results)
	}()

	// Collect results with context awareness
	discoveries := make([]*Discovery, 0, len(files))
	var warnings []string
collectLoop:
	for {
		select {
		case r, ok := <-results:
			if !ok {
				break collectLoop
			}
			if r.discovery != nil {
				discoveries = append(discoveries, r.discovery)
			} else if r.err != nil {
				// Collect warnings for malformed files (skip context errors)
				if !errors.Is(r.err, ctx.Err()) {
					warnings = append(warnings, fmt.Sprintf("%s: %v", filepath.Base(r.file), r.err))
				}
			}
		case <-ctx.Done():
			// Drain remaining results to unblock workers
			go func() {
				for range results {
				}
			}()
			<-closeDone
			return nil, nil, ctx.Err()
		}
	}

	// Sort by discovered_at descending (newest first)
	sort.Slice(discoveries, func(i, j int) bool {
		return discoveries[i].Context.DiscoveredAt.After(discoveries[j].Context.DiscoveredAt)
	})

	// Apply limit
	if filter.Limit > 0 && len(discoveries) > filter.Limit {
		discoveries = discoveries[:filter.Limit]
	}

	return discoveries, warnings, nil
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

// Promote stores the task ID in a discovery's lifecycle metadata for planning purposes.
// Status remains 'pending' until the task actually starts.
// Only pending discoveries can be promoted.
func (m *Manager) Promote(ctx context.Context, id, taskID string) (*Discovery, error) {
	d, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate discovery is pending
	if d.Status != StatusPending {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: can only promote pending discoveries, current status is %q",
				atlaserrors.ErrInvalidStatusTransition, d.Status))
	}

	// Store task ID for planning (status changes when task starts)
	d.Lifecycle.PromotedToTask = taskID
	// Status remains 'pending' until task starts with this discovery

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
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: can only dismiss pending discoveries, current status is %q",
				atlaserrors.ErrInvalidStatusTransition, d.Status))
	}

	// Update status and lifecycle
	d.Status = StatusDismissed
	d.Lifecycle.DismissedReason = reason

	if err := m.Update(ctx, d); err != nil {
		return nil, err
	}

	return d, nil
}

// Complete marks a promoted discovery as completed (task approved).
// Only promoted discoveries can be completed.
func (m *Manager) Complete(ctx context.Context, id string) (*Discovery, error) {
	d, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate status transition
	if d.Status != StatusPromoted {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: can only complete promoted discoveries, current status is %q",
				atlaserrors.ErrInvalidStatusTransition, d.Status))
	}

	// Update status and lifecycle
	d.Status = StatusCompleted
	d.Lifecycle.CompletedAt = time.Now().UTC()

	if err := m.Update(ctx, d); err != nil {
		return nil, err
	}

	return d, nil
}

// StartTask marks a discovery as promoted when a task starts working on it.
// Only pending discoveries can be started.
// Idempotent: returns success if already promoted with same task ID (handles resume).
func (m *Manager) StartTask(ctx context.Context, id, taskID string) (*Discovery, error) {
	d, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate status transition
	if d.Status != StatusPending {
		// Allow idempotent calls for task resume scenarios
		if d.Status == StatusPromoted && d.Lifecycle.PromotedToTask == taskID {
			return d, nil
		}
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: can only start pending discoveries, current status is %q",
				atlaserrors.ErrInvalidStatusTransition, d.Status))
	}

	// Update status and lifecycle
	d.Status = StatusPromoted
	d.Lifecycle.PromotedToTask = taskID

	if err := m.Update(ctx, d); err != nil {
		return nil, err
	}

	return d, nil
}

// Delete removes a discovery file from the backlog.
// This is typically called when a workspace is destroyed and we want to clean up
// the associated discovery file (git history provides the audit trail).
func (m *Manager) Delete(_ context.Context, id string) error {
	path := filepath.Join(m.dir, id+fileExtension)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", atlaserrors.ErrDiscoveryNotFound, id)
		}
		return fmt.Errorf("failed to delete discovery: %w", err)
	}
	return nil
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

// PromoteWithOptions promotes a discovery with full options support.
// It generates task configuration from the discovery based on category and severity.
// When opts.DryRun is true, it returns the result without modifying the discovery.
//
// Returns a PromoteResult with the generated task configuration.
func (m *Manager) PromoteWithOptions(ctx context.Context, id string, opts PromoteOptions, aiPromoter *AIPromoter) (*PromoteResult, error) {
	// Load discovery
	d, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate status transition
	if d.Status != StatusPending {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: can only promote pending discoveries, current status is %q",
				atlaserrors.ErrInvalidStatusTransition, d.Status))
	}

	// Check if already promoted to a task
	if d.Lifecycle.PromotedToTask != "" {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: discovery already promoted to task %q",
				atlaserrors.ErrInvalidStatusTransition, d.Lifecycle.PromotedToTask))
	}

	// Build promote result
	result := &PromoteResult{
		Discovery: d,
		DryRun:    opts.DryRun,
	}

	// Generate task configuration from discovery
	var analysis *AIAnalysis

	if opts.UseAI && aiPromoter != nil {
		// Use AI-assisted analysis
		aiCfg := &AIPromoterConfig{
			Agent:           opts.Agent,
			Model:           opts.Model,
			AvailableAgents: opts.AvailableAgents,
		}
		analysis = aiPromoter.AnalyzeWithFallback(ctx, d, aiCfg)
		result.AIAnalysis = analysis
	} else {
		// Use deterministic analysis
		analysis = &AIAnalysis{
			Template:      MapCategoryToTemplate(d.Content.Category, d.Content.Severity),
			Description:   GenerateTaskDescription(d),
			WorkspaceName: SanitizeWorkspaceName(d.Title),
			Priority:      severityToPriority(d.Content.Severity),
			Reasoning:     "Deterministic mapping based on category and severity",
		}
	}

	// Apply overrides from options
	if opts.Template != "" {
		result.TemplateName = opts.Template
	} else {
		result.TemplateName = analysis.Template
	}

	if opts.WorkspaceName != "" {
		result.WorkspaceName = opts.WorkspaceName
	} else {
		result.WorkspaceName = analysis.WorkspaceName
	}

	if opts.Description != "" {
		result.Description = opts.Description
	} else {
		result.Description = analysis.Description
	}

	// Generate branch name based on template
	branchPrefix := getBranchPrefixForTemplate(result.TemplateName)
	result.BranchName = GenerateBranchName(branchPrefix, result.WorkspaceName)

	// If not dry-run, we don't create the task here - that's done by the CLI
	// We just return the configuration for the CLI to use
	// The CLI will create the task and then call Promote() with the task ID

	return result, nil
}

// getBranchPrefixForTemplate returns the git branch prefix for a template name.
func getBranchPrefixForTemplate(templateName string) string {
	switch templateName {
	case "bugfix":
		return "fix"
	case "feature":
		return "feat"
	case "hotfix":
		return "hotfix"
	case "task":
		return "task"
	case "fix":
		return "fix"
	case "commit":
		return "chore"
	default:
		return "task"
	}
}

// loadFile reads and parses a single discovery YAML file.
// It auto-migrates legacy disc-* files to the new item-* format.
func (m *Manager) loadFile(path string) (*Discovery, error) {
	// Check file size before reading to prevent memory exhaustion
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.Size() > maxDiscoveryFileSize {
		return nil, fmt.Errorf("%w: file too large (%d > %d bytes)",
			atlaserrors.ErrMalformedDiscovery, info.Size(), maxDiscoveryFileSize)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from trusted directory
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var d Discovery
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%w: %w", atlaserrors.ErrMalformedDiscovery, err)
	}

	// Auto-migrate legacy disc-* files
	if d.IsLegacy() && d.GUID == "" {
		if err := m.migrateDiscovery(&d, path); err != nil {
			// Log warning but continue with unmigrated discovery
			// Migration failure shouldn't prevent reading
			return &d, nil //nolint:nilerr // intentional: continue with unmigrated discovery on migration failure
		}
	}

	return &d, nil
}

// migrateDiscovery upgrades a legacy discovery to the new format.
// It generates a GUID, derives a new short ID, and updates the file in place.
func (m *Manager) migrateDiscovery(d *Discovery, oldPath string) error {
	// Generate GUID
	guid, err := GenerateGUID()
	if err != nil {
		return fmt.Errorf("failed to generate GUID: %w", err)
	}

	// Derive new short ID from GUID
	newID, err := DeriveShortID(guid)
	if err != nil {
		return fmt.Errorf("failed to derive short ID: %w", err)
	}

	// Update discovery
	oldID := d.ID
	d.GUID = guid
	d.ID = newID
	d.SchemaVersion = SchemaVersion

	// Marshal to YAML
	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("failed to marshal migrated discovery: %w", err)
	}

	// Write to new file path
	newPath := filepath.Join(m.dir, newID+fileExtension)
	if err := createSafe(newPath, data); err != nil {
		// If file already exists, this is a collision - revert and return original
		if os.IsExist(err) {
			d.ID = oldID
			d.GUID = ""
			return fmt.Errorf("%w: new ID %s already exists", atlaserrors.ErrMigrationCollision, newID)
		}
		return fmt.Errorf("failed to write migrated discovery: %w", err)
	}

	// Remove old file only after successful write
	if err := os.Remove(oldPath); err != nil {
		// If we can't remove old file, at least log it but keep the new file
		// This is better than failing the migration entirely
		return nil //nolint:nilerr // intentional: succeed migration even if old file removal fails
	}

	return nil
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
