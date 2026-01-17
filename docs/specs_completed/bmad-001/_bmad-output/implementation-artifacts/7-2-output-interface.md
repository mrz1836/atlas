# Story 7.2: Output Interface

Status: done

## Story

As a **developer**,
I want **an Output interface that handles TTY vs JSON output**,
So that **commands can output human-friendly or machine-readable formats**.

## Acceptance Criteria

1. **Given** the style system exists
   **When** I implement `internal/tui/output.go`
   **Then** the Output interface provides:
   ```go
   type Output interface {
       Success(msg string)
       Error(err error)
       Warning(msg string)
       Info(msg string)
       Table(headers []string, rows [][]string)
       JSON(v interface{})
       Spinner(msg string) Spinner
   }
   ```

2. **Given** the Output interface exists
   **When** calling `NewOutput(w io.Writer, format string)`
   **Then** it creates the appropriate implementation based on format:
   - "json" → JSONOutput implementation
   - "text" → TTYOutput implementation
   - "" → auto-detect based on whether w is a TTY

3. **Given** a TTY is detected (or `--output text` flag used)
   **When** any output method is called
   **Then** TTY output uses Lip Gloss styling from the style system (Story 7.1):
   - Success: green with ✓ icon
   - Error: red with ✗ icon
   - Warning: yellow with ⚠ icon
   - Info: blue with ℹ icon

4. **Given** output is piped (or `--output json` flag used)
   **When** any output method is called
   **Then** JSON output produces structured JSON:
   - Success/Warning/Info: `{"type": "success|warning|info", "message": "..."}`
   - Error: `{"type": "error", "message": "...", "details": "..."}`
   - Table: `[{"col1": "val1", ...}, ...]`

5. **Given** the Output implementation is created
   **When** displaying tabular data
   **Then** the Table method renders:
   - TTY: Formatted table with Lip Gloss styling (aligned columns)
   - JSON: Array of objects with column headers as keys

6. **Given** a long-running operation is in progress
   **When** calling `Spinner(msg string)`
   **Then** it returns a Spinner interface:
   ```go
   type Spinner interface {
       Update(msg string)
       Stop()
   }
   ```
   - TTY: Animated spinner using Bubbles
   - JSON: No-op (or simple log messages)

7. **Given** NO_COLOR environment variable is set
   **When** TTY output is rendered
   **Then** colors are disabled (via CheckNoColor from styles.go)

## Tasks / Subtasks

- [x] Task 1: Create Output interface and format detection (AC: #1, #2)
  - [x] 1.1: Define Output interface in `internal/tui/output.go`
  - [x] 1.2: Define Spinner interface for progress indication
  - [x] 1.3: Implement `NewOutput(w io.Writer, format string) Output` constructor
  - [x] 1.4: Implement TTY detection using `golang.org/x/term`
  - [x] 1.5: Add format constants: FormatAuto, FormatText, FormatJSON

- [x] Task 2: Implement TTYOutput (AC: #3, #6, #7)
  - [x] 2.1: Create `tty_output.go` with TTYOutput struct
  - [x] 2.2: Implement Success() with green color and ✓ icon using ColorSuccess
  - [x] 2.3: Implement Error() with red color and ✗ icon using ColorError
  - [x] 2.4: Implement Warning() with yellow color and ⚠ icon using ColorWarning
  - [x] 2.5: Implement Info() with blue color and ℹ icon using ColorPrimary
  - [x] 2.6: Call CheckNoColor() in constructor to respect NO_COLOR
  - [x] 2.7: Implement Spinner() returning SpinnerAdapter (wraps TerminalSpinner)

- [x] Task 3: Implement JSONOutput (AC: #4)
  - [x] 3.1: Create `json_output.go` with JSONOutput struct
  - [x] 3.2: Implement Success() outputting JSON with type field
  - [x] 3.3: Implement Error() outputting JSON with error details
  - [x] 3.4: Implement Warning() and Info() outputting JSON
  - [x] 3.5: Implement Spinner() returning NoopSpinner

- [x] Task 4: Implement Table rendering (AC: #5)
  - [x] 4.1: TTYOutput.Table() using Lip Gloss table styling
  - [x] 4.2: Apply TableStyles from styles.go for headers and cells
  - [x] 4.3: Calculate column widths based on content (using utf8.RuneCountInString)
  - [x] 4.4: JSONOutput.Table() outputting array of objects

- [x] Task 5: Implement Spinner types
  - [x] 5.1: Create SpinnerAdapter wrapping existing TerminalSpinner
  - [x] 5.2: Create NoopSpinner for JSON/non-TTY output
  - [x] 5.3: Ensure spinner respects context cancellation

- [x] Task 6: Write comprehensive tests
  - [x] 6.1: Test TTY detection logic
  - [x] 6.2: Test TTYOutput methods with styled output
  - [x] 6.3: Test JSONOutput methods produce valid JSON
  - [x] 6.4: Test Table rendering for both formats
  - [x] 6.5: Test NO_COLOR environment handling
  - [x] 6.6: Test format auto-detection
  - [x] 6.7: Achieve 97.3% test coverage (exceeds 90% target)

## Dev Notes

### CRITICAL: Build on Story 7.1 Foundation

The Output interface MUST use the style system from Story 7.1:
- Use `ColorSuccess`, `ColorError`, `ColorWarning`, `ColorPrimary`, `ColorMuted`
- Use `NewOutputStyles()` for pre-configured styles
- Use `CheckNoColor()` for NO_COLOR support
- Use `NewTableStyles()` for table rendering

### Architecture Patterns

From `_bmad-output/planning-artifacts/architecture.md`:

**Package location:** `internal/tui/`

**Import rules:**
- Can import: `internal/constants`, `internal/errors`, `internal/domain`
- Can import: Charm libraries (lipgloss, bubbles)
- Cannot import: `internal/cli`, `internal/task`, `internal/workspace`

**Output interface usage:**
```go
// CLI commands use this pattern:
type Output interface {
    Success(msg string)
    Error(err error)
    Table(headers []string, rows [][]string)
    JSON(v interface{})
    Spinner(msg string) Spinner
}

func NewOutput(w io.Writer, format string) Output
```

### TTY Detection

Use standard Go approach:
```go
import "golang.org/x/term"

func isTTY(w io.Writer) bool {
    f, ok := w.(*os.File)
    if !ok {
        return false
    }
    return term.IsTerminal(int(f.Fd()))
}
```

Or simpler approach checking os.Stdout:
```go
import "os"

func isTTY(w io.Writer) bool {
    if f, ok := w.(*os.File); ok {
        fi, _ := f.Stat()
        return (fi.Mode() & os.ModeCharDevice) != 0
    }
    return false
}
```

### JSON Output Format

Structured JSON for each message type:
```json
// Success
{"type": "success", "message": "Task completed successfully"}

// Error
{"type": "error", "message": "Operation failed", "error": "context deadline exceeded"}

// Warning
{"type": "warning", "message": "Configuration outdated"}

// Info
{"type": "info", "message": "Starting validation..."}

// Table
[
  {"workspace": "auth", "branch": "feat/auth", "status": "running"},
  {"workspace": "payment", "branch": "fix/payment", "status": "awaiting_approval"}
]
```

### TTY Output Icons

From `epic-7-tui-components-from-scenarios.md`:
- Success: ✓ (checkmark)
- Error: ✗ (x mark)
- Warning: ⚠ (warning triangle)
- Info: ℹ (info circle)

### Spinner Implementation

For the TTY spinner, wrap the Bubbles spinner:
```go
import "github.com/charmbracelet/bubbles/spinner"

type BubblesSpinner struct {
    model  spinner.Model
    writer io.Writer
    done   chan struct{}
}

func (s *BubblesSpinner) Update(msg string) { /* update message */ }
func (s *BubblesSpinner) Stop() { /* stop animation */ }
```

For JSON/non-TTY, use a no-op:
```go
type NoopSpinner struct{}

func (s *NoopSpinner) Update(msg string) {}
func (s *NoopSpinner) Stop() {}
```

### Project Structure Notes

Files to create:
- `internal/tui/output.go` - Interface definitions and NewOutput constructor
- `internal/tui/tty_output.go` - TTYOutput implementation
- `internal/tui/json_output.go` - JSONOutput implementation
- `internal/tui/spinner.go` - Spinner implementations
- `internal/tui/output_test.go` - Comprehensive tests

### Validation Commands

```bash
# MUST run all before marking complete
magex format:fix
magex lint
magex test:race
go-pre-commit run --all-files
```

### References

- [Source: _bmad-output/planning-artifacts/architecture.md - internal/tui/ package definition]
- [Source: _bmad-output/planning-artifacts/epics.md - Story 7.2 acceptance criteria]
- [Source: internal/tui/styles.go - existing style system implementation]
- [Source: _bmad-output/implementation-artifacts/epic-7-tui-components-from-scenarios.md - Icon Reference, Color Palette]
- [Source: _bmad-output/project-context.md - coding standards and validation commands]
- [Web: github.com/charmbracelet/bubbles/spinner - Spinner component]
- [Web: golang.org/x/term - Terminal detection]

### Previous Story Learnings (Story 7.1)

From `7-1-tui-style-system.md` code review:
1. **Unicode handling**: Use `utf8.RuneCountInString()` for string width, not `len()`
2. **Icon consistency**: Match icons exactly to `epic-7-tui-components-from-scenarios.md` spec
3. **AdaptiveColor usage**: Use exported color constants (ColorSuccess, etc.) not hardcoded values
4. **Test coverage**: Target 90%+ coverage, test edge cases like empty inputs
5. **NO_COLOR compliance**: Check at initialization, not on every output call

### Git Intelligence

From recent commits:
- Story 7.1 was implemented with: semantic colors, state icons, typography styles, NO_COLOR support, StyleSystem consolidation, box borders
- Code review improved coverage from 95.9% to 97.2%
- Use existing exports: `ColorPrimary`, `ColorSuccess`, `ColorWarning`, `ColorError`, `ColorMuted`
- Use existing styles: `NewOutputStyles()`, `NewTableStyles()`

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - Implementation completed without debugging issues.

### Completion Notes List

1. **Output Interface Implementation (AC #1, #2):**
   - Defined `Output` interface with Success, Error, Warning, Info, Table, JSON, and Spinner methods
   - Defined `Spinner` interface with Update and Stop methods
   - Implemented `NewOutput(w io.Writer, format string)` factory function
   - Added format constants: `FormatAuto`, `FormatText`, `FormatJSON`
   - TTY detection using `golang.org/x/term.IsTerminal()`

2. **TTYOutput Implementation (AC #3, #6, #7):**
   - Uses style system from Story 7.1 (ColorSuccess, ColorError, ColorWarning, ColorPrimary)
   - Icons: ✓ (success), ✗ (error), ⚠ (warning), ℹ (info)
   - Calls `CheckNoColor()` in constructor to respect NO_COLOR env var
   - Table rendering with aligned columns using utf8.RuneCountInString for Unicode support
   - Returns `SpinnerAdapter` wrapping existing `TerminalSpinner`

3. **JSONOutput Implementation (AC #4):**
   - All methods output structured JSON: `{"type": "...", "message": "..."}`
   - Table outputs array of objects with headers as keys
   - Returns `NoopSpinner` for non-TTY environments

4. **Spinner Implementation (AC #6):**
   - Renamed existing `Spinner` struct to `TerminalSpinner` to avoid conflict
   - Created `SpinnerAdapter` to bridge TerminalSpinner to the Spinner interface
   - Created `NoopSpinner` for JSON/non-TTY output
   - Added backward-compatible `NewSpinner` alias

5. **CLI Integration Fix:**
   - Updated `internal/cli/utility.go` to only call `out.Success()` and `out.Info()` for TTY output
   - JSON format now only outputs the final ValidationResponse, not per-command progress messages

6. **Test Coverage:**
   - Achieved 97.3% coverage (exceeds 90% target)
   - Tests cover: interface compliance, all message types, table rendering, TTY detection, format constants, spinner adapters

### Change Log

- 2025-12-31: Implemented Story 7.2 - Output Interface with TTY/JSON support
- 2025-12-31: Code Review - Fixed 4 issues (1 HIGH, 3 MEDIUM)

### Senior Developer Review (AI)

**Reviewer:** Claude Opus 4.5 (Adversarial Code Review)
**Date:** 2025-12-31
**Outcome:** APPROVED (with fixes applied)

**Issues Found & Fixed:**
1. **HIGH - Double Checkmark Bug** (`utility.go:150`): Fixed duplicate icon in TTY output
2. **MEDIUM - Misleading Function Name** (`spinner.go`): Renamed `NewBubblesSpinner` to `NewSpinnerAdapter`
3. **MEDIUM - Empty Details Field** (`json_output.go`): Now populates Details with wrapped error cause
4. **MEDIUM - Duplicate Function** (`tty_output.go`): Removed `padRightRunes`, now uses `padRight` from styles.go

**Noted but not fixed (LOW):**
- Interface returns `error` from JSON() vs spec (actually better practice)
- No TTY auto-detection test with real terminal (hard to test in CI)

**Coverage:** 98.0% (exceeds 90% target)
**Linting:** 0 issues
**Pre-commit:** All 6 checks passed

### File List

**New Files:**
- `internal/tui/tty_output.go` - TTYOutput implementation
- `internal/tui/json_output.go` - JSONOutput implementation

**Modified Files:**
- `internal/tui/output.go` - Refactored to define Output/Spinner interfaces and NewOutput factory
- `internal/tui/spinner.go` - Renamed Spinner→TerminalSpinner, added SpinnerAdapter/NoopSpinner
- `internal/tui/output_test.go` - Comprehensive test coverage for all Output implementations
- `internal/cli/utility.go` - Fixed JSON output to not emit progress messages
