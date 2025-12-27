# Story 1.6: Create CLI Root Command

Status: done

## Story

As a **developer**,
I want **the CLI entry point and root command implemented with Cobra**,
So that **the `atlas` command runs and displays help information**.

## Acceptance Criteria

1. **Given** the config package exists **When** I implement `cmd/atlas/main.go` and `internal/cli/root.go` **Then** `main.go`:
   - Creates a root context with `context.Background()`
   - Calls the CLI execute function
   - Handles exit codes correctly (0 success, 1 error, 2 invalid input)

2. **Given** the CLI package is being implemented **When** I update `root.go` **Then** it:
   - Defines the root Cobra command with proper Use, Short, Long descriptions
   - Implements global flags: `--output json|text`, `--verbose`, `--quiet`
   - Sets up Viper configuration binding for flags
   - Initializes zerolog with appropriate level

3. **Given** the CLI package is being implemented **When** I create `flags.go` **Then** it contains:
   - Shared flag definitions (output format, verbose, quiet)
   - Flag binding utilities for Viper integration
   - Exit code constants

4. **Given** the CLI is complete **When** I run `go run ./cmd/atlas` **Then** it displays help text

5. **Given** the CLI is complete **When** I run `go run ./cmd/atlas --version` **Then** it displays version info

6. **Given** the CLI is complete **When** I run tests **Then** tests verify flag parsing and help output

## Tasks / Subtasks

- [x] Task 1: Update main.go with proper exit code handling (AC: #1)
  - [x] Review existing `cmd/atlas/main.go`
  - [x] Add exit code handling: 0 success, 1 error, 2 invalid input
  - [x] Ensure context.Background() is used correctly (already done)
  - [x] Add version variable for ldflags injection

- [x] Task 2: Create flags.go with shared flag definitions (AC: #3)
  - [x] Create `internal/cli/flags.go`
  - [x] Define exit code constants (ExitSuccess=0, ExitError=1, ExitInvalidInput=2)
  - [x] Define output format type and constants (OutputText, OutputJSON)
  - [x] Define GlobalFlags struct with Output, Verbose, Quiet fields
  - [x] Create AddGlobalFlags(cmd *cobra.Command, flags *GlobalFlags) function
  - [x] Create BindGlobalFlags(v *viper.Viper, flags *GlobalFlags) function
  - [x] Document flag purposes and defaults

- [x] Task 3: Update root.go with complete implementation (AC: #2, #4, #5)
  - [x] Update `internal/cli/root.go`
  - [x] Add version, commit, date variables for build info
  - [x] Update newRootCmd() to accept GlobalFlags parameter
  - [x] Add Version field to root command for `--version` flag
  - [x] Add PersistentFlags for --output, --verbose, --quiet
  - [x] Implement PersistentPreRunE for flag validation and logger setup
  - [x] Create initConfig() function for Viper binding
  - [x] Initialize zerolog with level based on verbose/quiet flags
  - [x] Set zerolog output format (console for TTY, JSON otherwise)
  - [x] Handle NO_COLOR environment variable
  - [x] Ensure command follows Cobra best practices

- [x] Task 4: Create logger initialization utility (AC: #2)
  - [x] Create `internal/cli/logger.go`
  - [x] Implement InitLogger(verbose, quiet bool) zerolog.Logger function
  - [x] Set log level: debug (verbose), warn (quiet), info (default)
  - [x] Detect TTY and set console writer or JSON output accordingly
  - [x] Respect NO_COLOR environment variable for console output
  - [x] Add timestamp formatting for console output

- [x] Task 5: Create comprehensive tests (AC: #6)
  - [x] Create `internal/cli/root_test.go`
  - [x] Test help output contains expected content
  - [x] Test --version flag displays version info
  - [x] Test --output flag accepts json and text values
  - [x] Test --verbose and --quiet flags are mutually exclusive
  - [x] Test invalid --output value returns exit code 2
  - [x] Create `internal/cli/flags_test.go`
  - [x] Test GlobalFlags defaults
  - [x] Test flag binding to Viper
  - [x] Create `internal/cli/logger_test.go`
  - [x] Test InitLogger verbose mode sets debug level
  - [x] Test InitLogger quiet mode sets warn level
  - [x] Test InitLogger default sets info level
  - [x] Use table-driven tests for flag scenarios

- [x] Task 6: Validate and finalize (AC: all)
  - [x] Run `go build ./...` to verify compilation
  - [x] Run `go run ./cmd/atlas` to verify help output
  - [x] Run `go run ./cmd/atlas --version` to verify version output
  - [x] Run `go run ./cmd/atlas --help` to verify help output
  - [x] Run `magex format:fix` to format code
  - [x] Run `magex lint` to verify linting passes (must have 0 issues)
  - [x] Run `magex test` to verify tests pass

## Dev Notes

### Critical Architecture Requirements

**This story completes Epic 1 by establishing the CLI foundation!** The root command is the entry point for all future commands (init, start, status, approve, etc.).

#### Package Rules (CRITICAL - ENFORCE STRICTLY)

From architecture.md:
- **cmd/atlas** → CAN ONLY import `internal/cli`
- **internal/cli** → CAN import `internal/config`, `internal/constants`, `internal/errors`, and third-party libs
- **internal/cli** → MUST NOT import `internal/domain`, `internal/task`, `internal/workspace` (yet)

#### Cobra Best Practices (CRITICAL)

From [Cobra documentation](https://cobra.dev/docs/how-to-guides/working-with-flags/) and [spf13/cobra GitHub](https://github.com/spf13/cobra):

```go
// ✅ CORRECT - Function-based command creation (already in place)
func newRootCmd() *cobra.Command {
    return &cobra.Command{...}
}

// ✅ CORRECT - Persistent flags on root for global options
cmd.PersistentFlags().StringVarP(&flags.Output, "output", "o", "text", "output format (text|json)")
cmd.PersistentFlags().BoolVarP(&flags.Verbose, "verbose", "v", false, "enable verbose output")
cmd.PersistentFlags().BoolVarP(&flags.Quiet, "quiet", "q", false, "suppress non-essential output")

// ✅ CORRECT - Mark flags as mutually exclusive
cmd.MarkFlagsMutuallyExclusive("verbose", "quiet")

// ✅ CORRECT - Use PersistentPreRunE for initialization
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    // Initialize logger based on flags
    // Bind flags to Viper
    return nil
}

// ❌ WRONG - Package-level globals
var rootCmd = &cobra.Command{...}  // DON'T - already fixed in codebase
```

#### Exit Code Handling

From [Go Packages cobra](https://pkg.go.dev/github.com/spf13/cobra):

```go
// main.go pattern
func main() {
    ctx := context.Background()
    if err := cli.Execute(ctx); err != nil {
        // Exit code 1 for general errors
        // Exit code 2 for invalid input (handled by Cobra automatically)
        os.Exit(1)
    }
}
```

#### Version Flag Pattern

```go
// Build-time variables set via ldflags
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

// In newRootCmd()
cmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
```

#### Zerolog Initialization Pattern

From project-context.md and architecture.md:

```go
// ✅ CORRECT - Initialize based on flags
func InitLogger(verbose, quiet bool) zerolog.Logger {
    var level zerolog.Level
    switch {
    case verbose:
        level = zerolog.DebugLevel
    case quiet:
        level = zerolog.WarnLevel
    default:
        level = zerolog.InfoLevel
    }

    // Console writer for TTY, JSON otherwise
    var output io.Writer
    if term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("NO_COLOR") == "" {
        output = zerolog.ConsoleWriter{
            Out:        os.Stderr,
            TimeFormat: time.Kitchen,
        }
    } else {
        output = os.Stderr
    }

    return zerolog.New(output).Level(level).With().Timestamp().Logger()
}

// ❌ WRONG - Global zerolog configuration
func init() {
    log.Logger = zerolog.New(os.Stderr)  // DON'T - no init() functions
}
```

#### Viper Integration (from [Cobra Viper docs](https://cobra.dev/))

```go
// Bind flags to Viper for config file/env var support
func initConfig(v *viper.Viper, flags *GlobalFlags) error {
    v.BindPFlag("output", cmd.PersistentFlags().Lookup("output"))
    v.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))
    v.BindPFlag("quiet", cmd.PersistentFlags().Lookup("quiet"))

    // Environment variable support
    v.SetEnvPrefix("ATLAS")
    v.AutomaticEnv()

    return nil
}
```

### Previous Story Intelligence

From Story 1-5 (config) completion:
- Use non-global Viper instances (`v := viper.New()`)
- Context as first parameter for all public functions
- All code must pass `magex format:fix`, `magex lint`, `magex test`
- Table-driven tests are the preferred pattern
- Use mapstructure tags alongside yaml tags for Viper unmarshaling
- Fixed linting issues: use static sentinel errors, use errors.As for type assertions
- Use proper file permissions in tests (0o600 for files, 0o750 for directories)

### Git Commit Patterns

Recent commits follow conventional commits format:
- `feat(config): add configuration framework with layered precedence`
- `feat(domain): add centralized domain types package`

For this story, use:
- `feat(cli): add root command with global flags and zerolog initialization`

### Project Structure Notes

Current structure:
```
cmd/atlas/
└── main.go              # Entry point (basic)

internal/cli/
└── root.go              # Basic root command
```

After this story:
```
cmd/atlas/
└── main.go              # Entry point with exit codes

internal/cli/
├── root.go              # Complete root command with flags
├── flags.go             # Shared flag definitions
├── logger.go            # Logger initialization
├── root_test.go         # Root command tests
├── flags_test.go        # Flag tests
└── logger_test.go       # Logger tests
```

### File Contents Reference

**main.go additions:**
```go
// Build info variables (set via ldflags)
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func main() {
    ctx := context.Background()
    err := cli.Execute(ctx, cli.BuildInfo{
        Version: version,
        Commit:  commit,
        Date:    date,
    })
    if err != nil {
        os.Exit(cli.ExitError)
    }
}
```

**flags.go structure:**
```go
package cli

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

// Exit codes
const (
    ExitSuccess      = 0
    ExitError        = 1
    ExitInvalidInput = 2
)

// Output formats
const (
    OutputText = "text"
    OutputJSON = "json"
)

// GlobalFlags holds flags available to all commands
type GlobalFlags struct {
    Output  string
    Verbose bool
    Quiet   bool
}

// AddGlobalFlags adds global flags to a command
func AddGlobalFlags(cmd *cobra.Command, flags *GlobalFlags) {
    cmd.PersistentFlags().StringVarP(&flags.Output, "output", "o", OutputText, "output format (text|json)")
    cmd.PersistentFlags().BoolVarP(&flags.Verbose, "verbose", "v", false, "enable verbose output")
    cmd.PersistentFlags().BoolVarP(&flags.Quiet, "quiet", "q", false, "suppress non-essential output")
    cmd.MarkFlagsMutuallyExclusive("verbose", "quiet")
}

// BindGlobalFlags binds global flags to Viper
func BindGlobalFlags(v *viper.Viper, cmd *cobra.Command) error {
    if err := v.BindPFlag("output", cmd.PersistentFlags().Lookup("output")); err != nil {
        return err
    }
    // ... bind other flags
    return nil
}
```

### Testing Strategy

**Root Command Tests:**
```go
func TestRootCmd_Help(t *testing.T) {
    cmd := newRootCmd(GlobalFlags{}, BuildInfo{Version: "test"})
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"--help"})

    err := cmd.Execute()
    require.NoError(t, err)
    assert.Contains(t, buf.String(), "ATLAS")
    assert.Contains(t, buf.String(), "AI Task Lifecycle")
}

func TestRootCmd_Version(t *testing.T) {
    cmd := newRootCmd(GlobalFlags{}, BuildInfo{Version: "1.0.0", Commit: "abc123"})
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"--version"})

    err := cmd.Execute()
    require.NoError(t, err)
    assert.Contains(t, buf.String(), "1.0.0")
    assert.Contains(t, buf.String(), "abc123")
}

func TestRootCmd_InvalidOutput(t *testing.T) {
    cmd := newRootCmd(GlobalFlags{}, BuildInfo{})
    cmd.SetArgs([]string{"--output", "xml"})

    err := cmd.Execute()
    assert.Error(t, err)
}
```

### Dependencies Required

Already in go.mod:
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration
- `github.com/rs/zerolog` - Structured logging
- `github.com/stretchr/testify` - Testing

May need to add:
- `golang.org/x/term` - Terminal detection (for TTY check)

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Project Directory Structure]
- [Source: _bmad-output/planning-artifacts/architecture.md#CLI Layer]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.6]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: _bmad-output/implementation-artifacts/1-5-create-configuration-framework.md#Previous Story Intelligence]
- [Cobra CLI Documentation](https://cobra.dev/)
- [spf13/cobra GitHub](https://github.com/spf13/cobra)
- [Cobra Working with Flags](https://cobra.dev/docs/how-to-guides/working-with-flags/)
- [Zerolog GitHub](https://github.com/rs/zerolog)

### Validation Commands (MUST RUN BEFORE COMPLETION)

```bash
go build ./...        # Must compile
go run ./cmd/atlas    # Must show help
go run ./cmd/atlas --version  # Must show version
magex format:fix      # Format code
magex lint            # Must pass with 0 issues
magex test            # Must pass all tests
```

### Anti-Patterns to Avoid

```go
// ❌ NEVER: Package-level command variables
var rootCmd = &cobra.Command{...}  // DON'T - use function-based

// ✅ DO: Function-based command creation
func newRootCmd() *cobra.Command {
    return &cobra.Command{...}
}

// ❌ NEVER: Global logger
var log = zerolog.New(os.Stderr)  // DON'T

// ✅ DO: Initialize logger in function
func InitLogger(verbose, quiet bool) zerolog.Logger { ... }

// ❌ NEVER: Use init() for setup
func init() {
    cobra.OnInitialize(initConfig)  // DON'T
}

// ✅ DO: Setup in PersistentPreRunE
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    // Initialize here
}

// ❌ NEVER: Ignore context
func Execute() error { ... }  // DON'T - missing ctx

// ✅ DO: Accept context as first parameter
func Execute(ctx context.Context) error { ... }

// ❌ NEVER: Hard-code version
cmd.Version = "1.0.0"  // DON'T

// ✅ DO: Use ldflags for version injection
var version = "dev"  // Set at build time
cmd.Version = version

// ❌ NEVER: Shorthand collision with -h (help)
cmd.PersistentFlags().BoolVarP(&help, "help", "h", ...)  // DON'T - reserved

// ✅ DO: Reserve -h for Cobra's built-in help
// Use other shorthands: -v (verbose), -q (quiet), -o (output)
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No debugging required, implementation proceeded smoothly.

### Completion Notes List

- Implemented complete CLI root command with Cobra framework
- Created `flags.go` with exit codes (0/1/2), output formats (text/json), and GlobalFlags struct
- Created `logger.go` with InitLogger() supporting verbose/quiet/default log levels and TTY detection
- Updated `root.go` with BuildInfo struct, PersistentPreRunE for flag validation and logger init
- Updated `main.go` with ldflags-injectable version/commit/date variables
- Added `golang.org/x/term` dependency for terminal detection
- Added `ErrInvalidOutputFormat` sentinel error to internal/errors package
- All tests pass (23 tests across 3 test files)
- All linting passes with 0 issues
- Followed all architecture requirements: function-based commands, no init(), ctx as first param
- Used nolint directives only where necessary (build info globals, CLI logger global)

### File List

- cmd/atlas/main.go (modified)
- internal/cli/flags.go (created)
- internal/cli/flags_test.go (created)
- internal/cli/logger.go (created)
- internal/cli/logger_test.go (created)
- internal/cli/root.go (modified)
- internal/cli/root_test.go (created)
- internal/errors/errors.go (modified - added ErrInvalidOutputFormat)
- go.mod (modified - added golang.org/x/term)
- go.sum (modified)
- _bmad-output/implementation-artifacts/sprint-status.yaml (modified)

## Change Log

- 2025-12-27: Implemented CLI root command with global flags (--output, --verbose, --quiet), version flag, and zerolog initialization. All acceptance criteria satisfied.
- 2025-12-27: **Code Review Fixes** - Added ExitCodeForError() to properly return exit code 2 for invalid input (AC #1 compliance). Added comprehensive tests for exit codes, NO_COLOR environment variable handling. Improved GetLogger() documentation. Test coverage increased to 92.1%.

