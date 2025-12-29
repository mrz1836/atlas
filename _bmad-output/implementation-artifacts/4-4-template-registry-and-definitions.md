# Story 4.4: Template Registry and Definitions

Status: done

## Story

As a **developer**,
I want **a template registry with bugfix, feature, and commit templates**,
So that **users can select predefined workflows for common task types**.

## Acceptance Criteria

1. **Given** the AIRunner exists **When** I implement `internal/template/` **Then** `registry.go` provides:
   - `Get(name string) (*Template, error)`
   - `List() []*Template`
   - `Register(template *Template)`

2. **Given** registry exists **When** I implement `bugfix.go` **Then** it defines the bugfix template:
   - Steps: analyze → implement → validate → git_commit → git_push → git_pr → ci_wait → review
   - Branch prefix: "fix"
   - Default model: sonnet

3. **Given** registry exists **When** I implement `feature.go` **Then** it defines the feature template:
   - Steps: specify → review_spec → plan → tasks → implement → validate → checklist → git_commit → git_push → git_pr → ci_wait → review
   - Branch prefix: "feat"
   - Default model: opus
   - Integrates Speckit SDD

4. **Given** registry exists **When** I implement `commit.go` **Then** it defines the commit template:
   - Steps: analyze_changes → smart_commit → git_push
   - Garbage detection, logical grouping
   - Branch prefix: "chore"

5. **Given** registry exists **When** I implement `variables.go` **Then** it handles template variable expansion

6. **Given** templates are defined **When** building ATLAS **Then** templates are Go code compiled into the binary (not external files)

7. **Given** configuration exists **When** templates are loaded **Then** template behavior is customizable via config (model, branch_prefix, auto_proceed_git)

## Tasks / Subtasks

- [x] Task 1: Create template registry (AC: #1)
  - [x] 1.1: Create `internal/template/registry.go` file
  - [x] 1.2: Define `Registry` struct with thread-safe storage (`sync.RWMutex`)
  - [x] 1.3: Implement `Get(name string) (*domain.Template, error)` - returns `ErrTemplateNotFound` if missing
  - [x] 1.4: Implement `List() []*domain.Template` - returns all registered templates
  - [x] 1.5: Implement `Register(template *domain.Template) error` - validates and adds template
  - [x] 1.6: Implement `NewRegistry() *Registry` constructor
  - [x] 1.7: Add `ErrTemplateNotFound` to `internal/errors/errors.go`

- [x] Task 2: Implement bugfix template (AC: #2)
  - [x] 2.1: Create `internal/template/bugfix.go` file
  - [x] 2.2: Define `NewBugfixTemplate() *domain.Template`
  - [x] 2.3: Configure steps: analyze, implement, validate, git_commit, git_push, git_pr, ci_wait, review
  - [x] 2.4: Set branch_prefix: "fix"
  - [x] 2.5: Set default_model: "sonnet"
  - [x] 2.6: Define appropriate timeouts and retry counts per step
  - [x] 2.7: Add step configurations for each step type

- [x] Task 3: Implement feature template (AC: #3)
  - [x] 3.1: Create `internal/template/feature.go` file
  - [x] 3.2: Define `NewFeatureTemplate() *domain.Template`
  - [x] 3.3: Configure steps: specify, review_spec, plan, tasks, implement, validate, checklist, git_commit, git_push, git_pr, ci_wait, review
  - [x] 3.4: Set branch_prefix: "feat"
  - [x] 3.5: Set default_model: "opus"
  - [x] 3.6: Configure SDD step types for specify, plan, tasks, checklist
  - [x] 3.7: Add appropriate timeouts (longer for AI spec steps)
  - [x] 3.8: Configure human review checkpoints (review_spec, review)

- [x] Task 4: Implement commit template (AC: #4)
  - [x] 4.1: Create `internal/template/commit.go` file
  - [x] 4.2: Define `NewCommitTemplate() *domain.Template`
  - [x] 4.3: Configure steps: analyze_changes, smart_commit, git_push
  - [x] 4.4: Set branch_prefix: "chore"
  - [x] 4.5: Set default_model: "sonnet"
  - [x] 4.6: Configure analyze_changes step with garbage detection config
  - [x] 4.7: Configure smart_commit step with logical grouping config

- [x] Task 5: Implement variable expansion (AC: #5)
  - [x] 5.1: Create `internal/template/variables.go` file
  - [x] 5.2: Define `VariableExpander` struct
  - [x] 5.3: Implement `Expand(template *domain.Template, values map[string]string) (*domain.Template, error)`
  - [x] 5.4: Handle missing required variables with clear errors
  - [x] 5.5: Apply defaults for optional variables
  - [x] 5.6: Support `{{variable}}` syntax in step descriptions and configs
  - [x] 5.7: Validate variable names are valid identifiers

- [x] Task 6: Create default registry with all templates (AC: #6)
  - [x] 6.1: Create `internal/template/defaults.go` file
  - [x] 6.2: Implement `NewDefaultRegistry() *Registry` constructor
  - [x] 6.3: Register bugfix template
  - [x] 6.4: Register feature template
  - [x] 6.5: Register commit template
  - [x] 6.6: Ensure templates are compiled into binary (no external files)

- [x] Task 7: Integrate with configuration (AC: #7)
  - [x] 7.1: Create `internal/template/config.go` file
  - [x] 7.2: Implement `ApplyConfig(template *domain.Template, cfg *config.Config) *domain.Template`
  - [x] 7.3: Allow override of: model, branch_prefix, auto_proceed_git
  - [x] 7.4: Support per-template overrides via `Overrides` struct
  - [x] 7.5: Implement `GetTemplateWithConfig` for easy template retrieval with config

- [x] Task 8: Write comprehensive tests (AC: all)
  - [x] 8.1: Create `internal/template/registry_test.go`
  - [x] 8.2: Test Get returns correct template
  - [x] 8.3: Test Get returns ErrTemplateNotFound for missing
  - [x] 8.4: Test List returns all templates
  - [x] 8.5: Test Register adds new template
  - [x] 8.6: Test Register rejects duplicate names
  - [x] 8.7: Create `internal/template/bugfix_test.go`
  - [x] 8.8: Test bugfix template has correct steps and order
  - [x] 8.9: Create `internal/template/feature_test.go`
  - [x] 8.10: Test feature template has correct steps and order
  - [x] 8.11: Create `internal/template/commit_test.go`
  - [x] 8.12: Test commit template has correct steps and order
  - [x] 8.13: Create `internal/template/variables_test.go`
  - [x] 8.14: Test variable expansion with valid values
  - [x] 8.15: Test variable expansion with missing required
  - [x] 8.16: Test variable expansion with defaults
  - [x] 8.17: Create `internal/template/defaults_test.go`
  - [x] 8.18: Test default registry has all three templates
  - [x] 8.19: Create `internal/template/config_test.go`
  - [x] 8.20: Test config overrides are applied correctly
  - [x] 8.21: Run `magex format:fix && magex lint && magex test:race` - ALL PASS

## Dev Notes

### Critical Warnings (READ FIRST)

1. **Domain types already exist**: `Template`, `StepDefinition`, `TemplateVariable`, and `StepType` are defined in `internal/domain/template.go`. Use those types, do NOT redefine.

2. **Use existing errors**: Add `ErrTemplateNotFound` to `internal/errors/errors.go` following existing patterns.

3. **Templates are compiled in**: Templates MUST be Go code in the binary. No external YAML/JSON files. This ensures version control and eliminates file loading errors.

4. **Context as first parameter**: Always check `ctx.Done()` at function entry for any long operations.

5. **Config types exist**: Use `config.TemplatesConfig` from `internal/config/config.go` for configuration integration.

### Package Locations

| File | Purpose |
|------|---------|
| `internal/template/registry.go` | NEW - Registry struct and methods |
| `internal/template/bugfix.go` | NEW - Bugfix template definition |
| `internal/template/feature.go` | NEW - Feature template definition |
| `internal/template/commit.go` | NEW - Commit template definition |
| `internal/template/variables.go` | NEW - Variable expansion utilities |
| `internal/template/defaults.go` | NEW - Default registry factory |
| `internal/template/config.go` | NEW - Configuration integration |
| `internal/template/registry_test.go` | NEW - Registry tests |
| `internal/template/bugfix_test.go` | NEW - Bugfix template tests |
| `internal/template/feature_test.go` | NEW - Feature template tests |
| `internal/template/commit_test.go` | NEW - Commit template tests |
| `internal/template/variables_test.go` | NEW - Variable expansion tests |
| `internal/template/defaults_test.go` | NEW - Default registry tests |
| `internal/template/config_test.go` | NEW - Config integration tests |
| `internal/domain/template.go` | REFERENCE - Template, StepDefinition types |
| `internal/config/config.go` | REFERENCE - TemplatesConfig |
| `internal/errors/errors.go` | MODIFY - Add ErrTemplateNotFound |
| `internal/constants/constants.go` | REFERENCE - Timeout constants |

### Import Rules (CRITICAL)

**`internal/template/` MAY import:**
- `internal/constants` - for timeout constants
- `internal/domain` - for Template, StepDefinition, StepType types
- `internal/errors` - for ErrTemplateNotFound
- `internal/config` - for TemplatesConfig type
- `context`, `fmt`, `regexp`, `strings`, `sync`, `time`

**MUST NOT import:**
- `internal/task` - avoid circular dependencies
- `internal/workspace` - avoid circular dependencies
- `internal/ai` - templates don't directly invoke AI
- `internal/cli` - domain packages don't import CLI

### Registry Pattern

```go
// internal/template/registry.go

package template

import (
    "fmt"
    "sync"

    "github.com/mrz1836/atlas/internal/domain"
    atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Registry provides thread-safe access to task templates.
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
// Returns ErrTemplateNotFound if the template doesn't exist.
func (r *Registry) Get(name string) (*domain.Template, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    t, ok := r.templates[name]
    if !ok {
        return nil, fmt.Errorf("%w: %s", atlaserrors.ErrTemplateNotFound, name)
    }
    return t, nil
}

// List returns all registered templates.
func (r *Registry) List() []*domain.Template {
    r.mu.RLock()
    defer r.mu.RUnlock()

    result := make([]*domain.Template, 0, len(r.templates))
    for _, t := range r.templates {
        result = append(result, t)
    }
    return result
}

// Register adds a template to the registry.
// Returns error if template name is empty or already exists.
func (r *Registry) Register(t *domain.Template) error {
    if t == nil || t.Name == "" {
        return fmt.Errorf("template name is required")
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    if _, exists := r.templates[t.Name]; exists {
        return fmt.Errorf("template '%s' already registered", t.Name)
    }

    r.templates[t.Name] = t
    return nil
}
```

### Template Definition Pattern

```go
// internal/template/bugfix.go

package template

import (
    "time"

    "github.com/mrz1836/atlas/internal/constants"
    "github.com/mrz1836/atlas/internal/domain"
)

// NewBugfixTemplate creates the bugfix template for fixing bugs.
// Steps: analyze → implement → validate → git_commit → git_push → git_pr → ci_wait → review
func NewBugfixTemplate() *domain.Template {
    return &domain.Template{
        Name:         "bugfix",
        Description:  "Fix a reported bug with analysis, implementation, and validation",
        BranchPrefix: "fix/",
        DefaultModel: "sonnet",
        Steps: []domain.StepDefinition{
            {
                Name:        "analyze",
                Type:        domain.StepTypeAI,
                Description: "Analyze the bug report and identify root cause",
                Required:    true,
                Timeout:     15 * time.Minute,
                RetryCount:  2,
                Config: map[string]any{
                    "permission_mode": "plan",
                    "prompt_template": "analyze_bug",
                },
            },
            {
                Name:        "implement",
                Type:        domain.StepTypeAI,
                Description: "Implement the fix for the identified issue",
                Required:    true,
                Timeout:     constants.DefaultAITimeout,
                RetryCount:  3,
                Config: map[string]any{
                    "permission_mode": "default",
                    "prompt_template": "implement_fix",
                },
            },
            {
                Name:        "validate",
                Type:        domain.StepTypeValidation,
                Description: "Run format, lint, and test commands",
                Required:    true,
                Timeout:     10 * time.Minute,
                RetryCount:  1,
            },
            {
                Name:        "git_commit",
                Type:        domain.StepTypeGit,
                Description: "Create commit with fix changes",
                Required:    true,
                Timeout:     1 * time.Minute,
                Config: map[string]any{
                    "operation": "commit",
                },
            },
            {
                Name:        "git_push",
                Type:        domain.StepTypeGit,
                Description: "Push branch to remote",
                Required:    true,
                Timeout:     2 * time.Minute,
                RetryCount:  3,
                Config: map[string]any{
                    "operation": "push",
                },
            },
            {
                Name:        "git_pr",
                Type:        domain.StepTypeGit,
                Description: "Create pull request",
                Required:    true,
                Timeout:     2 * time.Minute,
                RetryCount:  2,
                Config: map[string]any{
                    "operation": "create_pr",
                },
            },
            {
                Name:        "ci_wait",
                Type:        domain.StepTypeCI,
                Description: "Wait for CI pipeline to complete",
                Required:    true,
                Timeout:     constants.DefaultCITimeout,
                Config: map[string]any{
                    "poll_interval": constants.CIPollInterval,
                },
            },
            {
                Name:        "review",
                Type:        domain.StepTypeHuman,
                Description: "Human review of completed fix",
                Required:    true,
                Config: map[string]any{
                    "prompt": "Review the fix and approve or reject",
                },
            },
        },
        ValidationCommands: []string{
            "magex format:fix",
            "magex lint",
            "magex test",
        },
    }
}
```

### Feature Template with SDD Steps

```go
// internal/template/feature.go

package template

import (
    "time"

    "github.com/mrz1836/atlas/internal/constants"
    "github.com/mrz1836/atlas/internal/domain"
)

// NewFeatureTemplate creates the feature template with Speckit SDD integration.
// Steps: specify → review_spec → plan → tasks → implement → validate →
//        checklist → git_commit → git_push → git_pr → ci_wait → review
func NewFeatureTemplate() *domain.Template {
    return &domain.Template{
        Name:         "feature",
        Description:  "Develop a new feature with spec-driven development",
        BranchPrefix: "feat/",
        DefaultModel: "opus",
        Steps: []domain.StepDefinition{
            {
                Name:        "specify",
                Type:        domain.StepTypeSDD,
                Description: "Generate specification using Speckit",
                Required:    true,
                Timeout:     20 * time.Minute,
                Config: map[string]any{
                    "sdd_command": "specify",
                },
            },
            {
                Name:        "review_spec",
                Type:        domain.StepTypeHuman,
                Description: "Review generated specification",
                Required:    true,
                Config: map[string]any{
                    "prompt": "Review the specification and approve or request changes",
                },
            },
            {
                Name:        "plan",
                Type:        domain.StepTypeSDD,
                Description: "Generate implementation plan using Speckit",
                Required:    true,
                Timeout:     15 * time.Minute,
                Config: map[string]any{
                    "sdd_command": "plan",
                },
            },
            {
                Name:        "tasks",
                Type:        domain.StepTypeSDD,
                Description: "Generate task breakdown using Speckit",
                Required:    true,
                Timeout:     15 * time.Minute,
                Config: map[string]any{
                    "sdd_command": "tasks",
                },
            },
            {
                Name:        "implement",
                Type:        domain.StepTypeAI,
                Description: "Implement the feature according to plan",
                Required:    true,
                Timeout:     45 * time.Minute,
                RetryCount:  3,
                Config: map[string]any{
                    "permission_mode": "default",
                    "prompt_template": "implement_feature",
                },
            },
            {
                Name:        "validate",
                Type:        domain.StepTypeValidation,
                Description: "Run format, lint, and test commands",
                Required:    true,
                Timeout:     10 * time.Minute,
                RetryCount:  1,
            },
            {
                Name:        "checklist",
                Type:        domain.StepTypeSDD,
                Description: "Verify implementation against checklist",
                Required:    true,
                Timeout:     10 * time.Minute,
                Config: map[string]any{
                    "sdd_command": "checklist",
                },
            },
            {
                Name:        "git_commit",
                Type:        domain.StepTypeGit,
                Description: "Create commit with feature changes",
                Required:    true,
                Timeout:     1 * time.Minute,
                Config: map[string]any{
                    "operation": "commit",
                },
            },
            {
                Name:        "git_push",
                Type:        domain.StepTypeGit,
                Description: "Push branch to remote",
                Required:    true,
                Timeout:     2 * time.Minute,
                RetryCount:  3,
                Config: map[string]any{
                    "operation": "push",
                },
            },
            {
                Name:        "git_pr",
                Type:        domain.StepTypeGit,
                Description: "Create pull request",
                Required:    true,
                Timeout:     2 * time.Minute,
                RetryCount:  2,
                Config: map[string]any{
                    "operation": "create_pr",
                },
            },
            {
                Name:        "ci_wait",
                Type:        domain.StepTypeCI,
                Description: "Wait for CI pipeline to complete",
                Required:    true,
                Timeout:     constants.DefaultCITimeout,
                Config: map[string]any{
                    "poll_interval": constants.CIPollInterval,
                },
            },
            {
                Name:        "review",
                Type:        domain.StepTypeHuman,
                Description: "Human review of completed feature",
                Required:    true,
                Config: map[string]any{
                    "prompt": "Review the feature implementation and approve or reject",
                },
            },
        },
        ValidationCommands: []string{
            "magex format:fix",
            "magex lint",
            "magex test",
        },
    }
}
```

### Commit Template

```go
// internal/template/commit.go

package template

import (
    "time"

    "github.com/mrz1836/atlas/internal/domain"
)

// NewCommitTemplate creates the commit template for smart commits.
// Steps: analyze_changes → smart_commit → git_push
func NewCommitTemplate() *domain.Template {
    return &domain.Template{
        Name:         "commit",
        Description:  "Analyze changes and create smart commits with garbage detection",
        BranchPrefix: "chore/",
        DefaultModel: "sonnet",
        Steps: []domain.StepDefinition{
            {
                Name:        "analyze_changes",
                Type:        domain.StepTypeAI,
                Description: "Analyze working tree changes and detect garbage files",
                Required:    true,
                Timeout:     5 * time.Minute,
                Config: map[string]any{
                    "permission_mode": "plan",
                    "detect_garbage":  true,
                    "garbage_patterns": []string{
                        "*.tmp", "*.bak", "*.log",
                        "node_modules/**", ".DS_Store",
                        "*.exe", "*.dll",
                        ".env*", "*credentials*",
                    },
                },
            },
            {
                Name:        "smart_commit",
                Type:        domain.StepTypeGit,
                Description: "Create logical commits with meaningful messages",
                Required:    true,
                Timeout:     2 * time.Minute,
                Config: map[string]any{
                    "operation":        "smart_commit",
                    "group_by_package": true,
                    "conventional":     true,
                },
            },
            {
                Name:        "git_push",
                Type:        domain.StepTypeGit,
                Description: "Push commits to remote",
                Required:    true,
                Timeout:     2 * time.Minute,
                RetryCount:  3,
                Config: map[string]any{
                    "operation": "push",
                },
            },
        },
    }
}
```

### Variable Expansion Pattern

```go
// internal/template/variables.go

package template

import (
    "fmt"
    "regexp"
    "strings"

    "github.com/mrz1836/atlas/internal/domain"
)

// varPattern matches {{variable}} patterns.
var varPattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

// VariableExpander handles template variable substitution.
type VariableExpander struct{}

// NewVariableExpander creates a new variable expander.
func NewVariableExpander() *VariableExpander {
    return &VariableExpander{}
}

// Expand substitutes variables in a template with provided values.
// Missing required variables result in an error.
// Optional variables without values use their defaults.
func (e *VariableExpander) Expand(t *domain.Template, values map[string]string) (*domain.Template, error) {
    // Validate required variables
    for name, v := range t.Variables {
        if v.Required {
            if _, ok := values[name]; !ok {
                if v.Default == "" {
                    return nil, fmt.Errorf("required variable '%s' not provided", name)
                }
            }
        }
    }

    // Merge with defaults
    merged := make(map[string]string)
    for name, v := range t.Variables {
        if v.Default != "" {
            merged[name] = v.Default
        }
    }
    for name, val := range values {
        merged[name] = val
    }

    // Clone template
    result := cloneTemplate(t)

    // Expand in description
    result.Description = expandString(result.Description, merged)

    // Expand in step descriptions and configs
    for i := range result.Steps {
        result.Steps[i].Description = expandString(result.Steps[i].Description, merged)
        result.Steps[i].Config = expandConfig(result.Steps[i].Config, merged)
    }

    return result, nil
}

func expandString(s string, values map[string]string) string {
    return varPattern.ReplaceAllStringFunc(s, func(match string) string {
        name := strings.Trim(match, "{}")
        if val, ok := values[name]; ok {
            return val
        }
        return match // Leave unexpanded if not found
    })
}

func expandConfig(cfg map[string]any, values map[string]string) map[string]any {
    if cfg == nil {
        return nil
    }
    result := make(map[string]any)
    for k, v := range cfg {
        switch val := v.(type) {
        case string:
            result[k] = expandString(val, values)
        default:
            result[k] = v
        }
    }
    return result
}

func cloneTemplate(t *domain.Template) *domain.Template {
    result := &domain.Template{
        Name:               t.Name,
        Description:        t.Description,
        BranchPrefix:       t.BranchPrefix,
        DefaultModel:       t.DefaultModel,
        ValidationCommands: append([]string{}, t.ValidationCommands...),
    }

    result.Steps = make([]domain.StepDefinition, len(t.Steps))
    for i, s := range t.Steps {
        result.Steps[i] = domain.StepDefinition{
            Name:        s.Name,
            Type:        s.Type,
            Description: s.Description,
            Required:    s.Required,
            Timeout:     s.Timeout,
            RetryCount:  s.RetryCount,
        }
        if s.Config != nil {
            result.Steps[i].Config = make(map[string]any)
            for k, v := range s.Config {
                result.Steps[i].Config[k] = v
            }
        }
    }

    if t.Variables != nil {
        result.Variables = make(map[string]domain.TemplateVariable)
        for k, v := range t.Variables {
            result.Variables[k] = v
        }
    }

    return result
}
```

### Default Registry Factory

```go
// internal/template/defaults.go

package template

// NewDefaultRegistry creates a registry with all built-in templates.
func NewDefaultRegistry() *Registry {
    r := NewRegistry()

    // Register built-in templates (errors ignored as names are unique)
    _ = r.Register(NewBugfixTemplate())
    _ = r.Register(NewFeatureTemplate())
    _ = r.Register(NewCommitTemplate())

    return r
}
```

### Previous Story Learnings (from Story 4-3)

From Story 4-3 (AIRunner Interface and ClaudeCodeRunner):

1. **Interface naming**: Use simple names like `Runner` instead of `AIRunner` to avoid stuttering (`ai.AIRunner` → `ai.Runner`). Similarly, `Registry` is preferred over `TemplateRegistry`.
2. **Error sentinel placement**: Add errors to `internal/errors/errors.go`, not locally
3. **Test coverage target**: Aim for 90%+ coverage
4. **Use `//nolint:gochecknoglobals` for lookup tables**: Established pattern
5. **Run `magex test:race`**: Race detection is mandatory

### Dependencies Between Stories

This story **depends on:**
- **Story 4-3** (AIRunner Interface) - templates reference AI execution
- **Story 4-2** (Task State Machine) - templates produce steps that transition state
- **Story 4-1** (Task Data Model) - templates create Step instances

This story **is required for:**
- **Story 4-5** (Step Executor Framework) - executors use template step definitions
- **Story 4-6** (Task Engine Orchestrator) - engine loads and uses templates
- **Story 4-7** (atlas start command) - start uses templates

### Edge Cases to Handle

1. **Template not found** - Return `ErrTemplateNotFound` with template name in error
2. **Duplicate registration** - Return error, don't silently overwrite
3. **Nil template registration** - Return error for nil or empty name
4. **Empty steps list** - Valid (though unusual) template
5. **Missing required variable** - Error at expansion time
6. **Circular variable references** - Not currently supported (would need DAG resolution)
7. **Config override conflicts** - Config values override template defaults

### Performance Considerations

1. **Registry uses RWMutex** - Allows concurrent reads, synchronized writes
2. **Templates are immutable once registered** - No need to clone on Get
3. **Variable expansion clones template** - Each expansion returns new instance

### Testing Pattern

```go
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
    require.Error(t, err)
    assert.True(t, errors.Is(err, atlaserrors.ErrTemplateNotFound))
}

func TestBugfixTemplate_StepOrder(t *testing.T) {
    tmpl := NewBugfixTemplate()

    expectedSteps := []string{
        "analyze", "implement", "validate", "git_commit",
        "git_push", "git_pr", "ci_wait", "review",
    }

    assert.Len(t, tmpl.Steps, len(expectedSteps))
    for i, name := range expectedSteps {
        assert.Equal(t, name, tmpl.Steps[i].Name, "step %d", i)
    }
}
```

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 4.4]
- [Source: _bmad-output/planning-artifacts/architecture.md#Template Registry]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/domain/template.go - Template, StepDefinition types]
- [Source: internal/config/config.go - TemplatesConfig struct]
- [Source: internal/constants/constants.go - Timeout constants]
- [Source: _bmad-output/implementation-artifacts/4-3-airunner-interface-and-claudecoderunner.md - Previous story patterns]

### Project Structure Notes

- Template registry lives in `internal/template/`
- Templates are Go code, not external files
- Uses domain.Template and domain.StepDefinition from `internal/domain/`
- Configuration integration via `internal/config/`
- Thread-safe registry using sync.RWMutex

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Manual verification:
# - Review template step orders match acceptance criteria
# - Verify registry thread-safety with concurrent tests
# - Ensure config overrides work as expected
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No debug issues encountered

### Completion Notes List

- Created complete template registry package at `internal/template/`
- Implemented thread-safe `Registry` with `sync.RWMutex` for concurrent access
- Created three built-in templates: bugfix, feature, and commit
- Bugfix template: 8 steps (analyze → implement → validate → git_commit → git_push → git_pr → ci_wait → review), branch prefix "fix/", model "sonnet"
- Feature template: 12 steps with SDD integration (specify, plan, tasks, checklist), branch prefix "feat/", model "opus"
- Commit template: 3 steps with garbage detection and smart commits, branch prefix "chore/", model "sonnet"
- Implemented `VariableExpander` for `{{variable}}` syntax in templates with required/optional/default support
- Created `NewDefaultRegistry()` factory for compiled-in templates (no external files)
- Added configuration integration via `ApplyConfig` and `ApplyOverrides` functions
- Added new sentinel errors: `ErrTemplateNotFound`, `ErrTemplateNil`, `ErrTemplateNameEmpty`, `ErrTemplateDuplicate`, `ErrVariableRequired`
- All tests pass with race detection (78 tests in template package)
- Code passes all linting checks (golangci-lint with 62 linters)

### Change Log

- 2025-12-28: Implemented Story 4.4 - Template Registry and Definitions
- 2025-12-28: Code review fixes - improved cloneConfig comment, added whitespace-only name validation, created shared helpers_test.go, fixed gofmt issues

### File List

**New Files:**
- internal/template/registry.go
- internal/template/registry_test.go
- internal/template/bugfix.go
- internal/template/bugfix_test.go
- internal/template/feature.go
- internal/template/feature_test.go
- internal/template/commit.go
- internal/template/commit_test.go
- internal/template/variables.go
- internal/template/variables_test.go
- internal/template/defaults.go
- internal/template/defaults_test.go
- internal/template/config.go
- internal/template/config_test.go
- internal/template/helpers_test.go (shared test helper for findStep)

**Modified Files:**
- internal/errors/errors.go (added ErrTemplateNotFound, ErrTemplateNil, ErrTemplateNameEmpty, ErrTemplateDuplicate, ErrVariableRequired)
- _bmad-output/implementation-artifacts/sprint-status.yaml (story status sync)

