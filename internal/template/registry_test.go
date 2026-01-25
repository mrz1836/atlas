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

func TestRegistry_RegisterOrReplace_New(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "test", Description: "Test template"}

	err := r.RegisterOrReplace(tmpl)
	require.NoError(t, err)

	got, err := r.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "test", got.Name)
	assert.Equal(t, "Test template", got.Description)
}

func TestRegistry_RegisterOrReplace_Override(t *testing.T) {
	r := NewRegistry()
	tmpl1 := &domain.Template{Name: "test", Description: "First version"}
	tmpl2 := &domain.Template{Name: "test", Description: "Second version"}

	// Register first template
	err := r.Register(tmpl1)
	require.NoError(t, err)

	// Override with second template using RegisterOrReplace
	err = r.RegisterOrReplace(tmpl2)
	require.NoError(t, err)

	// Verify the template was replaced
	got, err := r.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "Second version", got.Description)

	// Verify only one template exists
	list := r.List()
	assert.Len(t, list, 1)
}

func TestRegistry_RegisterOrReplace_Nil(t *testing.T) {
	r := NewRegistry()

	err := r.RegisterOrReplace(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNil)
}

func TestRegistry_RegisterOrReplace_EmptyName(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "", Description: "Test"}

	err := r.RegisterOrReplace(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestRegistry_RegisterOrReplace_WhitespaceOnlyName(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "   \t\n", Description: "Test"}

	err := r.RegisterOrReplace(tmpl)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestRegistry_RegisterOrReplace_Concurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	// Register and replace templates concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// All templates have the same name to test concurrent replacement
			tmpl := &domain.Template{Name: "shared", Description: fmt.Sprintf("version-%d", n)}
			_ = r.RegisterOrReplace(tmpl)
		}(i)
	}

	wg.Wait()

	// Should have exactly one template
	list := r.List()
	assert.Len(t, list, 1)
	assert.Equal(t, "shared", list[0].Name)
}

func TestRegistry_RegisterAlias_Success(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "original", Description: "Original template"}
	require.NoError(t, r.Register(tmpl))

	err := r.RegisterAlias("alias", "original")
	require.NoError(t, err)

	// Getting by alias should return the original template
	got, err := r.Get("alias")
	require.NoError(t, err)
	assert.Equal(t, "original", got.Name)
}

func TestRegistry_RegisterAlias_EmptyAlias(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "original", Description: "Original template"}
	require.NoError(t, r.Register(tmpl))

	err := r.RegisterAlias("", "original")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestRegistry_RegisterAlias_EmptyTarget(t *testing.T) {
	r := NewRegistry()

	err := r.RegisterAlias("alias", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestRegistry_RegisterAlias_TargetNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.RegisterAlias("alias", "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNotFound)
}

func TestRegistry_RegisterAlias_ConflictWithTemplate(t *testing.T) {
	r := NewRegistry()
	tmpl1 := &domain.Template{Name: "original", Description: "Original"}
	tmpl2 := &domain.Template{Name: "other", Description: "Other"}
	require.NoError(t, r.Register(tmpl1))
	require.NoError(t, r.Register(tmpl2))

	// Trying to create alias with same name as existing template should fail
	err := r.RegisterAlias("original", "other")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateDuplicate)
}

func TestRegistry_RegisterAlias_MultipleAliases(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "original", Description: "Original template"}
	require.NoError(t, r.Register(tmpl))

	// Register multiple aliases to same template
	require.NoError(t, r.RegisterAlias("alias1", "original"))
	require.NoError(t, r.RegisterAlias("alias2", "original"))

	// Both aliases should resolve to original
	got1, err := r.Get("alias1")
	require.NoError(t, err)
	assert.Equal(t, "original", got1.Name)

	got2, err := r.Get("alias2")
	require.NoError(t, err)
	assert.Equal(t, "original", got2.Name)
}

func TestRegistry_Aliases(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "original", Description: "Original template"}
	require.NoError(t, r.Register(tmpl))
	require.NoError(t, r.RegisterAlias("alias1", "original"))
	require.NoError(t, r.RegisterAlias("alias2", "original"))

	aliases := r.Aliases()
	assert.Len(t, aliases, 2)
	assert.Equal(t, "original", aliases["alias1"])
	assert.Equal(t, "original", aliases["alias2"])
}

func TestRegistry_IsAlias(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "original", Description: "Original template"}
	require.NoError(t, r.Register(tmpl))
	require.NoError(t, r.RegisterAlias("alias", "original"))

	assert.True(t, r.IsAlias("alias"))
	assert.False(t, r.IsAlias("original"))
	assert.False(t, r.IsAlias("nonexistent"))
}

func TestRegistry_Alias_Concurrent(t *testing.T) {
	r := NewRegistry()
	tmpl := &domain.Template{Name: "original", Description: "Original template"}
	require.NoError(t, r.Register(tmpl))

	var wg sync.WaitGroup

	// Register aliases concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = r.RegisterAlias(fmt.Sprintf("alias-%d", n), "original")
		}(i)
	}

	// Read aliases concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = r.Get(fmt.Sprintf("alias-%d", n))
		}(i)
	}

	// Check aliases concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Aliases()
		}()
	}

	wg.Wait()
	// If we get here without a race condition panic, the test passes
}
