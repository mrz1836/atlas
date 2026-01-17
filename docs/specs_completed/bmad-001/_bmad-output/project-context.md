---
project_name: 'atlas'
user_name: 'MrZ'
date: '2025-12-27'
sections_completed: ['technology_stack', 'language_rules', 'testing_rules', 'code_quality', 'workflow', 'anti_patterns']
status: 'complete'
rule_count: 25
optimized_for_llm: true
---

# Project Context for AI Agents

_Critical rules and patterns for implementing ATLAS. Read before writing any code._

---

## Technology Stack & Versions

| Technology | Version | Purpose |
|------------|---------|---------|
| Go | 1.24+ | Primary language |
| spf13/cobra | latest | CLI framework |
| spf13/viper | latest | Configuration management |
| charmbracelet/huh | latest | Interactive forms |
| charmbracelet/lipgloss | latest | Terminal styling |
| charmbracelet/bubbles | latest | TUI widgets |
| rs/zerolog | latest | Structured JSON logging |
| stretchr/testify | latest | Testing assertions |

**Build Tools:**
- MAGE-X (`magex`) - Build automation
- golangci-lint - Code linting
- go-pre-commit - Git hooks

---

## Critical Implementation Rules

### Go Language Rules

**Context Handling (CRITICAL):**
```go
// ✅ ALWAYS: ctx as first parameter
func (s *Service) DoWork(ctx context.Context, input Input) error

// ✅ ALWAYS: Check cancellation for long operations
select {
case <-ctx.Done():
    return ctx.Err()
default:
}

// ✅ ALWAYS: Derive child contexts with timeouts
aiCtx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()

// ❌ NEVER: Store context in structs
// ❌ NEVER: Use context.Background() except in main()
```

**Error Handling:**
```go
// ✅ Action-first format
return fmt.Errorf("failed to create worktree: %w", err)

// ✅ Wrap at package boundaries only
return errors.Wrap(err, "ai execution failed")

// ❌ Don't over-wrap
return fmt.Errorf("executor: run: invoke: %w", err)  // Too deep
```

**Imports & Organization:**
- Import from `internal/constants` - never inline magic strings
- Import from `internal/errors` - never define local sentinel errors
- Import from `internal/config` - never read env vars directly in business logic
- Import from `internal/domain` - shared types live here

### Validation Commands (CRITICAL)

**ALWAYS run ALL FOUR before committing Go code:**
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

### Package Import Rules

**Allowed Imports:**
- `cmd/atlas` → only `internal/cli`
- `internal/cli` → task, workspace, tui, config
- `internal/task` → ai, git, validation, template, domain
- All packages → constants, errors, config, domain

**Forbidden Imports:**
- `internal/domain` → must not import other internal packages
- `internal/constants` → must not import any package
- `internal/errors` → only std lib

### Testing Rules

**Structure:**
- Co-located tests: `*_test.go` in same directory
- Use testify: `assert`, `require`, `suite`
- Table-driven tests for multiple cases
- Target 90%+ coverage

**Naming:**
```go
func TestServiceName_MethodName_Scenario(t *testing.T)
func TestTaskEngine_Execute_CancelsOnTimeout(t *testing.T)
```

**Integration tests:** Use build tag `//go:build integration`

### JSON & Logging Conventions

**JSON fields:** Always `snake_case`
```go
type Task struct {
    TaskID    string `json:"task_id"`
    CreatedAt string `json:"created_at"`
}
```

**Log fields:** Descriptive `snake_case`
```go
log.Info().
    Str("workspace_name", ws.Name).
    Str("task_id", task.ID).
    Dur("duration_ms", elapsed).
    Msg("step completed")
```

### Code Organization

**File Naming:** `lowercase.go`, `lowercase_test.go`

**Package Structure:**
```
internal/
├── constants/   # All constants (files, dirs, timeouts, retries)
├── errors/      # All sentinel errors
├── config/      # All configuration
├── domain/      # Shared types (Task, Workspace, Step)
├── cli/         # One file per command
├── task/        # Task engine, state machine
├── workspace/   # Workspace/worktree management
├── ai/          # AIRunner interface, ClaudeCodeRunner
├── git/         # Git/GitHub operations
├── validation/  # Validation executor
├── template/    # Template system, step executors
└── tui/         # Charm TUI components
```

---

## Anti-Patterns (NEVER DO)

```go
// ❌ Magic strings
if task.Status == "running" { ... }
// ✅ Use constants.StatusRunning

// ❌ Local sentinel errors
var errNotFound = errors.New("not found")
// ✅ Use errors.ErrNotFound from internal/errors

// ❌ Direct env var access
os.Getenv("ATLAS_TIMEOUT")
// ✅ Use config.Load().AI.Timeout

// ❌ Context in struct
type Service struct { ctx context.Context }

// ❌ Global state or init() functions
var globalConfig Config  // DON'T
func init() { ... }      // DON'T

// ❌ camelCase in JSON
`json:"taskId"`
// ✅ Use snake_case: `json:"task_id"`
```

---

## Gitleaks Compliance (CRITICAL)

**Test values MUST NOT look like secrets:**
- ❌ NEVER use numeric suffixes: `_12345`, `_123`, `_98765`
- ❌ NEVER use words: `secret`, `api_key`, `password`, `token` with numeric values
- ✅ DO use semantic names: `ATLAS_TEST_ENV_INHERITED`, `mock_value_for_test`
- ✅ DO use letter suffixes if needed: `_xyz`, `_abc`, `_test`

**Examples:**
```go
// ❌ BAD - triggers gitleaks (numeric suffix patterns)
testEnvKey := "MY_VAR_<numbers>"  // e.g., _12345, _98765

// ✅ GOOD - safe test value
testEnvKey := "ATLAS_TEST_ENV_INHERITED"
```

---

## Git Workflow

**Branch naming:** `<type>/<description>`
- `feat/add-user-auth`
- `fix/null-pointer-config`

**Commit format:** Conventional commits
```
<type>(<scope>): <description>

fix(task): handle nil options in parseConfig
feat(cli): add workspace destroy command
```

**Before PR:** All must pass
```bash
magex format:fix && magex lint && magex test && go-pre-commit run --all-files
```

---

## Usage Guidelines

**For AI Agents:**
- Read this file before implementing any code
- Follow ALL rules exactly as documented
- Run `magex format:fix && magex lint && magex test && go-pre-commit run --all-files` before completing any code task
- When in doubt, prefer the more restrictive option
- Refer to `_bmad-output/planning-artifacts/architecture.md` for detailed architectural decisions

**For Humans:**
- Keep this file lean and focused on agent needs
- Update when technology stack changes
- Review quarterly for outdated rules
- Remove rules that become obvious over time

---

_Last Updated: 2025-12-27_
