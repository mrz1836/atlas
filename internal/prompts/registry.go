package prompts

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

//go:embed templates/common/*.tmpl templates/git/*.tmpl templates/validation/*.tmpl templates/backlog/*.tmpl templates/verify/*.tmpl templates/task/*.tmpl
var templateFS embed.FS

// registry holds parsed templates and provides thread-safe access.
type registry struct {
	mu        sync.RWMutex
	templates map[PromptID]*template.Template
	sources   map[PromptID]string // stores original template source for GetTemplate
	funcMap   template.FuncMap
}

// globalRegistry is the singleton registry instance.
//
//nolint:gochecknoglobals // singleton pattern for template registry - provides thread-safe global access
var globalRegistry = &registry{
	templates: make(map[PromptID]*template.Template),
	sources:   make(map[PromptID]string),
	funcMap:   defaultFuncMap(),
}

// defaultFuncMap returns the default template functions.
func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// join concatenates strings with a separator
		"join": strings.Join,
		// hasContent checks if a string is non-empty
		"hasContent": func(s string) bool {
			return strings.TrimSpace(s) != ""
		},
		// formatFileChange formats a FileChange for display
		"formatFileChange": func(f FileChange) string {
			return fmt.Sprintf("- %s (%s)", f.Path, f.Status)
		},
		// formatPRFileChange formats a PRFileChange for display
		"formatPRFileChange": func(f PRFileChange) string {
			return fmt.Sprintf("- %s (+%d, -%d)", f.Path, f.Insertions, f.Deletions)
		},
		// contains checks if a slice contains a string
		"contains": func(slice []string, s string) bool {
			for _, item := range slice {
				if item == s {
					return true
				}
			}
			return false
		},
		// capitalizeFirst capitalizes the first letter
		"capitalizeFirst": func(s string) string {
			if s == "" {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		// lower converts to lowercase
		"lower": strings.ToLower,
		// upper converts to uppercase
		"upper": strings.ToUpper,
	}
}

// init loads all templates at startup.
//
//nolint:gochecknoinits // required to preload embedded templates at package initialization
func init() {
	if err := globalRegistry.loadAll(); err != nil {
		// Templates are embedded, so this should never fail
		// If it does, it's a compile-time bug we want to know about
		panic(fmt.Sprintf("failed to load embedded templates: %v", err))
	}
}

// loadAll loads all templates from the embedded filesystem.
// This method acquires the lock and calls loadAllUnlocked.
func (r *registry) loadAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadAllUnlocked()
}

// loadAllUnlocked loads all templates without acquiring the lock.
// Caller must hold r.mu.Lock() before calling this method.
func (r *registry) loadAllUnlocked() error {
	// First, load common templates that can be included by others
	commonTemplates, err := r.loadCommonTemplates()
	if err != nil {
		return fmt.Errorf("loading common templates: %w", err)
	}

	// Walk the templates directory and load each template
	return fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-template files
		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		// Skip common templates (already loaded)
		if strings.Contains(path, "/common/") {
			return nil
		}

		// Read template content
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		// Derive prompt ID from path: templates/git/commit_message.tmpl -> git/commit_message
		promptID := r.pathToPromptID(path)

		// Create template with common templates included
		tmpl := template.New(string(promptID)).Funcs(r.funcMap)

		// Add common templates
		for name, commonTmpl := range commonTemplates {
			if _, addErr := tmpl.AddParseTree(name, commonTmpl.Tree); addErr != nil {
				return fmt.Errorf("adding common template %s: %w", name, addErr)
			}
		}

		// Parse the main template
		_, err = tmpl.Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", path, err)
		}

		r.templates[promptID] = tmpl
		r.sources[promptID] = string(content) // store source for GetTemplate
		return nil
	})
}

// loadCommonTemplates loads templates from the common directory.
func (r *registry) loadCommonTemplates() (map[string]*template.Template, error) {
	common := make(map[string]*template.Template)

	entries, err := templateFS.ReadDir("templates/common")
	if err != nil {
		// common directory is optional, return empty map if not found
		return common, nil //nolint:nilerr // common templates are optional; missing directory is not an error
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		path := filepath.Join("templates/common", entry.Name())
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading common template %s: %w", path, err)
		}

		// Name is "common/<name>" without .tmpl extension
		name := "common/" + strings.TrimSuffix(entry.Name(), ".tmpl")

		tmpl, err := template.New(name).Funcs(r.funcMap).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parsing common template %s: %w", path, err)
		}

		common[name] = tmpl
	}

	return common, nil
}

// pathToPromptID converts a file path to a PromptID.
// templates/git/commit_message.tmpl -> git/commit_message
func (r *registry) pathToPromptID(path string) PromptID {
	// Remove "templates/" prefix and ".tmpl" suffix
	id := strings.TrimPrefix(path, "templates/")
	id = strings.TrimSuffix(id, ".tmpl")
	return PromptID(id)
}

// get retrieves a template by ID.
func (r *registry) get(id PromptID) (*template.Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tmpl, ok := r.templates[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
	}
	return tmpl, nil
}

// getSource retrieves the raw template source by ID.
func (r *registry) getSource(id PromptID) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	source, ok := r.sources[id]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
	}
	return source, nil
}

// list returns all registered prompt IDs.
func (r *registry) list() []PromptID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]PromptID, 0, len(r.templates))
	for id := range r.templates {
		ids = append(ids, id)
	}
	return ids
}

// RegisterCustomFuncs adds custom template functions.
// This should be called during initialization before any templates are rendered.
func RegisterCustomFuncs(funcs template.FuncMap) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	for name, fn := range funcs {
		globalRegistry.funcMap[name] = fn
	}

	// Reload templates with new functions (already holding lock, so use unlocked version)
	_ = globalRegistry.loadAllUnlocked()
}
