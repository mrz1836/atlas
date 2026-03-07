# Atlas Code Quality Prompts

Atlas provides 7 AI-driven code quality analysis prompts that integrate with the existing prompt system. Each prompt accepts structured Go input and produces CI-friendly JSON output.

## Overview

Quality prompts analyze source code files and detect specific improvement opportunities. They follow the same architecture as all Atlas prompts: Go templates with typed data structures, producing structured JSON output.

**When to use quality prompts:**
- During code review to catch common patterns
- In CI pipelines to enforce quality standards
- As part of `atlas start` automated analysis workflow
- For periodic codebase health checks

## Data Structures

### Input: QualityAnalysisData

```go
type QualityAnalysisData struct {
    Files          []SourceFile // Source files to analyze
    GoVersion      string       // Target Go version (e.g., "1.24")
    ProjectContext string       // Optional project context
}

type SourceFile struct {
    Path     string // File path relative to project root
    Content  string // Full file content
    Language string // File language (e.g., "go", "yaml")
}
```

### Output: QualityIssue (JSON)

All quality prompts return a JSON array of `QualityIssue` objects:

```go
type QualityIssue struct {
    Severity   string `json:"severity"`   // "critical", "warning", "suggestion"
    File       string `json:"file"`
    Line       int    `json:"line"`
    Message    string `json:"message"`
    Suggestion string `json:"suggestion"`
    Category   string `json:"category"`
}
```

## Integration

Call any quality prompt using `prompts.Render()`:

```go
import "github.com/mrz1836/atlas/internal/prompts"

data := prompts.QualityAnalysisData{
    Files: []prompts.SourceFile{
        {
            Path:     "cmd/server/main.go",
            Content:  string(fileBytes),
            Language: "go",
        },
    },
    GoVersion:      "1.24",
    ProjectContext: "HTTP API server for payment processing",
}

prompt, err := prompts.Render(prompts.Deduplication, data)
if err != nil {
    return err
}
// Send prompt to your LLM; parse JSON response as []QualityIssue
```

## Quality Prompts

### 1. Deduplication Detector

**PromptID:** `prompts.Deduplication` → `"quality/dedup"`
**Template:** `internal/prompts/templates/quality/dedup.tmpl`

Detects duplicated code that should be extracted into shared helpers or generics.

**Detects:**
- Exact duplicate code blocks (3+ lines)
- Near-duplicate functions with minor variations
- Copy-pasted logic that should be extracted to a helper
- Repeated patterns that could use generics (Go 1.18+)

**Output example:**
```json
[
  {
    "severity": "warning",
    "file": "pkg/auth/handler.go",
    "line": 42,
    "message": "Duplicate validation logic also found in pkg/user/handler.go:87",
    "suggestion": "Extract to shared validateRequest() helper in pkg/shared/",
    "category": "duplication"
  }
]
```

---

### 2. Goroutine Leak Detector

**PromptID:** `prompts.GoroutineLeak` → `"quality/goroutine_leak"`
**Template:** `internal/prompts/templates/quality/goroutine_leak.tmpl`

Finds goroutines that may leak due to missing cancellation, missing context, or blocked channels.

**Detects:**
- Goroutines without cancellation context
- Unbuffered channel sends without receivers
- Missing `defer cancel()` after `context.WithCancel`
- Blocking operations without timeout
- Goroutines in loops without proper cleanup
- Missing `sync.WaitGroup` coordination

**Severity levels:**
- `critical` — Definite leak (blocking forever)
- `warning` — Probable leak (missing context/timeout)
- `suggestion` — Could leak under certain conditions

**Output example:**
```json
[
  {
    "severity": "critical",
    "file": "internal/worker/pool.go",
    "line": 78,
    "message": "Goroutine started without context; no cancellation path",
    "suggestion": "Pass ctx to goroutine and select on ctx.Done()",
    "category": "goroutine-leak"
  }
]
```

---

### 3. Junior-to-Senior Pattern Detector

**PromptID:** `prompts.JrToSr` → `"quality/jr_to_sr"`
**Template:** `internal/prompts/templates/quality/jr_to_sr.tmpl`

Identifies common junior developer patterns in Go code and suggests senior-level alternatives.

**Detects:**
- Unnecessary else after return
- Empty `interface{}` where generics would work
- Error handling that loses context (bare returns, `fmt.Errorf` without `%w`)
- Mutable globals instead of dependency injection
- Manual string building instead of `strings.Builder`
- Slice capacity not pre-allocated when size is known
- `interface{}` maps instead of typed structs
- Not using table-driven tests
- Mixing business logic with I/O

**Output example:**
```json
[
  {
    "severity": "warning",
    "file": "internal/api/handler.go",
    "line": 33,
    "message": "Unnecessary else after return",
    "suggestion": "Remove else block; early return makes it redundant",
    "category": "jr-to-sr"
  }
]
```

---

### 4. Constant Hunter

**PromptID:** `prompts.ConstantHunter` → `"quality/constant_hunter"`
**Template:** `internal/prompts/templates/quality/constant_hunter.tmpl`

Finds magic values (hardcoded strings and numbers) that should be named constants.

**Detects:**
- Hardcoded strings (error messages, keys, URLs)
- Hardcoded numbers (timeouts, sizes, limits)
- Repeated literal values across files
- Status codes without named constants
- Format strings that appear multiple times

**Excludes:** Test assertions, obvious 0/1/-1 uses, single-use log strings.

**Output example:**
```json
[
  {
    "severity": "warning",
    "file": "internal/cache/redis.go",
    "line": 15,
    "message": "Magic number 3600 (1 hour timeout) appears 4 times",
    "suggestion": "const defaultCacheTTL = 3600 * time.Second",
    "category": "constant"
  }
]
```

---

### 5. Config Hunter

**PromptID:** `prompts.ConfigHunter` → `"quality/config_hunter"`
**Template:** `internal/prompts/templates/quality/config_hunter.tmpl`

Detects scattered configuration values that should be centralized in a config struct.

**Detects:**
- `os.Getenv()` calls scattered across packages
- Hardcoded defaults that should be configurable
- Environment variables without documentation
- Missing validation for config values
- Config values duplicated across files
- No centralized config struct

**Optional:** Pass `ProjectContext` for domain-specific config detection (e.g., "payment processor" flags sensitive credential handling).

**Output example:**
```json
[
  {
    "severity": "warning",
    "file": "internal/db/connect.go",
    "line": 12,
    "message": "os.Getenv(\"DATABASE_URL\") called directly; no validation or default",
    "suggestion": "Centralize in Config struct with validation in config.go",
    "category": "config"
  }
]
```

---

### 6. Go Optimizer

**PromptID:** `prompts.GoOptimize` → `"quality/go_optimize"`
**Template:** `internal/prompts/templates/quality/go_optimize.tmpl`

Finds opportunities to modernize code using newer Go features based on target version.

**Set `GoVersion` in `QualityAnalysisData` to control which features are suggested.**

**Detects by version:**
- **Go 1.21+:** `maps.Clone()`, `slices.Clone()`, `clear()`, `min()`/`max()` builtins
- **Go 1.22+:** `for i := range N`, loop variable scoping (no more `i := i`)
- **Go 1.23+:** iterators (`iter.Seq`), range over functions
- **Go 1.24+:** generic type aliases, `testing.B.Loop()`

**Output example:**
```json
[
  {
    "severity": "suggestion",
    "file": "internal/util/slice.go",
    "line": 8,
    "message": "Manual slice copy loop can use slices.Clone() (Go 1.21+)",
    "suggestion": "import \"slices\"\nresult := slices.Clone(input)",
    "category": "optimize"
  }
]
```

---

### 7. Test Creator

**PromptID:** `prompts.TestCreator` → `"quality/test_creator"`
**Template:** `internal/prompts/templates/quality/test_creator.tmpl`

Analyzes code and suggests intelligent tests for uncovered scenarios.

**Suggests tests for:**
- Functions without any test coverage
- Error paths not exercised by existing tests
- Edge cases (nil input, empty slices, zero values)
- Boundary conditions (off-by-one, max values)
- Concurrent access scenarios
- Table-driven test opportunities

**Output example:**
```json
[
  {
    "severity": "suggestion",
    "file": "internal/parser/parse_test.go",
    "line": 0,
    "message": "test needed for ParseConfig error path (missing required field)",
    "suggestion": "func TestParseConfig_MissingField(t *testing.T) {\n  _, err := ParseConfig([]byte(`{}`))\n  require.Error(t, err)\n  assert.Contains(t, err.Error(), \"required\")\n}",
    "category": "test"
  }
]
```

## Output Format Reference

All quality prompts return an empty array `[]` when no issues are found.

| Field | Type | Values |
|-------|------|--------|
| `severity` | string | `"critical"`, `"warning"`, `"suggestion"` |
| `file` | string | File path relative to project root |
| `line` | int | Line number (0 if not applicable) |
| `message` | string | Human-readable description of the issue |
| `suggestion` | string | Concrete fix or refactoring suggestion |
| `category` | string | `"duplication"`, `"goroutine-leak"`, `"jr-to-sr"`, `"constant"`, `"config"`, `"optimize"`, `"test"` |
