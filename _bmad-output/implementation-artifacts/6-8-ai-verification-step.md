# Story 6.8: AI Verification Step

Status: draft

## Story

As a **user**,
I want **an optional AI verification step that uses a different model to review my implementation**,
So that **I can catch issues before committing, with cross-model validation for higher confidence**.

## Acceptance Criteria

1. **Given** the `--verify` flag is passed to `atlas start`, **When** implementation completes, **Then** the system invokes a secondary AI model (configurable, default: different from primary) to review the implementation.

2. **Given** AI verification is enabled, **When** the verification runs, **Then** the verifier checks:
   - Code change correctness against the task description
   - Test coverage for new/modified code
   - No garbage files introduced (*.tmp, __debug_bin, coverage.out, etc.)
   - No obvious security issues (hardcoded secrets, SQL injection patterns, etc.)

3. **Given** verification completes successfully, **When** no issues are found, **Then** the system continues to the next step (validate or commit) and saves `verification-report.md` artifact.

4. **Given** verification finds issues, **When** issues are detected, **Then** the system presents options:
   - Auto-fix issues — AI attempts to fix based on verification feedback
   - Manual fix — User fixes, then resumes
   - Ignore and continue — Proceed despite warnings
   - View full report — Display complete verification-report.md

5. **Given** the `--no-verify` flag is passed, **When** starting a task, **Then** verification step is skipped regardless of template default.

6. **Given** a template configuration, **When** `verify: true` is set, **Then** verification runs by default (can be overridden with `--no-verify`).

7. **Given** the bugfix template, **When** default config is used, **Then** verification is OFF by default (enable with `--verify`).

8. **Given** the feature template, **When** default config is used, **Then** verification is ON by default (disable with `--no-verify`).

9. **Given** verification model configuration, **When** `verify_model` is set in config, **Then** that model is used for verification (e.g., `gemini-3-pro`, `claude-haiku`).

## Tasks / Subtasks

- [ ] Task 1: Create verification executor (AC: 1, 2, 3, 6)
  - [ ] 1.1: Create `internal/template/steps/verify.go` with `VerifyExecutor` struct
  - [ ] 1.2: Define `VerifyConfig` struct with Model, Checks (code_correctness, test_coverage, garbage_files, security)
  - [ ] 1.3: Implement `Execute(ctx, step, task) (*StepResult, error)` method
  - [ ] 1.4: Register `StepTypeVerify` in domain/step_type.go
  - [ ] 1.5: Add verify step executor to step executor factory

- [ ] Task 2: Implement verification checks (AC: 2)
  - [ ] 2.1: Create `checkCodeCorrectness(ctx, taskDescription, changedFiles) ([]Issue, error)` - uses AI to review changes
  - [ ] 2.2: Create `checkTestCoverage(ctx, changedFiles) ([]Issue, error)` - verifies tests exist for changes
  - [ ] 2.3: Create `checkGarbageFiles(ctx, stagedFiles) ([]Issue, error)` - reuses GarbageScanner from internal/git
  - [ ] 2.4: Create `checkSecurityIssues(ctx, changedFiles) ([]Issue, error)` - basic pattern matching for common issues
  - [ ] 2.5: Define `VerificationIssue` struct with Severity (error, warning, info), Category, File, Line, Message

- [ ] Task 3: Create verification report artifact (AC: 3)
  - [ ] 3.1: Define `VerificationReport` struct with Summary, Issues, Recommendations, PassedChecks
  - [ ] 3.2: Implement `GenerateVerificationReport(results []CheckResult) *VerificationReport`
  - [ ] 3.3: Implement `SaveVerificationReport(report *VerificationReport, artifactDir string) (string, error)`
  - [ ] 3.4: Format report as markdown with sections for each check category

- [ ] Task 4: Implement issue handling menu (AC: 4)
  - [ ] 4.1: Define `VerificationAction` enum: AutoFix, ManualFix, IgnoreContinue, ViewReport
  - [ ] 4.2: Create `internal/tui/verification_menu.go` with `RenderVerificationMenu()` (placeholder for Epic 8)
  - [ ] 4.3: Implement `HandleVerificationIssues(ctx, report, action) (*VerificationResult, error)`
  - [ ] 4.4: For AutoFix: invoke AI with issue context to generate fixes
  - [ ] 4.5: For ManualFix: return awaiting_approval status with instructions
  - [ ] 4.6: For IgnoreContinue: log warning and proceed

- [ ] Task 5: Add CLI flags and template configuration (AC: 5, 6, 7, 8, 9)
  - [ ] 5.1: Add `--verify` flag to `atlas start` command (enables verification)
  - [ ] 5.2: Add `--no-verify` flag to `atlas start` command (disables verification)
  - [ ] 5.3: Add `verify: bool` field to template definition
  - [ ] 5.4: Add `verify_model: string` field to template definition
  - [ ] 5.5: Update bugfix template: `verify: false` (default OFF)
  - [ ] 5.6: Update feature template: `verify: true` (default ON)
  - [ ] 5.7: Add `verify_model` to config.yaml schema (default: use different model family)

- [ ] Task 6: Integrate with task engine (AC: 1, 3, 4)
  - [ ] 6.1: Add `StepTypeVerify` case to step executor switch in task engine
  - [ ] 6.2: Add verify step to bugfix template (optional, after implement, before validate)
  - [ ] 6.3: Add verify step to feature template (after implement, before validate)
  - [ ] 6.4: Ensure step is skipped when verification disabled
  - [ ] 6.5: Handle verify step failure with state transition to validation_failed

- [ ] Task 7: Create comprehensive tests (AC: 1-9)
  - [ ] 7.1: Test VerifyExecutor with successful verification
  - [ ] 7.2: Test VerifyExecutor with issues found
  - [ ] 7.3: Test each check function (code correctness, test coverage, garbage, security)
  - [ ] 7.4: Test report generation and saving
  - [ ] 7.5: Test --verify flag enables verification
  - [ ] 7.6: Test --no-verify flag disables verification
  - [ ] 7.7: Test template default override behavior
  - [ ] 7.8: Test verify_model configuration
  - [ ] 7.9: Test auto-fix action flow
  - [ ] 7.10: Target 85%+ coverage for new code

## Dev Notes

### Verification Executor Design

```go
// StepTypeVerify is the step type for AI verification.
// Add to internal/domain/step_type.go
const StepTypeVerify StepType = "verify"

// VerifyExecutor handles AI verification steps.
type VerifyExecutor struct {
    aiRunner       ai.Runner
    garbageScanner git.GarbageScanner
    logger         zerolog.Logger
}

// VerifyConfig configures the verification step.
type VerifyConfig struct {
    // Model to use for verification (different from implementation model).
    Model string `json:"model"`
    // Checks to run during verification.
    Checks []string `json:"checks"` // code_correctness, test_coverage, garbage_files, security
    // FailOnWarnings treats warnings as errors.
    FailOnWarnings bool `json:"fail_on_warnings"`
}

// VerificationIssue represents a single issue found during verification.
type VerificationIssue struct {
    Severity  string // "error", "warning", "info"
    Category  string // "code_correctness", "test_coverage", "garbage", "security"
    File      string
    Line      int
    Message   string
    Suggestion string
}

// VerificationReport is the complete verification output.
type VerificationReport struct {
    Summary      string
    TotalIssues  int
    ErrorCount   int
    WarningCount int
    InfoCount    int
    Issues       []VerificationIssue
    PassedChecks []string
    FailedChecks []string
    Timestamp    time.Time
}
```

### AI Verification Prompt Template

```go
const verificationPromptTemplate = `You are a code reviewer validating an implementation.

## Task Description
{{.TaskDescription}}

## Changed Files
{{range .ChangedFiles}}
### {{.Path}}
` + "```" + `{{.Language}}
{{.Content}}
` + "```" + `
{{end}}

## Verification Checklist
1. Does the implementation correctly address the task description?
2. Are there tests for the new/modified code?
3. Are there any obvious bugs or logic errors?
4. Are there any security concerns (hardcoded secrets, injection vulnerabilities)?
5. Is the code style consistent with the project?

## Response Format
Respond with a JSON object:
{
  "passed": true/false,
  "issues": [
    {
      "severity": "error|warning|info",
      "category": "code_correctness|test_coverage|security|style",
      "file": "path/to/file.go",
      "line": 42,
      "message": "Description of issue",
      "suggestion": "How to fix"
    }
  ],
  "summary": "Brief overall assessment"
}
`
```

### Template Configuration Updates

```go
// internal/template/bugfix.go - ADD verify step (optional)
{
    Name:        "verify",
    Type:        domain.StepTypeVerify,
    Description: "Optional AI verification of implementation",
    Required:    false, // Optional for bugfix
    Timeout:     5 * time.Minute,
    Config: map[string]any{
        "model":  "gemini-3-pro", // Different model for cross-validation
        "checks": []string{"code_correctness", "garbage_files"},
    },
},

// internal/template/feature.go - ADD verify step (required by default)
{
    Name:        "verify",
    Type:        domain.StepTypeVerify,
    Description: "AI verification of implementation",
    Required:    true, // Required for feature
    Timeout:     10 * time.Minute,
    Config: map[string]any{
        "model":  "gemini-3-pro",
        "checks": []string{"code_correctness", "test_coverage", "garbage_files", "security"},
    },
},
```

### CLI Flag Integration

```go
// internal/cli/start.go - ADD flags
var (
    verifyFlag   bool
    noVerifyFlag bool
)

func init() {
    startCmd.Flags().BoolVar(&verifyFlag, "verify", false, "Enable AI verification step")
    startCmd.Flags().BoolVar(&noVerifyFlag, "no-verify", false, "Disable AI verification step")
}

// In command execution:
if verifyFlag && noVerifyFlag {
    return fmt.Errorf("cannot use both --verify and --no-verify")
}
// Override template default
if verifyFlag {
    task.EnableVerification = true
}
if noVerifyFlag {
    task.EnableVerification = false
}
```

### Reuse GarbageScanner

```go
// Verification can reuse internal/git/garbage.go
func (e *VerifyExecutor) checkGarbageFiles(ctx context.Context, workDir string) ([]VerificationIssue, error) {
    result, err := e.garbageScanner.Scan(ctx, workDir)
    if err != nil {
        return nil, err
    }

    var issues []VerificationIssue
    for _, file := range result.GarbageFiles {
        issues = append(issues, VerificationIssue{
            Severity: "warning",
            Category: "garbage",
            File:     file.Path,
            Message:  fmt.Sprintf("Garbage file detected: %s (%s)", file.Path, file.Category),
            Suggestion: "Remove this file before committing",
        })
    }
    return issues, nil
}
```

### Security Check Patterns

```go
// Basic security pattern detection
var securityPatterns = []struct {
    Pattern     *regexp.Regexp
    Description string
    Severity    string
}{
    {regexp.MustCompile(`(?i)(password|secret|api[_-]?key)\s*[:=]\s*["'][^"']+["']`), "Hardcoded credential", "error"},
    {regexp.MustCompile(`(?i)exec\s*\(`), "Potential command injection", "warning"},
    {regexp.MustCompile(`(?i)sql.*\+.*input|input.*\+.*sql`), "Potential SQL injection", "warning"},
    {regexp.MustCompile(`(?i)dangerouslySetInnerHTML`), "XSS vulnerability", "warning"},
}

func (e *VerifyExecutor) checkSecurityIssues(ctx context.Context, files []ChangedFile) ([]VerificationIssue, error) {
    var issues []VerificationIssue
    for _, file := range files {
        for _, pattern := range securityPatterns {
            if matches := pattern.Pattern.FindAllStringIndex(file.Content, -1); len(matches) > 0 {
                for _, match := range matches {
                    line := countLines(file.Content[:match[0]])
                    issues = append(issues, VerificationIssue{
                        Severity:   pattern.Severity,
                        Category:   "security",
                        File:       file.Path,
                        Line:       line,
                        Message:    pattern.Description,
                        Suggestion: "Review and fix potential security issue",
                    })
                }
            }
        }
    }
    return issues, nil
}
```

### Validation Commands Required

**Before marking story complete, run ALL FOUR:**
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

### References

- [Source: epic-6-user-scenarios.md - Scenario 1 Step 6, Scenario 5 Step 13]
- [Source: epic-6-traceability-matrix.md - GAP 5]
- [Source: internal/git/garbage.go - Reuse for garbage detection]
- [Source: internal/template/steps/ai.go - AIExecutor pattern to follow]
- [Source: internal/ai/claude.go - ClaudeCodeRunner for AI invocation]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (Step 6 - optional verification)
- Scenario 5: Feature Workflow with Speckit SDD (Step 13 - AI Verification)

Specific validation checkpoints:
| Checkpoint | Expected Behavior | AC |
|------------|-------------------|-----|
| --verify flag | Enables verification | AC1 |
| Verification checks | 4 check types run | AC2 |
| Report artifact | verification-report.md saved | AC3 |
| Issues menu | 4 options presented | AC4 |
| --no-verify flag | Skips verification | AC5 |
| Template default | Respects verify: true/false | AC6, 7, 8 |
| verify_model config | Uses specified model | AC9 |
