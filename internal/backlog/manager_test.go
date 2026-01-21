package backlog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewManager(t *testing.T) {
	t.Parallel()
	t.Run("uses current directory when empty", func(t *testing.T) {
		mgr, err := NewManager("")
		require.NoError(t, err)
		assert.NotEmpty(t, mgr.Dir())
		assert.NotEmpty(t, mgr.ProjectRoot())
	})

	t.Run("uses provided path", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)
		assert.Contains(t, mgr.Dir(), ".atlas/backlog")
		assert.Equal(t, tmpDir, mgr.ProjectRoot())
	})
}

func TestManager_EnsureDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("creates directory and gitkeep", func(t *testing.T) {
		err := mgr.EnsureDir()
		require.NoError(t, err)

		// Check directory exists
		info, err := os.Stat(mgr.Dir())
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		// Check .gitkeep exists
		gitkeepPath := filepath.Join(mgr.Dir(), ".gitkeep")
		_, err = os.Stat(gitkeepPath)
		assert.NoError(t, err)
	})

	t.Run("idempotent", func(t *testing.T) {
		err := mgr.EnsureDir()
		require.NoError(t, err)
		err = mgr.EnsureDir()
		require.NoError(t, err)
	})
}

func TestManager_Add(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("creates discovery file with new format", func(t *testing.T) {
		d := &Discovery{
			Title:  "Test discovery",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}

		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Check ID was generated in new format
		assert.Regexp(t, `^item-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$`, d.ID)

		// Check GUID was generated
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, d.GUID)

		// Check schema version was set
		assert.Equal(t, SchemaVersion, d.SchemaVersion)

		// Check file exists
		filePath := filepath.Join(mgr.Dir(), d.ID+".yaml")
		_, err = os.Stat(filePath)
		assert.NoError(t, err)
	})

	t.Run("respects provided legacy ID without GUID", func(t *testing.T) {
		d := &Discovery{
			ID:     "disc-custom",
			Title:  "Custom ID test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}

		err := mgr.Add(ctx, d)
		require.NoError(t, err)
		assert.Equal(t, "disc-custom", d.ID)
		// Legacy ID doesn't get a GUID automatically
		assert.Empty(t, d.GUID)
	})

	t.Run("respects provided new format ID and generates GUID", func(t *testing.T) {
		d := &Discovery{
			ID:     "item-ABC234",
			Title:  "Custom new format ID test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}

		err := mgr.Add(ctx, d)
		require.NoError(t, err)
		assert.Equal(t, "item-ABC234", d.ID)
		// New format ID gets a GUID generated
		assert.NotEmpty(t, d.GUID)
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, d.GUID)
	})

	t.Run("fails on duplicate ID", func(t *testing.T) {
		d := &Discovery{
			ID:     "disc-duplic",
			Title:  "Duplicate test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}

		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Try to add again with same ID
		d2 := *d
		err = mgr.Add(ctx, &d2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("fails on invalid discovery", func(t *testing.T) {
		d := &Discovery{
			Title: "", // Empty title is invalid
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
		}

		err := mgr.Add(ctx, d)
		assert.Error(t, err)
	})
}

func TestManager_Get(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Create a discovery first
	d := &Discovery{
		Title:  "Get test",
		Status: StatusPending,
		Content: Content{
			Description: "Test description",
			Category:    CategoryBug,
			Severity:    SeverityMedium,
			Tags:        []string{"test", "get"},
		},
		Context: Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	t.Run("retrieves existing discovery", func(t *testing.T) {
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, d.ID, got.ID)
		assert.Equal(t, d.Title, got.Title)
		assert.Equal(t, d.Content.Category, got.Content.Category)
		assert.Equal(t, d.Content.Description, got.Content.Description)
		assert.Equal(t, d.Content.Tags, got.Content.Tags)
	})

	t.Run("fails on non-existent ID", func(t *testing.T) {
		_, err := mgr.Get(ctx, "disc-notfnd")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_List(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Create some discoveries
	discoveries := []*Discovery{
		{
			Title:   "Bug 1",
			Status:  StatusPending,
			Content: Content{Category: CategoryBug, Severity: SeverityHigh},
			Context: Context{DiscoveredAt: time.Now().UTC().Add(-2 * time.Hour), DiscoveredBy: "human:tester"},
		},
		{
			Title:   "Bug 2",
			Status:  StatusPending,
			Content: Content{Category: CategoryBug, Severity: SeverityLow},
			Context: Context{DiscoveredAt: time.Now().UTC().Add(-1 * time.Hour), DiscoveredBy: "human:tester"},
		},
		{
			Title:     "Security issue",
			Status:    StatusPromoted,
			Content:   Content{Category: CategorySecurity, Severity: SeverityCritical},
			Context:   Context{DiscoveredAt: time.Now().UTC(), DiscoveredBy: "human:tester"},
			Lifecycle: Lifecycle{PromotedToTask: "task-001"},
		},
	}

	for _, d := range discoveries {
		err := mgr.Add(ctx, d)
		require.NoError(t, err)
	}

	t.Run("lists all discoveries", func(t *testing.T) {
		list, warnings, err := mgr.List(ctx, Filter{})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Len(t, list, 3)
	})

	t.Run("filters by status", func(t *testing.T) {
		pending := StatusPending
		list, warnings, err := mgr.List(ctx, Filter{Status: &pending})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Len(t, list, 2)
	})

	t.Run("filters by category", func(t *testing.T) {
		bug := CategoryBug
		list, warnings, err := mgr.List(ctx, Filter{Category: &bug})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Len(t, list, 2)
	})

	t.Run("applies limit", func(t *testing.T) {
		list, warnings, err := mgr.List(ctx, Filter{Limit: 2})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Len(t, list, 2)
	})

	t.Run("sorts by discovered_at descending", func(t *testing.T) {
		list, warnings, err := mgr.List(ctx, Filter{})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		require.Len(t, list, 3)
		// Newest should be first
		assert.True(t, list[0].Context.DiscoveredAt.After(list[1].Context.DiscoveredAt))
		assert.True(t, list[1].Context.DiscoveredAt.After(list[2].Context.DiscoveredAt))
	})

	t.Run("empty directory returns empty list", func(t *testing.T) {
		emptyDir := t.TempDir()
		emptyMgr, err := NewManager(emptyDir)
		require.NoError(t, err)

		list, warnings, err := emptyMgr.List(ctx, Filter{})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Empty(t, list)
	})
}

func TestManager_Update(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Create a discovery first
	d := &Discovery{
		Title:  "Update test",
		Status: StatusPending,
		Content: Content{
			Category: CategoryBug,
			Severity: SeverityLow,
		},
		Context: Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	t.Run("updates existing discovery", func(t *testing.T) {
		d.Title = "Updated title"
		d.Content.Severity = SeverityHigh
		d.Content.Description = "Added description"

		err := mgr.Update(ctx, d)
		require.NoError(t, err)

		// Verify update
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated title", got.Title)
		assert.Equal(t, SeverityHigh, got.Content.Severity)
		assert.Equal(t, "Added description", got.Content.Description)
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		nonExistent := &Discovery{
			ID:     "disc-notfnd",
			Title:  "Non-existent",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}

		err := mgr.Update(ctx, nonExistent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_Promote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("stores task ID but keeps status pending", func(t *testing.T) {
		d := &Discovery{
			Title:  "Promote test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		promoted, err := mgr.Promote(ctx, d.ID, "task-001")
		require.NoError(t, err)
		// Status should remain pending (changes when task starts)
		assert.Equal(t, StatusPending, promoted.Status)
		assert.Equal(t, "task-001", promoted.Lifecycle.PromotedToTask)

		// Verify persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusPending, got.Status)
		assert.Equal(t, "task-001", got.Lifecycle.PromotedToTask)
	})

	t.Run("fails on non-pending discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Already promoted",
			Status: StatusPromoted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{PromotedToTask: "task-old"},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.Promote(ctx, d.ID, "task-new")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		// Verify it's an ExitCode2Error for CLI handling
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error for invalid transition")
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		_, err := mgr.Promote(ctx, "disc-notfnd", "task-001")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_Dismiss(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("dismisses pending discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Dismiss test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		dismissed, err := mgr.Dismiss(ctx, d.ID, "duplicate")
		require.NoError(t, err)
		assert.Equal(t, StatusDismissed, dismissed.Status)
		assert.Equal(t, "duplicate", dismissed.Lifecycle.DismissedReason)

		// Verify persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusDismissed, got.Status)
	})

	t.Run("fails on non-pending discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Already dismissed",
			Status: StatusDismissed,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{DismissedReason: "old reason"},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.Dismiss(ctx, d.ID, "new reason")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		// Verify it's an ExitCode2Error for CLI handling
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error for invalid transition")
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		_, err := mgr.Dismiss(ctx, "disc-notfnd", "reason")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_Complete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("completes promoted discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Complete test",
			Status: StatusPromoted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{PromotedToTask: "task-001"},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		completed, err := mgr.Complete(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusCompleted, completed.Status)
		assert.False(t, completed.Lifecycle.CompletedAt.IsZero(), "CompletedAt should be set")

		// Verify persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusCompleted, got.Status)
		assert.False(t, got.Lifecycle.CompletedAt.IsZero())
	})

	t.Run("fails on pending discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Pending discovery",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.Complete(ctx, d.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		// Verify it's an ExitCode2Error for CLI handling
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error for invalid transition")
	})

	t.Run("fails on dismissed discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Dismissed discovery",
			Status: StatusDismissed,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{DismissedReason: "duplicate"},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.Complete(ctx, d.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		assert.True(t, atlaserrors.IsExitCode2Error(err))
	})

	t.Run("fails on already completed discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Already completed",
			Status: StatusCompleted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{
				PromotedToTask: "task-old",
				CompletedAt:    time.Now().UTC(),
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.Complete(ctx, d.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		assert.True(t, atlaserrors.IsExitCode2Error(err))
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		_, err := mgr.Complete(ctx, "disc-notfnd")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_StartTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("starts pending discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Start task test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		started, err := mgr.StartTask(ctx, d.ID, "task-001")
		require.NoError(t, err)
		assert.Equal(t, StatusPromoted, started.Status)
		assert.Equal(t, "task-001", started.Lifecycle.PromotedToTask)

		// Verify persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusPromoted, got.Status)
		assert.Equal(t, "task-001", got.Lifecycle.PromotedToTask)
	})

	t.Run("idempotent for same task ID", func(t *testing.T) {
		d := &Discovery{
			Title:  "Idempotent test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Start task first time
		_, err = mgr.StartTask(ctx, d.ID, "task-002")
		require.NoError(t, err)

		// Start task second time with same task ID (should succeed - handles resume)
		started, err := mgr.StartTask(ctx, d.ID, "task-002")
		require.NoError(t, err)
		assert.Equal(t, StatusPromoted, started.Status)
		assert.Equal(t, "task-002", started.Lifecycle.PromotedToTask)
	})

	t.Run("fails when already promoted with different task ID", func(t *testing.T) {
		d := &Discovery{
			Title:  "Different task ID test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Start with first task ID
		_, err = mgr.StartTask(ctx, d.ID, "task-003")
		require.NoError(t, err)

		// Try to start with different task ID (should fail)
		_, err = mgr.StartTask(ctx, d.ID, "task-004")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		assert.True(t, atlaserrors.IsExitCode2Error(err))
	})

	t.Run("fails on dismissed discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Dismissed discovery",
			Status: StatusDismissed,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{DismissedReason: "duplicate"},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.StartTask(ctx, d.ID, "task-005")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		assert.True(t, atlaserrors.IsExitCode2Error(err))
	})

	t.Run("fails on completed discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Completed discovery",
			Status: StatusCompleted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{
				PromotedToTask: "task-old",
				CompletedAt:    time.Now().UTC(),
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		_, err = mgr.StartTask(ctx, d.ID, "task-006")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		assert.True(t, atlaserrors.IsExitCode2Error(err))
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		_, err := mgr.StartTask(ctx, "disc-notfnd", "task-007")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("deletes existing discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Delete test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Verify it exists
		_, err = mgr.Get(ctx, d.ID)
		require.NoError(t, err)

		// Delete it
		err = mgr.Delete(ctx, d.ID)
		require.NoError(t, err)

		// Verify it's gone
		_, err = mgr.Get(ctx, d.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("can delete promoted discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Promoted discovery",
			Status: StatusPromoted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{PromotedToTask: "task-123"},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		err = mgr.Delete(ctx, d.ID)
		require.NoError(t, err)

		_, err = mgr.Get(ctx, d.ID)
		require.Error(t, err)
	})

	t.Run("can delete completed discovery", func(t *testing.T) {
		d := &Discovery{
			Title:  "Completed discovery",
			Status: StatusCompleted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{
				PromotedToTask: "task-456",
				CompletedAt:    time.Now().UTC(),
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		err = mgr.Delete(ctx, d.ID)
		require.NoError(t, err)

		_, err = mgr.Get(ctx, d.ID)
		require.Error(t, err)
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		err := mgr.Delete(ctx, "disc-notfnd")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("idempotent delete fails second time", func(t *testing.T) {
		d := &Discovery{
			Title:  "Double delete test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// First delete succeeds
		err = mgr.Delete(ctx, d.ID)
		require.NoError(t, err)

		// Second delete fails
		err = mgr.Delete(ctx, d.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManager_ListWithMalformedFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	// Ensure dir exists
	err = mgr.EnsureDir()
	require.NoError(t, err)

	// Create a valid discovery
	d := &Discovery{
		Title:  "Valid discovery",
		Status: StatusPending,
		Content: Content{
			Category: CategoryBug,
			Severity: SeverityLow,
		},
		Context: Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Create a malformed file
	malformedPath := filepath.Join(mgr.Dir(), "disc-broken.yaml")
	err = os.WriteFile(malformedPath, []byte("invalid: yaml: content: ["), 0o600)
	require.NoError(t, err)

	// List should succeed and skip malformed file with warning
	list, warnings, err := mgr.List(ctx, Filter{})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, d.ID, list[0].ID)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "disc-broken.yaml")
}

func TestManager_PromoteWithOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	createTestDiscovery := func(t *testing.T, mgr *Manager) *Discovery {
		t.Helper()
		d := &Discovery{
			Title:  "Test Bug Discovery",
			Status: StatusPending,
			Content: Content{
				Description: "Test description for bug",
				Category:    CategoryBug,
				Severity:    SeverityHigh,
				Tags:        []string{"test", "bug"},
			},
			Location: &Location{
				File: "main.go",
				Line: 42,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)
		return d
	}

	t.Run("generates task config from bug category", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		// Bug category should map to bugfix template
		assert.Equal(t, "bugfix", result.TemplateName)
		assert.Equal(t, "test-bug-discovery", result.WorkspaceName)
		assert.Equal(t, "fix/test-bug-discovery", result.BranchName)
		assert.Contains(t, result.Description, "Test Bug Discovery")
		assert.Contains(t, result.Description, "[HIGH]")
	})

	t.Run("generates task config from critical security category", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := &Discovery{
			Title:  "Critical Security Vulnerability",
			Status: StatusPending,
			Content: Content{
				Category: CategorySecurity,
				Severity: SeverityCritical,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err = mgr.Add(ctx, d)
		require.NoError(t, err)

		opts := PromoteOptions{}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		// Critical security should map to hotfix
		assert.Equal(t, "hotfix", result.TemplateName)
		assert.Equal(t, "hotfix/critical-security-vulnerability", result.BranchName)
	})

	t.Run("template override takes precedence", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{
			Template: "feature", // Override automatic mapping
		}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		assert.Equal(t, "feature", result.TemplateName)
		assert.Equal(t, "feat/test-bug-discovery", result.BranchName)
	})

	t.Run("workspace name override", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{
			WorkspaceName: "custom-workspace",
		}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		assert.Equal(t, "custom-workspace", result.WorkspaceName)
		assert.Equal(t, "fix/custom-workspace", result.BranchName)
	})

	t.Run("description override", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{
			Description: "Custom description for the task",
		}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		assert.Equal(t, "Custom description for the task", result.Description)
	})

	t.Run("fails on non-pending discovery", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := &Discovery{
			Title:  "Already promoted",
			Status: StatusPromoted,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Lifecycle: Lifecycle{PromotedToTask: "task-old"},
		}
		err = mgr.Add(ctx, d)
		require.NoError(t, err)

		opts := PromoteOptions{}

		_, err = mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status transition")
		assert.True(t, atlaserrors.IsExitCode2Error(err))
	})

	t.Run("fails on non-existent discovery", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		opts := PromoteOptions{}

		_, err = mgr.PromoteWithOptions(ctx, "disc-notfnd", opts, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetBranchPrefixForTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		template string
		expected string
	}{
		{"bugfix", "fix"},
		{"feature", "feat"},
		{"hotfix", "hotfix"},
		{"task", "task"},
		{"fix", "fix"},
		{"commit", "chore"},
		{"unknown", "task"},
		{"", "task"},
	}

	for _, tc := range tests {
		t.Run(tc.template, func(t *testing.T) {
			t.Parallel()
			result := getBranchPrefixForTemplate(tc.template)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestManager_MigrateLegacyDiscoveries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("migrates legacy disc-* file on List", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Ensure dir exists
		err = mgr.EnsureDir()
		require.NoError(t, err)

		// Create a legacy file manually
		legacyContent := `schema_version: "1.0"
id: disc-abc123
title: Legacy Discovery
status: pending
content:
  category: bug
  severity: high
context:
  discovered_at: 2024-01-15T10:00:00Z
  discovered_by: human:tester
`
		legacyPath := filepath.Join(mgr.Dir(), "disc-abc123.yaml")
		err = os.WriteFile(legacyPath, []byte(legacyContent), 0o600)
		require.NoError(t, err)

		// List should trigger migration
		list, warnings, err := mgr.List(ctx, Filter{})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		require.Len(t, list, 1)

		// Discovery should have new format ID
		d := list[0]
		assert.Regexp(t, `^item-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$`, d.ID)
		assert.NotEmpty(t, d.GUID)
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, d.GUID)
		assert.Equal(t, SchemaVersion, d.SchemaVersion)

		// Old file should be gone
		_, err = os.Stat(legacyPath)
		assert.True(t, os.IsNotExist(err), "legacy file should be deleted")

		// New file should exist
		newPath := filepath.Join(mgr.Dir(), d.ID+".yaml")
		_, err = os.Stat(newPath)
		assert.NoError(t, err, "new file should exist")
	})

	t.Run("migrates legacy disc-* file on Get", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Ensure dir exists
		err = mgr.EnsureDir()
		require.NoError(t, err)

		// Create a legacy file manually
		legacyContent := `schema_version: "1.0"
id: disc-xyz789
title: Legacy Discovery for Get
status: pending
content:
  category: security
  severity: critical
context:
  discovered_at: 2024-01-15T10:00:00Z
  discovered_by: human:tester
`
		legacyPath := filepath.Join(mgr.Dir(), "disc-xyz789.yaml")
		err = os.WriteFile(legacyPath, []byte(legacyContent), 0o600)
		require.NoError(t, err)

		// Get should trigger migration
		d, err := mgr.Get(ctx, "disc-xyz789")
		require.NoError(t, err)

		// Discovery should have new format ID
		assert.Regexp(t, `^item-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$`, d.ID)
		assert.NotEmpty(t, d.GUID)
		assert.Equal(t, "Legacy Discovery for Get", d.Title)

		// Old file should be gone
		_, err = os.Stat(legacyPath)
		assert.True(t, os.IsNotExist(err), "legacy file should be deleted")

		// New file should exist and be retrievable with new ID
		d2, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, d.ID, d2.ID)
		assert.Equal(t, d.GUID, d2.GUID)
	})

	t.Run("List handles both new and legacy files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create a new format discovery
		newDiscovery := &Discovery{
			Title:  "New Format Discovery",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err = mgr.Add(ctx, newDiscovery)
		require.NoError(t, err)

		// Create a legacy file manually
		legacyContent := `schema_version: "1.0"
id: disc-legacy
title: Legacy Format Discovery
status: pending
content:
  category: testing
  severity: medium
context:
  discovered_at: 2024-01-15T10:00:00Z
  discovered_by: human:tester
`
		legacyPath := filepath.Join(mgr.Dir(), "disc-legacy.yaml")
		err = os.WriteFile(legacyPath, []byte(legacyContent), 0o600)
		require.NoError(t, err)

		// List should find both
		list, warnings, err := mgr.List(ctx, Filter{})
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Len(t, list, 2)

		// Both should have new format IDs (legacy was migrated)
		for _, d := range list {
			assert.Regexp(t, `^item-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$`, d.ID)
		}
	})

	t.Run("does not re-migrate already migrated files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create a discovery with new format (already has GUID)
		d := &Discovery{
			Title:  "Already Migrated",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err = mgr.Add(ctx, d)
		require.NoError(t, err)

		originalID := d.ID
		originalGUID := d.GUID

		// Get should return same IDs (no re-migration)
		d2, err := mgr.Get(ctx, originalID)
		require.NoError(t, err)
		assert.Equal(t, originalID, d2.ID)
		assert.Equal(t, originalGUID, d2.GUID)
	})

	t.Run("preserves discovery content during migration", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = mgr.EnsureDir()
		require.NoError(t, err)

		// Create a legacy file with all fields populated
		legacyContent := `schema_version: "1.0"
id: disc-full01
title: Full Legacy Discovery
status: promoted
content:
  description: This is a detailed description
  category: performance
  severity: high
  tags:
    - perf
    - database
location:
  file: main.go
  line: 42
context:
  discovered_at: 2024-01-15T10:30:00Z
  discovered_by: ai:claude
  discovered_during_task: task-001
  git:
    branch: feature/test
    commit: abc1234
lifecycle:
  promoted_to_task: task-002
`
		legacyPath := filepath.Join(mgr.Dir(), "disc-full01.yaml")
		err = os.WriteFile(legacyPath, []byte(legacyContent), 0o600)
		require.NoError(t, err)

		// Get should trigger migration
		d, err := mgr.Get(ctx, "disc-full01")
		require.NoError(t, err)

		// Verify all content was preserved
		assert.Equal(t, "Full Legacy Discovery", d.Title)
		assert.Equal(t, StatusPromoted, d.Status)
		assert.Equal(t, "This is a detailed description", d.Content.Description)
		assert.Equal(t, CategoryPerformance, d.Content.Category)
		assert.Equal(t, SeverityHigh, d.Content.Severity)
		assert.Equal(t, []string{"perf", "database"}, d.Content.Tags)
		assert.NotNil(t, d.Location)
		assert.Equal(t, "main.go", d.Location.File)
		assert.Equal(t, 42, d.Location.Line)
		assert.Equal(t, "ai:claude", d.Context.DiscoveredBy)
		assert.Equal(t, "task-001", d.Context.DuringTask)
		assert.NotNil(t, d.Context.Git)
		assert.Equal(t, "feature/test", d.Context.Git.Branch)
		assert.Equal(t, "abc1234", d.Context.Git.Commit)
		assert.Equal(t, "task-002", d.Lifecycle.PromotedToTask)

		// Schema version should be updated
		assert.Equal(t, SchemaVersion, d.SchemaVersion)
	})
}

func TestManager_GUIDLookup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("GUID is stored in file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create a discovery
		d := &Discovery{
			Title:  "GUID Test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err = mgr.Add(ctx, d)
		require.NoError(t, err)

		// Read file directly to verify GUID is stored
		filePath := filepath.Join(mgr.Dir(), d.ID+".yaml")
		content, err := os.ReadFile(filePath) //nolint:gosec // test code with trusted path
		require.NoError(t, err)

		assert.Contains(t, string(content), "guid:")
		assert.Contains(t, string(content), d.GUID)
	})

	t.Run("GUID is consistent on reload", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create a discovery
		d := &Discovery{
			Title:  "GUID Reload Test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err = mgr.Add(ctx, d)
		require.NoError(t, err)

		originalGUID := d.GUID

		// Create new manager and load
		mgr2, err := NewManager(tmpDir)
		require.NoError(t, err)

		d2, err := mgr2.Get(ctx, d.ID)
		require.NoError(t, err)

		assert.Equal(t, originalGUID, d2.GUID)
	})
}

func TestManager_YAMLIndentation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	require.NoError(t, err)

	t.Run("creates files with 2-space indentation", func(t *testing.T) {
		d := &Discovery{
			Title:  "Indentation test",
			Status: StatusPending,
			Content: Content{
				Description: "Test description",
				Category:    CategoryBug,
				Severity:    SeverityHigh,
				Tags:        []string{"test", "formatting"},
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}

		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Read the file and verify indentation
		filePath := filepath.Join(mgr.Dir(), d.ID+".yaml")
		content, err := os.ReadFile(filePath) // #nosec G304 -- test file read with controlled path
		require.NoError(t, err)

		// Check for 2-space indentation
		assert.Contains(t, string(content), "content:\n  description:")
		assert.Contains(t, string(content), "  category:")

		// Ensure no 4-space indentation
		assert.NotContains(t, string(content), "    description:")
	})

	t.Run("updates files with 2-space indentation", func(t *testing.T) {
		d := &Discovery{
			Title:  "Update test",
			Status: StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
			},
			Context: Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Update the discovery
		d.Content.Description = "Updated"
		err = mgr.Update(ctx, d)
		require.NoError(t, err)

		// Verify 2-space indentation
		filePath := filepath.Join(mgr.Dir(), d.ID+".yaml")
		content, err := os.ReadFile(filePath) // #nosec G304 -- test file read with controlled path
		require.NoError(t, err)
		assert.Contains(t, string(content), "content:\n  description:")
		assert.NotContains(t, string(content), "    description:")
	})
}
