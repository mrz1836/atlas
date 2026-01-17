# Story 2.2: Implement `atlas init` Setup Wizard

Status: done

## Story

As a **user**,
I want **to run `atlas init` and complete a guided setup wizard**,
So that **ATLAS is configured correctly for my environment**.

## Acceptance Criteria

1. **Given** tool detection is implemented **When** I run `atlas init` **Then** the wizard displays the ATLAS header with branding

2. **Given** the wizard is running **When** tools are detected **Then** the system runs tool detection and displays a status table showing all 8 tools with their status (installed/missing/outdated)

3. **Given** required tools are missing **When** the status table is shown **Then** the system displays an error with install instructions and exits with code 1

4. **Given** managed tools are missing/outdated **When** required tools pass **Then** the wizard prompts: "Install/upgrade ATLAS-managed tools? [Y/n]"

5. **Given** the wizard proceeds **When** reaching the AI provider step **Then** the system prompts for model selection and API key environment variable name

6. **Given** the wizard proceeds **When** reaching the validation commands step **Then** the system suggests defaults based on detected tools

7. **Given** the wizard proceeds **When** reaching the notification preferences step **Then** the system prompts for terminal bell and event configuration

8. **Given** all steps complete **When** configuration is saved **Then** the wizard writes to `~/.atlas/config.yaml` and displays success message with suggested next command

9. **Given** the `--no-interactive` flag is passed **When** running `atlas init --no-interactive` **Then** the wizard uses sensible defaults without prompts

10. **Given** the wizard is running **When** the Charm Huh library is used **Then** all forms use the established Charm styling and are keyboard-navigable

## Tasks / Subtasks

- [x] Task 1: Create the init command structure (AC: #1, #10)
  - [x] 1.1: Create `internal/cli/init.go` with Cobra command structure
  - [x] 1.2: Implement ATLAS header display using Lip Gloss styling
  - [x] 1.3: Register init command with root command
  - [x] 1.4: Add `--no-interactive` flag

- [x] Task 2: Implement tool detection display (AC: #2, #3)
  - [x] 2.1: Call `NewToolDetector().Detect(ctx)` from story 2-1
  - [x] 2.2: Create styled status table component using Lip Gloss
  - [x] 2.3: Display tool name, required/managed, current version, min version, status
  - [x] 2.4: If required tools missing/outdated, display error with install hints and `os.Exit(1)`

- [x] Task 3: Implement managed tools prompt (AC: #4)
  - [x] 3.1: Check if any managed tools (mage-x, go-pre-commit, Speckit) are missing/outdated
  - [x] 3.2: Use Charm Huh `Confirm` component for install prompt
  - [x] 3.3: If user declines, continue without installing (managed tools are optional)
  - [x] 3.4: If user confirms, run `go install` commands for each missing managed tool

- [x] Task 4: Implement AI provider configuration step (AC: #5)
  - [x] 4.1: Create form for default model selection (Select: "sonnet" | "opus")
  - [x] 4.2: Create input for API key env var name (default: ANTHROPIC_API_KEY)
  - [x] 4.3: Validate API key exists in environment (warn if missing, don't block)
  - [x] 4.4: Create input for default timeout (default: 30m)
  - [x] 4.5: Create input for max turns per step (default: 10)

- [x] Task 5: Implement validation commands step (AC: #6)
  - [x] 5.1: Auto-detect installed tools and suggest defaults:
    - If mage-x: `magex format:fix`, `magex lint`, `magex test`
    - If go-pre-commit: add `go-pre-commit run --all-files`
  - [x] 5.2: Create multi-line text input for command customization
  - [x] 5.3: Allow adding custom pre-PR hooks

- [x] Task 6: Implement notification preferences step (AC: #7)
  - [x] 6.1: Create toggle for terminal bell (default: enabled)
  - [x] 6.2: Create multi-select for notification events:
    - Task awaiting approval
    - Validation failed
    - CI failed
    - GitHub operation failed

- [x] Task 7: Save configuration and display success (AC: #8)
  - [x] 7.1: Create config struct from wizard values
  - [x] 7.2: Create `~/.atlas/` directory if not exists
  - [x] 7.3: Write config to `~/.atlas/config.yaml` using YAML marshaling
  - [x] 7.4: Display success message with suggested next commands

- [x] Task 8: Implement non-interactive mode (AC: #9)
  - [x] 8.1: If `--no-interactive`, skip all prompts
  - [x] 8.2: Use sensible defaults for all configuration
  - [x] 8.3: Still run tool detection and display results
  - [x] 8.4: Still fail if required tools missing

- [x] Task 9: Write comprehensive tests (AC: all)
  - [x] 9.1: Test tool detection display formatting
  - [x] 9.2: Test config file creation and content
  - [x] 9.3: Test non-interactive mode defaults
  - [x] 9.4: Test error handling for missing required tools
  - [x] 9.5: Run `magex test:race` to verify no race conditions

## Dev Notes

### Technical Requirements

**Package Location:** `internal/cli/init.go` (and `init_test.go`)

**Dependencies:**
- `internal/config` - Tool detection from story 2-1, config loading/saving
- `internal/constants` - Shared constants
- `internal/errors` - Error handling
- `github.com/charmbracelet/huh` - Interactive forms
- `github.com/charmbracelet/lipgloss` - Terminal styling

**Import Rules (CRITICAL):**
- `internal/cli/init.go` MAY import: `internal/config`, `internal/constants`, `internal/errors`
- `internal/cli/init.go` MUST NOT import: `internal/domain`, `internal/task`, `internal/workspace`

### Charm Huh Usage Patterns

**Form Structure:**
```go
import "github.com/charmbracelet/huh"

// Create a form with groups (pages)
form := huh.NewForm(
    huh.NewGroup(
        huh.NewSelect[string]().
            Title("Default AI Model").
            Options(
                huh.NewOption("Claude Sonnet (faster)", "sonnet"),
                huh.NewOption("Claude Opus (more capable)", "opus"),
            ).
            Value(&model),
        huh.NewInput().
            Title("API Key Environment Variable").
            Value(&apiKeyEnv).
            Placeholder("ANTHROPIC_API_KEY"),
    ),
    huh.NewGroup(
        huh.NewConfirm().
            Title("Enable terminal bell notifications?").
            Value(&bellEnabled),
    ),
)

// Run the form
err := form.Run()
```

**Theming:**
```go
// Use default Charm theme or create custom
form := huh.NewForm(groups...).
    WithTheme(huh.ThemeCharm())
```

**Accessibility Mode:**
```go
// For screen reader support and non-TTY environments
form := huh.NewForm(groups...).
    WithAccessible(true)
```

### ATLAS Header Component

**Header Display Pattern:**
```go
import "github.com/charmbracelet/lipgloss"

var headerStyle = lipgloss.NewStyle().
    Bold(true).
    Foreground(lipgloss.Color("#00D7FF")).
    MarginBottom(1)

func displayHeader() {
    // For terminals >= 80 columns
    header := `
    ╔═══════════════════════════════════════╗
    ║             A T L A S                 ║
    ║     Autonomous Task & Lifecycle       ║
    ║      Automation System                ║
    ╚═══════════════════════════════════════╝`

    // For narrow terminals
    narrowHeader := "═══ ATLAS ═══"

    width := lipgloss.Width(header)
    if termWidth < 80 {
        fmt.Println(headerStyle.Render(narrowHeader))
    } else {
        fmt.Println(headerStyle.Render(header))
    }
}
```

### Tool Status Table

**Table Display Pattern (from UX Design):**
```go
var (
    installedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
    missingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
    outdatedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700"))
)

func formatToolStatus(tool config.Tool) string {
    icon := "●"
    style := installedStyle

    switch tool.Status {
    case config.ToolStatusInstalled:
        icon = "✓"
        style = installedStyle
    case config.ToolStatusMissing:
        icon = "✗"
        style = missingStyle
    case config.ToolStatusOutdated:
        icon = "⚠"
        style = outdatedStyle
    }

    return style.Render(fmt.Sprintf("%s %s", icon, tool.Status.String()))
}
```

**Table Structure:**
```
TOOL            REQUIRED   VERSION    STATUS
──────────────────────────────────────────────
go              yes        1.24.2     ✓ installed
git             yes        2.39.0     ✓ installed
gh              yes        2.62.0     ✓ installed
uv              yes        0.5.14     ✓ installed
claude          yes        2.0.76     ✓ installed
magex           managed    1.0.0      ✓ installed
go-pre-commit   managed    -          ✗ missing
speckit         managed    -          ✗ missing
```

### Config File Structure

**~/.atlas/config.yaml:**
```yaml
# ATLAS Configuration
# Generated by atlas init

ai:
  default_model: sonnet
  api_key_env: ANTHROPIC_API_KEY
  default_timeout: 30m
  max_turns: 10

validation:
  commands:
    format:
      - magex format:fix
    lint:
      - magex lint
    test:
      - magex test
    pre_commit:
      - go-pre-commit run --all-files

notifications:
  bell_enabled: true
  events:
    - awaiting_approval
    - validation_failed
    - ci_failed
    - github_failed
```

### Architecture Compliance

**Context-First Design (ARCH-13):**
```go
func runInit(ctx context.Context, flags InitFlags) error {
    // Check cancellation at entry
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Use ctx for tool detection
    result, err := config.NewToolDetector().Detect(ctx)
    if err != nil {
        return errors.Wrap(err, "failed to detect tools")
    }
    // ...
}
```

**Error Wrapping (ARCH-14):**
```go
// Wrap at package boundary only
if err != nil {
    return errors.Wrap(err, "failed to create config directory")
}
```

**Thread Safety:**
- No concurrent operations in init wizard
- Forms run synchronously
- Config file write uses atomic write pattern from architecture

### File Structure

```
internal/
├── cli/
│   ├── root.go          # Existing
│   ├── init.go          # NEW: Init command implementation
│   ├── init_test.go     # NEW: Init command tests
│   └── flags.go         # Existing
└── config/
    ├── tools.go         # Existing from 2-1
    └── tools_test.go    # Existing from 2-1
```

### Non-Interactive Mode Defaults

| Setting | Default Value |
|---------|---------------|
| AI Model | sonnet |
| API Key Env | ANTHROPIC_API_KEY |
| Timeout | 30m |
| Max Turns | 10 |
| Validation Commands | Auto-detected based on tools |
| Bell Enabled | true |
| Notification Events | All enabled |
| Install Managed Tools | false (skip) |

### Testing Patterns (from Epic 1)

**Mock Form Testing:**
```go
// For testing, skip interactive forms
type InitConfig struct {
    Interactive bool
    Model       string
    APIKeyEnv   string
    // ...
}

func TestInit_NonInteractive(t *testing.T) {
    cfg := InitConfig{
        Interactive: false,
    }

    err := runInitWithConfig(context.Background(), cfg)
    require.NoError(t, err)

    // Verify config file was created with defaults
    // ...
}
```

**Temp Directory for Config:**
```go
func TestInit_CreatesConfigFile(t *testing.T) {
    tmpDir := t.TempDir()
    os.Setenv("HOME", tmpDir)
    defer os.Unsetenv("HOME")

    // Run init...

    configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
    assert.FileExists(t, configPath)
}
```

### Previous Story Intelligence (Story 2-1 Learnings)

**CRITICAL: Race Condition Prevention**
- Story 2-1 established `sync.Mutex` pattern for shared state
- Init wizard is single-threaded, but tool detection uses goroutines
- Tool detection already handles concurrency correctly

**Patterns Established in 2-1:**
- `CommandExecutor` interface for testable subprocess calls
- `ToolDetector` interface for mockable tool detection
- `ToolStatus` type with JSON marshaling
- Table-driven tests with comprehensive scenarios

**Code to Reuse:**
- `config.NewToolDetector().Detect(ctx)` - call directly
- `config.FormatMissingToolsError(missing)` - use for error messages
- `config.ToolStatus` styling patterns

### Project Structure Notes

- Init command extends CLI layer with new subcommand
- Follows Cobra command patterns from root.go
- Config file writing uses Viper YAML marshaling
- No new packages needed - extends existing structure

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 2.2]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns]
- [Source: _bmad-output/planning-artifacts/architecture.md#External Tool Integration]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: _bmad-output/implementation-artifacts/2-1-implement-tool-detection-system.md]
- [Source: _bmad-output/implementation-artifacts/epic-1-retro-2025-12-27.md#Race Condition Finding]
- [Source: https://github.com/charmbracelet/huh - Charm Huh Library]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix    # Format code
magex lint          # Lint code (must pass)
magex test:race     # Run tests WITH race detection (CRITICAL)
go build ./...      # Verify compilation
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

- Implemented complete `atlas init` setup wizard with interactive and non-interactive modes
- Created styled ATLAS header banner using Lip Gloss
- Integrated tool detection from story 2-1 with formatted status table display
- Implemented managed tools installation prompt with `go install` commands
- Created AI provider configuration form (model, API key env var, timeout, max turns)
- Created validation commands configuration with auto-detection of installed tools
- Created notification preferences form (bell, events)
- Implemented config file saving to `~/.atlas/config.yaml` with YAML marshaling
- Non-interactive mode uses sensible defaults and still displays tool detection results
- Fixed `BindGlobalFlags` to use `cmd.Root().PersistentFlags()` for proper subcommand flag resolution
- Added nolint directive for contextcheck in Cobra command pattern
- All tests pass with race detection enabled
- All linting passes

**Code Review Fixes (2025-12-27):**
- Refactored `runInit` to return error instead of calling `os.Exit` (testability improvement)
- Added `ToolDetector` interface for mockable tool detection in tests
- Added comprehensive tests for `runInitWithDetector` covering success, missing tools, detection errors
- Added timeout format validation with warning and fallback to default
- Moved managed tool install paths to `internal/constants/tools.go`
- Added config file backup before overwriting existing config
- Test coverage improved from 58.8% to 67.1%; `runInit` now at 100% coverage

### Change Log

- 2025-12-27: Implemented `atlas init` command with full setup wizard functionality
- 2025-12-27: Code review fixes - improved testability, validation, and coverage

### File List

- internal/cli/init.go (NEW)
- internal/cli/init_test.go (NEW)
- internal/cli/root.go (MODIFIED - added init command registration, nolint for contextcheck)
- internal/cli/flags.go (MODIFIED - fixed BindGlobalFlags to use root command's flags)
- internal/constants/tools.go (MODIFIED - added managed tool install paths)

