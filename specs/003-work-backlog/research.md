# Research: Work Backlog Feature

**Branch**: `003-work-backlog` | **Date**: 2026-01-18

## Research Tasks

### 1. ID Generation Strategy

**Context**: Need unique, short, human-readable IDs for discovery files (`disc-<id>.yaml`)

**Decision**: Use crypto/rand to generate 6 alphanumeric characters

**Rationale**:
- 36^6 = ~2.2 billion possible combinations - sufficiently large, but we will use defensive programming
- 6 characters is short enough for humans to type/reference
- Pattern: `disc-` prefix + 6 lowercase alphanumeric characters (e.g., `disc-a1b2c3`)
- No external dependencies needed

**Alternatives Considered**:
- UUID v4: Rejected - too long for human use (32+ chars)
- Timestamp-based: Rejected - can collide on fast consecutive calls
- Sequential counter: Rejected - requires centralized state, merge conflicts
- Hash of content: Rejected - duplicate content would cause ID collision

**Implementation**:
```go
const idChars = "abcdefghijklmnopqrstuvwxyz0123456789"
const idLength = 6

func GenerateID() (string, error) {
    bytes := make([]byte, idLength)
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    for i := range bytes {
        bytes[i] = idChars[bytes[i]%byte(len(idChars))]
    }
    return "disc-" + string(bytes), nil
}
```

---

### 2. YAML Serialization Best Practices

**Context**: Discoveries are stored as YAML files, need consistent formatting

**Decision**: Use `gopkg.in/yaml.v3` with custom marshaling

**Rationale**:
- yaml.v3 is the standard Go YAML library, already used in ATLAS for config.yaml
- Supports field ordering control via struct tags
- Multi-line strings use `|` literal block style for readability
- Consistent indentation (2 spaces, standard YAML)

**Alternatives Considered**:
- JSON: Rejected - less human-readable, doesn't support comments
- TOML: Rejected - not standard in ATLAS, less common
- Custom format: Rejected - unnecessary complexity

**Key Patterns**:
- Use `yaml:"field_name"` tags for consistent naming
- Use `omitempty` for optional fields
- Use `time.RFC3339` for timestamps (ISO 8601)
- Add schema_version field for future compatibility

---

### 3. Concurrent Write Safety

**Context**: Multiple AI agents/worktrees may add discoveries simultaneously

**Decision**: Use `O_EXCL` (exclusive create) to prevent collisions, then safe write

 **Rationale**:
 - Although collisions are rare (1 in 2.2B), "Senior Level" code is defensive
 - `os.OpenFile(..., O_CREATE|O_EXCL)` fails if file exists
 - Prevents accidental overwrites of existing discoveries
 - Git handles merge: different files = no conflicts

 **Alternatives Considered**:
 - Atomic Rename: Rejected for *creation* because it can silently overwrite on POSIX
 - File locking (flock): Rejected - unnecessary for unique files, adds complexity
 - Database: Rejected - violates Constitution Principle III

**Implementation**:
 ```go
 func createSafe(path string, data []byte) error {
    // 1. Exclusive create - fails if file exists (avoids ID collision)
    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
    if err != nil {
        return err // caller handles (e.g., retry with new ID)
    }
    defer f.Close()

    // 2. Write data
    if _, err := f.Write(data); err != nil {
        return err
    }
    return f.Sync()
 }
 ```

---

### 4. Git Context Capture

**Context**: Need to capture current git branch and commit SHA when adding discoveries

**Decision**: Use existing `internal/git` package methods

**Rationale**:
- ATLAS already has git utilities in `internal/git/`
- Reuse `GetCurrentBranch()` and `GetCurrentCommit()`
- Handle errors gracefully - missing git context is not fatal (warn and continue)

**Alternatives Considered**:
- Shell out to git: Rejected - already have internal package
- New git library: Rejected - unnecessary dependency
- Ignore git context: Rejected - violates spec requirement SC-004

**Error Handling**:
- If git commands fail, log a warning and store empty strings
- Discovery is still valid without git context (user might be in non-git directory during testing)

---

### 5. Interactive Form Design (charmbracelet/huh)

**Context**: Need interactive form for human users adding discoveries

**Decision**: Use `charmbracelet/huh` multi-step form matching ATLAS patterns

**Rationale**:
- Already used throughout ATLAS (see `internal/tui/menus.go`)
- Provides consistent UX with existing commands
- Supports validation, theming, accessibility

**Form Structure**:
1. **Title** (required): Single-line input with validation (non-empty)
2. **Description** (optional): Multi-line text area
3. **Category** (required): Select from predefined list
4. **Severity** (required): Select from predefined list
5. **File** (optional): Single-line input for file path
6. **Line** (optional): Integer input for line number (only if file provided)

**Implementation Pattern**:
```go
form := huh.NewForm(
    huh.NewGroup(
        huh.NewInput().
            Title("What did you find?").
            Value(&title).
            Validate(validateNonEmpty),
    ),
    huh.NewGroup(
        huh.NewText().
            Title("Details (optional)").
            Value(&description).
            CharLimit(2000),
    ),
    huh.NewGroup(
        huh.NewSelect[string]().
            Title("Category").
            Options(categoryOptions...).
            Value(&category),
        huh.NewSelect[string]().
            Title("Severity").
            Options(severityOptions...).
            Value(&severity),
    ),
).WithTheme(huh.ThemeCharm())
```

---

### 6. List Command Performance

**Context**: Need to list 1000+ discoveries efficiently (< 2 seconds)

**Decision**: Parallel file reads with bounded concurrency

**Rationale**:
- Directory scan is fast (filepath.Glob)
- YAML parsing can be parallelized
- Limit concurrency to avoid file descriptor exhaustion

**Implementation**:
```go
const maxConcurrent = 50

func (m *Manager) List(ctx context.Context, filter Filter) ([]Discovery, error) {
    files, err := filepath.Glob(filepath.Join(m.dir, "disc-*.yaml"))
    if err != nil {
        return nil, err
    }

    // Use worker pool pattern for parallel reads
    results := make(chan Discovery, len(files))
    errors := make(chan error, len(files))
    sem := make(chan struct{}, maxConcurrent)

    var wg sync.WaitGroup
    for _, file := range files {
        wg.Add(1)
        go func(f string) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()

            d, err := m.loadFile(f)
            if err != nil {
                errors <- err
                return
            }
            if filter.Match(d) {
                results <- d
            }
        }(file)
    }

    // Collect results...
}
```

**Performance Notes**:
- Glob pattern `disc-*.yaml` avoids scanning non-discovery files
- Filter applied during load, not after (reduces memory)
- Malformed files logged as warnings, don't break list

---

### 7. CLI Command Structure

**Context**: How to organize CLI commands for the backlog feature

**Decision**: Command group pattern matching existing ATLAS commands

**Rationale**:
- Consistent with `atlas workspace`, `atlas hook`, `atlas config` patterns
- Clear hierarchy: `atlas backlog {add|list|view|promote|dismiss}`
- Each subcommand has dedicated file for maintainability

**Command Overview**:
| Command | Args | Key Flags | Output Modes |
|---------|------|-----------|--------------|
| `atlas backlog add [title]` | Optional title | `--file`, `--line`, `--severity`, `--category`, `--description`, `--tags` | text, json |
| `atlas backlog list` | None | `--status`, `--category`, `--limit` | text, json |
| `atlas backlog view <id>` | Discovery ID | None | text, json |
| `atlas backlog promote <id>` | Discovery ID | `--task-id` | text, json |
| `atlas backlog dismiss <id>` | Discovery ID | `--reason` (required) | text, json |

**Flag Patterns**:
- Short flags for common options: `-f` (file), `-s` (severity), `-c` (category)
- JSON output via global `--output json` flag
- Interactive mode when no title provided to `add`

---

### 8. Error Handling Strategy

**Context**: Define sentinel errors and error handling patterns

**Decision**: Add discovery-specific errors to `internal/errors/errors.go`

**New Sentinel Errors**:
```go
var (
    ErrDiscoveryNotFound    = errors.New("discovery not found")
    ErrInvalidDiscoveryID   = errors.New("invalid discovery ID format")
    ErrBacklogDirNotFound   = errors.New("backlog directory not found")
    ErrMalformedDiscovery   = errors.New("malformed discovery file")
    ErrDuplicateDiscoveryID = errors.New("discovery ID already exists")
)
```

**Error Wrapping Pattern**:
```go
return fmt.Errorf("failed to load discovery '%s': %w", id, ErrMalformedDiscovery)
```

**Exit Codes**:
- Exit 0: Success
- Exit 1: General errors (IO, unexpected)
- Exit 2: Invalid input (bad ID format, missing required args)

---

### 9. Discoverer Format

**Context**: How to identify who discovered an issue (AI agent vs human)

**Decision**: Typed format with identifier: `ai:<agent>:<model>` or `human:<github-username>`

**Rationale**:
- Distinguishes AI discoveries from human discoveries
- Records specific agent/model for AI traceability
- Uses GitHub username for humans (consistent with ATLAS PR workflow)

**Examples**:
- `ai:claude-code:claude-sonnet-4` - Claude Code with Sonnet 4 model
- `ai:gemini:flash` - Gemini CLI with Flash model
- `human:mrz` - Human with GitHub username

**Implementation**:
- Interactive mode: Auto-detect as `human:<git-user>` (from git config)
- Flag mode with `--ai` flag: Parse AI identifier
- Default for AI: Auto-detect from environment (ATLAS_AGENT, ATLAS_MODEL) if available

 ---

 ### 10. Rich Terminal Output

 **Context**: Spec requires "Amazing" user experience and "Premium Design"

 **Decision**: Use `charmbracelet/glamour` for rendering Markdown content in CLI

 **Rationale**:
 - **Constitution Compliance**: "Every dependency MUST justify its existence."
 - **Justification**: User requirement for "Premium Design" and "Amazing UX". `glamour` provides standard-compliant Markdown rendering with theme support (dark/light) at low implementation cost vs hand-rolling.
 - Renders standard Markdown (bold, code blocks, lists) as beautiful ANSI-colored text
 - Supports styles/themes (dark/light mode)
 - "Wow" factor when viewing issues in the terminal

 **Implementation**:
 - `atlas backlog view` renders the description using glamour
 - `atlas backlog list` uses `charmbracelet/lipgloss` for styled tables
 - Use color coding for severity (Critical=Red, High=Orange, etc.)

---

## Summary

All technical decisions align with:
- ATLAS codebase patterns (Cobra CLI, charmbracelet TUI, file-based state)
- Constitution principles (Git backbone, Text is Truth, MVP mindset)
- Performance requirements (< 5s create, < 2s list)
- Concurrency requirements (zero merge conflicts)

No NEEDS CLARIFICATION items remain. Ready to proceed to Phase 1 design.
