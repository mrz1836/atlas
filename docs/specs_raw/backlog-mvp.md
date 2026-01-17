# ATLAS Work Backlog: Discovered Issues Tracking

## Technical Specification v1.0

**Author:** Z
**Date:** January 2026
**Status:** Design Specification
**Target:** ATLAS AI Task Lifecycle Automation System

---

## Executive Summary

The Work Backlog captures issues discovered during AI-assisted development that can't be addressed in the current task. It's a lightweight, project-local queue that prevents good observations from getting lost.

**Core Principle:** The AI should never silently pass over problems it notices. Every discovery gets recorded, nothing falls through the cracks.

---

## Constitution Alignment

| Principle | Alignment | Status |
|-----------|-----------|--------|
| I. Human Authority at Checkpoints | Human reviews/promotes discoveries; AI only captures observations | ✅ PASS |
| II. Git is the Backbone | `.atlas/backlog.yaml` committed with project, travels with code | ✅ PASS |
| III. Text is Truth | YAML format, human-editable, `cat`-able, no database | ✅ PASS |
| IV. Ship Then Iterate | MVP scope defined; export formats and bulk ops deferred | ✅ PASS |
| V. Context-First Go | `context.Context` in all Manager operations | ✅ PASS |
| VI. Validation Before Delivery | Backlog operations tested with race detection | ✅ PASS |
| VII. Transparent State | Backlog visible in project `.atlas/` directory | ✅ PASS |

---

## Problem Statement

### The Discovery Problem

During implementation, the AI frequently notices things:

- "This function has no error handling"
- "There's a potential race condition here"
- "This test file is missing coverage for edge cases"
- "TODO comment from 2023 that should be addressed"
- "This API is deprecated and should be migrated"

Currently, these observations are either:
1. **Lost** - AI mentions it in passing, conversation ends, forgotten
2. **Scope creep** - AI tries to fix it now, derailing the current task
3. **Manual tracking** - Human has to remember to file an issue later

### Success Criteria

| ID | Criterion | Measurement |
|----|-----------|-------------|
| SC-001 | Discovery captured in < 5 seconds | Time from command to confirmation |
| SC-002 | Zero discoveries lost after capture | All persisted to YAML immediately |
| SC-003 | Backlog file always valid YAML | Atomic write prevents corruption |
| SC-004 | Human reviews full backlog in < 30 seconds | `atlas backlog` displays grouped by severity |
| SC-005 | Related discoveries shown within 2 seconds | `atlas start` integration |
| SC-006 | Zero overhead when no discoveries | Empty backlog = no performance impact |
| SC-007 | Works offline | No external services required |

---

## Design Philosophy

### Principles

1. **Frictionless Capture**: Recording a discovery should be one command, minimal fields required.

2. **Project-Local**: Backlog lives in the repo (`.atlas/backlog.yaml`), travels with the code.

3. **Human-Editable**: YAML format, easy to review and modify in any editor.

4. **Non-Blocking**: Discoveries don't affect current task flow - fire and forget.

5. **Lightweight**: This is NOT a full issue tracker. No dependencies, no workflows, no assignments.

### What This Is NOT

- Not a replacement for GitHub Issues or Jira
- Not a task management system
- Not a dependency tracker (use Beads for that)
- Not a planning tool

It's a **scratch pad** for observations that deserve follow-up.

### Development Context

**ATLAS is unreleased software.** This means:

- No backwards compatibility requirements
- Breaking changes are welcome when they improve the design
- No migration paths needed for existing users (there are none)
- Schema versions are for future-proofing, not current migrations
- We can delete, rename, and refactor freely

---

## Data Model

### Backlog File Location

```
<project-root>/
├── .atlas/
│   ├── config.yaml      # Existing: project config
│   └── backlog.yaml     # NEW: discovered issues
└── ...
```

### backlog.yaml Schema

```yaml
# .atlas/backlog.yaml
# ATLAS Work Backlog - Discovered issues pending triage
#
# This file is safe to commit. It tracks observations made during
# AI-assisted development that warrant future attention.

version: "1.0"

# Backlog items awaiting triage
pending:
  - id: "disc-a1b2c3"
    title: "Missing error handling in config.Parse"
    description: |
      The Parse function doesn't handle the case where the config file
      exists but is empty. This causes a nil pointer panic.
    discovered_at: "2026-01-17T14:32:15Z"
    discovered_during: "task-20260117-143022"
    discovered_in_file: "config/parser.go"
    discovered_at_line: 47
    category: "bug"
    severity: "medium"
    context: |
      Found while implementing fix for null pointer in config loading.
      The AI was reviewing related code paths.
    tags:
      - error-handling
      - config

  - id: "disc-f4e5d6"
    title: "Deprecated API usage in auth module"
    description: "Using oauth2.NoContext which is deprecated since Go 1.7"
    discovered_at: "2026-01-17T15:10:33Z"
    discovered_during: "task-20260117-150801"
    discovered_in_file: "auth/oauth.go"
    category: "tech-debt"
    severity: "low"
    tags:
      - deprecation
      - auth

# Items promoted to actual tasks (for reference)
promoted:
  - id: "disc-x7y8z9"
    title: "Race condition in cache invalidation"
    promoted_at: "2026-01-16T09:00:00Z"
    promoted_to: "task-20260116-090512"
    outcome: "completed"  # or "in_progress", "abandoned"

# Items explicitly dismissed (won't fix, not relevant, etc.)
dismissed:
  - id: "disc-m1n2o3"
    title: "Consider using sync.Pool for buffers"
    dismissed_at: "2026-01-15T16:00:00Z"
    dismissed_reason: "Premature optimization - not a bottleneck"
    dismissed_by: "human"  # or "ai" if AI determined it was invalid
```

### Go Data Structures

```go
// internal/backlog/types.go

package backlog

import "time"

// Backlog represents the entire backlog file
type Backlog struct {
    Version   string          `yaml:"version"`
    Pending   []Discovery     `yaml:"pending"`
    Promoted  []PromotedItem  `yaml:"promoted,omitempty"`
    Dismissed []DismissedItem `yaml:"dismissed,omitempty"`
}

// Discovery represents a discovered issue pending triage
type Discovery struct {
    // Required fields
    ID          string    `yaml:"id"`
    Title       string    `yaml:"title"`
    DiscoveredAt time.Time `yaml:"discovered_at"`

    // Context fields (highly recommended)
    Description      string `yaml:"description,omitempty"`
    DiscoveredDuring string `yaml:"discovered_during,omitempty"` // Task ID
    DiscoveredInFile string `yaml:"discovered_in_file,omitempty"`
    DiscoveredAtLine int    `yaml:"discovered_at_line,omitempty"`
    Context          string `yaml:"context,omitempty"` // Why AI noticed this

    // Classification
    Category string   `yaml:"category,omitempty"` // bug, tech-debt, security, performance, test, docs
    Severity string   `yaml:"severity,omitempty"` // critical, high, medium, low
    Tags     []string `yaml:"tags,omitempty"`

    // For linking related discoveries
    RelatedTo []string `yaml:"related_to,omitempty"` // Other discovery IDs
}

// Category constants
const (
    CategoryBug         = "bug"
    CategoryTechDebt    = "tech-debt"
    CategorySecurity    = "security"
    CategoryPerformance = "performance"
    CategoryTest        = "test"
    CategoryDocs        = "docs"
    CategoryRefactor    = "refactor"
    CategoryFeature     = "feature"  // Enhancement idea
)

// Severity constants
const (
    SeverityCritical = "critical" // Blocks functionality, security issue
    SeverityHigh     = "high"     // Should be addressed soon
    SeverityMedium   = "medium"   // Normal priority
    SeverityLow      = "low"      // Nice to have
)

// PromotedItem tracks discoveries that became real tasks
type PromotedItem struct {
    ID         string    `yaml:"id"`
    Title      string    `yaml:"title"`
    PromotedAt time.Time `yaml:"promoted_at"`
    PromotedTo string    `yaml:"promoted_to"` // Task ID
    Outcome    string    `yaml:"outcome,omitempty"` // completed, in_progress, abandoned
}

// DismissedItem tracks discoveries that were intentionally ignored
type DismissedItem struct {
    ID              string    `yaml:"id"`
    Title           string    `yaml:"title"`
    DismissedAt     time.Time `yaml:"dismissed_at"`
    DismissedReason string    `yaml:"dismissed_reason"`
    DismissedBy     string    `yaml:"dismissed_by"` // "human" or "ai"
}
```

---

## CLI Commands

### atlas backlog (alias: atlas bl)

```bash
# List pending discoveries
atlas backlog
atlas backlog list
atlas backlog list --category bug
atlas backlog list --severity high,critical
atlas backlog list --tag auth

# Add a discovery (AI typically does this)
atlas backlog add "Missing error handling in Parse" \
    --file config/parser.go \
    --line 47 \
    --category bug \
    --severity medium \
    --description "Doesn't handle empty config file" \
    --tag error-handling,config

# Quick add (minimal fields)
atlas backlog add "TODO: add retry logic to HTTP client"

# Show details of a discovery
atlas backlog show disc-a1b2c3

# Promote to a task (starts atlas workflow)
atlas backlog promote disc-a1b2c3 --template bugfix
# This runs: atlas start "<title>" --template bugfix
# And moves the discovery to "promoted" section

# Dismiss a discovery
atlas backlog dismiss disc-a1b2c3 --reason "Won't fix - edge case"

# Bulk operations
atlas backlog dismiss --older-than 90d --reason "Stale - cleaning up"

# Statistics
atlas backlog stats
# Output:
# Pending: 12 (3 critical, 4 high, 5 medium)
# Promoted: 8 (7 completed, 1 in_progress)
# Dismissed: 4

# Export for integration with external tools
atlas backlog export --format json
atlas backlog export --format github-issues  # GitHub issue format
atlas backlog export --format csv
```

### Integration with atlas start

```bash
# When starting a new task, show relevant backlog items
atlas start "fix auth bug"

# Output includes:
# 📋 Related backlog items found:
#   • disc-f4e5d6: Deprecated API usage in auth module (low)
#   • disc-g7h8i9: Missing tests for token refresh (medium)
#
# Promote any of these instead? [y/N/show]

# Or explicitly start from backlog
atlas start --from-backlog disc-a1b2c3 --template bugfix
```

### AI-Friendly Commands

```bash
# JSON output for programmatic use
atlas backlog add "Issue title" --json
# Returns: {"id": "disc-x1y2z3", "created": true}

atlas backlog list --json
atlas backlog show disc-a1b2c3 --json

# Check if similar discovery exists (avoid duplicates)
atlas backlog find --file config/parser.go --json
atlas backlog find --title-contains "error handling" --json
```

---

## Testing Requirements

| ID | Requirement | Rationale |
|----|-------------|-----------|
| TR-001 | All backlog operations MUST have unit tests | Core functionality coverage |
| TR-002 | YAML round-trip serialization MUST preserve all fields including timestamps | Data integrity |
| TR-003 | Duplicate detection MUST be tested with edge cases (similar titles, same file) | Prevent duplicates |
| TR-004 | All tests MUST pass with `-race` flag enabled | Concurrent access safety |
| TR-005 | Minimum 80% line coverage for `internal/backlog/` package | Quality gate |
| TR-006 | Test fixtures MUST be provided for common states (empty, populated, corrupted) | Reproducible tests |
| TR-007 | CLI commands MUST have integration tests | End-to-end validation |

### Test Scenarios

**Manager Operations:**
- Add discovery with all fields → verify persistence
- Add discovery with minimal fields → verify defaults applied
- Add duplicate discovery → verify rejection with existing ID
- Promote non-existent ID → verify error
- Dismiss with empty reason → verify error
- List with filters → verify correct subset returned

**YAML Persistence:**
- Load corrupted file → verify graceful error with line number
- Load empty file → verify empty backlog created
- Save with special characters in title → verify YAML escaping
- Concurrent save operations → verify no corruption (race test)

**CLI Integration:**
- `atlas backlog add` → verify ID returned
- `atlas backlog list --json` → verify valid JSON output
- `atlas backlog promote` → verify task integration

---

## Edge Cases

| Scenario | Handling |
|----------|----------|
| `.atlas/backlog.yaml` corrupted | Load returns error with line number; suggest manual fix or `atlas backlog repair` |
| User manually edits YAML incorrectly | Graceful error on next load with specific parse error location |
| Duplicate discovery attempt | Reject with error showing existing discovery ID |
| Promoted task is later deleted | Keep promoted entry for audit trail (outcome updated if detectable) |
| Discovery references non-existent file | Allow with warning (file may have been deleted or renamed) |
| Very long discovery title (>200 chars) | Truncate with ellipsis in display; full title in show/export |
| Empty backlog file | Initialize with version "1.0" and empty sections |
| Concurrent add operations | Atomic write pattern prevents corruption |

---

## AI Integration

### CLAUDE.md Addition

```markdown
## Discovered Issues Protocol

When you notice something that should be fixed but is **outside the current task scope**:

1. **Record it immediately** - don't lose the observation:
   ```bash
   atlas backlog add "Brief description" \
       --file <file-you-were-looking-at> \
       --category <bug|tech-debt|security|performance|test|docs> \
       --severity <critical|high|medium|low>
   ```

2. **Continue with your current task** - don't get sidetracked

3. **Add context** if it's not obvious why this matters:
   ```bash
   atlas backlog add "Memory leak in connection pool" \
       --file db/pool.go \
       --line 142 \
       --category bug \
       --severity high \
       --description "Connections not returned on error path" \
       --context "Found while reviewing DB code for auth changes"
   ```

### What to Record

✅ **Do record:**
- Bugs you notice in code you're reading
- Missing error handling
- Security concerns
- Deprecated API usage
- Missing or inadequate tests
- Confusing code that needs refactoring
- Performance issues
- Documentation gaps

❌ **Don't record:**
- Things you're about to fix in this task
- Vague observations ("this file is messy")
- Personal preferences ("I'd use a different pattern")

### Severity Guide

- **critical**: Security vulnerability, data loss risk, blocks users
- **high**: Significant bug, missing important functionality
- **medium**: Normal bug, tech debt that slows development
- **low**: Minor issue, nice-to-have improvement
```

### Auto-Discovery Hook

ATLAS can prompt the AI to record discoveries at key moments:

```go
// internal/backlog/prompt.go

// DiscoveryPrompt generates a prompt asking AI about discoveries
func DiscoveryPrompt(stepName string, filesReviewed []string) string {
    return fmt.Sprintf(`
Before completing the %s step, consider:

Did you notice any issues in these files that are OUTSIDE the current task scope?
- %s

If yes, record them now:
  atlas backlog add "<title>" --file <file> --category <type> --severity <level>

If no discoveries, just continue.
`, stepName, strings.Join(filesReviewed, "\n- "))
}
```

This prompt can be injected at the end of `analyze` or `implement` steps.

---

## Implementation

### Core Operations

```go
// internal/backlog/backlog.go

package backlog

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "gopkg.in/yaml.v3"
)

// Manager handles backlog operations
type Manager struct {
    path    string
    backlog *Backlog
}

// Load loads or creates a backlog for the given project
func Load(projectRoot string) (*Manager, error) {
    path := filepath.Join(projectRoot, ".atlas", "backlog.yaml")

    m := &Manager{path: path}

    if _, err := os.Stat(path); os.IsNotExist(err) {
        // Create new backlog
        m.backlog = &Backlog{
            Version: "1.0",
            Pending: []Discovery{},
        }
        return m, nil
    }

    // Load existing
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("reading backlog: %w", err)
    }

    var bl Backlog
    if err := yaml.Unmarshal(data, &bl); err != nil {
        return nil, fmt.Errorf("parsing backlog: %w", err)
    }

    m.backlog = &bl
    return m, nil
}

// Add creates a new discovery
func (m *Manager) Add(d Discovery) (*Discovery, error) {
    // Generate ID if not provided
    if d.ID == "" {
        d.ID = generateDiscoveryID()
    }

    // Set timestamp
    if d.DiscoveredAt.IsZero() {
        d.DiscoveredAt = time.Now()
    }

    // Validate required fields
    if d.Title == "" {
        return nil, fmt.Errorf("title is required")
    }

    // Check for duplicates (same file + similar title)
    if dup := m.findSimilar(d); dup != nil {
        return nil, fmt.Errorf("similar discovery exists: %s", dup.ID)
    }

    m.backlog.Pending = append(m.backlog.Pending, d)

    if err := m.save(); err != nil {
        return nil, err
    }

    return &d, nil
}

// Promote moves a discovery to a task
func (m *Manager) Promote(id string, taskID string) error {
    var discovery *Discovery
    var idx int

    for i, d := range m.backlog.Pending {
        if d.ID == id {
            discovery = &d
            idx = i
            break
        }
    }

    if discovery == nil {
        return fmt.Errorf("discovery not found: %s", id)
    }

    // Move to promoted
    promoted := PromotedItem{
        ID:         discovery.ID,
        Title:      discovery.Title,
        PromotedAt: time.Now(),
        PromotedTo: taskID,
        Outcome:    "in_progress",
    }

    m.backlog.Promoted = append(m.backlog.Promoted, promoted)

    // Remove from pending
    m.backlog.Pending = append(m.backlog.Pending[:idx], m.backlog.Pending[idx+1:]...)

    return m.save()
}

// Dismiss marks a discovery as intentionally ignored
func (m *Manager) Dismiss(id string, reason string, by string) error {
    var discovery *Discovery
    var idx int

    for i, d := range m.backlog.Pending {
        if d.ID == id {
            discovery = &d
            idx = i
            break
        }
    }

    if discovery == nil {
        return fmt.Errorf("discovery not found: %s", id)
    }

    // Move to dismissed
    dismissed := DismissedItem{
        ID:              discovery.ID,
        Title:           discovery.Title,
        DismissedAt:     time.Now(),
        DismissedReason: reason,
        DismissedBy:     by,
    }

    m.backlog.Dismissed = append(m.backlog.Dismissed, dismissed)

    // Remove from pending
    m.backlog.Pending = append(m.backlog.Pending[:idx], m.backlog.Pending[idx+1:]...)

    return m.save()
}

// ListPending returns filtered pending discoveries
func (m *Manager) ListPending(opts ListOptions) []Discovery {
    results := make([]Discovery, 0)

    for _, d := range m.backlog.Pending {
        if opts.Category != "" && d.Category != opts.Category {
            continue
        }
        if opts.Severity != "" && d.Severity != opts.Severity {
            continue
        }
        if opts.Tag != "" && !containsTag(d.Tags, opts.Tag) {
            continue
        }
        if opts.File != "" && d.DiscoveredInFile != opts.File {
            continue
        }
        results = append(results, d)
    }

    return results
}

// FindRelated finds discoveries related to a given query (for atlas start)
func (m *Manager) FindRelated(keywords []string, files []string) []Discovery {
    results := make([]Discovery, 0)

    for _, d := range m.backlog.Pending {
        // Match by file
        for _, f := range files {
            if d.DiscoveredInFile == f {
                results = append(results, d)
                break
            }
        }

        // Match by keyword in title/description
        for _, kw := range keywords {
            if containsIgnoreCase(d.Title, kw) || containsIgnoreCase(d.Description, kw) {
                results = append(results, d)
                break
            }
        }
    }

    return deduplicate(results)
}

// Stats returns backlog statistics
func (m *Manager) Stats() BacklogStats {
    stats := BacklogStats{
        PendingTotal: len(m.backlog.Pending),
    }

    for _, d := range m.backlog.Pending {
        switch d.Severity {
        case SeverityCritical:
            stats.PendingCritical++
        case SeverityHigh:
            stats.PendingHigh++
        case SeverityMedium:
            stats.PendingMedium++
        case SeverityLow:
            stats.PendingLow++
        }
    }

    stats.PromotedTotal = len(m.backlog.Promoted)
    for _, p := range m.backlog.Promoted {
        if p.Outcome == "completed" {
            stats.PromotedCompleted++
        }
    }

    stats.DismissedTotal = len(m.backlog.Dismissed)

    return stats
}

// save writes the backlog to disk
func (m *Manager) save() error {
    // Ensure directory exists
    dir := filepath.Dir(m.path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }

    data, err := yaml.Marshal(m.backlog)
    if err != nil {
        return err
    }

    // Add header comment
    header := `# ATLAS Work Backlog - Discovered issues pending triage
#
# This file is safe to commit. It tracks observations made during
# AI-assisted development that warrant future attention.
#
# Commands:
#   atlas backlog list          - View pending items
#   atlas backlog promote <id>  - Convert to task
#   atlas backlog dismiss <id>  - Mark as won't fix

`
    return os.WriteFile(m.path, []byte(header+string(data)), 0644)
}

func generateDiscoveryID() string {
    bytes := make([]byte, 4)
    rand.Read(bytes)
    return "disc-" + hex.EncodeToString(bytes)
}

// ListOptions for filtering
type ListOptions struct {
    Category string
    Severity string
    Tag      string
    File     string
}

// BacklogStats for reporting
type BacklogStats struct {
    PendingTotal     int
    PendingCritical  int
    PendingHigh      int
    PendingMedium    int
    PendingLow       int
    PromotedTotal    int
    PromotedCompleted int
    DismissedTotal   int
}
```

---

## Workflow Examples

### Example 1: AI Discovers Bug During Implementation

```bash
# AI is working on auth feature, notices a bug in config loading

# AI runs:
atlas backlog add "Config parser panics on empty file" \
    --file config/parser.go \
    --line 47 \
    --category bug \
    --severity medium \
    --description "Parse() doesn't handle case where file exists but is empty" \
    --context "Noticed while reviewing config loading for auth changes"

# Output:
# ✓ Added to backlog: disc-a1b2c3d4
#   Config parser panics on empty file (bug/medium)

# AI continues with current task...
```

### Example 2: Human Reviews Backlog

```bash
# End of day, human reviews discoveries
atlas backlog

# Output:
# ATLAS Work Backlog
# ==================
#
# Pending: 5 items
#
# CRITICAL (1):
#   disc-x1y2z3  SQL injection in user search  [security]
#                db/queries.go:142
#
# HIGH (2):
#   disc-a1b2c3  Config parser panics on empty file  [bug]
#                config/parser.go:47
#   disc-b2c3d4  Race condition in cache refresh  [bug]
#                cache/refresh.go:89
#
# MEDIUM (2):
#   disc-c3d4e5  Deprecated oauth2.NoContext usage  [tech-debt]
#                auth/oauth.go:23
#   disc-d4e5f6  Missing test for error path  [test]
#                handlers/user_test.go

# Human promotes the critical security issue
atlas backlog promote disc-x1y2z3 --template bugfix

# Output:
# 🚀 Promoting: SQL injection in user search
#
# Starting task with template: bugfix
# ...
```

### Example 3: Atlas Start Shows Related Items

```bash
# Human starts a task related to auth
atlas start "add OAuth refresh token support" --template feature

# Output:
# 📋 Related backlog items found:
#
#   HIGH    disc-b2c3d4  Race condition in cache refresh
#           Found in: cache/refresh.go (may affect token caching)
#
#   MEDIUM  disc-c3d4e5  Deprecated oauth2.NoContext usage
#           Found in: auth/oauth.go (directly related to OAuth)
#
# Would you like to:
#   [1] Start fresh task (ignore backlog)
#   [2] Promote disc-c3d4e5 instead (address tech debt first)
#   [3] Include both in task scope (larger task)
#
# Choice [1]:
```

### Example 4: Bulk Cleanup

```bash
# Clean up old discoveries that are no longer relevant
atlas backlog list --older-than 60d

# Output:
# Found 3 discoveries older than 60 days:
#   disc-old001  Improve logging in auth (92 days old)
#   disc-old002  Add metrics to handler (78 days old)
#   disc-old003  Consider caching user prefs (65 days old)

atlas backlog dismiss disc-old001 disc-old002 disc-old003 \
    --reason "Stale - code has changed significantly"

# Output:
# ✓ Dismissed 3 discoveries
```

---

## Integration with Hook Files

The backlog integrates with the Hook Files system:

```go
// When a task completes, check if AI discovered anything
func (h *Hook) OnTaskComplete() {
    if len(h.Discoveries) > 0 {
        // AI recorded discoveries during this task
        backlog, _ := backlog.Load(h.ProjectRoot)

        for _, d := range h.Discoveries {
            d.DiscoveredDuring = h.TaskID
            backlog.Add(d)
        }
    }
}
```

The `HOOK.md` file can include a discoveries section:

```markdown
## Discoveries Made This Session

You recorded 2 discoveries during this task:

| ID | Title | Severity |
|----|-------|----------|
| disc-a1b2c3 | Config parser panics on empty file | medium |
| disc-b2c3d4 | Missing test for auth edge case | low |

These are saved in `.atlas/backlog.yaml` and won't be lost.
```

---

## Configuration

```yaml
# .atlas/config.yaml

backlog:
  # Prompt AI to check for discoveries at end of certain steps
  prompt_after_steps:
    - analyze
    - implement

  # Auto-suggest related backlog items when starting tasks
  suggest_on_start: true

  # Severity threshold for suggestions (only show high+ by default)
  suggest_min_severity: medium

  # Auto-archive promoted items after N days
  archive_promoted_after: 90
```

---

## Clarifications

Design decisions made during specification:

### Session 2026-01-17

| Question | Decision | Rationale |
|----------|----------|-----------|
| Which YAML library? | `gopkg.in/yaml.v3` | Already in project dependencies, standard choice |
| Where does backlog.yaml live? | `.atlas/backlog.yaml` in project root | Project-local, travels with code (not `~/.atlas/`) |
| Duplicate detection algorithm? | Same file path + title Levenshtein similarity > 70% | Balance precision vs recall |
| Should backlog integrate with hook system? | Yes, discoveries recorded during task execution | Seamless AI workflow |
| Max discoveries before performance? | No hard limit; recommend triage if > 1000 | YAML scales adequately |
| ID format? | `disc-{8 hex chars}` | Short, unique, collision-resistant |
| Category/severity required? | Optional with defaults | Frictionless capture is priority |

---

## MVP Scope

### In Scope (MVP)

- `atlas backlog list/add/show/promote/dismiss` commands
- `atlas backlog stats` (simple counts)
- JSON output for all commands (`--json` flag)
- Duplicate detection (same file + similar title)
- Related discovery suggestions in `atlas start`
- Hook system integration (record discoveries during tasks)
- CLAUDE.md documentation for AI usage

### Deferred (Post-MVP)

| Feature | Rationale |
|---------|-----------|
| Export formats (GitHub Issues, CSV) | Add when real usage demands integration |
| Bulk dismiss `--older-than` | Individual dismiss sufficient for MVP |
| Auto-discovery prompts at step boundaries | Hook integration covers the core use case |
| `atlas backlog repair` command | Manual YAML editing is fallback |
| Related item matching with ML | Simple keyword matching sufficient |

---

## Migration Plan

### Phase 1: Setup & Types
*Prerequisites: None*

- [ ] Create `internal/backlog/types.go` with Backlog, Discovery, PromotedItem, DismissedItem structs
- [ ] Add constants to `internal/constants/backlog.go` (categories, severities, ID prefix)
- [ ] Add `BacklogConfig` to `internal/config/config.go` (if needed)
- [ ] Write type validation tests

**Deliverable:** Domain types ready for manager implementation

### Phase 2: Core Manager (US1+US2 - Capture & Storage)
*Prerequisites: Phase 1*

- [ ] Implement `Manager` struct with Load/Save using atomic write pattern
- [ ] Implement `Add()` with ID generation and timestamp
- [ ] Implement `ListPending()` with filter options
- [ ] Implement duplicate detection (`findSimilar()`)
- [ ] Unit tests with race detection (`-race` flag)
- [ ] Test fixtures for empty, populated, corrupted states

**Deliverable:** Backlog can be created, loaded, and items added

### Phase 3: Triage Operations (US3)
*Prerequisites: Phase 2*

- [ ] Implement `Promote()` - move to promoted section with task ID
- [ ] Implement `Dismiss()` - move to dismissed section with reason
- [ ] Implement `Get()` - retrieve single discovery by ID
- [ ] Implement `Stats()` - count by severity and outcome
- [ ] Unit tests for all triage operations

**Deliverable:** Full triage workflow functional

### Phase 4: CLI Commands
*Prerequisites: Phase 3*

- [ ] Add `atlas backlog` command group with `bl` alias
- [ ] Implement `atlas backlog list` with `--category`, `--severity`, `--tag`, `--json` flags
- [ ] Implement `atlas backlog add` with all field flags
- [ ] Implement `atlas backlog show <id>` with `--json` flag
- [ ] Implement `atlas backlog promote <id>` with `--template` flag
- [ ] Implement `atlas backlog dismiss <id>` with `--reason` flag
- [ ] Implement `atlas backlog stats`
- [ ] CLI integration tests

**Deliverable:** All CLI commands functional

### Phase 5: Integration (US4+US5)
*Prerequisites: Phase 4, Hook System MVP*

- [ ] Integrate with Hook system - record discoveries during task execution
- [ ] Add `FindRelated()` for keyword/file matching
- [ ] Integrate with `atlas start` - show related discoveries before task creation
- [ ] Add `--from-backlog` flag to `atlas start`
- [ ] Integration tests

**Deliverable:** Seamless AI and task workflow integration

### Phase 6: Documentation & Polish
*Prerequisites: Phase 5*

- [ ] Add CLAUDE.md section for AI discovery protocol
- [ ] Update `docs/internal/quick-start.md`:
  - [ ] Add `atlas backlog` to CLI Commands Reference
  - [ ] Document categories and severities
  - [ ] Add workflow examples
  - [ ] Add troubleshooting entries
- [ ] Verify all success criteria (SC-001 through SC-007)
- [ ] Verify all testing requirements (TR-001 through TR-007)

**Deliverable:** Feature complete with documentation

---

## Comparison with Beads

| Aspect | ATLAS Backlog | Beads |
|--------|---------------|-------|
| Scope | Single project | Multi-project |
| Storage | YAML file | JSONL + SQLite |
| Dependencies | None | Full graph |
| Workflows | None | Formulas, molecules |
| Multi-agent | No | Yes |
| Complexity | ~500 LOC | ~130k LOC |
| Purpose | Scratch pad | Full issue tracker |

The backlog is intentionally minimal. If you need Beads-level features, use Beads. The backlog is for quick capture during focused work.

---

*End of Specification*
