# Story 7.9: Action Indicators

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **clear action indicators showing what command to run**,
So that **I never have to guess what to do next**.

## Acceptance Criteria

1. **Given** tasks are in various states
   **When** displaying status
   **Then** ACTION column shows appropriate action for each state

2. **Given** attention-required tasks exist
   **When** displaying status footer
   **Then** footer shows copy-paste command: `Run: atlas approve payment`

3. **Given** multiple attention items exist
   **When** displaying status footer
   **Then** footer lists all commands for attention-required tasks

4. **Given** action indicators are displayed
   **When** task status requires attention
   **Then** action text is styled appropriately (warning color for attention states)

## Tasks / Subtasks

- [x] Task 1: Enhance ACTION Column in Status Table (AC: #1)
  - [x] 1.1: Review existing `SuggestedAction()` in `internal/tui/styles.go` - verify completeness for all attention states
  - [x] 1.2: Update `SuggestedAction()` to include workspace name placeholder: `atlas approve {workspace}`
  - [x] 1.3: Update `renderActionCell()` in `internal/tui/table.go` to apply warning color styling for attention states
  - [x] 1.4: Ensure em-dash (—) is displayed for states that don't need action (running, pending, completed)
  - [x] 1.5: Test action column displays correctly for all TaskStatus values

- [x] Task 2: Create Footer Component with Actionable Commands (AC: #2, #3)
  - [x] 2.1: Create `StatusFooter` struct in `internal/tui/footer.go` with attention rows slice
  - [x] 2.2: Implement `NewStatusFooter(rows []StatusRow)` constructor that filters for attention statuses
  - [x] 2.3: Implement `FormatSingleAction(workspace string, action string) string` - returns `Run: atlas approve workspace-name`
  - [x] 2.4: Implement `FormatMultipleActions(items []ActionItem) string` for multiple attention tasks
  - [x] 2.5: Create `Render(w io.Writer) error` method to output footer to writer
  - [x] 2.6: Add blank line separator between status table and footer

- [x] Task 3: Style Action Indicators with Warning Colors (AC: #4)
  - [x] 3.1: Create `ActionStyle` function returning lipgloss.Style with ColorWarning for attention states
  - [x] 3.2: Apply ActionStyle to action text in ACTION column for attention statuses
  - [x] 3.3: Apply bold styling to the copy-paste command in footer
  - [x] 3.4: Ensure NO_COLOR support - use plain text prefix "(!)" instead of color when NO_COLOR is set
  - [x] 3.5: Verify triple redundancy: warning icon (!) + color + text for attention actions

- [x] Task 4: Integrate Footer into Status Command (AC: #2, #3)
  - [x] 4.1: Modify `internal/cli/status.go` to create and render StatusFooter after status table
  - [x] 4.2: Show footer only when attention-required tasks exist (skip if none)
  - [x] 4.3: Ensure footer renders in both static and watch modes
  - [x] 4.4: Ensure `--output json` includes action commands in structured output

- [x] Task 5: Integrate Footer into Watch Mode (AC: #2, #3)
  - [x] 5.1: Update `WatchModel` in `internal/tui/watch.go` to track attention items
  - [x] 5.2: Render StatusFooter below status table in watch view
  - [x] 5.3: Update footer dynamically when task statuses change
  - [x] 5.4: Ensure footer doesn't flicker during refresh cycle

- [x] Task 6: Write Comprehensive Tests in `internal/tui/footer_test.go`
  - [x] 6.1: Test footer with zero attention items (should not render)
  - [x] 6.2: Test footer with single attention item (shows `Run: atlas approve workspace`)
  - [x] 6.3: Test footer with multiple attention items (lists all commands)
  - [x] 6.4: Test action styling: warning color applied for attention states
  - [x] 6.5: Test NO_COLOR mode: plain text indicators work correctly
  - [x] 6.6: Test action column for all TaskStatus values
  - [x] 6.7: Test em-dash (—) displayed for non-actionable states

- [x] Task 7: Validate and finalize
  - [x] 7.1: Run `magex format:fix` - must pass
  - [x] 7.2: Run `magex lint` - must pass
  - [x] 7.3: Run `magex test:race` - must pass
  - [x] 7.4: Run `go-pre-commit run --all-files` - must pass

## Dev Notes

### Existing Action Indicator Infrastructure

**DO NOT recreate existing components.** Extend these from `internal/tui/`:

**From `internal/tui/styles.go`:**
```go
// SuggestedAction already exists - returns the command for each status
func SuggestedAction(status constants.TaskStatus) string {
    actions := map[constants.TaskStatus]string{
        constants.TaskStatusValidationFailed: "atlas resume",
        constants.TaskStatusAwaitingApproval: "atlas approve",
        constants.TaskStatusGHFailed:         "atlas retry",
        constants.TaskStatusCIFailed:         "atlas retry",
        constants.TaskStatusCITimeout:        "atlas retry",
    }
    if action, ok := actions[status]; ok {
        return action
    }
    return ""
}

// IsAttentionStatus already exists - returns true for attention-required statuses
func IsAttentionStatus(status constants.TaskStatus) bool

// ColorWarning is available for styling
var ColorWarning = lipgloss.AdaptiveColor{Light: "#AF8700", Dark: "#FFD700"}
```

**From `internal/tui/table.go`:**
```go
// StatusRow already has Action field
type StatusRow struct {
    Workspace   string
    Branch      string
    Status      constants.TaskStatus
    CurrentStep int
    TotalSteps  int
    Action      string // Custom action or uses SuggestedAction()
}

// renderActionCell already renders the action column
func (t *StatusTable) renderActionCell(status constants.TaskStatus, customAction string) string
```

### Footer Component Design

Per epic-7-tui-components-from-scenarios.md, the footer should show actionable commands:

**Single attention item:**
```
Run: atlas approve payment
```

**Multiple attention items:**
```
Run: atlas approve auth
Run: atlas retry payment
Run: atlas resume fix-null
```

**No attention items:** Footer is not displayed.

### Action Column Enhancement

Current ACTION column shows the command name (`approve`, `retry`). Enhancement needed:

1. **Apply warning color** to attention state actions
2. **Keep em-dash (—)** for non-actionable states (running, pending, completed, etc.)

**Example status table with styled ACTION column:**
```
WORKSPACE   BRANCH          STATUS               STEP    ACTION
auth        feat/auth       ● running            3/7     —
payment     fix/payment     ⚠ awaiting_approval  6/7     approve   <- Yellow/Warning color
fix-null    fix/null        ✗ ci_failed          4/7     retry     <- Yellow/Warning color
```

### NO_COLOR Support

When NO_COLOR is set, action indicators should use plain text prefix:
- `(!) approve` instead of styled `approve`
- This maintains triple redundancy: indicator + color + text

**Check using:**
```go
if !HasColorSupport() {
    return "(!) " + action
}
return warningStyle.Render(action)
```

### Previous Story Learnings (Story 7.8)

From Story 7.8 (Progress Dashboard Component):
1. Use existing `HasColorSupport()` from styles.go for NO_COLOR detection
2. Build on existing `StatusRow` structure - don't duplicate
3. Shared helpers like `BuildProgressRowsFromStatus()` reduce code duplication
4. Use `runeWidth()` for proper Unicode character width calculation

### Git Intelligence

Recent Epic 7 commits follow the pattern:
```
2392448 feat(tui): add visual progress dashboard for active tasks
bad49ad feat(tui): add responsive ATLAS header component
2b2bad1 feat(tui): implement watch mode with live status updates
```

Commit message format: `feat(tui): <description>` or `feat(cli): <description>`

### Project Structure Notes

**Files to create:**
- `internal/tui/footer.go` - StatusFooter component for actionable commands
- `internal/tui/footer_test.go` - Comprehensive tests

**Files to modify:**
- `internal/tui/styles.go` - Add ActionStyle function (optional enhancement)
- `internal/tui/table.go` - Enhance renderActionCell with warning color
- `internal/cli/status.go` - Integrate StatusFooter rendering
- `internal/tui/watch.go` - Integrate StatusFooter in watch mode

**DO NOT modify:**
- `internal/tui/progress.go` - Progress bar is separate
- `internal/tui/header.go` - Header is separate

**Alignment with project structure:**
- All TUI components live in `internal/tui/`
- Follow existing patterns from `styles.go`, `table.go`, `progress.go`
- Use existing color constants from `styles.go`

### Integration Strategy

1. Create `footer.go` with `StatusFooter` component
2. Enhance `renderActionCell()` in `table.go` to apply warning styling
3. Update `status.go` CLI command to render footer after table
4. Update `watch.go` to include footer in view
5. Write comprehensive tests

### JSON Output Considerations

For `--output json`, include action commands in structured format:
```json
{
  "workspaces": [...],
  "attention_items": [
    {"workspace": "payment", "action": "atlas approve payment"},
    {"workspace": "fix-null", "action": "atlas retry fix-null"}
  ]
}
```

This allows scripted usage: `atlas status --output json | jq '.attention_items[0].action'`

### Validation Commands

```bash
# MUST run ALL FOUR before marking complete
magex format:fix
magex lint
magex test:race
go-pre-commit run --all-files
```

### References

- [Source: _bmad-output/planning-artifacts/epics.md - Story 7.9 acceptance criteria]
- [Source: _bmad-output/implementation-artifacts/epic-7-tui-components-from-scenarios.md - Failure Menu Component (Story 7.9)]
- [Source: _bmad-output/implementation-artifacts/7-8-progress-dashboard-component.md - Previous story patterns and learnings]
- [Source: internal/tui/styles.go - Existing SuggestedAction(), IsAttentionStatus(), ColorWarning]
- [Source: internal/tui/table.go - StatusRow structure, renderActionCell()]
- [Source: internal/tui/watch.go - WatchModel integration point]
- [Source: internal/cli/status.go - Command integration point]
- [Source: _bmad-output/project-context.md - Validation commands and coding standards]
- [Source: _bmad-output/planning-artifacts/architecture.md - Package organization rules]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

- All 4 ACs implemented and verified
- StatusFooter component created with full test coverage
- ACTION column enhanced with warning color styling for attention states
- NO_COLOR support implemented with "(!) " prefix for accessibility
- Footer integrated into both static status and watch mode
- JSON output includes `attention_items` array per spec
- All validation commands pass: format, lint, test:race, pre-commit

### File List

**Created:**
- `internal/tui/footer.go` - StatusFooter component for actionable commands
- `internal/tui/footer_test.go` - Comprehensive tests for footer component

**Modified:**
- `internal/tui/styles.go` - Added ActionStyle() function for warning color styling
- `internal/tui/table.go` - Enhanced renderActionCell() with warning colors and NO_COLOR support
- `internal/tui/table_test.go` - Added tests for action column styling and NO_COLOR mode
- `internal/cli/status.go` - Integrated StatusFooter rendering after status table
- `internal/cli/status_test.go` - Added tests for JSON attention_items output
- `internal/tui/watch.go` - Integrated StatusFooter in watch mode view

