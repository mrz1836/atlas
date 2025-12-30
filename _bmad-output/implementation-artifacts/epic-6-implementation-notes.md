# Epic 6: Git & PR Automation - Implementation Notes

Created during Epic 4 Retrospective (2025-12-28)

---

## User Scenarios - Critical Reference

**IMPORTANT**: Use `epic-6-user-scenarios.md` as the primary validation document for all Epic 6 stories.

```
_bmad-output/implementation-artifacts/epic-6-user-scenarios.md
```

### Purpose

This document provides real-world user workflows that validate Epic 6 stories against actual user needs. Each scenario includes:
- Complete user journeys with terminal output examples
- State machine diagrams for task flow
- Validation checkpoints mapped to acceptance criteria
- Error handling paths and recovery options

### Scenario Coverage

| Scenario | Description | Stories Covered |
|----------|-------------|-----------------|
| 1 | Bugfix Workflow | 6.3, 6.4, 6.5, 6.6, 6.7 |
| 2 | Garbage Detection | 6.3 |
| 3 | PR Rate Limit Handling | 6.5, 6.7 |
| 4 | Multi-File Logical Grouping | 6.3 |
| 5 | Feature Workflow with Speckit SDD | 6.3, 6.4, 6.5, 6.6, 6.7 |

### Usage During Implementation

1. **Reference validation checkpoints** when implementing acceptance criteria
2. **Follow state machine flows** for error handling and state transitions
3. **Use terminal output examples** as design specs for TUI components
4. **Check scenario prerequisites** for integration requirements

### Story Integration

Each story file should reference its validating scenarios:

```markdown
## User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoints 4-12)
- Scenario 2: Garbage Detection (all checkpoints)
```

---

## Story 6.3: Smart Commit System - Key Reference

### Source Material

**CRITICAL**: The smart commit system prompt already exists and should be used as the foundation:

```
~/.claude/commands/sc.md
```

This file contains a comprehensive smart commit implementation with:

1. **Command Structure**: `/sc [p] [skip-hooks]`
   - `p` - Auto-push after commit
   - `skip-hooks` - Bypass pre-commit hooks (emergency only)

2. **AI Attribution Prevention** - Critical requirement to NEVER include Claude/Anthropic attribution in commits

3. **Pre-commit Hook Integration** - Allows hooks to run by default, only bypass when explicitly requested

4. **Complete Workflow**:
   - Parse arguments
   - Read project conventions (.github/AGENTS.md)
   - Analyze ALL changes (modified, new, deleted, renamed)
   - Group related changes logically
   - Generate conventional commit messages
   - Present commit plan for approval
   - Execute commits with attribution sanitization
   - Optional auto-push

5. **Commit Message Format**:
   ```
   <type>(<scope>): <description>

   [optional body]
   [optional footer]
   ```

   Types: feat, fix, docs, style, refactor, test, chore, build, ci

6. **Smart Grouping Categories**:
   - Feature changes
   - Bug fixes
   - Documentation
   - Refactoring
   - Tests
   - Chores
   - Deletions
   - Reorganization

7. **Post-Commit Sanitization** - Always check and remove any AI attribution that slips through

### Implementation Approach for Story 6.3

When implementing `internal/git/commit.go`, the SmartCommitRunner should:

1. **Load and parse** the sc.md prompt structure
2. **Implement** the change analysis logic in Go
3. **Use** the existing AIRunner to generate commit messages
4. **Apply** the attribution sanitization checks
5. **Integrate** with GitRunner for actual commit operations

### Key Code Patterns from sc.md

```bash
# Change detection
git status --porcelain -uall
git diff --staged --stat
git ls-files --others --exclude-standard

# Attribution check
LAST_MSG=$(git log -1 --pretty=%B)
if echo "$LAST_MSG" | grep -E "(Claude|Anthropic|\\bAI\\b|Generated|Co-Authored-By)" > /dev/null; then
    # Sanitize commit
fi

# Clean commit
git commit -m "$COMMIT_MSG" --cleanup=strip
```

### Testing Considerations

1. Mock git commands for unit tests
2. Test attribution detection and removal
3. Test commit message generation quality
4. Test hook bypass scenarios
5. Test auto-push functionality

### Dependencies

- Story 6.1: GitRunner Implementation (provides git command execution)
- Story 6.2: Branch Creation and Naming (provides branch context)

---

## Other Epic 6 Notes

### GitRunner Design

The GitRunner should wrap git commands similar to how CommandRunner wraps shell commands:

```go
type GitRunner interface {
    Status(ctx context.Context, path string) (*GitStatus, error)
    Diff(ctx context.Context, path string, staged bool) (string, error)
    Add(ctx context.Context, path string, files ...string) error
    Commit(ctx context.Context, path, message string, opts CommitOptions) error
    Push(ctx context.Context, path string, opts PushOptions) error
    // ... etc
}
```

### GitHubRunner Design

For PR creation (Story 6.5), use `gh` CLI:

```go
type GitHubRunner interface {
    CreatePR(ctx context.Context, opts PROptions) (*PullRequest, error)
    GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)
    GetCIStatus(ctx context.Context, ref string) (*CIStatus, error)
}
```
