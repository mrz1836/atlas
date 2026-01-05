package workspace

import (
	"context"
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

	// Track calls for verification
	removeCallCount       int
	removeForceCallCount  int
	deleteBranchCallCount int
	pruneCallCount        int
	fetchCallCount        int

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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "")

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
	ws, err := mgr.Create(context.Background(), "existing", "/tmp/repo", "feat", "")

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceExists)
}

func TestDefaultManager_Create_ValidatesEmptyName(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "", "/tmp/repo", "feat", "")

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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "")

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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "")

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

	ws, err := mgr.Create(ctx, "test", "/tmp/repo", "feat", "")

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
	// Verify status was updated
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
	// Verify worktree path was cleared
	assert.Empty(t, ws.WorktreePath)
	// Verify UpdatedAt was set (Manager owns timestamp)
	assert.False(t, ws.UpdatedAt.IsZero())
	// Verify worktree was removed
	assert.Equal(t, 1, runner.removeCallCount)
	// Verify branch was NOT deleted
	assert.Equal(t, 0, runner.deleteBranchCallCount)
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
	// Worktree should NOT be removed (store update happens first now)
	assert.Equal(t, 0, runner.removeCallCount)
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
	assert.Empty(t, ws.WorktreePath)
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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "")

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
	store := newMockStore()
	store.workspaces["test"] = &domain.Workspace{
		Name:         "test",
		WorktreePath: "", // No worktree path
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
	}
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	result, err := mgr.Close(context.Background(), "test")

	require.NoError(t, err)
	require.NotNil(t, result)
	// No warning since there was no worktree to remove
	assert.Empty(t, result.WorktreeWarning)
	// Remove should not have been called
	assert.Equal(t, 0, runner.removeCallCount)
	// Status should be updated
	ws := store.workspaces["test"]
	assert.Equal(t, constants.WorkspaceStatusClosed, ws.Status)
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
		ws, err := mgr.Create(context.Background(), name, "/tmp/repo", "feat", "")
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
	ws, err := mgr.Create(context.Background(), "test", "", "feat", "")

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	assert.Contains(t, err.Error(), "repoPath")
}

func TestDefaultManager_Create_ValidatesEmptyBranchType(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "", "")

	require.Error(t, err)
	assert.Nil(t, ws)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	assert.Contains(t, err.Error(), "branchType")
}

func TestDefaultManager_Create_ValidatesNilWorktreeRunner(t *testing.T) {
	store := newMockStore()

	// Pass nil worktree runner
	mgr := NewManager(store, nil)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "task", "")

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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop")

	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "test", ws.Name)
	// Should not have fetched since local branch exists
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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop")

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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "nonexistent")

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
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop")

	require.NoError(t, err)
	require.NotNil(t, ws)
}

func TestDefaultManager_Create_WithBaseBranch_RemoteCheckError(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExists = false
	runner.remoteBranchExistsErr = atlaserrors.ErrGitOperation

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop")

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to check remote branch")
}

func TestDefaultManager_Create_WithBaseBranch_LocalCheckError(t *testing.T) {
	store := newMockStore()
	runner := newMockWorktreeRunner()
	runner.branchExistsErr = atlaserrors.ErrGitOperation

	mgr := NewManager(store, runner)
	ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat", "develop")

	require.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "failed to check local branch")
}
