# Story 2.1: Implement Tool Detection System

Status: done

## Story

As a **user**,
I want **ATLAS to automatically detect installed tools**,
So that **I know which dependencies are available and which need to be installed**.

## Acceptance Criteria

1. **Given** the CLI root command exists **When** I implement the tool detection system in `internal/config/tools.go` **Then** the system can detect and report status for:
   - Go (version 1.24+, required)
   - Git (version 2.20+, required)
   - gh CLI (version 2.20+, required)
   - uv (version 0.5.x, required)
   - claude CLI (version 2.0.76+, required)
   - mage-x (managed by ATLAS)
   - go-pre-commit (managed by ATLAS)
   - Speckit (managed by ATLAS)

2. **Each tool reports:** installed/missing/outdated status, current version, required version

3. **Detection uses** `exec.LookPath` and version parsing

4. **Detection completes** in under 2 seconds

5. **Missing required tools** return clear error with install instructions

6. **Tests mock command execution** for reliable testing

## Tasks / Subtasks

- [x] Task 1: Create tool types and constants (AC: #1, #2)
  - [x] 1.1: Define `ToolStatus` type (Installed, Missing, Outdated) in `internal/config/tools.go`
  - [x] 1.2: Define `Tool` struct with Name, Required, Managed, MinVersion, CurrentVersion, Status, InstallHint
  - [x] 1.3: Define `ToolDetectionResult` struct to hold all tool detection results
  - [x] 1.4: Add tool-specific constants to `internal/constants/tools.go`

- [x] Task 2: Implement tool detection interface (AC: #3)
  - [x] 2.1: Create `ToolDetector` interface with `Detect(ctx context.Context) (*ToolDetectionResult, error)`
  - [x] 2.2: Create `DefaultToolDetector` struct implementing the interface
  - [x] 2.3: Implement `exec.LookPath` for tool existence checking
  - [x] 2.4: Implement version command execution for each tool

- [x] Task 3: Implement version parsing for each tool (AC: #1, #2)
  - [x] 3.1: Go: `go version` → parse "go1.24.2"
  - [x] 3.2: Git: `git --version` → parse "git version 2.39.0"
  - [x] 3.3: gh: `gh --version` → parse "gh version 2.62.0"
  - [x] 3.4: uv: `uv --version` → parse "uv 0.5.x"
  - [x] 3.5: claude: `claude --version` → parse version string
  - [x] 3.6: mage-x: `magex --version` → parse version
  - [x] 3.7: go-pre-commit: `go-pre-commit --version` → parse version
  - [x] 3.8: Speckit: `specify --version` or `speckit --version` → parse version

- [x] Task 4: Implement version comparison logic (AC: #2)
  - [x] 4.1: Create `CompareVersions(current, required string) int` utility
  - [x] 4.2: Handle semver comparison (major.minor.patch)
  - [x] 4.3: Handle version strings with prefixes (e.g., "go1.24.2", "v2.0.76")

- [x] Task 5: Implement install hints for missing tools (AC: #4)
  - [x] 5.1: Add install instructions for each required tool
  - [x] 5.2: Format as actionable user message

- [x] Task 6: Implement parallel detection with timeout (AC: #5)
  - [x] 6.1: Use `errgroup` for parallel tool detection
  - [x] 6.2: Add context timeout (2 second max)
  - [x] 6.3: Use `sync.Mutex` for thread-safe result accumulation

- [x] Task 7: Write comprehensive tests (AC: #6)
  - [x] 7.1: Create mock command executor for testing
  - [x] 7.2: Test each tool detection scenario (installed, missing, outdated)
  - [x] 7.3: Test version parsing for all tools
  - [x] 7.4: Test parallel detection timing
  - [x] 7.5: Run `magex test:race` to verify no race conditions

## Dev Notes

### Technical Requirements

**Package Location:** `internal/config/tools.go` (and `tools_test.go`)

**New Constants File:** `internal/constants/tools.go` for tool-related constants

**Import Rules (CRITICAL):**
- `internal/config/tools.go` MAY import: `internal/constants`, `internal/errors`
- `internal/config/tools.go` MUST NOT import: `internal/domain`, `internal/cli`
- `internal/constants/tools.go` MUST NOT import any internal packages

### Tool Detection Requirements

| Tool | Command | Version Flag | Min Version | Required | Managed |
|------|---------|--------------|-------------|----------|---------|
| Go | `go` | `version` | 1.24+ | Yes | No |
| Git | `git` | `--version` | 2.20+ | Yes | No |
| gh | `gh` | `--version` | 2.20+ | Yes | No |
| uv | `uv` | `--version` | 0.5.x | Yes | No |
| claude | `claude` | `--version` | 2.0.76+ | Yes | No |
| mage-x | `magex` | `--version` | any | No | Yes |
| go-pre-commit | `go-pre-commit` | `--version` | any | No | Yes |
| Speckit | `specify` | `--version` | any | No | Yes |

### Version Parsing Patterns

```go
// Go: "go version go1.24.2 darwin/arm64" → "1.24.2"
// Git: "git version 2.39.0" → "2.39.0"
// gh: "gh version 2.62.0 (2024-11-06)" → "2.62.0"
// uv: "uv 0.5.14 (bb7af57b8 2025-01-03)" → "0.5.14"
// claude: "Claude Code 2.0.76" or similar → parse appropriately
```

### Install Instructions (FR5)

```go
var installHints = map[string]string{
    "go":            "Install Go from https://go.dev/dl/ (version 1.24+)",
    "git":           "Install Git from https://git-scm.com/downloads (version 2.20+)",
    "gh":            "Install GitHub CLI: brew install gh (version 2.20+)",
    "uv":            "Install uv: curl -LsSf https://astral.sh/uv/install.sh | sh",
    "claude":        "Install Claude CLI: npm install -g @anthropic-ai/claude-code",
    "magex":         "Install with: go install github.com/mage-x/magex@latest",
    "go-pre-commit": "Install with: go install github.com/mrz1836/go-pre-commit@latest",
    "speckit":       "Install Speckit following https://github.com/speckit/speckit",
}
```

### Architecture Compliance

**Context-First Design (ARCH-13):**
```go
func (d *DefaultToolDetector) Detect(ctx context.Context) (*ToolDetectionResult, error) {
    // Check cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // Use ctx with timeout for detection
    detectCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    // ...
}
```

**Error Wrapping (ARCH-14):**
```go
// Wrap at package boundary only
if err != nil {
    return nil, errors.Wrap(err, "tool detection failed")
}
```

**Thread Safety (from Epic 1 learning):**
```go
// Use mutex for concurrent result accumulation
var (
    result   ToolDetectionResult
    resultMu sync.Mutex
)

g, ctx := errgroup.WithContext(detectCtx)
for _, tool := range tools {
    tool := tool // capture for goroutine
    g.Go(func() error {
        status := detectTool(ctx, tool)
        resultMu.Lock()
        result.Tools = append(result.Tools, status)
        resultMu.Unlock()
        return nil
    })
}
```

### File Structure

```
internal/
├── constants/
│   ├── constants.go      # Existing
│   ├── paths.go          # Existing
│   ├── status.go         # Existing
│   └── tools.go          # NEW: Tool-related constants
├── config/
│   ├── config.go         # Existing
│   ├── load.go           # Existing
│   ├── tools.go          # NEW: ToolDetector implementation
│   └── tools_test.go     # NEW: Tool detection tests
```

### Testing Patterns (from Epic 1)

**Mock Command Executor:**
```go
type MockCommandExecutor struct {
    responses map[string]struct {
        output string
        err    error
    }
}

func (m *MockCommandExecutor) LookPath(file string) (string, error) {
    // Return configured response
}

func (m *MockCommandExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
    // Return configured response based on name+args
}
```

**Table-Driven Tests:**
```go
func TestToolDetector_DetectGo(t *testing.T) {
    tests := []struct {
        name           string
        lookPathErr    error
        versionOutput  string
        versionErr     error
        expectedStatus ToolStatus
        expectedVersion string
    }{
        {
            name:           "installed and current",
            versionOutput:  "go version go1.24.2 darwin/arm64",
            expectedStatus: ToolStatusInstalled,
            expectedVersion: "1.24.2",
        },
        {
            name:           "outdated version",
            versionOutput:  "go version go1.21.0 darwin/arm64",
            expectedStatus: ToolStatusOutdated,
            expectedVersion: "1.21.0",
        },
        {
            name:           "not installed",
            lookPathErr:    exec.ErrNotFound,
            expectedStatus: ToolStatusMissing,
        },
    }
    // ...
}
```

### Previous Story Intelligence (Epic 1 Learnings)

**CRITICAL: Race Condition Prevention**
- Epic 1 had a race condition in `globalLogger` that was only caught by CI with `-race` flag
- **Always use `sync.Mutex` or `sync.RWMutex`** for any shared state in concurrent operations
- **Always run `magex test:race`** before marking story done

**Pattern Established:**
- Function-based Cobra commands (avoids gochecknoglobals)
- `snake_case` JSON tags consistently
- Comprehensive table-driven tests
- Boundary error wrapping with `%w`

### Project Structure Notes

- Tool detection lives in `internal/config/` alongside other configuration logic
- Constants for tools go in `internal/constants/tools.go`
- No new packages needed - extend existing structure
- Follows established pattern: one main file + one test file

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 2.1]
- [Source: _bmad-output/planning-artifacts/architecture.md#External Tool Integration]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns]
- [Source: _bmad-output/planning-artifacts/prd.md#FR5]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: _bmad-output/implementation-artifacts/epic-1-retro-2025-12-27.md#Race Condition Finding]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix    # Format code
magex lint          # Lint code (must pass)
magex test:race     # Run tests WITH race detection (CRITICAL)
go build ./...      # Verify compilation
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

No debug issues encountered during implementation.

### Completion Notes List

- Implemented complete tool detection system with 8 tools (Go, Git, gh, uv, Claude, mage-x, go-pre-commit, Speckit)
- Created `ToolStatus` type with Installed/Missing/Outdated states
- Created `Tool` struct with all required fields including InstallHint
- Implemented `ToolDetector` interface with `DefaultToolDetector` implementation
- Used `CommandExecutor` interface for testability with mock in tests
- Implemented version parsing for all 8 tools with regex patterns
- Created `CompareVersions` function for semver comparison handling 'v' prefixes
- Used `errgroup` for parallel detection with 2-second timeout
- Used `sync.Mutex` for thread-safe result accumulation
- All tests pass with race detection (`magex test:race`)
- Added `ErrCommandNotConfigured` and `ErrCommandFailed` sentinel errors to errors package
- 31 test cases covering all detection scenarios, version parsing, timeout behavior, and JSON marshaling

### Change Log

- 2025-12-27: Implemented tool detection system with all tasks complete
- 2025-12-27: Code review - Added timeout test, JSON marshaling, fixed receiver consistency

### File List

- `internal/constants/tools.go` (new) - Tool-related constants (tool names, min versions, timeout)
- `internal/config/tools.go` (new) - ToolDetector implementation with parallel detection
- `internal/config/tools_test.go` (new) - Comprehensive tests with mock executor
- `internal/errors/errors.go` (modified) - Added ErrCommandNotConfigured and ErrCommandFailed
- `go.mod` (modified) - Added golang.org/x/sync dependency
- `go.sum` (modified) - Updated checksums for new dependency

## Senior Developer Review (AI)

**Reviewer:** Claude Opus 4.5
**Date:** 2025-12-27
**Outcome:** ✅ APPROVED with fixes applied

### Issues Found and Fixed

| Severity | Issue | Resolution |
|----------|-------|------------|
| HIGH | go.mod/go.sum modified but not in File List | Added to File List |
| MEDIUM | Missing timeout behavior test (AC #4) | Added `TestToolDetector_TimeoutBehavior` with `SlowMockExecutor` |
| MEDIUM | ToolStatus JSON marshals as int, not string | Added `MarshalJSON()` and `UnmarshalJSON()` methods |
| MEDIUM | Mixed receiver types on ToolStatus | Changed `String()` to pointer receiver |

### Review Summary

- All 8 tools correctly detected: Go, Git, gh, uv, Claude, mage-x, go-pre-commit, Speckit
- Parallel detection with 2-second timeout verified
- 31 test cases now covering all scenarios including timeout behavior
- JSON serialization produces human-readable status strings
- All lint and race tests pass
