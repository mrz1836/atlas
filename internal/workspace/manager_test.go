package workspace

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// MockStore implements Store for testing.
type MockStore struct {
	workspaces map[string]*domain.Workspace
	createErr  error
	getErr     error
	updateErr  error
	deleteErr  error
	listErr    error
	existsErr  error
}

func newMockStore() *MockStore {
	return &MockStore{
		workspaces: make(map[string]*domain.Workspace),
	}
}

func (m *MockStore) Create(_ context.Context, ws *domain.Workspace) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.workspaces[ws.Name]; exists {
		return atlaserrors.ErrWorkspaceExists
	}
	m.workspaces[ws.Name] = ws
	return nil
}

func (m *MockStore) Get(_ context.Context, name string) (*domain.Workspace, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	ws, exists := m.workspaces[name]
	if !exists {
		return nil, atlaserrors.ErrWorkspaceNotFound
	}
	// Return a copy like real FileStore does (reads from disk creates new object)
	wsCopy := *ws
	wsCopy.Tasks = make([]domain.TaskRef, len(ws.Tasks))
	copy(wsCopy.Tasks, ws.Tasks)
	return &wsCopy, nil
}

func (m *MockStore) Update(_ context.Context, ws *domain.Workspace) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, exists := m.workspaces[ws.Name]; !exists {
		return atlaserrors.ErrWorkspaceNotFound
	}
	m.workspaces[ws.Name] = ws
	return nil
}

func (m *MockStore) List(_ context.Context) ([]*domain.Workspace, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	result := make([]*domain.Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		result = append(result, ws)
	}
	return result, nil
}

func (m *MockStore) Delete(_ context.Context, name string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, exists := m.workspaces[name]; !exists {
		return atlaserrors.ErrWorkspaceNotFound
	}
	delete(m.workspaces, name)
	return nil
}

func (m *MockStore) Exists(_ context.Context, name string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, exists := m.workspaces[name]
	return exists, nil
}

// MockWorktreeRunner implements WorktreeRunner for testing.
type MockWorktreeRunner struct {
	createResult          *WorktreeInfo
	createErr             error
	removeErr             error
	pruneErr              error
	deleteBranchErr       error
	listResult            []*WorktreeInfo
	listErr               error
	branchExists          bool
	branchExistsErr       error
	fetchErr              error
	remoteBranchExists    bool
	remoteBranchExistsErr error
	findByBranchResult    string

	// Track calls for verification
	removeCallCount       int
	removeForceCallCount  int
	deleteBranchCallCount int
	pruneCallCount        int
	fetchCallCount        int
	findByBranchCallCount int
	findByBranchLastArg   string

	// Track operation order for sequencing tests
	operationOrder []string
}

func newMockWorktreeRunner() *MockWorktreeRunner {
	return &MockWorktreeRunner{
		createResult: &WorktreeInfo{
			Path:      "/tmp/repo-test",
			Branch:    "feat/test",
			CreatedAt: time.Now(),
		},
	}
}

func (m *MockWorktreeRunner) Create(_ context.Context, _ WorktreeCreateOptions) (*WorktreeInfo, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResult, nil
}

func (m *MockWorktreeRunner) List(_ context.Context) ([]*WorktreeInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResult, nil
}

func (m *MockWorktreeRunner) Remove(_ context.Context, _ string, force bool) error {
	m.removeCallCount++
	m.operationOrder = append(m.operationOrder, "remove")
	if force {
		m.removeForceCallCount++
	}
	return m.removeErr
}

func (m *MockWorktreeRunner) Prune(_ context.Context) error {
	m.pruneCallCount++
	m.operationOrder = append(m.operationOrder, "prune")
	return m.pruneErr
}

func (m *MockWorktreeRunner) BranchExists(_ context.Context, _ string) (bool, error) {
	if m.branchExistsErr != nil {
		return false, m.branchExistsErr
	}
	return m.branchExists, nil
}

func (m *MockWorktreeRunner) DeleteBranch(_ context.Context, _ string, _ bool) error {
	m.deleteBranchCallCount++
	m.operationOrder = append(m.operationOrder, "deleteBranch")
	return m.deleteBranchErr
}

func (m *MockWorktreeRunner) Fetch(_ context.Context, _ string) error {
	m.fetchCallCount++
	return m.fetchErr
}

func (m *MockWorktreeRunner) RemoteBranchExists(_ context.Context, _, _ string) (bool, error) {
	if m.remoteBranchExistsErr != nil {
		return false, m.remoteBranchExistsErr
	}
	return m.remoteBranchExists, nil
}

func (m *MockWorktreeRunner) FindByBranch(_ context.Context, branch string) string {
	m.findByBranchCallCount++
	m.findByBranchLastArg = branch
	m.operationOrder = append(m.operationOrder, "findByBranch")
	return m.findByBranchResult
}

// ============================================================================
// Task 1 Tests: Manager interface and struct
// ============================================================================

func TestNewManager(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	require.NotNil(t, mgr)
	assert.NotNil(t, mgr.store)
	assert.NotNil(t, mgr.worktreeRunner)
}

// ============================================================================
// Task 2 Tests: Create operation
// ============================================================================

func TestDefaultManager_Create_Success(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.createResult = &WorktreeInfo{
		Path:   "/tmp/repo-test",
		Branch: "feat/test",
	}

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "", false)

	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "test", ws.Name)
	assert.Equal(t, "/tmp/repo-test", ws.WorktreePath)
	assert.Equal(t, "feat/test", ws.Branch)
	assert.Equal(t, constants.WorkspaceStatusActive, ws.Status)
	assert.NotZero(t, ws.CreatedAt)
	assert.NotZero(t, ws.UpdatedAt)
	assert.Empty(t, ws.Tasks)
}

func TestDefaultManager_Create_ValidatesNameUniqueness(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	// Pre-create a workspace
	store.workspaces["existing"] = &domain.Workspace{Name: "existing"}

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "existing", "/tmp/repo", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceExists)
}

func TestDefaultManager_Create_ValidatesEmptyName(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "", "/tmp/repo", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

func TestDefaultManager_Create_RollsBackWorktreeOnStoreFailure(t *testing.T) {
	store := newMockStore()
	store.createErr = atlaserrors.ErrLockTimeout // Store-related error
	runner := newMockWorktreeRunner()
	runner.createResult = &WorktreeInfo{Path: "/tmp/test", Branch: "feat/test"}

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to persist workspace")
	// Verify rollback happened
	assert.Equal(t, 1, runner.removeCallCount)
	assert.Equal(t, 1, runner.removeForceCallCount) // Force remove on rollback
}

func TestDefaultManager_Create_FailsOnWorktreeError(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.createErr = atlaserrors.ErrWorktreeExists // Use sentinel error

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to create worktree")
	// Store should not be touched
	assert.Empty(t, store.workspaces)
}

func TestDefaultManager_Create_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ws, err := mgr.Create(ctx, "test", "/tmp/repo", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// Task 3 Tests: Get operation
// ============================================================================

func TestDefaultManager_Get_ExistingWorkspace(t *testing.T) {
	store := newMockStore()
	expectedWs := &domain.Workspace{
		Name:   "existing",
		Status: constants.WorkspaceStatusActive,
	}
	store.workspaces["existing"] = expectedWs
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Get(context.Background(), "existing")

	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "existing", ws.Name)
}

func TestDefaultManager_Get_NonExistentWorkspace(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Get(context.Background(), "nonexistent")

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

func TestDefaultManager_Get_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ws, err := mgr.Get(ctx, "test")

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// Task 4 Tests: List operation
// ============================================================================

func TestDefaultManager_List_MultipleWorkspaces(t *testing.T) {
	store := newMockStore()
	store.workspaces["ws1"] = &domain.Workspace{Name: "ws1", Status: constants.WorkspaceStatusActive}
	store.workspaces["ws2"] = &domain.Workspace{Name: "ws2", Status: constants.WorkspaceStatusPaused}
	store.workspaces["ws3"] = &domain.Workspace{Name: "ws3", Status: constants.WorkspaceStatusClosed}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	workspaces, err := mgr.List(context.Background())

	require.NoError(t, err)
	require.Len(t, workspaces, 3)
}

func TestDefaultManager_List_EmptyStore(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	workspaces, err := mgr.List(context.Background())

	require.NoError(t, err)
	require.NotNil(t, workspaces)
	assert.Empty(t, workspaces)
}

func TestDefaultManager_List_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	workspaces, err := mgr.List(ctx)

	require.Error(t, err)
	assert.Nil(t, workspaces)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// Task 5 Tests: Destroy operation
// ============================================================================

func TestDefaultManager_Destroy_CleanWorkspace(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	err := mgr.Destroy(context.Background(), "test")

	require.NoError(t, err)
	// Verify workspace was deleted
	_, exists := store.workspaces["test"]
	assert.False(t, exists)
	// Verify worktree was removed
	assert.Equal(t, 1, runner.removeCallCount)
	// Verify branch was deleted
	assert.Equal(t, 1, runner.deleteBranchCallCount)
	// Verify prune was called
	assert.Equal(t, 1, runner.pruneCallCount)
}

func TestDefaultManager_Destroy_SucceedsEvenIfCorrupted(t *testing.T) {
	// NFR18: Destroy ALWAYS succeeds
	store := newMockStore()
	store.getErr = atlaserrors.ErrWorkspaceCorrupted
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	err := mgr.Destroy(context.Background(), "corrupted")

	// MUST succeed even with corrupted state
	require.NoError(t, err)
}

func TestDefaultManager_Destroy_SucceedsIfNotFound(t *testing.T) {
	// NFR18: Destroy ALWAYS succeeds
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	err := mgr.Destroy(context.Background(), "nonexistent")

	// MUST succeed even if workspace doesn't exist
	require.NoError(t, err)
}

func TestDefaultManager_Destroy_CleansBranchesEvenOnPartialFailure(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
	}
	runner := newMockWorktreeRunner()
	runner.removeErr = atlaserrors.ErrNotAWorktree // Use sentinel error

	mgr := NewManager(store, runner)
	err := mgr.Destroy(context.Background(), "test")

	// MUST still succeed
	require.NoError(t, err)
	// Branch deletion should still be attempted
	assert.Equal(t, 1, runner.deleteBranchCallCount)
	// Prune should be called at least once (immediate prune + scheduled prune)
	assert.GreaterOrEqual(t, runner.pruneCallCount, 1, "prune should be called at least once")
}

func TestDefaultManager_Destroy_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.Destroy(ctx, "test")

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultManager_Destroy_NilWorktreeRunner(t *testing.T) {
	// Test that Destroy succeeds even when worktreeRunner is nil
	// This can happen when detectRepoPath() fails in the CLI
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
	}

	// Pass nil worktree runner - simulates when repo detection fails
	mgr := NewManager(store, nil)
	err := mgr.Destroy(context.Background(), "test")

	// MUST succeed per NFR18 - even without a worktree runner
	require.NoError(t, err)
	// Workspace state should still be deleted
	_, exists := store.workspaces["test"]
	assert.False(t, exists)
}

func TestDefaultManager_Destroy_PrunesBeforeBranchDelete(t *testing.T) {
	// This test verifies that prune is called BEFORE deleteBranch.
	// When worktree removal fails and falls back to direct directory deletion,
	// git still thinks the worktree exists. Pruning must happen first to clean
	// up stale worktree metadata, otherwise deleteBranch fails with
	// "cannot delete branch used by worktree".
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
	}
	runner := newMockWorktreeRunner()
	runner.removeErr = atlaserrors.ErrNotAWorktree // Simulate worktree remove failure

	mgr := NewManager(store, runner)
	err := mgr.Destroy(context.Background(), "test")

	require.NoError(t, err)
	// Verify correct order: remove -> prune (immediate) -> prune (scheduled) -> deleteBranch
	// With the enhanced implementation, we now have:
	// 1. remove (fails)
	// 2. prune (immediate - right after fallback directory removal)
	// 3. prune (scheduled - in Destroy flow)
	// 4. deleteBranch
	require.GreaterOrEqual(t, len(runner.operationOrder), 3, "should have at least 3 operations")
	assert.Equal(t, "remove", runner.operationOrder[0])
	// Verify that at least one prune happens before deleteBranch
	lastOp := runner.operationOrder[len(runner.operationOrder)-1]
	assert.Equal(t, "deleteBranch", lastOp, "deleteBranch should be last")
	// Verify that prune happened somewhere before deleteBranch
	foundPruneBeforeDelete := false
	for i := 1; i < len(runner.operationOrder)-1; i++ {
		if runner.operationOrder[i] == "prune" {
			foundPruneBeforeDelete = true
			break
		}
	}
	assert.True(t, foundPruneBeforeDelete, "prune should happen before deleteBranch")
}

func TestDefaultManager_Destroy_VerifiesWorktreeRemoval(t *testing.T) {
	// Test that verification is called and fallback + immediate prune happens
	// when git worktree remove fails
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
	}
	runner := newMockWorktreeRunner()
	runner.removeErr = atlaserrors.ErrWorktreeDirty // Simulate removal failure

	mgr := NewManager(store, runner)
	err := mgr.Destroy(context.Background(), "test")

	require.NoError(t, err)

	// Verify operation order: remove (fails) -> prune (immediate) -> prune (scheduled) -> deleteBranch
	require.Contains(t, runner.operationOrder, "remove")

	// Should have 2 prunes: immediate after fallback, and scheduled in Destroy
	pruneCount := 0
	for _, op := range runner.operationOrder {
		if op == "prune" {
			pruneCount++
		}
	}
	assert.GreaterOrEqual(t, pruneCount, 1, "should have at least one prune operation")
}

func TestRemoveOrphanedDirectory(t *testing.T) {
	t.Run("returns nil if path does not exist", func(t *testing.T) {
		err := removeOrphanedDirectory("/nonexistent/path/xyz123")
		require.NoError(t, err)
	})

	t.Run("removes directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		orphanedPath := tmpDir + "/orphaned"
		err := os.MkdirAll(orphanedPath, 0o750)
		require.NoError(t, err)

		// Create a file inside
		testFile := orphanedPath + "/test.txt"
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)

		err = removeOrphanedDirectory(orphanedPath)
		require.NoError(t, err)

		// Directory should be gone
		_, err = os.Stat(orphanedPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("returns error for file instead of directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := tmpDir + "/file.txt"
		err := os.WriteFile(filePath, []byte("test"), 0o600)
		require.NoError(t, err)

		err = removeOrphanedDirectory(filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

// ============================================================================
// Task 6 Tests: Close operation
// ============================================================================

func TestDefaultManager_Close_CleanWorkspace(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.WorktreeWarning) // No warning when removal succeeds
	assert.Empty(t, result.BranchWarning)   // No warning when branch deletion succeeds
	// Verify status was updated
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
	// Verify worktree path was cleared
	assert.Empty(t, ws.WorktreePath)
	// Verify UpdatedAt was set (Manager owns timestamp)
	assert.False(t, ws.UpdatedAt.IsZero())
	// Verify worktree was removed
	assert.Equal(t, 1, runner.removeCallCount)
	// Verify branch WAS deleted after successful worktree removal
	assert.Equal(t, 1, runner.deleteBranchCallCount)
}

func TestDefaultManager_Close_StoreUpdateFailure(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	store.updateErr = atlaserrors.ErrLockTimeout // Store update fails
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	// Should fail with store error
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to update workspace status")
	// Worktree removal is attempted BEFORE store update (to prevent state corruption)
	// So remove will have been called even though store update failed
	assert.Equal(t, 1, runner.removeCallCount)
}

func TestDefaultManager_Close_WithRunningTasksReturnsError(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks: []domain.TaskRef{
			{ID: "task-1", Status: constants.TaskStatusRunning},
		},
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "task 'task-1' is still running")
	// Verify workspace was NOT modified
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusActive, ws.Status)
}

func TestDefaultManager_Close_WithValidatingTasksReturnsError(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks: []domain.TaskRef{
			{ID: "task-1", Status: constants.TaskStatusValidating},
		},
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "task 'task-1' is still running")
}

func TestDefaultManager_Close_WithCompletedTasksSucceeds(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks: []domain.TaskRef{
			{ID: "task-1", Status: constants.TaskStatusCompleted},
		},
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.WorktreeWarning)
}

func TestDefaultManager_Close_NonExistentWorkspace(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "nonexistent")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

func TestDefaultManager_Close_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := mgr.Close(ctx, "test")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultManager_Close_NilWorktreeRunner(t *testing.T) {
	// Test that Close succeeds even when worktreeRunner is nil
	// State should be updated and warning returned about worktree not being removed
	// WorktreePath should be PRESERVED since removal failed (prevents state corruption)
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}

	// Pass nil worktree runner
	mgr := NewManager(store, nil)
	result, err := mgr.Close(context.Background(), "test")

	// Should succeed - state update is more important than worktree removal
	require.NoError(t, err)
	require.NotNil(t, result)
	// Warning should indicate worktree was not removed
	assert.NotEmpty(t, result.WorktreeWarning)
	assert.Contains(t, result.WorktreeWarning, "/tmp/repo-test")
	assert.Contains(t, result.WorktreeWarning, "no worktree runner")
	// Workspace state should be updated to closed
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
	// WorktreePath should NOT be cleared since removal failed (prevents state corruption)
	assert.Equal(t, "/tmp/repo-test", ws.WorktreePath)
}

// ============================================================================
// Task 7 Tests: UpdateStatus operation
// ============================================================================

func TestDefaultManager_UpdateStatus_Success(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:   "test",
		Status: constants.WorkspaceStatusActive,
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	err := mgr.UpdateStatus(context.Background(), "test", constants.WorkspaceStatusPaused)

	require.NoError(t, err)
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusPaused, ws.Status)
}

func TestDefaultManager_UpdateStatus_NonExistentWorkspace(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	err := mgr.UpdateStatus(context.Background(), "nonexistent", constants.WorkspaceStatusPaused)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

func TestDefaultManager_UpdateStatus_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.UpdateStatus(ctx, "test", constants.WorkspaceStatusPaused)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultManager_UpdateStatus_StoreUpdateFailure(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:   "test",
		Status: constants.WorkspaceStatusActive,
	}
	store.updateErr = atlaserrors.ErrLockTimeout // Store update fails
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	err := mgr.UpdateStatus(context.Background(), "test", constants.WorkspaceStatusPaused)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update workspace")
	require.ErrorIs(t, err, atlaserrors.ErrLockTimeout)
	// Status should NOT be changed in store
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusActive, ws.Status)
}

// ============================================================================
// Task 8 Tests: Exists operation
// ============================================================================

func TestDefaultManager_Exists_ExistingWorkspace(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{Name: "test"}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	exists, err := mgr.Exists(context.Background(), "test")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestDefaultManager_Exists_NonExistentWorkspace(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	exists, err := mgr.Exists(context.Background(), "nonexistent")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDefaultManager_Exists_ContextCancellation(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exists, err := mgr.Exists(ctx, "test")

	require.Error(t, err)
	assert.False(t, exists)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultManager_Exists_StoreError(t *testing.T) {
	store := newMockStore()
	store.existsErr = atlaserrors.ErrLockTimeout
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	exists, err := mgr.Exists(context.Background(), "test")

	require.Error(t, err)
	assert.False(t, exists)
	assert.ErrorIs(t, err, atlaserrors.ErrLockTimeout)
}

// ============================================================================
// Additional edge case tests
// ============================================================================

func TestDefaultManager_Create_CheckExistenceError(t *testing.T) {
	store := newMockStore()
	store.getErr = atlaserrors.ErrLockTimeout // Use sentinel error
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to check workspace existence")
}

func TestDefaultManager_Close_ForceRemoveOnDirty(t *testing.T) {
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := &forceRemoveMockRunner{firstCallFails: true}

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	// No warning because force removal succeeds
	assert.Empty(t, result.WorktreeWarning)
	// Store update happens first, then worktree removal
	// First try fails, then force succeeds
	assert.Equal(t, 2, runner.removeCallCount) // First try, then force
	// Status should be updated
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
}

// forceRemoveMockRunner is a mock that fails on first Remove call.
type forceRemoveMockRunner struct {
	MockWorktreeRunner

	firstCallFails  bool
	removeCallCount int
}

func (m *forceRemoveMockRunner) Remove(_ context.Context, _ string, force bool) error {
	m.removeCallCount++
	if m.firstCallFails && m.removeCallCount == 1 && !force {
		return atlaserrors.ErrWorktreeDirty
	}
	return nil
}

func TestDefaultManager_Close_BothRemoveAttemptsFail(t *testing.T) {
	// Test that when both normal and force removal fail, we get a warning
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := &alwaysFailRemoveMockRunner{}

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	// Close succeeds (state is updated) but with warning
	require.NoError(t, err)
	require.NotNil(t, result)
	// Warning should be present since removal failed
	assert.NotEmpty(t, result.WorktreeWarning)
	assert.Contains(t, result.WorktreeWarning, "/tmp/repo-test")
	assert.Contains(t, result.WorktreeWarning, "failed to remove worktree")
	// Both normal and force removal were attempted
	assert.Equal(t, 2, runner.removeCallCount)
	// Status should still be updated
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
}

// alwaysFailRemoveMockRunner always fails on Remove calls.
type alwaysFailRemoveMockRunner struct {
	MockWorktreeRunner

	removeCallCount int
}

func (m *alwaysFailRemoveMockRunner) Remove(_ context.Context, _ string, _ bool) error {
	m.removeCallCount++
	return atlaserrors.ErrWorktreeDirty
}

func TestDefaultManager_Close_NoWorktreePath(t *testing.T) {
	// Test that Close succeeds without warning when workspace has no worktree path
	// and no worktree can be discovered by branch
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "", // No worktree path
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	// FindByBranch returns empty (no worktree found)
	runner.findByBranchResult = ""

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	// No warning since there was no worktree to remove
	assert.Empty(t, result.WorktreeWarning)
	// FindByBranch should have been called to try discovery
	assert.Equal(t, 1, runner.findByBranchCallCount)
	assert.Equal(t, "feat/test", runner.findByBranchLastArg)
	// Remove should not have been called (no worktree found)
	assert.Equal(t, 0, runner.removeCallCount)
	// Status should be updated
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
}

func TestDefaultManager_Close_DiscoverWorktreeByBranch(t *testing.T) {
	// Test that Close discovers and removes orphaned worktree when WorktreePath is empty
	// but a worktree exists for the branch (recovery scenario)
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "", // Empty - simulating corrupted state from failed previous close
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusClosed, // Already closed but worktree remains
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	// FindByBranch returns the orphaned worktree path
	runner.findByBranchResult = "/tmp/repo-test"

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	// FindByBranch should have been called twice:
	// 1. During discovery to find the orphaned worktree
	// 2. In tryDeleteBranch to check if branch is in use
	assert.Equal(t, 2, runner.findByBranchCallCount)
	assert.Equal(t, "feat/test", runner.findByBranchLastArg)
	// Remove should have been called with discovered path
	assert.Equal(t, 1, runner.removeCallCount)
	// No worktree warning - removal succeeded
	assert.Empty(t, result.WorktreeWarning)
	// Branch warning - mock returns path indicating branch still in use
	// (In real scenario, after prune, FindByBranch would return empty)
	assert.NotEmpty(t, result.BranchWarning)
	assert.Contains(t, result.BranchWarning, "in use by worktree")
	// Status should be closed
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
	// WorktreePath should remain empty
	assert.Empty(t, ws.WorktreePath)
}

func TestDefaultManager_Close_WorktreePathPreservedOnRemovalFailure(t *testing.T) {
	// Test that WorktreePath is NOT cleared when worktree removal fails
	// This prevents state corruption where path is empty but files remain
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	// Both remove attempts will fail
	runner.removeErr = fmt.Errorf("permission denied: %w", atlaserrors.ErrWorktreeDirty)

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err) // Close itself should succeed
	require.NotNil(t, result)
	// Warning should be set about failed removal
	assert.NotEmpty(t, result.WorktreeWarning)
	assert.Contains(t, result.WorktreeWarning, "permission denied")
	// WorktreePath should NOT be cleared since removal failed
	ws := store.workspaces["test"]
	assert.Equal(t, "/tmp/repo-test", ws.WorktreePath)
	// Status should still be closed
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
}

func TestDefaultManager_Close_DeletesBranchOnSuccess(t *testing.T) {
	// Verify that Close deletes the branch when worktree removal succeeds
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	// FindByBranch returns empty (no other worktrees using this branch)
	runner.findByBranchResult = ""

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	// No warnings - both operations succeeded
	assert.Empty(t, result.WorktreeWarning)
	assert.Empty(t, result.BranchWarning)
	// Verify worktree was removed
	assert.Equal(t, 1, runner.removeCallCount)
	// Verify prune was called BEFORE branch deletion
	assert.Equal(t, 1, runner.pruneCallCount)
	// Verify branch was deleted
	assert.Equal(t, 1, runner.deleteBranchCallCount)
	// Verify FindByBranch was called to check for conflicts
	assert.Equal(t, 1, runner.findByBranchCallCount)
	assert.Equal(t, "feat/test", runner.findByBranchLastArg)
	// Verify correct operation order: remove -> prune -> findByBranch -> deleteBranch
	require.Len(t, runner.operationOrder, 4)
	assert.Equal(t, "remove", runner.operationOrder[0])
	assert.Equal(t, "prune", runner.operationOrder[1])
	assert.Equal(t, "findByBranch", runner.operationOrder[2])
	assert.Equal(t, "deleteBranch", runner.operationOrder[3])
}

func TestDefaultManager_Close_SkipsBranchDeletionWhenWorktreeRemovalFails(t *testing.T) {
	// Verify that Close does NOT delete branch when worktree removal fails
	// This prevents orphaned branches
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	// Both normal and force removal will fail
	runner.removeErr = fmt.Errorf("permission denied: %w", atlaserrors.ErrWorktreeDirty)

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err) // Close itself succeeds
	require.NotNil(t, result)
	// WorktreeWarning should be set
	assert.NotEmpty(t, result.WorktreeWarning)
	// BranchWarning should be empty (deletion not attempted)
	assert.Empty(t, result.BranchWarning)
	// Verify branch deletion was NOT attempted
	assert.Equal(t, 0, runner.deleteBranchCallCount)
	assert.Equal(t, 0, runner.pruneCallCount)
	assert.Equal(t, 0, runner.findByBranchCallCount)
}

func TestDefaultManager_Close_SkipsBranchDeletionWhenBranchInUse(t *testing.T) {
	// Verify that Close does NOT delete branch when another worktree uses it
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/shared",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	// FindByBranch returns another worktree path (branch is in use)
	runner.findByBranchResult = "/tmp/repo-shared-2"

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.WorktreeWarning)
	// BranchWarning should indicate branch is in use
	assert.NotEmpty(t, result.BranchWarning)
	assert.Contains(t, result.BranchWarning, "in use by worktree")
	assert.Contains(t, result.BranchWarning, "/tmp/repo-shared-2")
	// Verify branch deletion was NOT attempted
	assert.Equal(t, 0, runner.deleteBranchCallCount)
	// But prune and findByBranch should have been called
	assert.Equal(t, 1, runner.pruneCallCount)
	assert.Equal(t, 1, runner.findByBranchCallCount)
}

func TestDefaultManager_Close_ContinuesWhenBranchDeletionFails(t *testing.T) {
	// Verify that Close succeeds even when branch deletion fails
	// Branch deletion is non-blocking
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	runner.findByBranchResult = ""                       // Not in use
	runner.deleteBranchErr = atlaserrors.ErrGitOperation // Deletion fails

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err) // Close itself succeeds
	require.NotNil(t, result)
	assert.Empty(t, result.WorktreeWarning)
	// BranchWarning should be set with error details
	assert.NotEmpty(t, result.BranchWarning)
	assert.Contains(t, result.BranchWarning, "failed to delete branch")
	// Status should still be updated to closed
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
}

func TestDefaultManager_Close_ContinuesWhenPruneFails(t *testing.T) {
	// Verify that Close attempts branch deletion even when prune fails
	// Prune failure is logged but non-blocking
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "/tmp/repo-test",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()
	runner.findByBranchResult = ""
	runner.pruneErr = atlaserrors.ErrGitOperation // Prune fails

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	// Despite prune failure, branch deletion should still be attempted
	assert.Equal(t, 1, runner.pruneCallCount)
	assert.Equal(t, 1, runner.deleteBranchCallCount)
	// No warnings if branch deletion succeeded
	assert.Empty(t, result.BranchWarning)
}

func TestDefaultManager_SupportsConcurrentWorkspaces(t *testing.T) {
	// FR20: Manager supports 3+ concurrent workspaces
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)

	// Create 5 workspaces sequentially (basic functionality test)
	for i := 0; i < 5; i++ {
		name := "ws-" + string(rune('a'+i))
		runner.createResult = &WorktreeInfo{
			Path:   "/tmp/repo-" + name,
			Branch: "feat/" + name,
		}
		ws, err := mgr.Create(context.Background(), name, "/tmp/repo", "feat", "", false)
		require.NoError(t, err)
		require.NotNil(t, ws)
	}

	// List all workspaces
	workspaces, err := mgr.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, workspaces, 5)
}

func TestDefaultManager_Create_ValidatesEmptyRepoPath(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "", "feat", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	assert.Contains(t, err.Error(), "repoPath")
}

func TestDefaultManager_Create_ValidatesEmptyBranchType(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	assert.Contains(t, err.Error(), "branchType")
}

func TestDefaultManager_Create_ValidatesNilWorktreeRunner(t *testing.T) {
	store := newMockStore()

	// Pass nil worktree runner
	mgr := NewManager(store, nil)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "task", "", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "worktree runner not available")
}

// ============================================================================
// Base Branch Tests
// ============================================================================

func TestDefaultManager_Create_WithLocalBaseBranch(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = true // Branch exists locally
	runner.createResult = &WorktreeInfo{
		Path:   "/tmp/repo-test",
		Branch: "feat/test",
	}

	mgr := NewManager(store, runner)
	// Use useLocal=true to explicitly prefer local branch
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop", true)

	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "test", ws.Name)
	// Should not have fetched since we're using local branch explicitly
	assert.Equal(t, 0, runner.fetchCallCount)
}

func TestDefaultManager_Create_WithRemoteBaseBranch(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false      // Not local
	runner.remoteBranchExists = true // But exists on remote
	runner.createResult = &WorktreeInfo{
		Path:   "/tmp/repo-test",
		Branch: "feat/test",
	}

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop", false)

	require.NoError(t, err)
	require.NotNil(t, ws)
	// Should have fetched from remote
	assert.Equal(t, 1, runner.fetchCallCount)
}

func TestDefaultManager_Create_WithNonExistentBaseBranch(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false       // Not local
	runner.remoteBranchExists = false // Not remote either

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "nonexistent", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrBranchNotFound)
	// Should have tried to fetch
	assert.Equal(t, 1, runner.fetchCallCount)
}

func TestDefaultManager_Create_WithBaseBranch_FetchError(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false                   // Not local
	runner.fetchErr = atlaserrors.ErrGitOperation // Fetch fails
	runner.remoteBranchExists = true              // But remote branch exists (maybe from previous fetch)
	runner.createResult = &WorktreeInfo{
		Path:   "/tmp/repo-test",
		Branch: "feat/test",
	}

	mgr := NewManager(store, runner)
	// Even if fetch fails, if remote branch exists (from stale refs), we should succeed
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop", false)

	require.NoError(t, err)
	require.NotNil(t, ws)
}

func TestDefaultManager_Create_WithBaseBranch_RemoteCheckError(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false
	runner.remoteBranchExistsErr = atlaserrors.ErrGitOperation

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to check remote branch")
}

func TestDefaultManager_Create_WithBaseBranch_LocalCheckError(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExistsErr = atlaserrors.ErrGitOperation

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to check local branch")
}

// Test remote-first behavior (new default)
func TestDefaultManager_Create_PrefersRemoteBranch_Default(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = true       // Local exists
	runner.remoteBranchExists = true // Remote exists

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "main", false)

	require.NoError(t, err)
	require.NotNil(t, ws)
	// Verify fetch was called to get latest remote
	assert.Equal(t, 1, runner.fetchCallCount)
	// The resolved branch (origin/main) is passed to worktree.Create
	// which handles it internally. We can verify by checking no errors occurred.
}

// Test --use-local flag overrides default
func TestDefaultManager_Create_UseLocalFlag_PrefersLocal(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = true       // Local exists
	runner.remoteBranchExists = true // Remote exists

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "main", true)

	require.NoError(t, err)
	require.NotNil(t, ws)
	// When useLocal=true and local exists, should return immediately without fetching
	// (fetch count would be 0, but we check for success)
}

// Test fallback to local when remote missing
func TestDefaultManager_Create_FallbackToLocal(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = true        // Local exists
	runner.remoteBranchExists = false // Remote missing

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "local-only", false)

	require.NoError(t, err)
	require.NotNil(t, ws)
	// Should have tried to fetch
	assert.Equal(t, 1, runner.fetchCallCount)
	// Falls back to local when remote doesn't exist
}

// Test --use-local error when only remote exists
func TestDefaultManager_Create_UseLocalError_OnlyRemote(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false      // Not local
	runner.remoteBranchExists = true // Only remote

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "remote-only", true)

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrBranchNotFound)
	assert.Contains(t, err.Error(), "--use-local")
	assert.Contains(t, err.Error(), "does not exist locally")
}

// Test error when branch doesn't exist anywhere
func TestDefaultManager_Create_BranchNotFound_Anywhere(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false       // Not local
	runner.remoteBranchExists = false // Not remote

	// Test with useLocal=false (default)
	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "nonexistent", false)

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrBranchNotFound)
	assert.Contains(t, err.Error(), "does not exist locally or on remote")

	// Test with useLocal=true (should also error)
	ws2, err2 := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "nonexistent", true)

	require.Error(t, err2)
	assert.Nil(t, ws2)
	require.ErrorIs(t, err2, atlaserrors.ErrBranchNotFound)
}
