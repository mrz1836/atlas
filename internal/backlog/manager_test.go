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

	t.Run("legacy behavior with TaskID", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{
			TaskID: "task-legacy-001",
		}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		assert.Equal(t, "task-legacy-001", result.TaskID)
		assert.Equal(t, StatusPromoted, result.Discovery.Status)

		// Verify persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusPromoted, got.Status)
		assert.Equal(t, "task-legacy-001", got.Lifecycle.PromotedToTask)
	})

	t.Run("dry-run with TaskID does not persist", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{
			TaskID: "task-dry-run",
			DryRun: true,
		}

		result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
		require.NoError(t, err)

		assert.True(t, result.DryRun)
		assert.Equal(t, "task-dry-run", result.TaskID)

		// Verify not persisted
		got, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusPending, got.Status)
		assert.Empty(t, got.Lifecycle.PromotedToTask)
	})

	t.Run("generates task config from bug category", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		mgr, err := NewManager(tmpDir)
		require.NoError(t, err)

		d := createTestDiscovery(t, mgr)

		opts := PromoteOptions{} // No TaskID - generate config

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
