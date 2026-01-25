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
// Aliases can map alternative names to existing templates.
type Registry struct {
	mu        sync.RWMutex
	templates map[string]*domain.Template
	aliases   map[string]string // maps alias name to target template name
}

// NewRegistry creates a new empty template registry.
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]*domain.Template),
		aliases:   make(map[string]string),
	}
}

// Get retrieves a template by name or alias.
// Returns a clone of the template to prevent mutation of registry state.
// Returns ErrTemplateNotFound if the template doesn't exist.
func (r *Registry) Get(name string) (*domain.Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if name is an alias and resolve to target
	resolvedName := name
	if target, isAlias := r.aliases[name]; isAlias {
		resolvedName = target
	}

	t, ok := r.templates[resolvedName]
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

// RegisterAlias creates an alias that points to an existing template.
// When Get is called with the alias name, it returns the target template.
// Returns error if:
// - alias or target is empty
// - target template doesn't exist
// - alias name conflicts with an existing template name
func (r *Registry) RegisterAlias(alias, target string) error {
	alias = strings.TrimSpace(alias)
	target = strings.TrimSpace(target)

	if alias == "" {
		return fmt.Errorf("%w: alias name cannot be empty", atlaserrors.ErrTemplateNameEmpty)
	}
	if target == "" {
		return fmt.Errorf("%w: alias target cannot be empty", atlaserrors.ErrTemplateNameEmpty)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check that target template exists
	if _, exists := r.templates[target]; !exists {
		return fmt.Errorf("%w: alias target %q", atlaserrors.ErrTemplateNotFound, target)
	}

	// Check that alias doesn't conflict with an existing template name
	if _, exists := r.templates[alias]; exists {
		return fmt.Errorf("%w: alias %q conflicts with existing template", atlaserrors.ErrTemplateDuplicate, alias)
	}

	r.aliases[alias] = target
	return nil
}

// Aliases returns all registered aliases as a map from alias to target template name.
func (r *Registry) Aliases() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]string, len(r.aliases))
	for alias, target := range r.aliases {
		result[alias] = target
	}
	return result
}

// IsAlias returns true if the given name is a registered alias.
func (r *Registry) IsAlias(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, isAlias := r.aliases[name]
	return isAlias
}
