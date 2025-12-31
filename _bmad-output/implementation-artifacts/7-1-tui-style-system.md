# Story 7.1: TUI Style System

Status: done

## Story

As a **developer**,
I want **a centralized style system using Lip Gloss**,
So that **all TUI components have consistent styling**.

## Acceptance Criteria

1. **Given** the Charm libraries are installed
   **When** I implement `internal/tui/styles.go`
   **Then** the style system provides semantic colors with AdaptiveColor for light/dark terminals:
   - Primary (Blue): `#0087AF` / `#00D7FF`
   - Success (Green): `#008700` / `#00FF87`
   - Warning (Yellow): `#AF8700` / `#FFD700`
   - Error (Red): `#AF0000` / `#FF5F5F`
   - Muted (Gray): `#585858` / `#6C6C6C`

2. **Given** the style system exists
   **When** displaying status information
   **Then** state icons with color mapping are available:
   - Running: `●` Blue
   - Awaiting Approval: `✓` Green
   - Needs Attention: `⚠` Yellow
   - Failed: `✗` Red
   - Completed: `✓` Dim
   - Pending: `○` Gray

3. **Given** the style system exists
   **When** rendering text
   **Then** typography styles are available: Bold, Dim, Underline, Reverse

4. **Given** the NO_COLOR environment variable is set
   **When** any styled output is rendered
   **Then** colors are disabled per UX-7 accessibility requirement

5. **Given** any status is displayed
   **When** the user views the output
   **Then** triple redundancy is maintained: icon + color + text for all states per UX-8

6. **Given** the style system is implemented
   **When** other TUI components are created
   **Then** styles are exported and reusable across all TUI components

## Tasks / Subtasks

- [x] Task 1: Enhance semantic color palette (AC: #1)
  - [x] 1.1: Add Primary (Blue) AdaptiveColor constant
  - [x] 1.2: Add Warning (Yellow) AdaptiveColor constant (currently missing)
  - [x] 1.3: Verify all 5 semantic colors match UX-4 spec exactly
  - [x] 1.4: Add color constants as exported variables for component reuse

- [x] Task 2: Complete state icon system (AC: #2, #5)
  - [x] 2.1: Review existing TaskStatusIcon function for completeness
  - [x] 2.2: Add WorkspaceStatusIcon function with corresponding icons
  - [x] 2.3: Ensure icon + color + text triple redundancy pattern is documented
  - [x] 2.4: Add helper function: FormatStatusWithIcon(status, text) string

- [x] Task 3: Add typography styles (AC: #3)
  - [x] 3.1: Add Bold style constant
  - [x] 3.2: Add Dim style constant (exists, verify)
  - [x] 3.3: Add Underline style constant
  - [x] 3.4: Add Reverse style constant
  - [x] 3.5: Document usage patterns for each typography style

- [x] Task 4: Enhance NO_COLOR support (AC: #4)
  - [x] 4.1: Review existing CheckNoColor() function
  - [x] 4.2: Ensure TERM=dumb also disables colors (already done)
  - [x] 4.3: Add HasColorSupport() bool helper function
  - [x] 4.4: Document NO_COLOR behavior in package docs

- [x] Task 5: Export reusable style components (AC: #6)
  - [x] 5.1: Create StyleSystem struct to hold all style configurations
  - [x] 5.2: Add NewStyleSystem() constructor with defaults
  - [x] 5.3: Ensure all color/icon/typography functions are exported
  - [x] 5.4: Add comprehensive package documentation

- [x] Task 6: Add box border styles for TUI components
  - [x] 6.1: Create BoxStyle struct for bordered containers
  - [x] 6.2: Add box border characters (rounded corners per UX specs)
  - [x] 6.3: Add box width constants (default 65 chars per scenarios)
  - [x] 6.4: Support variable width boxes

- [x] Task 7: Write comprehensive tests
  - [x] 7.1: Test all semantic colors have light/dark variants
  - [x] 7.2: Test all task statuses have icons defined
  - [x] 7.3: Test NO_COLOR environment variable handling
  - [x] 7.4: Test IsAttentionStatus for all attention states
  - [x] 7.5: Test SuggestedAction for all actionable states
  - [x] 7.6: Achieve 90%+ test coverage

## Dev Notes

### CRITICAL: Existing Implementation

**IMPORTANT:** Much of Story 7.1 is ALREADY IMPLEMENTED in `internal/tui/styles.go`. The dev agent MUST:
1. Read the existing implementation thoroughly before making changes
2. Enhance and extend, not replace, the existing code
3. Fill gaps identified below rather than reimplementing

### What Already Exists (DO NOT RECREATE)

From `internal/tui/styles.go`:
- `StatusColors()` - WorkspaceStatus color map
- `TaskStatusColors()` - TaskStatus color map with all 11 statuses
- `TaskStatusIcon()` - Icons for all task statuses
- `IsAttentionStatus()` - Identifies attention-required statuses
- `SuggestedAction()` - CLI commands for actionable states
- `CheckNoColor()` - NO_COLOR environment variable handling
- `NewTableStyles()` - Table styling
- `NewOutputStyles()` - Success/Error/Warning/Info/Dim styles

### What Needs Enhancement

1. **Missing Colors**: The Warning color (Yellow: `#AF8700` / `#FFD700`) is not explicitly exported as a standalone constant
2. **Box Styles**: Box border rendering for TUI components (per epic-7-tui-components-from-scenarios.md) not yet implemented
3. **Typography Styles**: Bold, Underline, Reverse not explicitly exported (only Dim exists)
4. **StyleSystem Consolidation**: Consider consolidating into a single StyleSystem struct for easier access

### Architecture Patterns

From `_bmad-output/planning-artifacts/architecture.md`:
- Package location: `internal/tui/`
- Import from `internal/constants` for status constants
- Export functions for use across TUI components
- No circular dependencies with other internal packages

### TUI Component Specifications

From `epic-7-tui-components-from-scenarios.md`:

**Box Border Style (for all components):**
```
┌─────────────────────────────────────────────────────────────────┐
│ Title line                                                      │
├─────────────────────────────────────────────────────────────────┤
│ Content                                                         │
└─────────────────────────────────────────────────────────────────┘
```

**Icon Reference:**
| State | Icon | Color |
|-------|------|-------|
| Running | ● or ⟳ | Blue |
| Awaiting Approval | ✓ or ⚠ | Green/Yellow |
| Needs Attention | ⚠ | Yellow |
| Failed | ✗ | Red |
| Completed | ✓ | Dim/Gray |
| Pending | ○ or [ ] | Gray |

**Color Palette (from UX-4):**
| Semantic | Light Terminal | Dark Terminal |
|----------|----------------|---------------|
| Primary (Blue) | #0087AF | #00D7FF |
| Success (Green) | #008700 | #00FF87 |
| Warning (Yellow) | #AF8700 | #FFD700 |
| Error (Red) | #AF0000 | #FF5F5F |
| Muted (Gray) | #585858 | #6C6C6C |

### Lipgloss Best Practices (2025)

From web research on charmbracelet/lipgloss:
- Use `AdaptiveColor{Light: "...", Dark: "..."}` for all colors
- Terminal dark/light detection is automatic
- For precise control, use `CompleteAdaptiveColor` with TrueColor, ANSI256, and ANSI values
- Use `lipgloss.Println` for proper color output in standalone mode
- NO_COLOR support via `lipgloss.SetColorProfile(termenv.Ascii)`

### Project Structure Notes

- File: `internal/tui/styles.go` (enhance existing)
- Test file: `internal/tui/styles_test.go` (enhance existing)
- Imports allowed: `internal/constants`, standard library, Charm libs
- Follow functional options pattern if adding configurable styles

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
- [Source: _bmad-output/implementation-artifacts/epic-7-tui-components-from-scenarios.md - Icon Reference, Color Palette]
- [Source: _bmad-output/planning-artifacts/epics.md - Story 7.1 acceptance criteria]
- [Source: internal/tui/styles.go - existing implementation]
- [Source: _bmad-output/project-context.md - coding standards and validation commands]
- [Web: github.com/charmbracelet/lipgloss - AdaptiveColor patterns]

### Previous Epic Learnings (Epic 6 Retrospective)

From `epic-6-retro-2025-12-30.md`:
- Use functional options pattern for dependency injection
- Maintain 90%+ test coverage
- Follow Go idiomatic interface naming (no stutter)
- Run traceability analysis before implementation

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

None - implementation completed without issues.

### Completion Notes List

1. **Task 1 - Semantic Colors**: Added 5 exported AdaptiveColor constants (ColorPrimary, ColorSuccess, ColorWarning, ColorError, ColorMuted) matching UX-4 spec exactly.

2. **Task 2 - State Icons**: Added WorkspaceStatusIcon() for workspace status icons and FormatStatusWithIcon() generic helper for triple redundancy pattern. All icons documented in package docs.

3. **Task 3 - Typography**: Added 4 exported style constants (StyleBold, StyleDim, StyleUnderline, StyleReverse) for text formatting.

4. **Task 4 - NO_COLOR**: Added HasColorSupport() helper function following NO_COLOR spec (any value including empty string disables color). Also handles TERM=dumb. Package docs updated.

5. **Task 5 - StyleSystem**: Created consolidated StyleSystem struct with ColorPalette, TypographyStyles, and IconFunctions. NewStyleSystem() constructor provides easy access to all styling.

6. **Task 6 - Box Borders**: Created BoxStyle struct with RoundedBorder (using Unicode rounded corner characters), DefaultBoxWidth=65, WithWidth() for variable widths, and Render() method.

7. **Task 7 - Tests**: Achieved 95.9% coverage on internal/tui package. All tests pass with race detection. All validation commands pass.

### File List

- internal/tui/styles.go (modified - enhanced with new exports)
- internal/tui/styles_test.go (modified - comprehensive tests added)

## Code Review Record

### Review Date
2025-12-31

### Reviewer
Claude Opus 4.5 (Adversarial Code Review)

### Issues Found and Fixed

**HIGH Severity (3):**
1. **Icon Mismatch with Spec** - Running icon was `▶` but spec says `●`; AwaitingApproval was `◉` but spec says `✓`. Fixed to match epic-7-tui-components-from-scenarios.md Icon Reference.
2. **Unicode Bug in padRight** - Used `len(s)` which counts bytes, not runes. Unicode chars like `●` are 3 bytes but 1 char. Fixed to use `utf8.RuneCountInString()`.
3. **Box Border Style Mismatch** - Used rounded corners `╭╮╰╯` but spec says square `┌┐└┘`. Added `DefaultBorder` with square corners per UX spec.

**MEDIUM Severity (4):**
4. **Duplicate Icon Logic** - StyleSystem.Icons.FormatWithIcon duplicated switch logic. Refactored to delegate to `formatStatusWithIconAny` helper.
5. **Single-Line Box Content** - BoxStyle.Render only handled single-line content. Added multi-line support with `strings.Split`.
6. **Missing Unknown TaskStatus Test** - Added `TestTaskStatusIcon_UnknownStatus` test.
7. **OutputStyles Hardcoded Colors** - Changed from hardcoded dark colors to use AdaptiveColor constants.

**LOW Severity (2):**
8. **Inefficient repeatString** - Replaced with `strings.Repeat()` from stdlib.
9. **Test Coverage Gaps** - Added Unicode padding tests and multi-line box tests.

### Validation Results
- Tests: PASS (97.2% coverage, up from 95.9%)
- Lint: PASS (0 issues)
- Pre-commit: PASS (all 6 checks)

### Files Modified
- internal/tui/styles.go
- internal/tui/styles_test.go

## Change Log

- 2025-12-31: Code review completed - 9 issues found and fixed, coverage improved to 97.2%
- 2025-12-30: Story 7.1 implemented - TUI Style System complete with semantic colors, icons, typography, NO_COLOR support, StyleSystem consolidation, box borders, and 95.9% test coverage.
