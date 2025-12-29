package template

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	assert.Empty(t, r.List())
}

func TestRegistry_Register_Success(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "test", Description: "Test template"}

	err := r.Register(tmpl)
	require.NoError(t, err)

	got, err := r.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "test", got.Name)
	assert.Equal(t, "Test template", got.Description)
}

func TestRegistry_Register_Nil(t *testing.T) {
	r := NewRegistry()

	err := r.Register(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNil)
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "", Description: "Test"}

	err := r.Register(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestRegistry_Register_WhitespaceOnlyName(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "   ", Description: "Test"}

	err := r.Register(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	tmpl1 := &domain.Template{Name: "test", Description: "First"}
	tmpl2 := &domain.Template{Name: "test", Description: "Second"}

	err := r.Register(tmpl1)
	require.NoError(t, err)

	err = r.Register(tmpl2)
	require.ErrorIs(t, err, atlaserrors.ErrTemplateDuplicate)
	assert.Contains(t, err.Error(), "test")
}

func TestRegistry_Get_Success(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "test", Description: "Test template"}
	require.NoError(t, r.Register(tmpl))

	got, err := r.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "test", got.Name)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("nonexistent")
	require.ErrorIs(t, err, atlaserrors.ErrTemplateNotFound)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestRegistry_List_Empty(t *testing.T) {
	r := NewRegistry()
	list := r.List()
	assert.Empty(t, list)
}

func TestRegistry_List_Multiple(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&domain.Template{Name: "a"}))
	require.NoError(t, r.Register(&domain.Template{Name: "b"}))
	require.NoError(t, r.Register(&domain.Template{Name: "c"}))

	list := r.List()
	assert.Len(t, list, 3)

	// Verify all templates are present
	names := make(map[string]bool)
	for _, t := range list {
		names[t.Name] = true
	}
	assert.True(t, names["a"])
	assert.True(t, names["b"])
	assert.True(t, names["c"])
}

func TestRegistry_Concurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	// Register templates concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tmpl := &domain.Template{Name: fmt.Sprintf("tmpl-%d", n)}
			_ = r.Register(tmpl)
		}(i)
	}

	// Read templates concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = r.Get(fmt.Sprintf("tmpl-%d", n))
		}(i)
	}

	// List templates concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
		}()
	}

	wg.Wait()
	// If we get here without a race condition panic, the test passes
	assert.NotEmpty(t, r.List())
}
