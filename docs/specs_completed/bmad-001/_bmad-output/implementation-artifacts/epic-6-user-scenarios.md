# Epic 6: User Scenarios for Validation

**Purpose:** These scenarios provide real-world user workflows that reviewers can use to validate Epic 6 stories against actual user needs. Each scenario maps to specific acceptance criteria with inline error handling paths.

**Source:** `docs/external/vision.md`, `docs/external/templates.md`

---

## Scenario 1: Bugfix Workflow (Stories 6.3, 6.4, 6.5, 6.6, 6.7)

### Template Configuration

```yaml
Template: bugfix
Model: claude-sonnet-4-5-20250916
Steps: 8 (analyze → implement → validate → commit → push → pr → ci_wait → review)
Human Checkpoints: 1-2 (configurable)
AI Verification: OFF by default (enable with --verify)
Commit Style: Multiple logical commits (grouped by package)
Timeout: 30 minutes per AI step
```

### State Machine

```
┌─────────┐     ┌─────────┐     ┌────────────┐     ┌──────────────────┐     ┌───────────┐
│ Pending │────►│ Running │────►│ Validating │────►│ AwaitingApproval │────►│ Completed │
└─────────┘     └────┬────┘     └─────┬──────┘     └────────┬─────────┘     └───────────┘
                     │                │                     │
                     │                ▼                     │
                     │         ┌──────────────────┐         │
                     │         │ ValidationFailed │         │
                     │         └────────┬─────────┘         │
                     │                  │                   │
                     │    ┌─────────────┼─────────────┐     │
                     │    │             │             │     │
                     │    ▼             ▼             ▼     │
                     │ [Retry]    [Manual Fix]   [Abandon]  │
                     │    │             │             │     │
                     │    └─────────────┴──────┐      │     │
                     │                         │      │     │
                     └─────────────────────────┘      │     │
                                                      ▼     │
                     ┌──────────┐              ┌───────────┐│
                     │ GHFailed │◄─────────────│ Abandoned ││
                     └────┬─────┘              └───────────┘│
                          │                                 │
             ┌────────────┼────────────┐                    │
             │            │            │                    │
             ▼            ▼            ▼                    │
          [Retry]   [Fix Auth]   [Abandon]                  │
             │                        │                     │
             └────────────────────────┘                     │
```

### User Story

As a Go developer, I want to fix a null pointer panic and have ATLAS handle the entire git workflow automatically, with clear feedback at each step and the ability to recover from failures.

### Preconditions

- ATLAS is initialized (`atlas init` completed)
- User is in a Go repository with existing code
- `go-pre-commit`, `magex`, and `gh` CLI are installed
- User has GitHub push access
- Claude Code CLI is authenticated

### User Journey

```
1. User discovers bug in production logs
   └─► Error: "nil pointer dereference in parseConfig"
   └─► Stack trace points to pkg/config/parser.go:45

2. User starts ATLAS bugfix workflow
   └─► atlas start "fix null pointer in parseConfig when options is nil" --template bugfix
   └─► IF --workspace not provided:
       └─► Auto-generate: fix-null-pointer-parseconfig
   └─► IF --verify flag passed:
       └─► AI verification step will be enabled

3. **Workspace & Worktree Creation**
   └─► State: Pending → Running
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Creating workspace 'fix-null-pointer'...                        │
       ├─────────────────────────────────────────────────────────────────┤
       │   Workspace: ~/.atlas/workspaces/fix-null-pointer/              │
       │   Worktree:  ../atlas-fix-null-pointer/                         │
       │   Branch:    fix/fix-null-pointer                               │
       │   Base:      main                                               │
       └─────────────────────────────────────────────────────────────────┘
   └─► Creates: git worktree add ../atlas-fix-null-pointer fix/fix-null-pointer
   └─► IF branch already exists:
       └─► Append timestamp: fix/fix-null-pointer-20251229
       └─► WARN: "Branch 'fix/fix-null-pointer' exists, using timestamped name"
   └─► IF worktree path exists:
       └─► Append numeric suffix: ../atlas-fix-null-pointer-2/
   └─► Artifact: workspace.json created

4. **Step 1: Analyze** (AI, plan mode)
   └─► AI invoked with permission-mode: plan (read-only)
   └─► Timeout: 10 minutes, 2 retries
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 1/8: Analyzing problem                              1:23   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [⟳] Scanning codebase for parseConfig...                        │
       └─────────────────────────────────────────────────────────────────┘
   └─► AI actions:
       - Scans codebase for parseConfig function
       - Identifies nil dereference at pkg/config/parser.go:45
       - Reviews existing test coverage
       - Identifies root cause: cfg.Options accessed without nil check
   └─► Artifact: analyze.md with root cause analysis
   └─► IF AI fails or timeout:
       └─► State: Running → ValidationFailed
       └─► Options menu:
           ┌─────────────────────────────────────────────────────────────┐
           │ ⚠ Analysis step failed: timeout after 10 minutes            │
           ├─────────────────────────────────────────────────────────────┤
           │ ? What would you like to do?                                │
           │   ❯ Retry analysis                                          │
           │     Provide manual analysis                                 │
           │     Abandon task                                            │
           └─────────────────────────────────────────────────────────────┘

5. **Step 2: Implement** (AI, implement mode)
   └─► AI invoked with permission-mode: implement (full access)
   └─► Timeout: 30 minutes, 3 retries
   └─► Uses analyze.md as context
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 2/8: Implementing fix                               5:42   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [⟳] Applying fix to pkg/config/parser.go...                     │
       │     Adding test case to pkg/config/parser_test.go...            │
       └─────────────────────────────────────────────────────────────────┘
   └─► AI actions:
       - Adds nil check at pkg/config/parser.go:45
       - Creates test case for nil options scenario
       - Updates any affected documentation
   └─► Files changed:
       - pkg/config/parser.go (+5, -1)
       - pkg/config/parser_test.go (+28, -0)
   └─► Artifact: implement.log with session details
   └─► IF AI fails:
       └─► Error context preserved for retry
       └─► State: Running → ValidationFailed
       └─► Options: [Retry with feedback] / [Manual fix] / [Abandon]

6. **Step 3: AI Verification** (if --verify flag or config enabled)
   └─► SKIP if: --no-verify flag or template default (OFF for bugfix)
   └─► Different model (gemini-3-pro) reviews implementation
   └─► Checks:
       - Code change correctness
       - Test coverage adequacy
       - No garbage files introduced
       - No obvious security issues
   └─► Terminal output (if enabled):
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 3/9: Verifying implementation                       0:45   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [✓] Code correctness verified                                   │
       │ [✓] Test coverage adequate                                      │
       │ [✓] No garbage files detected                                   │
       │ [✓] No security issues found                                    │
       └─────────────────────────────────────────────────────────────────┘
   └─► Artifact: verification-report.md
   └─► IF issues found:
       ┌─────────────────────────────────────────────────────────────────┐
       │ ⚠ Verification found 1 issue:                                   │
       ├─────────────────────────────────────────────────────────────────┤
       │ 1. Test case doesn't cover empty options scenario               │
       ├─────────────────────────────────────────────────────────────────┤
       │ ? How would you like to proceed?                                │
       │   ❯ Auto-fix issues                                             │
       │     Manual fix                                                  │
       │     Ignore and continue                                         │
       │     View full report                                            │
       └─────────────────────────────────────────────────────────────────┘

7. **Step 4: Validate** (Validation pipeline)
   └─► State: Running → Validating
   └─► Sequential then parallel execution:
       Phase 1 (sequential): magex format:fix
       Phase 2 (parallel):   magex lint || magex test:race
       Phase 3 (sequential): go-pre-commit run --all-files
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 4/8: Validation Pipeline                            2:34   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [✓] Format     magex format:fix                          0.8s   │
       │ [⟳] Lint       magex lint                              running  │
       │ [⟳] Test       magex test:race                         running  │
       │ [ ] Pre-commit go-pre-commit run --all-files           pending  │
       └─────────────────────────────────────────────────────────────────┘
   └─► On success: State: Validating → Running (continues to next step)
   └─► Artifact: validation.json with results
   └─► IF any step fails:
       └─► State: Validating → ValidationFailed
       └─► Bell notification (BEL character)
       └─► Terminal output:
           ┌─────────────────────────────────────────────────────────────┐
           │ ✗ Validation failed: magex lint                             │
           ├─────────────────────────────────────────────────────────────┤
           │ Error: pkg/config/parser.go:47: ineffective assignment      │
           ├─────────────────────────────────────────────────────────────┤
           │ ? What would you like to do?                                │
           │   ❯ Retry — AI attempts fix with error context              │
           │     Manual Fix — You fix, then 'atlas resume'               │
           │     Abandon — End task, keep branch for manual work         │
           └─────────────────────────────────────────────────────────────┘

8. **Step 5: Smart Commit** (Multiple Logical Commits)
   └─► **[Story 6.3]** Garbage detection scan first:
       └─► Scans for: *.tmp, *.bak, __debug_bin, coverage.out, .env*, etc.
       └─► IF garbage found:
           ┌─────────────────────────────────────────────────────────────┐
           │ ⚠ Potential garbage files detected:                         │
           ├─────────────────────────────────────────────────────────────┤
           │ CATEGORY        FILE                    REASON              │
           │ Build artifact  coverage.out            Coverage file       │
           │ Debug           __debug_bin             Go debug binary     │
           ├─────────────────────────────────────────────────────────────┤
           │ ? What would you like to do?                                │
           │   ❯ Remove and continue                                     │
           │     Include anyway (confirm each)                           │
           │     Abort and fix manually                                  │
           └─────────────────────────────────────────────────────────────┘
   └─► File grouping analysis:
       └─► Groups files by package/concern
       └─► Default: Multiple logical commits
       └─► Terminal output:
           ┌─────────────────────────────────────────────────────────────┐
           │ Smart Commit Analysis                                       │
           ├─────────────────────────────────────────────────────────────┤
           │ Group 1: pkg/config (source + test)                         │
           │   • pkg/config/parser.go (+5, -1)                           │
           │   • pkg/config/parser_test.go (+28, -0)                     │
           │   → "fix(config): handle nil options in parseConfig"        │
           │                                                             │
           │ ? Create 1 commit? [Y/n/edit/single]                        │
           └─────────────────────────────────────────────────────────────┘
   └─► IF multiple packages changed:
       └─► Groups separately:
           Group 1: pkg/config → "fix(config): handle nil options"
           Group 2: internal/cli → "fix(cli): update config usage"
   └─► Commits include ATLAS trailers:
       ```
       fix(config): handle nil options in parseConfig

       Added nil check before accessing cfg.Options to prevent
       panic when options are not provided.

       ATLAS-Task: task-20251229-100000
       ATLAS-Template: bugfix
       ```
   └─► Artifact: commit-message.md

9. **Step 6: Push** (Git push)
   └─► **[Story 6.4]** git push -u origin fix/fix-null-pointer
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 6/8: Pushing to remote                              0:03   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [⟳] git push -u origin fix/fix-null-pointer...                  │
       └─────────────────────────────────────────────────────────────────┘
   └─► On transient failure: Retry with exponential backoff (3 attempts)
       └─► Attempt 1: Failed (network timeout)
       └─► Wait 2 seconds
       └─► Attempt 2: Success
   └─► IF all retries exhausted:
       └─► State: Running → GHFailed
       └─► Terminal output:
           ┌─────────────────────────────────────────────────────────────┐
           │ ✗ Push failed after 3 attempts                              │
           ├─────────────────────────────────────────────────────────────┤
           │ Error: Authentication failed for 'https://github.com/...'   │
           ├─────────────────────────────────────────────────────────────┤
           │ ? What would you like to do?                                │
           │   ❯ Retry now — Try the operation again                     │
           │     Fix and retry — You fix the auth issue, then retry      │
           │     Abandon task — End task, keep branch for manual work    │
           └─────────────────────────────────────────────────────────────┘

10. **Step 7: Create PR** (GitHub PR)
    └─► **[Story 6.5]** Generate PR description from:
        - Task description
        - Commit messages
        - Files changed (2 files: +33, -1)
        - Diff summary
    └─► Artifact: pr-description.md
    └─► gh pr create command:
        ```bash
        gh pr create \
          --title "fix(config): handle nil options in parseConfig" \
          --body-file ~/.atlas/workspaces/fix-null-pointer/tasks/.../artifacts/pr-description.md \
          --base main \
          --head fix/fix-null-pointer
        ```
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Step 7/8: Creating PR                                    0:02   │
        ├─────────────────────────────────────────────────────────────────┤
        │ [✓] PR created: https://github.com/user/repo/pull/42            │
        └─────────────────────────────────────────────────────────────────┘
    └─► PR URL captured and displayed
    └─► IF PR creation fails (rate limit, auth, etc.):
        └─► Retry with exponential backoff (3 attempts)
        └─► IF still fails: State: Running → GHFailed
        └─► Options: [Retry] / [Fix and retry] / [Abandon]

11. **Step 8: CI Wait** (GitHub Actions)
    └─► **[Story 6.6]** Poll GitHub Actions API every 2 minutes
    └─► Timeout: 30 minutes (configurable)
    └─► Watch configured workflows: CI, Lint
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Step 8/8: Waiting for CI                                12:45   │
        ├─────────────────────────────────────────────────────────────────┤
        │ PR: https://github.com/user/repo/pull/42                        │
        │                                                                 │
        │ [✓] Lint           Passed                                2m 15s │
        │ [⟳] CI             Running (tests)                       8m 30s │
        └─────────────────────────────────────────────────────────────────┘
    └─► On all workflows pass: Continue to human review
    └─► IF any required workflow fails:
        └─► State: Running → CIFailed
        └─► Terminal output:
            ┌─────────────────────────────────────────────────────────────┐
            │ ✗ CI workflow "CI" failed                                   │
            ├─────────────────────────────────────────────────────────────┤
            │ ? What would you like to do?                                │
            │   ❯ View logs — Open GitHub Actions in browser              │
            │     Retry from implement — AI tries to fix based on output  │
            │     Manual fix — You fix, then 'atlas resume'               │
            │     Abandon — End task, keep PR as draft                    │
            └─────────────────────────────────────────────────────────────┘
    └─► IF timeout exceeded:
        └─► State: Running → CITimeout
        └─► Additional option: "Continue waiting"

12. **Human Review: Final**
    └─► **[Story 6.7]** State: Running → AwaitingApproval
    └─► Bell notification (BEL character)
    └─► Full review interface:
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
    └─► IF "Approve":
        └─► State: AwaitingApproval → Completed
        └─► Task marked complete
        └─► Terminal output:
            ┌─────────────────────────────────────────────────────────────┐
            │ ✓ Task completed successfully                               │
            │                                                             │
            │ PR ready for review: https://github.com/user/repo/pull/42   │
            │                                                             │
            │ To cleanup after PR merge:                                  │
            │   atlas workspace retire fix-null-pointer                   │
            │   atlas workspace destroy fix-null-pointer                  │
            └─────────────────────────────────────────────────────────────┘
    └─► IF "Reject and retry":
        └─► User provides feedback
        └─► State: AwaitingApproval → Running
        └─► Return to specified step with feedback context
    └─► IF "Merge PR now":
        └─► gh pr merge --squash
        └─► Workspace auto-retired
```

### Configuration Variants

```yaml
# .atlas/config.yaml
templates:
  bugfix:
    model: claude-sonnet-4-5-20250916
    verify: false                   # Cross-model verification OFF by default
    verify_model: gemini-3-pro      # Model for verification (if enabled)
    commit_style: multiple          # single | multiple
    auto_proceed_validation: true   # Continue on validation success
    validation:
      - magex format:fix
      - magex lint
      - magex test:race
      - go-pre-commit run --all-files
```

### CLI Flag Overrides

```bash
# Enable AI verification (default OFF for bugfix)
atlas start "fix bug" --template bugfix --verify

# Force single commit instead of multiple
atlas start "fix bug" --template bugfix --single-commit

# Skip human approval (CI/automation mode)
atlas start "fix bug" --template bugfix --auto-approve

# Override model
atlas start "fix bug" --template bugfix --model claude-opus-4-5
```

### Validation Checkpoints

| Step | Checkpoint | Expected Behavior | Story AC |
|------|------------|-------------------|----------|
| 4 | Analyze output | Root cause identified in artifact | 6.3 |
| 5 | Implement | Code changes + tests generated | 6.3 |
| 8 | Garbage scan | No false positives on Go source | 6.3 |
| 8 | File grouping | Source + test in same commit group | 6.3 |
| 8 | Multi-commit | Commits grouped by package/concern | 6.3 |
| 8 | Trailers | ATLAS-Task and ATLAS-Template present | 6.3 |
| 7 | Validation | All 4 commands pass | 6.4 |
| 9 | Push retry | 3 attempts with exponential backoff | 6.4 |
| 10 | PR description | Contains summary, files, test plan | 6.5 |
| 10 | PR title | Conventional format | 6.5 |
| 11 | CI wait | Polls until pass/fail/timeout | 6.6 |
| 12 | Final review | All options available | 6.7 |

---

## Scenario 2: Feature Workflow with Garbage Detection (Story 6.3)

### User Story

As a developer, I want ATLAS to warn me if I accidentally added debug files before committing.

### User Journey

```
1. User implements feature
   └─► Creates new HTTP client with retry logic

2. During development, user added debug artifacts
   └─► console.log statements in JS helper
   └─► .env.local with test credentials
   └─► __debug_bin executable
   └─► coverage.out from test run

3. **[Story 6.3]** Smart Commit detects garbage
   └─► EXPECTED: Pauses with warning:
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

4. User selects "Remove garbage and continue"
   └─► EXPECTED: Files removed from staging
   └─► EXPECTED: Commit proceeds with clean files only

5. Commit completes successfully
   └─► EXPECTED: Only production code committed
```

### Validation Checkpoints for Story 6.3 (Garbage Detection)

| Checkpoint | Expected Behavior | Acceptance Criteria |
|------------|-------------------|---------------------|
| Debug detection | Finds console.log, print statements | AC: "Debug files" |
| Secrets detection | Finds .env, credentials | AC: "Secrets (.env, credentials, API keys)" |
| Build artifacts | Finds __debug_bin, coverage.out | AC: "Build artifacts" |
| Temp files | Finds *.tmp, *.bak | AC: "Temporary files" |
| User choice | Menu with 3 options | AC: "pauses with warning and options" |
| Remove action | Removes from staging | AC: "Remove garbage and continue" |

---

## Scenario 3: PR Creation with Rate Limit (Story 6.5, 6.7)

### User Story

As a developer pushing multiple PRs, I want ATLAS to handle GitHub rate limits gracefully.

### User Journey

```
1. User has created 10 PRs in quick succession
   └─► GitHub API rate limit approaching

2. **[Story 6.5]** PR creation attempted
   └─► gh pr create returns "rate limit exceeded"

3. ATLAS retries with exponential backoff
   └─► Attempt 1: Failed (rate limit)
   └─► Wait 2 seconds
   └─► Attempt 2: Failed (rate limit)
   └─► Wait 4 seconds
   └─► Attempt 3: Failed (rate limit)

4. After 3 retries, enters gh_failed state
   └─► EXPECTED: Task status → gh_failed
   └─► EXPECTED: Terminal bell notification
   └─► EXPECTED: Error message with rate limit info

5. **[Story 6.7]** User sees options
   └─► EXPECTED: Menu appears:
       ┌─────────────────────────────────────────────────────────────────┐
       │ ✗ GitHub operation failed: rate limit exceeded                  │
       ├─────────────────────────────────────────────────────────────────┤
       │ ? What would you like to do?                                    │
       │   ❯ Retry now — Try the operation again                         │
       │     Fix and retry — You fix the issue, then retry               │
       │     Abandon task — End task, keep branch for manual work        │
       └─────────────────────────────────────────────────────────────────┘

6. User waits, then selects "Retry now"
   └─► EXPECTED: PR creation succeeds
   └─► EXPECTED: Task proceeds to ci_wait step
```

---

## Scenario 4: Multi-File Logical Grouping (Story 6.3)

### User Story

As a developer, I want ATLAS to group related files into logical commits when I've made changes across multiple packages.

### User Journey

```
1. User implements feature touching 3 packages
   └─► internal/config/loader.go (new config option)
   └─► internal/config/loader_test.go (tests)
   └─► internal/cli/root.go (CLI flag)
   └─► internal/cli/root_test.go (CLI tests)
   └─► docs/config.md (documentation)

2. **[Story 6.3]** Smart Commit analyzes changes
   └─► EXPECTED: Detects 3 logical groups:
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

3. User confirms with 'Y'
   └─► EXPECTED: 3 commits created in order:
       1. "feat(config): add verbose logging option"
       2. "feat(cli): add --verbose-logging flag"
       3. "docs: update configuration documentation"

4. Each commit includes ATLAS trailers
   └─► EXPECTED:
       ```
       feat(config): add verbose logging option

       Add VerboseLogging field to Config struct with corresponding
       getter method and unit tests.

       ATLAS-Task: task-20251229-120000
       ATLAS-Template: feature
       ```

5. IF user selects 'single'
   └─► EXPECTED: Single comprehensive commit:
       ```
       feat(config): add verbose logging option

       - Add VerboseLogging field to Config struct
       - Add --verbose-logging CLI flag
       - Update documentation

       ATLAS-Task: task-20251229-120000
       ATLAS-Template: feature
       ```
```

---

## Scenario 5: Feature Workflow with Speckit SDD (Stories 6.3-6.7 + SDD)

### Template Configuration

```yaml
Template: feature
Model: claude-opus-4-5-20251101 (more capable for complex features)
Steps: 18 (specify → clarify → commit_spec → review_spec → plan → tasks →
           analyze → commit_plan → review_plan → implement → verify →
           validate → checklist → commit → push → pr → ci_wait → review)
Human Checkpoints: 3 (review_spec, review_plan, final review)
AI Verification: ON by default (disable with --no-verify)
Commit Style: Multiple logical commits (grouped by package)
Pre-Review Commits: Commit before each human checkpoint for Git-based feedback
SDD Commands: /speckit.specify, /speckit.clarify, /speckit.plan,
              /speckit.tasks, /speckit.analyze, /speckit.implement,
              /speckit.checklist
```

### User Story

As a Go developer, I want to add retry logic to an HTTP client with proper specification, planning, and implementation using a structured SDD workflow.

### Preconditions

- ATLAS initialized with Speckit (`atlas init` completed)
- `.speckit/constitution.md` exists in repo
- `gh` CLI authenticated
- User has feature requirements in mind

### User Journey

```
1. User initiates feature workflow
   └─► atlas start "Add retry logic to HTTP client with exponential backoff" --template feature
   └─► IF --workspace not provided:
       └─► Auto-generate: add-retry-logic-http-client

2. **Workspace & Worktree Creation**
   └─► State: Pending → Running
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Creating workspace 'add-retry-logic'...                         │
       ├─────────────────────────────────────────────────────────────────┤
       │   Workspace: ~/.atlas/workspaces/add-retry-logic/               │
       │   Worktree:  ../atlas-add-retry-logic/                          │
       │   Branch:    feat/add-retry-logic                               │
       │   Base:      main                                               │
       │   Template:  feature (Speckit SDD)                              │
       └─────────────────────────────────────────────────────────────────┘
   └─► Creates: git worktree add ../atlas-add-retry-logic feat/add-retry-logic
   └─► IF branch exists:
       └─► Append timestamp: feat/add-retry-logic-20251229
       └─► WARN: "Branch already exists, using timestamped name"

3. **Step 1: Specify** (/speckit.specify)
   └─► AI generates spec.md (timeout: 20 min)
   └─► Reads: .speckit/constitution.md for project context
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 1/18: Generating specification                      3:45   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [⟳] Running /speckit.specify...                                 │
       │     Analyzing requirements...                                   │
       │     Generating user stories...                                  │
       └─────────────────────────────────────────────────────────────────┘
   └─► Output includes:
       - User stories with acceptance criteria
       - Functional requirements (FR1-FR7)
       - Non-functional requirements (NFR1-NFR4)
       - Edge cases and error scenarios
       - Out of scope items
   └─► Artifact: spec.md
   └─► IF AI fails or timeout:
       └─► State: Running → ValidationFailed
       └─► Options: [Retry] / [Manual spec] / [Abandon]

4. **Step 2: Clarify** (/speckit.clarify)
   └─► AI analyzes spec.md for ambiguities
   └─► Presents clarifying questions:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Clarification needed for "Add Retry Logic":                     │
       ├─────────────────────────────────────────────────────────────────┤
       │ 1. What is the maximum number of retry attempts?                │
       │    [3] [5] [10] [Configurable]                                  │
       │                                                                 │
       │ 2. Should retries apply to all HTTP methods or only             │
       │    idempotent ones (GET, PUT, DELETE)?                          │
       │    [All] [Idempotent only] [Configurable]                       │
       │                                                                 │
       │ 3. What backoff strategy?                                       │
       │    [Exponential] [Linear] [Fibonacci] [Custom]                  │
       │                                                                 │
       │ 4. Should there be a jitter factor to prevent thundering herd?  │
       │    [Yes, random jitter] [No] [Configurable]                     │
       └─────────────────────────────────────────────────────────────────┘
   └─► User answers inline or presses Enter for defaults
   └─► Spec.md updated with clarifications
   └─► Artifact: spec.md (version 2 if changed)

5. **Step 3: Commit - Specification Checkpoint**
   └─► Commits spec artifacts before human review
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Step 3/18: Committing specification                      0:02   │
       ├─────────────────────────────────────────────────────────────────┤
       │ [✓] Committed: "docs(spec): add retry logic specification"      │
       │                                                                 │
       │     Staged files:                                               │
       │       • .atlas/.../artifacts/spec.md                            │
       └─────────────────────────────────────────────────────────────────┘
   └─► Commit message:
       ```
       docs(spec): add retry logic specification

       Generated specification document for HTTP client retry feature
       with exponential backoff. Includes user stories, functional
       requirements, and edge cases.

       ATLAS-Task: task-20251229-143022
       ATLAS-Template: feature
       ATLAS-Phase: specification
       ```
   └─► Purpose: Human reviewer can checkout this commit to review spec in isolation
   └─► Enables: `git diff HEAD~1` to see spec changes if revision requested

6. **Step 4: Human Review - Specification**
   └─► State: Running → AwaitingApproval
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Specification Review                        feat/add-retry      │
       ├─────────────────────────────────────────────────────────────────┤
       │ User Stories: 3                                                 │
       │ Functional Requirements: 7                                      │
       │ Non-Functional Requirements: 4                                  │
       │ Edge Cases: 5                                                   │
       │                                                                 │
       │ Key Features:                                                   │
       │ • Exponential backoff with jitter                               │
       │ • Configurable max retries (default: 3)                         │
       │ • Retry only on 5xx and network errors                          │
       │ • Circuit breaker integration point                             │
       ├─────────────────────────────────────────────────────────────────┤
       │ ? What would you like to do?                                    │
       │   ❯ Approve and continue                                        │
       │     Request changes (with feedback)                             │
       │     View full spec.md                                           │
       │     Abort task                                                  │
       └─────────────────────────────────────────────────────────────────┘
   └─► IF "Request changes":
       └─► User provides feedback
       └─► State: AwaitingApproval → Running
       └─► AI revises spec.md with feedback context
       └─► Return to step 5

7. **Step 5: Plan** (/speckit.plan)
   └─► AI generates plan.md (timeout: 15 min)
   └─► Uses: spec.md as input
   └─► Output includes:
       - Technical architecture
       - Component breakdown with interfaces
       - Package structure decisions
       - Error handling strategy
       - Testing approach
   └─► Artifact: plan.md

8. **Step 6: Tasks** (/speckit.tasks)
   └─► AI generates tasks.md (timeout: 15 min)
   └─► Uses: spec.md + plan.md
   └─► Output includes:
       - Ordered task list with dependencies
       - Estimated complexity (S/M/L)
       - Files to create/modify per task
   └─► Artifact: tasks.md

9. **Step 7: Analyze** (/speckit.analyze)
   └─► AI validates cross-artifact consistency
   └─► Checks:
       - Every requirement in spec.md has implementing task
       - Every task aligns with plan.md approach
       - No orphaned requirements
       - No conflicting decisions
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Consistency Analysis                                            │
       ├─────────────────────────────────────────────────────────────────┤
       │ [✓] All 7 functional requirements mapped to tasks               │
       │ [✓] All 4 non-functional requirements addressed                 │
       │ [✓] No conflicting decisions found                              │
       │ [✓] Task dependencies valid                                     │
       └─────────────────────────────────────────────────────────────────┘
   └─► IF inconsistencies found:
       └─► Output: analysis-issues.md with specific problems
       └─► Options: [Auto-fix] / [Manual review] / [Ignore and proceed]

10. **Step 8: Commit - Planning Checkpoint**
    └─► Commits planning artifacts before human review
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Step 8/18: Committing planning artifacts                 0:03   │
        ├─────────────────────────────────────────────────────────────────┤
        │ [✓] Committed: "docs(plan): add implementation plan and tasks"  │
        │                                                                 │
        │     Staged files:                                               │
        │       • .atlas/.../artifacts/plan.md                            │
        │       • .atlas/.../artifacts/tasks.md                           │
        │       • .atlas/.../artifacts/analysis-issues.md (if exists)     │
        └─────────────────────────────────────────────────────────────────┘
    └─► Commit message:
        ```
        docs(plan): add implementation plan and tasks

        Technical design for retry middleware with exponential backoff.
        Includes 12 implementation tasks with dependencies and complexity
        estimates. Consistency analysis passed.

        ATLAS-Task: task-20251229-143022
        ATLAS-Template: feature
        ATLAS-Phase: planning
        ```
    └─► Purpose: Human reviewer can checkout this commit to review plan in isolation
    └─► Enables: `git log --oneline` shows clear progression through SDD phases
    └─► Enables: `git diff <spec-commit>..<plan-commit>` to compare phases

11. **Step 9: Human Review - Implementation Plan** (configurable)
   └─► State: Running → AwaitingApproval
   └─► Shows: plan.md + tasks.md summary
   └─► Terminal output:
       ┌─────────────────────────────────────────────────────────────────┐
       │ Implementation Plan Review                  feat/add-retry      │
       ├─────────────────────────────────────────────────────────────────┤
       │ Plan Summary:                                                   │
       │ • Create internal/http/retry.go with RetryConfig                │
       │ • Implement exponential backoff with jitter                     │
       │ • Add retry middleware wrapping http.RoundTripper               │
       │ • Configure via internal/config/retry.go                        │
       │                                                                 │
       │ Tasks: 12 total                                                 │
       │ • 4 Small, 6 Medium, 2 Large                                    │
       │ • Estimated 8 files to create/modify                            │
       ├─────────────────────────────────────────────────────────────────┤
       │ ? What would you like to do?                                    │
       │   ❯ Approve and continue                                        │
       │     Request changes                                             │
       │     View full plan.md                                           │
       │     View full tasks.md                                          │
       └─────────────────────────────────────────────────────────────────┘
   └─► IF --auto-proceed: Skip this step

12. **Step 10: Implement** (/speckit.implement)
    └─► AI executes tasks from tasks.md (timeout: 45 min, 3 retries)
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Step 10/18: Implementing feature                        12:34   │
        ├─────────────────────────────────────────────────────────────────┤
        │ [✓] Task 1/12: Create RetryConfig struct                        │
        │ [✓] Task 2/12: Implement backoff algorithm                      │
        │ [✓] Task 3/12: Add jitter calculation                           │
        │ [⟳] Task 4/12: Create retry middleware...                       │
        │ [ ] Task 5/12: Add configuration loading                        │
        │ ...                                                             │
        └─────────────────────────────────────────────────────────────────┘
    └─► For each task:
        - Creates/modifies files
        - Adds corresponding tests
        - Updates documentation if needed
    └─► Files changed tracked for commit grouping
    └─► IF implementation fails:
        └─► State: Running → ValidationFailed
        └─► Error context preserved for retry
        └─► Options: [Retry with feedback] / [Manual fix] / [Abandon]

13. **Step 11: AI Verification** (default ON for feature template)
    └─► Different model (gemini-3-pro) reviews implementation
    └─► Checks:
        - Implementation matches spec requirements
        - All tasks from tasks.md completed
        - No garbage files (*.tmp, __debug_bin, etc.)
        - Test coverage for new code
        - No obvious security issues
    └─► Artifact: verification-report.md
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ AI Verification                                          1:23   │
        ├─────────────────────────────────────────────────────────────────┤
        │ [✓] All 7 functional requirements implemented                   │
        │ [✓] All 12 tasks completed                                      │
        │ [✓] No garbage files detected                                   │
        │ [✓] Test coverage: 87% for new code                             │
        │ [✓] No security issues found                                    │
        └─────────────────────────────────────────────────────────────────┘
    └─► IF issues found:
        ┌─────────────────────────────────────────────────────────────────┐
        │ ⚠ Verification found 2 issues:                                  │
        ├─────────────────────────────────────────────────────────────────┤
        │ 1. Missing test for timeout scenario (spec FR-3)                │
        │ 2. Unused variable in retry.go:45                               │
        ├─────────────────────────────────────────────────────────────────┤
        │ ? How would you like to proceed?                                │
        │   ❯ Auto-fix issues                                             │
        │     Manual fix                                                  │
        │     Ignore and continue                                         │
        │     View full report                                            │
        └─────────────────────────────────────────────────────────────────┘
    └─► IF "Auto-fix": AI addresses issues and re-verifies
    └─► SKIP if: --no-verify flag

14. **Step 12: Validate** (Validation pipeline)
    └─► State: Running → Validating
    └─► Sequential then parallel execution:
        Phase 1 (sequential): magex format:fix
        Phase 2 (parallel):   magex lint || magex test:race
        Phase 3 (sequential): go-pre-commit run --all-files
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Validation Pipeline                                      4:56   │
        ├─────────────────────────────────────────────────────────────────┤
        │ [✓] Format     magex format:fix                          0.8s   │
        │ [✓] Lint       magex lint                                2m 15s │
        │ [✓] Test       magex test:race                           3m 42s │
        │ [✓] Pre-commit go-pre-commit run --all-files             1m 23s │
        └─────────────────────────────────────────────────────────────────┘
    └─► On success: State: Validating → Running (continues)
    └─► IF any step fails:
        └─► State: Validating → ValidationFailed
        └─► Bell notification (BEL character)
        └─► Options: [Retry] / [Manual Fix] / [Abandon]

15. **Step 13: Checklist** (/speckit.checklist)
    └─► AI generates checklist.md (timeout: 10 min)
    └─► Output includes:
        - Feature completeness verification
        - Test coverage summary
        - Documentation updates checklist
        - Performance considerations
        - Security review items
    └─► Artifact: checklist.md

16. **Step 14: Smart Commit** (Multiple Logical Commits)
    └─► Garbage detection scan (same as bugfix)
    └─► File grouping analysis:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Smart Commit Analysis                                           │
        ├─────────────────────────────────────────────────────────────────┤
        │ Group 1: internal/http (client changes)                        │
        │   • internal/http/retry.go (+156, -0)                          │
        │   • internal/http/retry_test.go (+234, -0)                     │
        │   • internal/http/client.go (+12, -3)                          │
        │   → "feat(http): add retry with exponential backoff"           │
        │                                                                │
        │ Group 2: internal/config (configuration)                       │
        │   • internal/config/retry.go (+45, -0)                         │
        │   • internal/config/retry_test.go (+67, -0)                    │
        │   → "feat(config): add retry configuration options"            │
        │                                                                │
        │ Group 3: Documentation                                         │
        │   • docs/retry.md (+89, -0)                                    │
        │   • README.md (+5, -1)                                         │
        │   → "docs: add retry logic documentation"                      │
        │                                                                │
        │ ? Create 3 commits? [Y/n/edit/single]                          │
        └─────────────────────────────────────────────────────────────────┘
    └─► Commits created with ATLAS trailers:
        ```
        feat(http): add retry with exponential backoff

        Implement retry middleware with configurable exponential backoff
        and jitter. Supports max retries, base delay, max delay, and
        retryable status codes.

        ATLAS-Task: task-20251229-143022
        ATLAS-Template: feature
        ```

17. **Step 15: Push** (Git push)
    └─► git push -u origin feat/add-retry-logic
    └─► Retry with exponential backoff on transient failure (3 attempts)
    └─► IF all retries exhausted: State: Running → GHFailed

18. **Step 16: Create PR** (GitHub PR)
    └─► Generate PR description from:
        - Task description
        - Commit messages (all 3)
        - spec.md summary
        - Files changed (8 files: +608, -4)
        - checklist.md items
    └─► Artifact: pr-description.md
    └─► gh pr create --title "feat(http): add retry with exponential backoff" --body-file pr-description.md
    └─► PR URL captured and displayed
    └─► IF PR creation fails: State: Running → GHFailed

19. **Step 17: CI Wait** (GitHub Actions)
    └─► Poll GitHub Actions API every 2 minutes
    └─► Watch configured workflows: CI, Lint, Security Scan
    └─► Timeout: 30 minutes (configurable)
    └─► Terminal output:
        ┌─────────────────────────────────────────────────────────────────┐
        │ CI Status                                               15:23   │
        ├─────────────────────────────────────────────────────────────────┤
        │ PR: https://github.com/user/repo/pull/42                        │
        │                                                                 │
        │ [✓] Lint           Passed                                2m 15s │
        │ [✓] CI             Passed                               12m 45s │
        │ [✓] Security Scan  Passed                                3m 12s │
        └─────────────────────────────────────────────────────────────────┘
    └─► IF any required workflow fails: State: Running → CIFailed
    └─► IF timeout exceeded: State: Running → CITimeout

20. **Step 18: Human Review - Final**
    └─► State: Running → AwaitingApproval
    └─► Full review interface:
        ┌─────────────────────────────────────────────────────────────────┐
        │ Feature Complete                          feat/add-retry        │
        ├─────────────────────────────────────────────────────────────────┤
        │ PR: https://github.com/user/repo/pull/42                        │
        │ Commits: 3                                                      │
        │ Files: 8 (+608, -4)                                             │
        │ CI: All checks passed                                           │
        │                                                                 │
        │ Artifacts:                                                      │
        │  • spec.md - Requirements (7 FR, 4 NFR)                         │
        │  • plan.md - Technical design                                   │
        │  • tasks.md - 12 tasks completed                                │
        │  • checklist.md - Quality checklist                             │
        │  • verification-report.md - AI review passed                    │
        ├─────────────────────────────────────────────────────────────────┤
        │ ? What would you like to do?                                   │
        │   ❯ Approve (mark task complete)                               │
        │     Reject and retry (with feedback)                           │
        │     View diff                                                  │
        │     View artifacts                                             │
        │     Open PR in browser                                         │
        │     Merge PR now                                               │
        │     Cancel                                                     │
        └─────────────────────────────────────────────────────────────────┘
    └─► IF "Approve":
        └─► State: AwaitingApproval → Completed
        └─► Task marked complete
        └─► Workspace can be retired after PR merge
    └─► IF "Reject and retry":
        └─► User provides feedback
        └─► State: AwaitingApproval → Running
        └─► Select step to return to with feedback context
```

### Artifacts Summary

| Step | Command | Artifact | Purpose | Versioned |
|------|---------|----------|---------|-----------|
| 3 | /speckit.specify | spec.md | Requirements document | Yes |
| 4 | /speckit.clarify | (updates spec.md) | Refined requirements | — |
| 5 | (commit) | git commit | **Spec checkpoint** - enables Git-based review | — |
| 7 | /speckit.plan | plan.md | Technical design | Yes |
| 8 | /speckit.tasks | tasks.md | Implementation breakdown | Yes |
| 9 | /speckit.analyze | analysis-issues.md | Consistency report (if issues) | No |
| 10 | (commit) | git commit | **Plan checkpoint** - enables Git-based review | — |
| 13 | (verification) | verification-report.md | Cross-model review | Yes |
| 15 | /speckit.checklist | checklist.md | Quality validation | Yes |
| 16 | (commit) | commit-message.md | Implementation commits | — |
| 18 | (pr) | pr-description.md | PR body | Yes |

### Configuration Options

```yaml
# ~/.atlas/config.yaml or .atlas/config.yaml
templates:
  feature:
    model: claude-opus-4-5-20251101
    verify: true                    # Cross-model verification ON by default
    verify_model: gemini-3-pro      # Model for verification
    commit_style: multiple          # single | multiple
    human_checkpoints:
      review_spec: true             # Pause after specify
      review_plan: true             # Pause after plan/tasks
      final_review: true            # Always pause at end
    validation:
      - magex format:fix
      - magex lint
      - magex test:race
      - go-pre-commit run --all-files
```

### Validation Checkpoints

| Step | Checkpoint | Expected Behavior | Story AC |
|------|------------|-------------------|----------|
| 3 | Specify | spec.md artifact with FR/NFR | SDD |
| 4 | Clarify | Ambiguities resolved via prompts | SDD |
| 5 | **Commit spec** | Git checkpoint before human review | 6.3 |
| 6 | Review spec | Human approval with options | 6.7 |
| 7 | Plan | plan.md artifact | SDD |
| 8 | Tasks | tasks.md with dependencies | SDD |
| 9 | Analyze | Cross-artifact consistency check | SDD |
| 10 | **Commit plan** | Git checkpoint before human review | 6.3 |
| 11 | Review plan | Human approval with options | 6.7 |
| 12 | Implement | All tasks executed | SDD |
| 13 | Verify | Cross-model review report | 6.3 |
| 14 | Validation | Parallel lint+test execution | 6.4 |
| 16 | Multi-commit | 3+ commits grouped logically | 6.3 |
| 18 | PR | Description includes spec summary | 6.5 |
| 19 | CI wait | Configurable timeout | 6.6 |
| 20 | Final review | All artifacts accessible | 6.7 |

---

## Integration with Acceptance Criteria

These scenarios should be referenced in the story files when they are created. Add a new section:

```markdown
## User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoints 4-12)
- Scenario 2: Garbage Detection (all checkpoints)
- Scenario 5: Feature with Speckit (checkpoints 3-18)
```
