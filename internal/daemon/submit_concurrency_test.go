package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubmit_ConcurrentSameWorktree_OnlyOneWins stresses the worktree
// exclusivity lock: N goroutines submit for the SAME repo path at once, and
// exactly one must win while the rest are rejected as worktree-locked. Run under
// -race, it also guards the submit path against data races. This is the
// double-execution guarantee that keeps two workers off one worktree.
func TestSubmit_ConcurrentSameWorktree_OnlyOneWins(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	repoPath := t.TempDir()

	// Pre-marshal once; json.RawMessage is read-only and safe to share.
	params, err := json.Marshal(TaskSubmitRequest{
		Description: "concurrent same-worktree task",
		Template:    "task",
		RepoPath:    repoPath,
	})
	require.NoError(t, err)

	const n = 12
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		successes   int
		lockedCount int
		otherErrs   []error
	)

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			_, submitErr := d.handleTaskSubmit(context.Background(), params)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case submitErr == nil:
				successes++
			case errors.Is(submitErr, errWorktreeLocked):
				lockedCount++
			default:
				otherErrs = append(otherErrs, submitErr)
			}
		}()
	}
	wg.Wait()

	assert.Empty(t, otherErrs, "no unexpected errors across concurrent submits: %v", otherErrs)
	assert.Equal(t, 1, successes, "exactly one concurrent submit must win the worktree lock")
	assert.Equal(t, n-1, lockedCount, "every other concurrent submit must be rejected as worktree-locked")

	// Exactly one task should be tracked in the active set (the winner).
	active, err := cache.SetMembers(context.Background(), client, "atlas:active")
	require.NoError(t, err)
	assert.Len(t, active, 1, "only the winning submit should appear in the active set")
}

// TestSubmit_ConcurrentDistinctWorktrees_AllSucceed verifies that concurrent
// submits on DIFFERENT worktrees do not contend and all succeed.
func TestSubmit_ConcurrentDistinctWorktrees_AllSucceed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	const n = 8
	paths := make([]string, n)
	for i := range paths {
		paths[i] = t.TempDir()
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			params, mErr := json.Marshal(TaskSubmitRequest{
				Description: "distinct worktree task",
				Template:    "task",
				RepoPath:    paths[i],
			})
			if mErr != nil {
				mu.Lock()
				errs = append(errs, mErr)
				mu.Unlock()
				return
			}
			if _, sErr := d.handleTaskSubmit(context.Background(), params); sErr != nil {
				mu.Lock()
				errs = append(errs, sErr)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Empty(t, errs, "submits on distinct worktrees must all succeed: %v", errs)

	active, err := cache.SetMembers(context.Background(), client, "atlas:active")
	require.NoError(t, err)
	assert.Len(t, active, n, "all distinct-worktree submits should be tracked as active")
}
