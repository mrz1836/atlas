// Package git provides Git operations for ATLAS.
package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockArtifactStore is a test double for ArtifactStore.
type mockArtifactStore struct {
	saveFunc func(ctx context.Context, workspaceName, taskID, filename string, data []byte) error
	saved    map[string][]byte
}

func newMockArtifactStore() *mockArtifactStore {
	return &mockArtifactStore{
		saved: make(map[string][]byte),
	}
}

func (m *mockArtifactStore) SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, workspaceName, taskID, filename, data)
	}
	key := workspaceName + "/" + taskID + "/" + filename
	m.saved[key] = data
	return nil
}

func TestTaskStoreArtifactSaver_Save(t *testing.T) {
	t.Run("saves artifact successfully", func(t *testing.T) {
		store := newMockArtifactStore()
		saver := NewTaskStoreArtifactSaver(store,
			WithTaskStoreLogger(zerolog.Nop()),
		)

		desc := &PRDescription{
			Title:            "fix(config): handle nil",
			Body:             "## Summary\nFixed bug.\n\n## Changes\n- file.go\n\n## Test Plan\nPass.",
			ConventionalType: "fix",
			Scope:            "config",
		}

		opts := PRDescOptions{
			WorkspaceName: "fix/null-pointer",
			TaskID:        "task-atlas-test-abc",
			BaseBranch:    "main",
			HeadBranch:    "fix/null-pointer",
		}

		filename, err := saver.Save(context.Background(), desc, opts)

		require.NoError(t, err)
		assert.Equal(t, PRDescriptionFilename, filename)

		// Verify content was saved
		key := "fix/null-pointer/task-atlas-test-abc/pr-description.md"
		assert.Contains(t, store.saved, key)
		content := string(store.saved[key])
		assert.Contains(t, content, "fix(config): handle nil")
		assert.Contains(t, content, "## Summary")
	})

	t.Run("requires workspace name", func(t *testing.T) {
		store := newMockArtifactStore()
		saver := NewTaskStoreArtifactSaver(store)

		_, err := saver.Save(context.Background(), &PRDescription{}, PRDescOptions{
			TaskID: "task-id",
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("requires task ID", func(t *testing.T) {
		store := newMockArtifactStore()
		saver := NewTaskStoreArtifactSaver(store)

		_, err := saver.Save(context.Background(), &PRDescription{}, PRDescOptions{
			WorkspaceName: "ws",
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		store := newMockArtifactStore()
		saver := NewTaskStoreArtifactSaver(store)

		_, err := saver.Save(ctx, &PRDescription{}, PRDescOptions{
			WorkspaceName: "ws",
			TaskID:        "task",
		})

		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("handles store error", func(t *testing.T) {
		store := &mockArtifactStore{
			saveFunc: func(_ context.Context, _, _, _ string, _ []byte) error {
				return assert.AnError
			},
		}
		saver := NewTaskStoreArtifactSaver(store)

		_, err := saver.Save(context.Background(), &PRDescription{
			Title: "test",
			Body:  "body",
		}, PRDescOptions{
			WorkspaceName: "ws",
			TaskID:        "task",
		})

		require.Error(t, err)
	})
}

func TestFileArtifactSaver_Save(t *testing.T) {
	t.Run("saves to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		saver := NewFileArtifactSaver(tmpDir,
			WithFileArtifactLogger(zerolog.Nop()),
		)

		desc := &PRDescription{
			Title:            "feat: add feature",
			Body:             "## Summary\nNew feature.\n\n## Changes\n- file.go\n\n## Test Plan\nTests.",
			ConventionalType: "feat",
		}

		path, err := saver.Save(context.Background(), desc, PRDescOptions{
			BaseBranch: "main",
			HeadBranch: "feat/new",
		})

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, PRDescriptionFilename), path)

		// Verify file was created
		content, err := os.ReadFile(path) //nolint:gosec // test uses controlled temp directory path
		require.NoError(t, err)
		assert.Contains(t, string(content), "feat: add feature")
		assert.Contains(t, string(content), "## Summary")
	})

	t.Run("creates directory if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "dir")
		saver := NewFileArtifactSaver(nestedDir)

		desc := &PRDescription{
			Title: "test: title",
			Body:  "## Summary\nTest.\n\n## Changes\n- x\n\n## Test Plan\nOk.",
		}

		path, err := saver.Save(context.Background(), desc, PRDescOptions{})

		require.NoError(t, err)
		assert.FileExists(t, path)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		saver := NewFileArtifactSaver(t.TempDir())

		_, err := saver.Save(ctx, &PRDescription{}, PRDescOptions{})

		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestFormatPRArtifact(t *testing.T) {
	desc := &PRDescription{
		Title:            "fix(api): handle timeout",
		Body:             "## Summary\nFixed timeout.\n\n## Changes\n- api.go\n\n## Test Plan\nPass.",
		ConventionalType: "fix",
		Scope:            "api",
	}

	opts := PRDescOptions{
		TaskID:        "task-abc-xyz",
		WorkspaceName: "fix/timeout",
		BaseBranch:    "main",
		HeadBranch:    "fix/timeout",
	}

	content := formatPRArtifact(desc, opts)

	// Check YAML front matter
	assert.Contains(t, content, "---")
	assert.Contains(t, content, `title: "fix(api): handle timeout"`)
	assert.Contains(t, content, "type: fix")
	assert.Contains(t, content, "scope: api")
	assert.Contains(t, content, "base: main")
	assert.Contains(t, content, "head: fix/timeout")
	assert.Contains(t, content, "task_id: task-abc-xyz")
	assert.Contains(t, content, "workspace: fix/timeout")
	assert.Contains(t, content, "generated:")

	// Check content
	assert.Contains(t, content, "# fix(api): handle timeout")
	assert.Contains(t, content, "## Summary")
	assert.Contains(t, content, "Fixed timeout.")
}

func TestFormatPRArtifact_MinimalFields(t *testing.T) {
	desc := &PRDescription{
		Title:            "chore: update deps",
		Body:             "## Summary\nUpdated.\n\n## Changes\n- go.mod\n\n## Test Plan\nOk.",
		ConventionalType: "chore",
	}

	opts := PRDescOptions{} // No optional fields

	content := formatPRArtifact(desc, opts)

	assert.Contains(t, content, `title: "chore: update deps"`)
	assert.Contains(t, content, "type: chore")
	assert.NotContains(t, content, "scope:")
	assert.NotContains(t, content, "base:")
	assert.NotContains(t, content, "head:")
	assert.NotContains(t, content, "task_id:")
	assert.NotContains(t, content, "workspace:")
}

func TestPRDescriptionFilename(t *testing.T) {
	assert.Equal(t, "pr-description.md", PRDescriptionFilename)
}
