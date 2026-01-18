# Data Model: Work Backlog Feature

**Branch**: `003-work-backlog` | **Date**: 2026-01-18

## Entities

### Discovery

A captured issue or observation found during development that cannot be addressed in the current task scope.

**Stored as**: Individual YAML file in `.atlas/backlog/disc-<id>.yaml`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | string | Yes | Schema version for forward compatibility. Value: `"1.0"` |
| `id` | string | Yes | Unique identifier. Format: `disc-<6-alphanumeric>` (e.g., `disc-a1b2c3`) |
| `title` | string | Yes | Brief summary of the discovery. Max 200 characters |
| `status` | enum | Yes | Current lifecycle status: `pending`, `promoted`, `dismissed` |
| `content` | object | Yes | Discovery details (see Content object) |
| `location` | object | No | Code location if applicable (see Location object) |
| `context` | object | Yes | When/who/where discovered (see Context object) |
| `lifecycle` | object | No | Status transition metadata (see Lifecycle object) |

### Content (embedded in Discovery)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | No | Detailed explanation of the discovery. Multi-line allowed |
| `category` | enum | Yes | Classification: `bug`, `security`, `performance`, `maintainability`, `testing`, `documentation` |
| `severity` | enum | Yes | Priority level: `low`, `medium`, `high`, `critical` |
| `tags` | []string | No | Optional labels for organization. Max 10 tags, each max 50 chars |

### Location (embedded in Discovery)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | No | Relative path to file from project root |
| `line` | int | No | Line number (1-indexed). Only valid if `file` is set |

### Context (embedded in Discovery)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `discovered_at` | datetime | Yes | ISO 8601 UTC timestamp of discovery creation |
| `discovered_during_task` | string | No | ATLAS task ID if discovered during automated task |
| `discovered_by` | string | Yes | Discoverer identifier (see format below) |
| `git` | object | No | Git context at time of discovery (see GitContext object) |

**Discoverer Format**:
- AI agent: `ai:<agent>:<model>` (e.g., `ai:claude-code:claude-sonnet-4`, `ai:gemini:flash`)
- Human: `human:<github-username>` (e.g., `human:mrz1836`)

### GitContext (embedded in Context)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `branch` | string | No | Git branch name at time of discovery |
| `commit` | string | No | Git commit SHA (short form, 7 chars) at time of discovery |

### Lifecycle (embedded in Discovery)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `promoted_to_task` | string | No | ATLAS task ID if status changed to `promoted` |
| `dismissed_reason` | string | No | Reason if status changed to `dismissed` |

---

## State Transitions

```
┌─────────────────────────────────────────────────────────────┐
│                    Discovery Lifecycle                       │
└─────────────────────────────────────────────────────────────┘

                         ┌──────────┐
                         │ CREATED  │
                         └────┬─────┘
                              │
                              ▼
                      ┌───────────────┐
                      │   pending     │ ◄─── Initial state
                      └───────┬───────┘
                              │
            ┌─────────────────┼─────────────────┐
            │                 │                 │
            ▼                 │                 ▼
    ┌───────────────┐         │         ┌───────────────┐
    │   promoted    │         │         │   dismissed   │
    │               │         │         │               │
    │ + task_id     │         │         │ + reason      │
    └───────────────┘         │         └───────────────┘
                              │
                    [No self-transitions]
                    [No promoted→dismissed]
                    [No dismissed→promoted]
```

**Transition Rules**:
1. New discoveries start as `pending`
2. `pending` → `promoted`: Records linked task ID
3. `pending` → `dismissed`: Requires reason
4. Terminal states (`promoted`, `dismissed`) are immutable
5. All metadata from creation is preserved through transitions

---

## Validation Rules

### Discovery ID
- Pattern: `^disc-[a-z0-9]{6}$`
- Must be unique within `.atlas/backlog/` directory
- Generated automatically on creation

### Title
- Required, non-empty
- Maximum 200 characters
- No leading/trailing whitespace

### Category
- Required
- Must be one of: `bug`, `security`, `performance`, `maintainability`, `testing`, `documentation`
- Case-sensitive, lowercase

### Severity
- Required
- Must be one of: `low`, `medium`, `high`, `critical`
- Case-sensitive, lowercase

### Tags
- Optional
- Maximum 10 tags per discovery
- Each tag: 1-50 characters, alphanumeric with hyphens/underscores
- Pattern per tag: `^[a-z0-9][a-z0-9_-]*$`

### Location
- If `line` is provided, `file` must also be provided
- `line` must be positive integer (>= 1)
- `file` should be relative path from project root

### Timestamps
- Format: ISO 8601 with timezone (RFC 3339)
- Example: `2026-01-18T14:32:15Z`
- Always stored in UTC

---

## Relationships

```
┌─────────────────────────────────────────────────────────────┐
│                      ATLAS Ecosystem                         │
└─────────────────────────────────────────────────────────────┘

┌──────────────┐     promotes to    ┌──────────────┐
│  Discovery   │ ─────────────────► │    Task      │
│              │                    │ (existing)   │
│  .atlas/     │                    │              │
│  backlog/    │     during task    │  ~/.atlas/   │
│              │ ◄───────────────── │  workspaces/ │
└──────────────┘                    └──────────────┘
        │
        │ discovered in
        ▼
┌──────────────┐
│  Git Repo    │
│              │
│  branch      │
│  commit      │
└──────────────┘
```

**Relationships**:
1. **Discovery → Task**: A discovery can be promoted to a task (one-way reference via `promoted_to_task`)
2. **Task → Discovery**: A task can discover issues (one-way reference via `discovered_during_task`)
3. **Discovery → Git**: Captures git context at time of creation (informational, no foreign key)

---

## Go Type Definitions

```go
// internal/backlog/types.go

package backlog

import "time"

// Status represents the lifecycle state of a discovery
type Status string

const (
    StatusPending   Status = "pending"
    StatusPromoted  Status = "promoted"
    StatusDismissed Status = "dismissed"
)

// Category classifies the type of discovery
type Category string

const (
    CategoryBug            Category = "bug"
    CategorySecurity       Category = "security"
    CategoryPerformance    Category = "performance"
    CategoryMaintainability Category = "maintainability"
    CategoryTesting        Category = "testing"
    CategoryDocumentation  Category = "documentation"
)

// Severity indicates the priority level
type Severity string

const (
    SeverityLow      Severity = "low"
    SeverityMedium   Severity = "medium"
    SeverityHigh     Severity = "high"
    SeverityCritical Severity = "critical"
)

// Discovery represents a captured issue or observation
type Discovery struct {
    SchemaVersion string    `yaml:"schema_version"`
    ID            string    `yaml:"id"`
    Title         string    `yaml:"title"`
    Status        Status    `yaml:"status"`
    Content       Content   `yaml:"content"`
    Location      *Location `yaml:"location,omitempty"`
    Context       Context   `yaml:"context"`
    Lifecycle     Lifecycle `yaml:"lifecycle,omitempty"`
}

// Content holds the discovery details
type Content struct {
    Description string   `yaml:"description,omitempty"`
    Category    Category `yaml:"category"`
    Severity    Severity `yaml:"severity"`
    Tags        []string `yaml:"tags,omitempty"`
}

// Location identifies where in code the discovery was found
type Location struct {
    File string `yaml:"file,omitempty"`
    Line int    `yaml:"line,omitempty"`
}

// Context captures when/who/where the discovery was made
type Context struct {
    DiscoveredAt   time.Time  `yaml:"discovered_at"`
    DuringTask     string     `yaml:"discovered_during_task,omitempty"`
    DiscoveredBy   string     `yaml:"discovered_by"`
    Git            *GitContext `yaml:"git,omitempty"`
}

// GitContext holds git repository state at discovery time
type GitContext struct {
    Branch string `yaml:"branch,omitempty"`
    Commit string `yaml:"commit,omitempty"`
}

// Lifecycle tracks status transitions
type Lifecycle struct {
    PromotedToTask  string `yaml:"promoted_to_task,omitempty"`
    DismissedReason string `yaml:"dismissed_reason,omitempty"`
}
```

---

## Filter Object

Used for list queries:

```go
// Filter specifies criteria for listing discoveries
type Filter struct {
    Status   *Status   // nil = all statuses
    Category *Category // nil = all categories
    Severity *Severity // nil = all severities
    Limit    int       // 0 = unlimited
}

// Match returns true if discovery matches filter criteria
func (f Filter) Match(d *Discovery) bool {
    if f.Status != nil && d.Status != *f.Status {
        return false
    }
    if f.Category != nil && d.Content.Category != *f.Category {
        return false
    }
    if f.Severity != nil && d.Content.Severity != *f.Severity {
        return false
    }
    return true
}
```

---

## Example YAML File

```yaml
# .atlas/backlog/disc-a1b2c3.yaml
schema_version: "1.0"
id: disc-a1b2c3
title: Missing error handling in config.Parse
status: pending

content:
  description: |
    The Parse function doesn't handle the case where the config file
    exists but is empty. This causes a nil pointer panic at line 52.
  category: bug
  severity: medium
  tags:
    - config
    - error-handling

location:
  file: config/parser.go
  line: 47

context:
  discovered_at: 2026-01-18T14:32:15Z
  discovered_during_task: task-20260118-143022
  discovered_by: ai:claude-code:claude-sonnet-4
  git:
    branch: feat/auth-refactor
    commit: 7b3f1a2

lifecycle:
  promoted_to_task: ""
  dismissed_reason: ""
```
