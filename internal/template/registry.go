// Package template provides task template management for ATLAS.
// Templates define the sequence of steps for automated task execution.
package template

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Registry provides thread-safe access to task templates.
// Templates are stored by name and can be retrieved or listed.
type Registry struct {
	mu        sync.RWMutex
	templates map[string]*domain.Template
}

// NewRegistry creates a new empty template registry.
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]*domain.Template),
	}
}

// Get retrieves a template by name.
// Returns a clone of the template to prevent mutation of registry state.
// Returns ErrTemplateNotFound if the template doesn't exist.
func (r *Registry) Get(name string) (*domain.Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.templates[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", atlaserrors.ErrTemplateNotFound, name)
	}
	return t.Clone(), nil
}

// List returns all registered templates.
// The returned slice and templates are clones, safe to modify without affecting the registry.
func (r *Registry) List() []*domain.Template {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*domain.Template, 0, len(r.templates))
	for _, t := range r.templates {
		result = append(result, t.Clone())
	}
	return result
}

// Register adds a template to the registry.
// Returns error if template is nil, has empty name, or already exists.
func (r *Registry) Register(t *domain.Template) error {
	if t == nil {
		return atlaserrors.ErrTemplateNil
	}
	if strings.TrimSpace(t.Name) == "" {
		return atlaserrors.ErrTemplateNameEmpty
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.templates[t.Name]; exists {
		return fmt.Errorf("%w: %s", atlaserrors.ErrTemplateDuplicate, t.Name)
	}

	r.templates[t.Name] = t
	return nil
}

// RegisterOrReplace adds a template to the registry, replacing any existing template with the same name.
// This is used for custom templates that should override built-in templates.
// Returns error if template is nil or has empty name.
func (r *Registry) RegisterOrReplace(t *domain.Template) error {
	if t == nil {
		return atlaserrors.ErrTemplateNil
	}
	if strings.TrimSpace(t.Name) == "" {
		return atlaserrors.ErrTemplateNameEmpty
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.templates[t.Name] = t
	return nil
}
