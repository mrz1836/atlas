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

	t.Run("creates discovery file", func(t *testing.T) {
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

		// Check ID was generated
		assert.Regexp(t, `^disc-[a-z0-9]{6}$`, d.ID)

		// Check schema version was set
		assert.Equal(t, SchemaVersion, d.SchemaVersion)

		// Check file exists
		filePath := filepath.Join(mgr.Dir(), d.ID+".yaml")
		_, err = os.Stat(filePath)
		assert.NoError(t, err)
	})

	t.Run("respects provided ID", func(t *testing.T) {
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

	t.Run("promotes pending discovery", func(t *testing.T) {
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
		assert.Equal(t, StatusPromoted, promoted.Status)
		assert.Equal(t, "task-001", promoted.Lifecycle.PromotedToTask)

		// Verify persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusPromoted, got.Status)
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
