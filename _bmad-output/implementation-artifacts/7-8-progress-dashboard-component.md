# Story 7.8: Progress Dashboard Component

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **visual progress bars for active tasks**,
So that **I can see at a glance how far along each task is**.

## Acceptance Criteria

1. **Given** tasks are running
   **When** displaying the status dashboard
   **Then** active tasks show progress visualization with progress bars

2. **Given** progress bars are being rendered
   **When** viewed in any supported terminal
   **Then** progress bar width adapts to terminal width

3. **Given** task status is displayed
   **When** viewing progress
   **Then** step count shows current/total (FR44)

4. **Given** detailed view is requested
   **When** viewing progress with optional flag
   **Then** current step name can be shown on hover or with flag

5. **Given** multiple tasks are displayed
   **When** task count is ≤5
   **Then** 2-line mode is used (progress + details)

6. **Given** multiple tasks are displayed
   **When** task count is >5
   **Then** 1-line mode is used (progress + name + step)

7. **Given** watch mode is active
   **When** progress changes
   **Then** progress updates smoothly in watch mode

## Tasks / Subtasks

- [x] Task 1: Create Progress Bar Component in `internal/tui/progress.go` (AC: #1, #2)
  - [x] 1.1: Add `github.com/charmbracelet/bubbles/progress` dependency if not present
  - [x] 1.2: Create `ProgressBar` wrapper struct with width configuration
  - [x] 1.3: Implement `NewProgressBar(width int, options ...ProgressOption)` constructor
  - [x] 1.4: Apply ColorPrimary gradient using `progress.WithScaledGradient()` for ATLAS branding
  - [x] 1.5: Implement `Render(percent float64) string` method using `ViewAs()` for static rendering
  - [x] 1.6: Support width adaptation via `WithWidth(w int)` builder method
  - [x] 1.7: Respect NO_COLOR via existing `HasColorSupport()` - use solid fill in no-color mode

- [x] Task 2: Implement Step Counter Display (AC: #3, #4)
  - [x] 2.1: Create `StepProgress` struct to hold current step, total steps, and step name
  - [x] 2.2: Implement `FormatStepCounter(current, total int) string` - returns "3/7" format
  - [x] 2.3: Implement `FormatStepWithName(current, total int, name string) string` - returns "3/7 Validating"
  - [x] 2.4: Add step name lookup function that maps step types to human-readable names

- [x] Task 3: Implement Auto-Density Mode (AC: #5, #6)
  - [x] 3.1: Create `DensityMode` type with constants: `DensityCompact` (1-line), `DensityExpanded` (2-line)
  - [x] 3.2: Implement `DetermineMode(taskCount int) DensityMode` - returns Expanded if ≤5, Compact if >5
  - [x] 3.3: Create `ProgressRowCompact` renderer (1-line: `[███░░░] 3/7 workspace-name`)
  - [x] 3.4: Create `ProgressRowExpanded` renderer (2-line with details below progress)
  - [x] 3.5: Test density mode switching at boundary (5 vs 6 tasks)

- [x] Task 4: Create Progress Dashboard Component (AC: #1, #5, #6)
  - [x] 4.1: Create `ProgressDashboard` struct with rows, width, and mode
  - [x] 4.2: Implement `NewProgressDashboard(rows []ProgressRow, opts ...DashboardOption)` constructor
  - [x] 4.3: Add `WithTerminalWidth(width int)` option
  - [x] 4.4: Add `WithDensityMode(mode DensityMode)` option for manual override
  - [x] 4.5: Implement `Render(w io.Writer) error` method
  - [x] 4.6: Auto-detect density mode based on row count

- [x] Task 5: Integrate with Watch Mode (AC: #7)
  - [x] 5.1: Update `WatchModel` to track progress percentage per workspace
  - [x] 5.2: Calculate progress as `currentStep / totalSteps` for each workspace
  - [x] 5.3: Optionally use progress bar in place of or alongside status table
  - [x] 5.4: Ensure smooth visual updates during watch refresh cycle
  - [x] 5.5: Maintain status table as primary view; progress is supplementary

- [x] Task 6: Add `--progress` Flag to Status Command (AC: #1, #4)
  - [x] 6.1: Add `--progress` or `-p` flag to `atlas status` command
  - [x] 6.2: When flag is set, render progress bars below status table
  - [x] 6.3: Progress flag should work in both static and watch modes
  - [x] 6.4: Update help text to describe the progress flag

- [x] Task 7: Write Comprehensive Tests in `internal/tui/progress_test.go`
  - [x] 7.1: Test ProgressBar renders at various percentages (0%, 25%, 50%, 75%, 100%)
  - [x] 7.2: Test ProgressBar width adaptation (narrow vs wide terminals)
  - [x] 7.3: Test StepCounter format correctness ("3/7" format)
  - [x] 7.4: Test StepWithName format ("3/7 Validating")
  - [x] 7.5: Test auto-density mode: 5 tasks = expanded, 6 tasks = compact
  - [x] 7.6: Test NO_COLOR support doesn't break rendering
  - [x] 7.7: Test ProgressDashboard renders correctly with multiple rows
  - [x] 7.8: Test edge cases: 0 tasks, 1 task, negative percentages

- [x] Task 8: Validate and finalize
  - [x] 8.1: Run `magex format:fix` - must pass
  - [x] 8.2: Run `magex lint` - must pass
  - [x] 8.3: Run `magex test:race` - must pass
  - [x] 8.4: Run `go-pre-commit run --all-files` - must pass

## Dev Notes

### Existing TUI Infrastructure

**DO NOT recreate existing components.** Reuse these from `internal/tui/`:

**From `internal/tui/styles.go`:**
```go
// Colors for progress bar gradient
var (
    ColorPrimary = lipgloss.AdaptiveColor{Light: "#0087AF", Dark: "#00D7FF"} // Cyan/Blue
    ColorSuccess = lipgloss.AdaptiveColor{Light: "#008700", Dark: "#00FF87"} // Green
    ColorMuted   = lipgloss.AdaptiveColor{Light: "#585858", Dark: "#6C6C6C"} // Gray
)

// NO_COLOR support
func HasColorSupport() bool
func CheckNoColor()
```

**From `internal/tui/table.go`:**
```go
// StatusRow is the existing data structure - extend, don't replace
type StatusRow struct {
    Workspace   string
    Branch      string
    Status      constants.TaskStatus
    CurrentStep int
    TotalSteps  int
    Action      string
}
```

**From `internal/tui/watch.go`:**
```go
// WatchModel already tracks step progress
// m.rows[i].CurrentStep and m.rows[i].TotalSteps are available
```

### Bubbles Progress Bar Integration

The `github.com/charmbracelet/bubbles/progress` package provides a progress bar component.

**Key API:**
```go
import "github.com/charmbracelet/bubbles/progress"

// Create with gradient (ATLAS branding)
bar := progress.New(
    progress.WithWidth(50),
    progress.WithScaledGradient("#0087AF", "#00D7FF"), // Match ColorPrimary
)

// Static rendering (recommended for dashboard)
rendered := bar.ViewAs(0.75) // 75% complete

// For NO_COLOR mode, use solid fill
barNoColor := progress.New(
    progress.WithWidth(50),
    progress.WithSolidFill("#808080"),
)
```

**Progress Characters:**
- Default filled: `█` (full block)
- Default empty: `░` (light shade)
- Or use custom with `progress.WithFillCharacters('▓', '░')`

### Auto-Density Mode Design

Per epic-7-tui-components-from-scenarios.md (UX-14):
- **2-line mode (≤5 tasks):** More detail, progress bar on first line, details on second
- **1-line mode (>5 tasks):** Compact, everything on one line

**Example 2-line mode:**
```
auth         [████████████░░░░░░░░░░░░░░░░░░] 40%
             Step 3/7: Validating • 2m 15s

payment      [██████████████████████████░░░░] 85%
             Step 6/7: Waiting for CI • 8m 30s
```

**Example 1-line mode:**
```
auth         [████████░░░░] 40% 3/7 Validating
payment      [██████████░░] 85% 6/7 CI Wait
fix-null     [████████████] 100% 7/7 Complete
```

### Integration Strategy

Progress dashboard is **supplementary** to the status table, not a replacement:

1. **Default:** Status table only (existing behavior)
2. **With `--progress` flag:** Status table + progress bars below
3. **Watch mode:** Optionally show progress bars that update smoothly

**DO NOT modify the status table itself.** Add progress as a separate component that renders below it.

### Terminal Width Adaptation

Progress bar width should scale with terminal:
- **< 80 cols:** Minimal progress bar (20 chars) or skip entirely
- **80-120 cols:** Standard width (40 chars)
- **> 120 cols:** Expanded width (60+ chars)

Use existing `detectTerminalWidth()` from `internal/tui/table.go`.

### Previous Story Learnings (Story 7.7)

From Story 7.7 (ATLAS Header Component):
1. Use `runeWidth()` for proper Unicode character width calculation
2. Center text using visual width, not byte length
3. Context propagation is critical for async Bubble Tea commands
4. Gradient effects require careful evaluation - sometimes solid colors are better
5. Test at various terminal widths (40, 80, 100, 120)

### Git Intelligence

Recent Epic 7 commits show the pattern:
```
bad49ad feat(tui): add responsive ATLAS header component
1e4095c feat(notifications): add terminal bell for task state transitions
2b2bad1 feat(tui): implement watch mode with live status updates
ef0ccb0 feat(tui): implement StatusTable component with proportional column expansion
```

Commit message format: `feat(tui): <description>`

### Project Structure Notes

**Files to create:**
- `internal/tui/progress.go` - Progress bar wrapper and dashboard component
- `internal/tui/progress_test.go` - Comprehensive tests

**Files to modify:**
- `internal/cli/status.go` - Add `--progress` flag
- `internal/tui/watch.go` - Optionally integrate progress display
- `go.mod` - Add bubbles/progress dependency if not present

**DO NOT modify:**
- `internal/tui/styles.go` - Colors already defined
- `internal/tui/table.go` - Status table stays as-is
- `internal/tui/header.go` - Header is separate

**Alignment with project structure:**
- All TUI components live in `internal/tui/`
- Follow existing patterns from `styles.go`, `table.go`, `header.go`
- Use existing color constants from `styles.go`

### Validation Commands

```bash
# MUST run ALL FOUR before marking complete
magex format:fix
magex lint
magex test:race
go-pre-commit run --all-files
```

### References

- [Source: _bmad-output/planning-artifacts/epics.md - Story 7.8 acceptance criteria]
- [Source: _bmad-output/implementation-artifacts/epic-7-tui-components-from-scenarios.md - Step Progress Component, Validation Pipeline Component]
- [Source: _bmad-output/implementation-artifacts/7-7-atlas-header-component.md - Previous story patterns and learnings]
- [Source: internal/tui/styles.go - Existing color definitions and style patterns]
- [Source: internal/tui/table.go - StatusRow structure, width detection]
- [Source: internal/tui/watch.go - WatchModel integration point]
- [Source: internal/cli/status.go - Command integration point]
- [Source: _bmad-output/project-context.md - Validation commands and coding standards]
- [Source: _bmad-output/planning-artifacts/architecture.md - Package organization rules]
- [Source: pkg.go.dev/github.com/charmbracelet/bubbles/progress - Progress bar API documentation]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

- All 8 tasks completed successfully
- All validation checks pass: magex format:fix, magex lint, magex test:race, go-pre-commit run --all-files
- Progress bar component wraps charmbracelet/bubbles/progress with ATLAS branding
- Auto-density mode automatically switches between expanded (≤5 tasks) and compact (>5 tasks)
- --progress / -p flag added to atlas status command for both static and watch modes
- Progress bars only shown for active tasks (running or validating states)
- 26+ comprehensive tests covering all functionality

### File List

**Created:**
- `internal/tui/progress.go` - Progress bar wrapper, step counter, density modes, and dashboard component
- `internal/tui/progress_test.go` - Comprehensive test suite (26+ tests)

**Modified:**
- `internal/tui/watch.go` - Added ShowProgress config and buildProgressRows/renderStatusContent methods
- `internal/cli/status.go` - Added --progress flag and buildProgressRows function
- `internal/cli/status_test.go` - Added tests for --progress flag
- `go.mod` / `go.sum` - Added harmonica dependency (transitive from bubbles/progress)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` - Sprint tracking status

### Senior Developer Review (AI)

**Reviewer:** Claude Opus 4.5 (Code Review Workflow)
**Date:** 2025-12-31
**Outcome:** APPROVED (with fixes applied)

**Issues Found and Fixed:**

| Severity | Issue | Fix Applied |
|----------|-------|-------------|
| HIGH | H1: StepNameLookup logic error - lookup table keys didn't match status values being passed, causing AC#4 to not work correctly | Added status value mappings to defaultStepNameLookup in progress.go |
| MEDIUM | M1/M2: Code duplication - buildProgressRows() duplicated in status.go and watch.go | Created shared BuildProgressRowsFromStatus() in tui package |
| MEDIUM | M3: Name truncation using byte length instead of rune width | Updated to use runeWidth() and truncateToRuneWidth() helpers |
| MEDIUM | M4: DefaultWatchConfig missing explicit ShowProgress field | Added explicit ShowProgress: false |
| LOW | L1: sprint-status.yaml not documented in File List | Added to File List (above) |

**Validation:**
- All fixes pass: magex format:fix, magex lint, magex test:race, go-pre-commit run --all-files
- Added 4 new tests for truncateToRuneWidth and BuildProgressRowsFromStatus
- Extended StepNameLookup tests to cover all status values

### Change Log

| Date | Change | Author |
|------|--------|--------|
| 2025-12-31 | Initial implementation | Claude Opus 4.5 |
| 2025-12-31 | Code review fixes: StepNameLookup bug, code deduplication, rune width truncation | Claude Opus 4.5 (Review) |
