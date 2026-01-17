# Epic 7 TUI Components - Extracted from User Scenarios

**Source:** `epic-6-user-scenarios.md`
**Purpose:** Map user scenario terminal outputs to Epic 7 TUI stories for implementation alignment

---

## Component Inventory

### 1. Status Box Component (Story 7.3, 7.4)

**From Scenario 1, Step 3 - Workspace Creation:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Creating workspace 'fix-null-pointer'...                        │
├─────────────────────────────────────────────────────────────────┤
│   Workspace: ~/.atlas/workspaces/fix-null-pointer/              │
│   Worktree:  ../atlas-fix-null-pointer/                         │
│   Branch:    fix/fix-null-pointer                               │
│   Base:      main                                               │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Box border with rounded corners
- Title line with action description
- Key-value pairs with aligned colons
- Consistent width (65+ chars)

---

### 2. Step Progress Component (Story 7.8)

**From Scenario 1, Step 4 - Analyze:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Step 1/8: Analyzing problem                              1:23   │
├─────────────────────────────────────────────────────────────────┤
│ [⟳] Scanning codebase for parseConfig...                        │
└─────────────────────────────────────────────────────────────────┘
```

**From Scenario 1, Step 5 - Implement:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Step 2/8: Implementing fix                               5:42   │
├─────────────────────────────────────────────────────────────────┤
│ [⟳] Applying fix to pkg/config/parser.go...                     │
│     Adding test case to pkg/config/parser_test.go...            │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Step counter (current/total) left-aligned
- Duration timer right-aligned
- Action icon prefix: [⟳] running, [✓] complete, [ ] pending
- Indented sub-actions for multi-line progress

---

### 3. Validation Pipeline Component (Story 7.8)

**From Scenario 1, Step 7:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Step 4/8: Validation Pipeline                            2:34   │
├─────────────────────────────────────────────────────────────────┤
│ [✓] Format     magex format:fix                          0.8s   │
│ [⟳] Lint       magex lint                              running  │
│ [⟳] Test       magex test:race                         running  │
│ [ ] Pre-commit go-pre-commit run --all-files           pending  │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Fixed-width columns: icon, name, command, status
- Status values: time (completed), "running", "pending"
- Parallel execution indicated by multiple [⟳] rows
- Sequential execution shows dependency order

---

### 4. Smart Commit Analysis Component (Story 7.8)

**From Scenario 1, Step 8:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Smart Commit Analysis                                           │
├─────────────────────────────────────────────────────────────────┤
│ Group 1: pkg/config (source + test)                             │
│   • pkg/config/parser.go (+5, -1)                               │
│   • pkg/config/parser_test.go (+28, -0)                         │
│   → "fix(config): handle nil options in parseConfig"            │
│                                                                 │
│ ? Create 1 commit? [Y/n/edit/single]                            │
└─────────────────────────────────────────────────────────────────┘
```

**From Scenario 4 (Multi-File Logical Grouping):**
```
┌─────────────────────────────────────────────────────────────────┐
│ Smart Commit Analysis                                           │
├─────────────────────────────────────────────────────────────────┤
│ Group 1: internal/config (source + test)                        │
│   • internal/config/loader.go (+45, -0)                         │
│   • internal/config/loader_test.go (+67, -0)                    │
│   → "feat(config): add verbose logging option"                  │
│                                                                 │
│ Group 2: internal/cli (source + test)                           │
│   • internal/cli/root.go (+12, -3)                              │
│   • internal/cli/root_test.go (+28, -0)                         │
│   → "feat(cli): add --verbose-logging flag"                     │
│                                                                 │
│ Group 3: Documentation                                          │
│   • docs/config.md (+15, -2)                                    │
│   → "docs: update configuration documentation"                  │
│                                                                 │
│ ? Create 3 commits? [Y/n/edit/single]                           │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Group header with package/category name
- Bullet list of files with diff stats (+N, -M)
- Arrow → prefix for generated commit message (italicized)
- Prompt with bracketed options at bottom
- Blank line separator between groups

---

### 5. Garbage Detection Warning Component (Story 7.8)

**From Scenario 2:**
```
┌─────────────────────────────────────────────────────────────────┐
│ ⚠ Potential garbage files detected:                             │
├─────────────────────────────────────────────────────────────────┤
│ CATEGORY        FILE                    REASON                  │
│ Debug           src/helper.js           Contains console.log    │
│ Secrets         .env.local              Credentials file        │
│ Build artifact  __debug_bin             Go debug binary         │
│ Test artifact   coverage.out            Coverage output         │
├─────────────────────────────────────────────────────────────────┤
│ ? What would you like to do?                                    │
│   ❯ Remove garbage and continue                                 │
│     Include anyway (confirm each)                               │
│     Abort and fix manually                                      │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Warning icon ⚠ in title with yellow color
- Table with fixed columns: CATEGORY, FILE, REASON
- Separator line before menu
- Menu with cursor indicator ❯
- Indented options without bullets

---

### 6. CI Status Component (Story 7.5, 7.6)

**From Scenario 1, Step 11:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Step 8/8: Waiting for CI                                12:45   │
├─────────────────────────────────────────────────────────────────┤
│ PR: https://github.com/user/repo/pull/42                        │
│                                                                 │
│ [✓] Lint           Passed                                2m 15s │
│ [⟳] CI             Running (tests)                       8m 30s │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- PR URL as clickable link (OSC 8)
- Check rows with icon, name, status, duration
- Status values: "Passed", "Failed", "Running (current step)", "Pending"
- Duration right-aligned

---

### 7. Failure Menu Component (Story 7.9, Epic 8)

**From Scenario 1, CI Failure:**
```
┌─────────────────────────────────────────────────────────────────┐
│ ✗ CI workflow "CI" failed                                       │
├─────────────────────────────────────────────────────────────────┤
│ ? What would you like to do?                                    │
│   ❯ View logs — Open GitHub Actions in browser                  │
│     Retry from implement — AI tries to fix based on output      │
│     Manual fix — You fix, then 'atlas resume'                   │
│     Abandon — End task, keep PR as draft                        │
└─────────────────────────────────────────────────────────────────┘
```

**From Scenario 3, GH Failed:**
```
┌─────────────────────────────────────────────────────────────────┐
│ ✗ GitHub operation failed: rate limit exceeded                  │
├─────────────────────────────────────────────────────────────────┤
│ ? What would you like to do?                                    │
│   ❯ Retry now — Try the operation again                         │
│     Fix and retry — You fix the issue, then retry               │
│     Abandon task — End task, keep branch for manual work        │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Error icon ✗ with red color in title
- Question mark ? before prompt
- Cursor indicator ❯ on selected option
- Option format: "Label — Description"
- Em-dash (—) separates label from description

---

### 8. Review Summary Component (Epic 8)

**From Scenario 1, Step 12:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Task Complete                          fix/fix-null-pointer     │
├─────────────────────────────────────────────────────────────────┤
│ PR: https://github.com/user/repo/pull/42                        │
│ Commits: 1                                                      │
│ Files: 2 (+33, -1)                                              │
│ CI: All checks passed                                           │
│                                                                 │
│ Summary:                                                        │
│   Fixed nil pointer in parseConfig by adding nil check before   │
│   accessing cfg.Options.                                        │
│                                                                 │
│ Files changed:                                                  │
│   • pkg/config/parser.go (+5, -1)                               │
│   • pkg/config/parser_test.go (+28, -0)                         │
├─────────────────────────────────────────────────────────────────┤
│ ? What would you like to do?                                    │
│   ❯ Approve and continue                                        │
│     Reject and retry (with feedback)                            │
│     View diff                                                   │
│     View logs                                                   │
│     Open PR in browser                                          │
│     Merge PR now                                                │
│     Cancel                                                      │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Two-column title: left title, right branch name
- Metadata section: PR, Commits, Files, CI
- Summary section with wrapped text
- Files changed with bullet list
- Menu with 7+ options

---

### 9. Success Banner Component (Story 7.4)

**From Scenario 1, Completion:**
```
┌─────────────────────────────────────────────────────────────────┐
│ ✓ Task completed successfully                                   │
│                                                                 │
│ PR ready for review: https://github.com/user/repo/pull/42       │
│                                                                 │
│ To cleanup after PR merge:                                      │
│   atlas workspace retire fix-null-pointer                       │
│   atlas workspace destroy fix-null-pointer                      │
└─────────────────────────────────────────────────────────────────┘
```

**Requirements:**
- Success icon ✓ with green color
- Clickable PR URL
- Command examples in monospace

---

### 10. Status Table Component (Story 7.3, 7.4)

**From Story 7.3 requirements:**
```
WORKSPACE   BRANCH          STATUS              STEP    ACTION
auth        feat/auth       ● running           3/7     —
payment     fix/payment     ⚠ awaiting_approval 6/7     approve
```

**Requirements:**
- Fixed columns: WORKSPACE, BRANCH, STATUS, STEP, ACTION
- STATUS shows icon + colored state
- STEP shows current/total
- ACTION shows command or — if none
- Sort by status priority (attention first)

---

## Icon Reference

| State | Icon | Color |
|-------|------|-------|
| Running | ● or ⟳ | Blue |
| Awaiting Approval | ✓ or ⚠ | Green/Yellow |
| Needs Attention | ⚠ | Yellow |
| Failed | ✗ | Red |
| Completed | ✓ | Dim/Gray |
| Pending | ○ or [ ] | Gray |

---

## Color Palette (from UX-4)

| Semantic | Light Terminal | Dark Terminal |
|----------|----------------|---------------|
| Primary (Blue) | #0087AF | #00D7FF |
| Success (Green) | #008700 | #00FF87 |
| Warning (Yellow) | #AF8700 | #FFD700 |
| Error (Red) | #AF0000 | #FF5F5F |
| Muted (Gray) | #585858 | #6C6C6C |

---

## Story Mapping

| Component | Epic 7 Story | Epic 8 Story |
|-----------|--------------|--------------|
| Status Box | 7.3, 7.4 | — |
| Step Progress | 7.8 | — |
| Validation Pipeline | 7.8 | — |
| Smart Commit Analysis | 7.8 | — |
| Garbage Detection | 7.8 | 8.5 |
| CI Status | 7.5, 7.6 | — |
| Failure Menu | 7.9 | 8.5 |
| Review Summary | — | 8.1, 8.2 |
| Success Banner | 7.4 | — |
| Status Table | 7.3, 7.4 | — |

---

## Implementation Notes

1. **All boxes use same border style**: Single-line box drawing characters (┌┐└┘─│├┤)

2. **Width adaptation**:
   - Default 65 chars for standard scenarios
   - Expand to terminal width for tables
   - Compress headers for narrow terminals (<80 cols)

3. **Interactive elements** (Epic 8):
   - Cursor ❯ for selection
   - Bracketed options [Y/n/edit] for quick prompts
   - Full menus for complex decisions

4. **Accessibility (UX-7, UX-8)**:
   - NO_COLOR support
   - Triple redundancy: icon + color + text
   - Keyboard navigation only
