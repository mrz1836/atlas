package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cache "github.com/mrz1836/go-cache"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/lifecycle"
	"github.com/mrz1836/atlas/internal/workspace"
)

// Drift type constants for reconciliation reports.
const (
	// DriftTypeRedisOnly indicates the task is in the Redis active set but has no
	// corresponding workspace directory on disk.
	DriftTypeRedisOnly = "redis_only"

	// DriftTypeFileOnly indicates a workspace is active on disk but its task is
	// not tracked in the Redis active set.
	DriftTypeFileOnly = "file_only"

	// DriftTypeStatusMismatch indicates that Redis and the filesystem disagree on
	// the task/workspace status.
	DriftTypeStatusMismatch = "status_mismatch"
)

// ReconcileRequest is the params for daemon.reconcile.
type ReconcileRequest struct {
	// AtlasHome overrides the ~/.atlas directory used for filesystem scanning.
	// Leave empty to use the OS default.
	AtlasHome string `json:"atlas_home,omitempty"`
}

// DriftItem describes a single reconciliation discrepancy between Redis and the filesystem.
type DriftItem struct {
	// Type is one of DriftTypeRedisOnly, DriftTypeFileOnly, DriftTypeStatusMismatch.
	Type string `json:"type"`
	// TaskID is the daemon-assigned task UUID (empty for file-only drift).
	TaskID string `json:"task_id,omitempty"`
	// Workspace is the workspace name.
	Workspace string `json:"workspace,omitempty"`
	// RedisStatus is the canonical label of the status stored in Redis (empty if not in Redis).
	RedisStatus string `json:"redis_status,omitempty"`
	// FileStatus is the status from the filesystem workspace (empty if not on disk).
	FileStatus string `json:"file_status,omitempty"`
	// SuggestedAction is a human-readable remediation hint.
	SuggestedAction string `json:"suggested_action"`
}

// ReconcileResponse is the result for daemon.reconcile.
type ReconcileResponse struct {
	// DriftItems lists all detected discrepancies.
	DriftItems []DriftItem `json:"drift_items"`
	// Total is the count of drift items.
	Total int `json:"total"`
	// Summary is a one-line human-readable overview.
	Summary string `json:"summary"`
	// AtlasHome is the directory scanned for workspace files.
	AtlasHome string `json:"atlas_home"`
}

// workspaceEntry holds the minimal fields the reconcile operation needs.
type workspaceEntry struct {
	Name   string
	Status constants.WorkspaceStatus
}

// defaultWorkspaceLoader is the production workspace loader.
// It opens a FileStore for the given atlasHome and returns all workspace entries.
func defaultWorkspaceLoader(atlasHome string) ([]*workspaceEntry, error) {
	store, err := workspace.NewFileStore(atlasHome)
	if err != nil {
		return nil, err
	}
	wss, err := store.List(context.Background())
	if err != nil {
		return nil, err
	}
	entries := make([]*workspaceEntry, len(wss))
	for i, ws := range wss {
		entries[i] = &workspaceEntry{Name: ws.Name, Status: ws.Status}
	}
	return entries, nil
}

// loadWorkspaces calls the daemon's injectable workspace loader, falling back to
// the production implementation when none has been set.
func (d *Daemon) loadWorkspaces(atlasHome string) ([]*workspaceEntry, error) {
	if d.workspaceLoader != nil {
		return d.workspaceLoader(atlasHome)
	}
	return defaultWorkspaceLoader(atlasHome)
}

// Reconcile walks the Redis active set and the filesystem workspace store,
// then reports any drift with offending IDs and suggested remediation.
func (d *Daemon) Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResponse, error) { //nolint:gocognit // complexity is inherent to multi-source drift detection logic
	keyPrefix := d.cfg.Redis.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "atlas:"
	}

	atlasHome := req.AtlasHome
	if atlasHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ReconcileResponse{}, fmt.Errorf("reconcile: get home dir: %w", err)
		}
		atlasHome = filepath.Join(home, constants.AtlasHome)
	}

	// ── Step 1: collect Redis active-set tasks ──────────────────────────────
	activeSetKey := keyPrefix + "active"
	activeMembers, err := cache.SetMembers(ctx, d.redis, activeSetKey)
	if err != nil {
		return ReconcileResponse{}, fmt.Errorf("reconcile: get active set: %w", err)
	}

	type redisTask struct {
		taskID    string
		workspace string
		status    string
	}
	redisTasks := make([]redisTask, 0, len(activeMembers))
	redisTasksByWS := make(map[string]redisTask)

	for _, taskID := range activeMembers {
		fields, ferr := d.getReconcileTaskFields(ctx, taskID, keyPrefix)
		if ferr != nil {
			d.logger.Warn().Err(ferr).Str("task_id", taskID).Msg("reconcile: failed to read task fields")
			continue
		}
		rt := redisTask{
			taskID:    taskID,
			workspace: fields["workspace"],
			status:    fields["status"],
		}
		redisTasks = append(redisTasks, rt)
		if rt.workspace != "" {
			redisTasksByWS[rt.workspace] = rt
		}
	}

	// ── Step 2: scan filesystem workspaces ──────────────────────────────────
	fsEntries, wsErr := d.loadWorkspaces(atlasHome) //nolint:contextcheck // loadWorkspaces is filesystem-only, no context needed
	if wsErr != nil {
		d.logger.Warn().Err(wsErr).Msg("reconcile: failed to list filesystem workspaces")
		fsEntries = nil
	}

	fsWSByName := make(map[string]constants.WorkspaceStatus)
	for _, entry := range fsEntries {
		fsWSByName[entry.Name] = entry.Status
	}

	var items []DriftItem

	// ── Step 3: Redis-only drift ─────────────────────────────────────────────
	// Tasks in Redis active set whose workspace is missing on disk.
	for _, rt := range redisTasks {
		if rt.workspace == "" {
			continue
		}
		if _, onDisk := fsWSByName[rt.workspace]; !onDisk {
			items = append(items, DriftItem{
				Type:            DriftTypeRedisOnly,
				TaskID:          rt.taskID,
				Workspace:       rt.workspace,
				RedisStatus:     lifecycle.DaemonStateLabel(rt.status),
				SuggestedAction: fmt.Sprintf("workspace %q is not on disk; run 'atlas cleanup' or restart daemon to reconcile", rt.workspace),
			})
		}
	}

	// ── Step 4: file-only drift ──────────────────────────────────────────────
	// Filesystem workspaces that are active but have no task in the Redis active set.
	for _, entry := range fsEntries {
		if entry.Status != constants.WorkspaceStatusActive {
			continue
		}
		if _, inRedis := redisTasksByWS[entry.Name]; !inRedis {
			items = append(items, DriftItem{
				Type:            DriftTypeFileOnly,
				Workspace:       entry.Name,
				FileStatus:      string(entry.Status),
				SuggestedAction: fmt.Sprintf("workspace %q is active on disk but absent from daemon queue; check 'atlas status' or restart 'atlas daemon start'", entry.Name),
			})
		}
	}

	// ── Step 5: status-mismatch detection ───────────────────────────────────
	// Workspaces present in both Redis and filesystem with conflicting states.
	for wsName, rt := range redisTasksByWS {
		if fsStatus, onDisk := fsWSByName[wsName]; onDisk {
			if lifecycle.IsTerminalDaemonState(rt.status) && fsStatus == constants.WorkspaceStatusActive {
				items = append(items, DriftItem{
					Type:            DriftTypeStatusMismatch,
					TaskID:          rt.taskID,
					Workspace:       wsName,
					RedisStatus:     lifecycle.DaemonStateLabel(rt.status),
					FileStatus:      string(fsStatus),
					SuggestedAction: fmt.Sprintf("task is terminal in Redis (%s) but workspace still active on disk; run 'atlas cleanup'", lifecycle.DaemonStateLabel(rt.status)),
				})
			}
		}
	}

	resp := ReconcileResponse{
		DriftItems: items,
		Total:      len(items),
		AtlasHome:  atlasHome,
	}
	if len(items) == 0 {
		resp.Summary = "no drift detected — Redis and filesystem are in sync"
	} else {
		resp.Summary = fmt.Sprintf("%d drift item(s) detected; review items for suggested actions", len(items))
	}

	return resp, nil
}

// getReconcileTaskFields reads the status, workspace, and priority fields for a task.
// This is used by Reconcile to cross-reference Redis tasks with filesystem workspaces.
func (d *Daemon) getReconcileTaskFields(ctx context.Context, taskID, keyPrefix string) (map[string]string, error) {
	hashKey := keyPrefix + "task:" + taskID
	keys := []interface{}{"status", "workspace", "priority"}

	values, err := cache.HashMapGet(ctx, d.redis, hashKey, keys...)
	if err != nil {
		return nil, fmt.Errorf("hash map get %q: %w", hashKey, err)
	}

	result := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(values) {
			if field, ok := k.(string); ok {
				result[field] = values[i]
			}
		}
	}
	return result, nil
}

// handleDaemonReconcile handles the daemon.reconcile JSON-RPC method.
func (d *Daemon) handleDaemonReconcile(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req ReconcileRequest
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("reconcile: invalid params: %w", err)
		}
	}

	resp, err := d.Reconcile(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
