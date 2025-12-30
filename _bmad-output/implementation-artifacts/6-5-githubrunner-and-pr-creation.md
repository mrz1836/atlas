# Story 6.5: GitHubRunner and PR Creation

Status: done

## Story

As a **user**,
I want **ATLAS to create pull requests via gh CLI**,
So that **my completed work is ready for review without manual PR creation**.

## Acceptance Criteria

1. **Given** the branch is pushed to remote, **When** the git_pr step executes, **Then** the system generates a PR description via AI based on task description, commit messages, files changed, and diff summary

2. **Given** a PR description is generated, **When** creating the PR, **Then** the description is saved as artifact (pr-description.md) before PR creation

3. **Given** the PR content is ready, **When** executing gh CLI, **Then** the system runs:
   ```bash
   gh pr create \
     --title "<type>: <summary>" \
     --body "$(cat pr-description.md)" \
     --base main \
     --head <branch>
   ```

4. **Given** the PR is created, **When** parsing the response, **Then** the PR URL is captured and returned to the caller

5. **Given** the PR title is generated, **When** formatting, **Then** it follows conventional commits format: `<type>(<scope>): <description>`

6. **Given** a PR body is generated, **When** formatting, **Then** it includes:
   - Summary of changes
   - Files modified with brief descriptions
   - Test plan / validation results
   - Link to task artifacts (if applicable)

7. **Given** gh CLI fails, **When** the error is detected, **Then** the system transitions to `gh_failed` state with actionable error message

8. **Given** a transient failure (rate limit, network), **When** the failure is detected, **Then** retry logic with exponential backoff executes (3 attempts max)

## Tasks / Subtasks

- [x] Task 1: Create `internal/git/github.go` with GitHubRunner interface (AC: 1, 3, 4, 7, 8)
  - [x] 1.1: Define GitHubRunner interface:
    - `CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error)`
    - `GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)` (for future CI monitoring)
  - [x] 1.2: Define PRCreateOptions struct with Title, Body, BaseBranch, HeadBranch, Draft fields
  - [x] 1.3: Define PRResult struct with Number, URL, State fields
  - [x] 1.4: Define PRErrorType enum (None, Auth, RateLimit, Network, NotFound, Other)

- [x] Task 2: Implement CLIGitHubRunner using gh CLI (AC: 3, 4)
  - [x] 2.1: Create CLIGitHubRunner struct with workDir, logger, retryConfig fields
  - [x] 2.2: Implement CreatePR method calling `gh pr create` with proper flags
  - [x] 2.3: Parse gh CLI output to extract PR number and URL
  - [x] 2.4: Handle `--base` and `--head` flag construction
  - [x] 2.5: Support `--draft` flag for draft PRs

- [x] Task 3: Implement retry logic with error classification (AC: 7, 8)
  - [x] 3.1: Create `classifyGHError(err error) PRErrorType` function
  - [x] 3.2: Detect rate limit errors from gh output patterns:
    - "rate limit exceeded"
    - "API rate limit"
    - "secondary rate limit"
  - [x] 3.3: Detect authentication errors:
    - "authentication required"
    - "Bad credentials"
    - "not logged into"
  - [x] 3.4: Detect network errors (same patterns as push.go)
  - [x] 3.5: Implement exponential backoff (3 attempts, 2s initial, 2.0 multiplier)
  - [x] 3.6: Only retry on rate limit and network errors; fail immediately on auth errors

- [x] Task 4: Create PRDescriptionGenerator service (AC: 1, 2, 5, 6)
  - [x] 4.1: Define PRDescriptionGenerator interface:
    - `Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error)`
  - [x] 4.2: Define PRDescOptions struct with TaskDescription, CommitMessages, FilesChanged, DiffSummary, ValidationResults fields
  - [x] 4.3: Define PRDescription struct with Title, Body, ConventionalType, Scope fields
  - [x] 4.4: Implement title formatting with conventional commits pattern
  - [x] 4.5: Implement body formatting with required sections

- [x] Task 5: Implement AI-based PR description generation (AC: 1, 6)
  - [x] 5.1: Create AIDescriptionGenerator struct with AIRunner dependency
  - [x] 5.2: Build prompt from PRDescOptions including all context
  - [x] 5.3: Parse AI response into PRDescription struct
  - [x] 5.4: Validate generated title format (conventional commits)
  - [x] 5.5: Validate generated body contains required sections

- [x] Task 6: Add artifact saving for PR description (AC: 2)
  - [x] 6.1: Before CreatePR, save body to `pr-description.md` in task artifacts
  - [x] 6.2: Use task store's SaveArtifact method if available, else direct file write
  - [x] 6.3: Include PR metadata in artifact header (title, base, head)

- [x] Task 7: Add sentinel errors to internal/errors (AC: 7)
  - [x] 7.1: Add `ErrPRCreationFailed` sentinel
  - [x] 7.2: Add `ErrGHRateLimited` sentinel
  - [x] 7.3: Add `ErrGHAuthFailed` sentinel
  - [x] 7.4: Document errors with clear descriptions

- [x] Task 8: Create comprehensive tests (AC: 1-8)
  - [x] 8.1: Test successful PR creation with URL extraction
  - [x] 8.2: Test PR title conventional commits format
  - [x] 8.3: Test PR body required sections
  - [x] 8.4: Test rate limit error classification
  - [x] 8.5: Test auth error classification
  - [x] 8.6: Test network error classification
  - [x] 8.7: Test retry logic with mock failures
  - [x] 8.8: Test artifact saving
  - [x] 8.9: Test draft PR creation
  - [x] 8.10: Target 90%+ coverage

## Dev Notes

### Existing Code to Reuse/Extend

**CRITICAL: Follow patterns from Story 6.4 (PushRunner)**

The PushRunner in `internal/git/push.go` establishes the retry and error classification patterns to follow:

```go
// From internal/git/push.go - REUSE THIS PATTERN
type PushErrorType int

const (
    PushErrorNone PushErrorType = iota
    PushErrorAuth     // Don't retry
    PushErrorNetwork  // Retry with backoff
    PushErrorTimeout  // Retry with backoff
    PushErrorOther    // Don't retry
)

// Retry config pattern to reuse
type RetryConfig struct {
    MaxAttempts  int           // Default: 3
    InitialDelay time.Duration // Default: 2s
    MaxDelay     time.Duration // Default: 30s
    Multiplier   float64       // Default: 2.0
}
```

**CRITICAL: Use existing AIRunner for description generation**

```go
// From internal/ai/runner.go
type AIRunner interface {
    Run(ctx context.Context, req *AIRequest) (*AIResult, error)
}

// Use claude CLI in plan mode for description generation
```

### GitHubRunner Interface Design

```go
// internal/git/github.go

package git

import (
    "context"
    "fmt"
    "strings"
    "time"

    atlaserrors "github.com/mrz1836/atlas/internal/errors"
    "github.com/rs/zerolog"
)

// PRErrorType classifies GitHub operation failures for appropriate handling.
type PRErrorType int

const (
    PRErrorNone PRErrorType = iota
    PRErrorAuth       // Authentication failed - don't retry
    PRErrorRateLimit  // Rate limited - retry with backoff
    PRErrorNetwork    // Network issue - retry with backoff
    PRErrorNotFound   // Resource not found - don't retry
    PRErrorOther      // Other error - don't retry
)

// PRCreateOptions configures the PR creation operation.
type PRCreateOptions struct {
    Title      string
    Body       string
    BaseBranch string // Default: "main"
    HeadBranch string // Required: the feature branch
    Draft      bool   // Create as draft PR
}

// PRResult contains the outcome of a PR creation.
type PRResult struct {
    Number   int
    URL      string
    State    string // "open", "draft"
    ErrorType PRErrorType
    Attempts int
    FinalErr error
}

// GitHubRunner defines operations for GitHub via gh CLI.
type GitHubRunner interface {
    // CreatePR creates a pull request and returns the result.
    CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error)

    // GetPRStatus gets the current status of a PR (for CI monitoring in Story 6.6).
    GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)
}

// PRStatus contains PR and CI check status (for Story 6.6).
type PRStatus struct {
    Number     int
    State      string
    Mergeable  bool
    ChecksPass bool
    CIStatus   string // pending, success, failure
}
```

### gh CLI Command Pattern

```bash
# Create PR command
gh pr create \
    --title "fix(config): handle nil options in parseConfig" \
    --body "$(cat pr-description.md)" \
    --base main \
    --head fix/fix-null-pointer

# Response parsing - gh outputs the PR URL on success
# Example: https://github.com/owner/repo/pull/42

# Get PR number from URL
# URL format: https://github.com/{owner}/{repo}/pull/{number}
```

### Error Classification Patterns

```go
func classifyGHError(err error) PRErrorType {
    if err == nil {
        return PRErrorNone
    }

    errStr := strings.ToLower(err.Error())

    // Rate limit errors (retry with longer backoff)
    rateLimitPatterns := []string{
        "rate limit exceeded",
        "api rate limit",
        "secondary rate limit",
        "abuse detection",
    }
    for _, pattern := range rateLimitPatterns {
        if strings.Contains(errStr, pattern) {
            return PRErrorRateLimit
        }
    }

    // Authentication errors (don't retry)
    authPatterns := []string{
        "authentication required",
        "bad credentials",
        "not logged into",
        "must be authenticated",
        "gh auth login",
    }
    for _, pattern := range authPatterns {
        if strings.Contains(errStr, pattern) {
            return PRErrorAuth
        }
    }

    // Network errors (retry with backoff)
    networkPatterns := []string{
        "could not resolve host",
        "connection refused",
        "network is unreachable",
        "connection timed out",
        "no route to host",
    }
    for _, pattern := range networkPatterns {
        if strings.Contains(errStr, pattern) {
            return PRErrorNetwork
        }
    }

    // Not found errors (don't retry)
    notFoundPatterns := []string{
        "not found",
        "no such",
        "repository not found",
    }
    for _, pattern := range notFoundPatterns {
        if strings.Contains(errStr, pattern) {
            return PRErrorNotFound
        }
    }

    return PRErrorOther
}
```

### PR Description Format

```markdown
## Summary

Brief description of changes derived from task description and commit messages.

## Changes

- `pkg/config/parser.go` - Added nil check for options parameter
- `pkg/config/parser_test.go` - Added test case for nil options scenario

## Test Plan

- [x] Unit tests pass
- [x] Lint passes
- [x] Pre-commit hooks pass

## ATLAS Metadata

- Task: task-20251229-100000
- Template: bugfix
- Workspace: fix-null-pointer
```

### Conventional Commits Title Format

```go
// Title format: <type>(<scope>): <description>
// Types: feat, fix, docs, style, refactor, test, chore, build, ci

func formatPRTitle(commitType, scope, description string) string {
    if scope != "" {
        return fmt.Sprintf("%s(%s): %s", commitType, scope, description)
    }
    return fmt.Sprintf("%s: %s", commitType, description)
}

// Derive type from template
func typeFromTemplate(templateName string) string {
    switch templateName {
    case "bugfix":
        return "fix"
    case "feature":
        return "feat"
    case "commit":
        return "chore"
    default:
        return "feat"
    }
}

// Derive scope from files changed
func scopeFromFiles(files []string) string {
    // Find common package/directory
    // e.g., if all files in pkg/config, scope = "config"
}
```

### Project Structure Notes

**File Locations:**
- `internal/git/github.go` - GitHubRunner interface, CLIGitHubRunner implementation
- `internal/git/github_test.go` - Comprehensive tests
- `internal/git/pr_description.go` - PRDescriptionGenerator interface and AI implementation
- `internal/git/pr_description_test.go` - Description generator tests
- `internal/errors/errors.go` - Add ErrPRCreationFailed, ErrGHRateLimited, ErrGHAuthFailed

**Import Rules (from architecture.md):**
- `internal/git` can import: constants, errors, domain, ai (for AIRunner)
- `internal/git` cannot import: task, workspace, cli, validation, template, tui

### Context-First Pattern

From project-context.md:

```go
// ALWAYS: ctx as first parameter
func (g *CLIGitHubRunner) CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error) {
    // ALWAYS: Check cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... implementation
}
```

### Error Handling

Use existing sentinels and action-first format:

```go
import atlaserrors "github.com/mrz1836/atlas/internal/errors"

// Action-first error format
return nil, fmt.Errorf("failed to create pull request: %w", err)

// For PR-specific errors, wrap with appropriate sentinel
return nil, fmt.Errorf("authentication failed: %w", atlaserrors.ErrGHAuthFailed)
return nil, fmt.Errorf("rate limited: %w", atlaserrors.ErrGHRateLimited)
```

### Validation Commands Required

**Before marking story complete, run ALL FOUR:**
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

### Gitleaks Compliance (CRITICAL)

**Test values MUST NOT look like secrets:**
- Use semantic names: `ATLAS_TEST_PR_PATTERN`
- Avoid numeric suffixes that look like keys: `_12345`
- Use `mock_value_for_test` patterns

### References

- [Source: epics.md - Story 6.5: GitHubRunner and PR Creation]
- [Source: architecture.md - GitHubRunner Interface section]
- [Source: architecture.md - Retry Strategy section]
- [Source: project-context.md - Context Handling (CRITICAL)]
- [Source: epic-6-implementation-notes.md - GitHubRunner Design]
- [Source: epic-6-user-scenarios.md - Scenario 1 steps 10, Scenario 3 all, Scenario 5 step 18]
- [Source: internal/git/push.go - PushRunner pattern to follow]
- [Source: internal/errors/errors.go - Existing error sentinels]
- [Source: 6-1-gitrunner-implementation.md - Story 6.1 learnings]
- [Source: 6-4-push-to-remote.md - Story 6.4 learnings for retry patterns]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoint 10 - PR creation step)
- Scenario 3: PR Rate Limit Handling (all checkpoints)
- Scenario 5: Feature Workflow with Speckit SDD (checkpoint 18 - PR creation step)

Specific validation checkpoints from scenarios:
| Checkpoint | Expected Behavior | AC |
|------------|-------------------|-----|
| PR description | Generated from task/commits/files/diff | AC1 |
| Artifact saving | pr-description.md saved before creation | AC2 |
| gh pr create | Command executed with proper flags | AC3 |
| PR URL capture | URL extracted and returned | AC4 |
| Title format | Conventional commits format | AC5 |
| Body sections | Summary, files, test plan included | AC6 |
| Error handling | Transition to gh_failed on failure | AC7 |
| Rate limit retry | 3 attempts with exponential backoff | AC8 |

### Previous Story Intelligence

**From Story 6.1 (GitRunner Implementation):**
- Runner interface pattern established: `git.Runner` not `git.GitRunner`
- Shared `git.RunCommand` utility in `internal/git/command.go`
- Tests use `t.TempDir()` with temp git repos
- Context cancellation check at method entry
- Errors wrapped with `atlaserrors.ErrGitOperation`
- Action-first error format: `failed to <action>: %w`

**From Story 6.4 (Push to Remote):**
- PushService pattern with PushRunner implementation
- RetryConfig struct for retry behavior configuration
- Error classification pattern: `classifyPushError()`
- Exponential backoff implementation with context cancellation during wait
- Helper functions for pattern matching: `isAuthError()`, `isNetworkError()`
- Test coverage at 93.4%
- Function decomposition to reduce cognitive complexity:
  - `validateAndNormalizeOpts()`
  - `handleConfirmation()`
  - `executePushWithRetry()`
  - etc.

### Git Intelligence (Recent Commits)

Recent commits in Epic 6 branch show patterns to follow:
- `feat(git): implement push service with retry and error handling` - PushRunner pattern
- `feat(git): implement smart commit system with garbage detection` - SmartCommitRunner pattern
- `feat(git): add branch creation and naming system` - BranchCreatorService pattern
- `feat(git): implement GitRunner for git CLI operations` - Base Runner interface

File patterns established:
- Implementation: `internal/git/<feature>.go`
- Tests: `internal/git/<feature>_test.go`
- Interface + types in same file when small, separate when large

### Testing Strategy

**Unit Tests (mock gh CLI execution):**
```go
func TestCLIGitHubRunner_CreatePR_Success(t *testing.T) {
    mockCmd := &MockCommandRunner{
        RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
            // Verify gh pr create command
            assert.Equal(t, "gh", name)
            assert.Contains(t, args, "pr")
            assert.Contains(t, args, "create")
            // Return mock PR URL
            return []byte("https://github.com/owner/repo/pull/42\n"), nil
        },
    }
    runner := NewCLIGitHubRunner(mockCmd)
    result, err := runner.CreatePR(context.Background(), PRCreateOptions{
        Title:      "fix(config): handle nil options",
        Body:       "Test body",
        BaseBranch: "main",
        HeadBranch: "fix/test",
    })
    require.NoError(t, err)
    assert.Equal(t, 42, result.Number)
    assert.Equal(t, "https://github.com/owner/repo/pull/42", result.URL)
}
```

**Error Classification Tests:**
```go
func TestClassifyGHError(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        expected PRErrorType
    }{
        {
            name:     "rate limit error",
            err:      errors.New("API rate limit exceeded"),
            expected: PRErrorRateLimit,
        },
        {
            name:     "auth error",
            err:      errors.New("gh auth login - not logged into any GitHub hosts"),
            expected: PRErrorAuth,
        },
        {
            name:     "network error",
            err:      errors.New("Could not resolve host: api.github.com"),
            expected: PRErrorNetwork,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := classifyGHError(tt.err)
            assert.Equal(t, tt.expected, got)
        })
    }
}
```

**Retry Logic Tests:**
```go
func TestCLIGitHubRunner_CreatePR_RetryOnRateLimit(t *testing.T) {
    attempts := 0
    mockCmd := &MockCommandRunner{
        RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
            attempts++
            if attempts < 3 {
                return nil, errors.New("API rate limit exceeded")
            }
            return []byte("https://github.com/owner/repo/pull/42\n"), nil
        },
    }
    runner := NewCLIGitHubRunner(mockCmd, WithGHRetryConfig(RetryConfig{
        MaxAttempts:  3,
        InitialDelay: 10 * time.Millisecond, // Fast for tests
        Multiplier:   2.0,
    }))
    result, err := runner.CreatePR(context.Background(), PRCreateOptions{...})
    require.NoError(t, err)
    assert.Equal(t, 3, attempts)
    assert.Equal(t, 3, result.Attempts)
}
```

### AI Description Generation

For the AIDescriptionGenerator, use the AIRunner from `internal/ai`:

```go
// internal/git/pr_description.go

package git

import (
    "context"
    "fmt"
    "strings"

    "github.com/mrz1836/atlas/internal/ai"
)

// PRDescriptionGenerator generates PR descriptions.
type PRDescriptionGenerator interface {
    Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error)
}

// PRDescOptions contains inputs for PR description generation.
type PRDescOptions struct {
    TaskDescription   string
    CommitMessages    []string
    FilesChanged      []FileChange
    DiffSummary       string
    ValidationResults string
    TemplateName      string
    TaskID            string
    WorkspaceName     string
}

// FileChange represents a changed file.
type FileChange struct {
    Path       string
    Insertions int
    Deletions  int
}

// PRDescription contains the generated PR content.
type PRDescription struct {
    Title           string
    Body            string
    ConventionalType string // feat, fix, etc.
    Scope           string  // derived from files
}

// AIDescriptionGenerator uses AI to generate PR descriptions.
type AIDescriptionGenerator struct {
    aiRunner ai.AIRunner
    logger   zerolog.Logger
}

// NewAIDescriptionGenerator creates a new AI-based generator.
func NewAIDescriptionGenerator(runner ai.AIRunner, opts ...AIDescGenOption) *AIDescriptionGenerator {
    // ...
}

func (g *AIDescriptionGenerator) Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error) {
    // Build prompt
    prompt := g.buildPrompt(opts)

    // Call AI
    result, err := g.aiRunner.Run(ctx, &ai.AIRequest{
        Prompt:         prompt,
        PermissionMode: "plan", // Read-only for description generation
        MaxTurns:       1,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to generate PR description: %w", err)
    }

    // Parse response
    return g.parseResponse(result.Result, opts.TemplateName)
}

func (g *AIDescriptionGenerator) buildPrompt(opts PRDescOptions) string {
    var sb strings.Builder
    sb.WriteString("Generate a pull request title and body for the following changes:\n\n")
    sb.WriteString("## Task Description\n")
    sb.WriteString(opts.TaskDescription)
    sb.WriteString("\n\n## Commits\n")
    for _, msg := range opts.CommitMessages {
        sb.WriteString("- " + msg + "\n")
    }
    sb.WriteString("\n## Files Changed\n")
    for _, f := range opts.FilesChanged {
        sb.WriteString(fmt.Sprintf("- %s (+%d, -%d)\n", f.Path, f.Insertions, f.Deletions))
    }
    // ... rest of prompt construction
    return sb.String()
}
```

### Fallback Description Generator

If AI is unavailable, provide a template-based fallback:

```go
// TemplateDescriptionGenerator generates descriptions without AI.
type TemplateDescriptionGenerator struct{}

func (g *TemplateDescriptionGenerator) Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error) {
    // Derive type from template
    commitType := typeFromTemplate(opts.TemplateName)

    // Derive scope from files
    scope := scopeFromFiles(opts.FilesChanged)

    // Build title from first commit message or task description
    title := formatPRTitle(commitType, scope, summarize(opts.TaskDescription))

    // Build body from template
    body := fmt.Sprintf(`## Summary

%s

## Changes

%s

## Test Plan

- [ ] Tests pass
- [ ] Lint passes

## ATLAS Metadata

- Task: %s
- Template: %s
- Workspace: %s
`, opts.TaskDescription, formatFileChanges(opts.FilesChanged), opts.TaskID, opts.TemplateName, opts.WorkspaceName)

    return &PRDescription{
        Title:           title,
        Body:            body,
        ConventionalType: commitType,
        Scope:           scope,
    }, nil
}
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

1. **Interface Naming**: Renamed `GitHubRunner` to `HubRunner` to avoid Go stutter warning (`git.GitHubRunner`)
2. **FileChange Type Conflict**: Renamed to `PRFileChange` since `FileChange` already exists in `internal/git/types.go`
3. **CommandExecutor Pattern**: Used existing `CommandExecutor` interface from `internal/git/command.go` for testability
4. **Functional Options**: Implemented `WithGH*` option functions following project patterns
5. **Error Classification**: Implemented comprehensive pattern matching for rate limit, auth, network, and not-found errors
6. **Retry Logic**: Exponential backoff with context-aware wait (respects cancellation during sleep)
7. **PR Description Validation**: Both title format (conventional commits) and body sections (Summary, Changes, Test Plan) are validated
8. **Artifact Saving**: Two implementations - `TaskStoreArtifactSaver` for task store integration, `FileArtifactSaver` for direct file writes
9. **Template Fallback**: `TemplateDescriptionGenerator` provides fallback when AI is unavailable
10. **Test Coverage**: Comprehensive tests for all components with 90%+ coverage target achieved

### Code Review Fixes (2025-12-30)

11. **Test Coverage Improvements**: Added tests for `buildPrompt()` all branches, `buildPRFinalError()` PRErrorOther cases, `waitForPRRetry()` max delay cap - coverage increased from 93.1% to 94.2%
12. **scopeFromFiles Enhancement**: Expanded directory exclusion list to include `src`, `lib`, `test`, `tests`, `spec`, `vendor`, `node_modules` for better scope detection across different project structures
13. **summarizeDescription Case Preservation**: Changed from `ToLower()` to `lowercaseFirst()` to preserve acronyms and proper nouns (e.g., "Update API endpoint" → "update API endpoint" instead of "update api endpoint")
14. **New Helper Function**: Added `lowercaseFirst()` for UTF-8 safe first-character lowercasing

### File List

**New Files:**
- `internal/git/github.go` - HubRunner interface, CLIGitHubRunner implementation, PR types, error classification, retry logic
- `internal/git/github_test.go` - Comprehensive tests for HubRunner
- `internal/git/pr_description.go` - PRDescriptionGenerator interface, AI and Template implementations
- `internal/git/pr_description_test.go` - Tests for PR description generation
- `internal/git/pr_artifact.go` - PRArtifactSaver interface and implementations
- `internal/git/pr_artifact_test.go` - Tests for artifact saving

**Modified Files:**
- `internal/errors/errors.go` - Added ErrPRCreationFailed, ErrGHRateLimited, ErrGHAuthFailed, ErrAIEmptyResponse, ErrAIInvalidFormat sentinels

