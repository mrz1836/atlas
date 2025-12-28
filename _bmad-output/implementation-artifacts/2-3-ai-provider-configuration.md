# Story 2.3: AI Provider Configuration

Status: done

## Story

As a **user**,
I want **to configure my AI provider settings during init**,
So that **ATLAS knows which AI model to use and how to authenticate**.

## Acceptance Criteria

1. **Given** the init wizard is running **When** I reach the AI provider configuration step **Then** I can select the default model (sonnet or opus)

2. **Given** the AI config step is active **When** I specify API key environment variable name **Then** the default is ANTHROPIC_API_KEY

3. **Given** an API key environment variable name is configured **When** the system validates it **Then** the system checks if the API key exists in the environment

4. **Given** the API key is missing from the environment **When** validation runs **Then** the system displays a warning but allows continuing (does not block)

5. **Given** the AI config step is active **When** I configure timeout **Then** the default is 30m and the format is validated

6. **Given** the AI config step is active **When** I configure max turns per step **Then** the default is 10 and the range is validated (1-100)

7. **Given** all AI settings are configured **When** saving configuration **Then** settings are saved to config under `ai:` section

8. **Given** API key configuration **When** saving to file **Then** API keys are NEVER written to config files (only env var references)

## Tasks / Subtasks

- [x] Task 1: Refactor AI configuration out of init.go (AC: #1, #2)
  - [x] 1.1: Create `internal/cli/ai_config.go` for AI configuration step
  - [x] 1.2: Extract AI configuration form logic from `init.go` into reusable functions
  - [x] 1.3: Define `AIProviderConfig` struct for AI configuration collection
  - [x] 1.4: Create `NewAIConfigForm()` function using Charm Huh

- [x] Task 2: Implement model selection (AC: #1)
  - [x] 2.1: Create `huh.Select` for model choice with "sonnet" and "opus" options
  - [x] 2.2: Add "haiku" option for cost-conscious users (maps to `claude-3-5-haiku-latest`)
  - [x] 2.3: Ensure model selection has descriptive labels (e.g., "Claude Sonnet (faster, balanced)")

- [x] Task 3: Implement API key environment variable configuration (AC: #2, #3, #4, #8)
  - [x] 3.1: Create `huh.Input` for API key env var name (default: ANTHROPIC_API_KEY)
  - [x] 3.2: Add validation to check if env var exists in current environment
  - [x] 3.3: If env var missing, show warning message but allow continuation
  - [x] 3.4: Never write actual API key values to config files
  - [x] 3.5: Add validation that env var name is a valid identifier (no spaces, starts with letter)

- [x] Task 4: Implement timeout configuration (AC: #5)
  - [x] 4.1: Create `huh.Input` for timeout with default "30m"
  - [x] 4.2: Implement timeout format validation (accepts: 30m, 1h, 1h30m, 90m)
  - [x] 4.3: If invalid format, show error and use default with warning
  - [x] 4.4: Store timeout as `time.Duration` compatible string

- [x] Task 5: Implement max turns configuration (AC: #6)
  - [x] 5.1: Create `huh.Input` for max turns with default 10
  - [x] 5.2: Implement range validation (1-100)
  - [x] 5.3: If invalid range, show error and use default with warning

- [x] Task 6: Implement configuration persistence (AC: #7)
  - [x] 6.1: Ensure AI config is written to `~/.atlas/config.yaml` under `ai:` key
  - [x] 6.2: Use YAML field names matching `internal/config/config.go` AIConfig struct
  - [x] 6.3: Verify config can be loaded by `config.Load()` after saving

- [x] Task 7: Implement standalone `atlas config ai` command (enhancement)
  - [x] 7.1: Create `internal/cli/config_ai.go` for standalone AI config command
  - [x] 7.2: Allow users to reconfigure AI settings without running full init
  - [x] 7.3: Show current values and allow editing
  - [x] 7.4: Merge with existing config (don't overwrite other sections)

- [x] Task 8: Write comprehensive tests (AC: all)
  - [x] 8.1: Test model selection options and validation
  - [x] 8.2: Test API key env var validation (exists/missing cases)
  - [x] 8.3: Test timeout format validation (valid/invalid formats)
  - [x] 8.4: Test max turns range validation
  - [x] 8.5: Test config file output format matches expected YAML structure
  - [x] 8.6: Run `magex test:race` to verify no race conditions

## Dev Notes

### Technical Requirements

**Package Location:**
- Primary: `internal/cli/ai_config.go` (and `ai_config_test.go`)
- Secondary: `internal/cli/config_ai.go` (standalone command)

**Existing Code to Reference:**
- `internal/cli/init.go` - Lines 33-70 define `AtlasConfig`, `AIConfig` structs already used
- `internal/config/config.go` - Lines 43-62 define the canonical `AIConfig` struct
- Story 2-2 already implemented basic AI config in init wizard

**Import Rules (CRITICAL):**
- `internal/cli/ai_config.go` MAY import: `internal/config`, `internal/constants`, `internal/errors`
- `internal/cli/ai_config.go` MUST NOT import: `internal/domain`, `internal/task`, `internal/workspace`

### Architecture Compliance

**Context-First Design (ARCH-13):**
```go
func validateAPIKeyExists(ctx context.Context, envVarName string) (bool, error) {
    // Check cancellation at entry
    select {
    case <-ctx.Done():
        return false, ctx.Err()
    default:
    }

    value := os.Getenv(envVarName)
    return value != "", nil
}
```

**Error Wrapping (ARCH-14):**
```go
// Wrap at package boundary only
if err != nil {
    return errors.Wrap(err, "failed to validate AI configuration")
}
```

### Charm Huh Form Patterns

**Model Selection:**
```go
var model string
huh.NewSelect[string]().
    Title("Default AI Model").
    Description("Choose the default Claude model for ATLAS tasks").
    Options(
        huh.NewOption("Claude Sonnet (faster, balanced)", "sonnet"),
        huh.NewOption("Claude Opus (most capable)", "opus"),
        huh.NewOption("Claude Haiku (fast, cost-efficient)", "haiku"),
    ).
    Value(&model)
```

**API Key Environment Variable:**
```go
var apiKeyEnv string = defaultAPIKeyEnv
huh.NewInput().
    Title("API Key Environment Variable").
    Description("Name of the environment variable containing your Anthropic API key").
    Value(&apiKeyEnv).
    Placeholder("ANTHROPIC_API_KEY").
    Validate(func(s string) error {
        if s == "" {
            return fmt.Errorf("environment variable name cannot be empty")
        }
        if !isValidEnvVarName(s) {
            return fmt.Errorf("invalid environment variable name")
        }
        return nil
    })
```

**Timeout Validation:**
```go
var timeout string = defaultTimeout
huh.NewInput().
    Title("Default AI Timeout").
    Description("Maximum time for AI operations (e.g., 30m, 1h, 1h30m)").
    Value(&timeout).
    Placeholder("30m").
    Validate(func(s string) error {
        _, err := time.ParseDuration(s)
        if err != nil {
            return fmt.Errorf("invalid duration format: use formats like 30m, 1h, 1h30m")
        }
        return nil
    })
```

### Config File Structure

**~/.atlas/config.yaml AI section:**
```yaml
ai:
  model: sonnet                      # or "opus" or "haiku"
  api_key_env_var: ANTHROPIC_API_KEY # env var name, NOT the actual key
  timeout: 30m
  max_turns: 10
```

**Field Mapping to internal/config/config.go:**
| CLI Form Field | YAML Key | Config Struct Field |
|----------------|----------|---------------------|
| Model | `model` | `AIConfig.Model` |
| API Key Env Var | `api_key_env_var` | `AIConfig.APIKeyEnvVar` |
| Timeout | `timeout` | `AIConfig.Timeout` |
| Max Turns | `max_turns` | `AIConfig.MaxTurns` |

### Validation Logic

**API Key Environment Variable Validation:**
```go
func isValidEnvVarName(name string) bool {
    if name == "" {
        return false
    }
    // Must start with letter or underscore
    first := name[0]
    if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
        return false
    }
    // Rest can be letters, digits, or underscore
    for _, c := range name[1:] {
        if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
            return false
        }
    }
    return true
}

func checkAPIKeyExists(envVarName string) (exists bool, warning string) {
    value := os.Getenv(envVarName)
    if value == "" {
        return false, fmt.Sprintf("Warning: %s is not set in your environment. ATLAS will fail to run AI operations until this is configured.", envVarName)
    }
    return true, ""
}
```

**Max Turns Validation:**
```go
func validateMaxTurns(turns int) error {
    if turns < 1 || turns > 100 {
        return fmt.Errorf("max turns must be between 1 and 100")
    }
    return nil
}
```

### Previous Story Intelligence (Stories 2-1, 2-2)

**CRITICAL: Patterns Established in Epic 2**
- Story 2-1: `ToolDetector` interface pattern for mockable dependencies
- Story 2-2: Charm Huh forms with validation and theming
- Story 2-2: Config file backup before overwriting
- Story 2-2: Non-interactive mode using sensible defaults

**Code to Reuse from init.go:**
- `initStyles` struct for consistent styling
- `displayHeader()` for consistent branding
- YAML marshaling pattern for config file writing
- `AtlasConfig` and `AIConfig` structs (or refactor to use `config.AIConfig`)

**Review Feedback from 2-2:**
- Refactored `runInit` to return error instead of calling `os.Exit` (testability)
- Added `ToolDetector` interface for mockable dependencies
- Added timeout format validation with warning and fallback

### Testing Patterns

**Mock Form Testing (non-interactive):**
```go
func TestAIConfig_DefaultValues(t *testing.T) {
    cfg := collectAIConfigNonInteractive()

    assert.Equal(t, "sonnet", cfg.Model)
    assert.Equal(t, "ANTHROPIC_API_KEY", cfg.APIKeyEnvVar)
    assert.Equal(t, "30m", cfg.Timeout)
    assert.Equal(t, 10, cfg.MaxTurns)
}
```

**API Key Validation Testing:**
```go
func TestAIConfig_APIKeyValidation(t *testing.T) {
    tests := []struct {
        name      string
        envVar    string
        envValue  string
        wantWarn  bool
    }{
        {"exists", "TEST_API_KEY", "sk-test-value", false},
        {"missing", "MISSING_KEY", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if tt.envValue != "" {
                t.Setenv(tt.envVar, tt.envValue)
            }
            exists, warning := checkAPIKeyExists(tt.envVar)
            if tt.wantWarn {
                assert.False(t, exists)
                assert.NotEmpty(t, warning)
            } else {
                assert.True(t, exists)
                assert.Empty(t, warning)
            }
        })
    }
}
```

**Timeout Format Testing:**
```go
func TestAIConfig_TimeoutValidation(t *testing.T) {
    tests := []struct {
        input   string
        valid   bool
    }{
        {"30m", true},
        {"1h", true},
        {"1h30m", true},
        {"90m", true},
        {"30", false},      // missing unit
        {"abc", false},     // not a duration
        {"-30m", false},    // negative
    }

    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            _, err := time.ParseDuration(tt.input)
            if tt.valid {
                assert.NoError(t, err)
            } else {
                assert.Error(t, err)
            }
        })
    }
}
```

### File Structure

```
internal/
├── cli/
│   ├── init.go           # Existing - calls AI config step
│   ├── init_test.go      # Existing
│   ├── ai_config.go      # NEW: AI configuration step logic
│   ├── ai_config_test.go # NEW: AI configuration tests
│   ├── config_ai.go      # NEW: Standalone `atlas config ai` command
│   └── config_ai_test.go # NEW: Standalone command tests
└── config/
    ├── config.go         # Existing - canonical AIConfig struct
    └── load.go           # Existing - config loading
```

### Project Structure Notes

- AI configuration is a step WITHIN the init wizard (already exists in init.go)
- This story enhances and extracts AI config for better testing and reuse
- Adds standalone `atlas config ai` command for reconfiguration
- No new packages needed - extends existing CLI structure

### Security Considerations

**CRITICAL: API Key Handling**
- NEVER write actual API key values to config files
- Only store environment variable NAMES in config
- Validate env var exists but never log or display the actual key value
- When displaying config, mask any value that looks like a key

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 2.3]
- [Source: _bmad-output/planning-artifacts/architecture.md#AIConfig]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/config/config.go#AIConfig (lines 43-62)]
- [Source: internal/cli/init.go#AIConfig (lines 39-49)]
- [Source: _bmad-output/implementation-artifacts/2-2-implement-atlas-init-setup-wizard.md]
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

N/A - No debug issues encountered

### Completion Notes List

- Created `internal/cli/ai_config.go` with reusable AI configuration functions
- Created `internal/cli/ai_config_test.go` with comprehensive unit tests
- Created `internal/cli/config_ai.go` implementing standalone `atlas config ai` command
- Created `internal/cli/config_ai_test.go` with tests for the standalone command
- Refactored `internal/cli/init.go` to use the new AI config functions
- Updated `internal/cli/init_test.go` to match new field names
- Fixed YAML field names in `AIConfig` struct to match canonical `config.AIConfig`:
  - `default_model` → `model`
  - `api_key_env` → `api_key_env_var`
  - `default_timeout` → `timeout`
- Added sentinel errors to `internal/errors/errors.go` for validation errors
- Added `atlas config` command with `ai` subcommand to root command

### Change Log

- 2025-12-27: Implemented all 8 tasks for AI Provider Configuration story
- 2025-12-27: All validation commands pass (format, lint, test:race, build)
- 2025-12-27: **Code Review Fixes Applied:**
  - [HIGH] Fixed MaxTurns input value being silently discarded - restructured `NewAIConfigForm` and `CollectAIConfigInteractive` to properly capture and parse maxTurnsStr after form completion
  - [MEDIUM] Added warning logging for backup file creation failures instead of silently ignoring errors
  - [MEDIUM] Extracted shared `saveAtlasConfig` function to eliminate duplicate code between init.go and config_ai.go
  - [LOW] Removed unused `parseIntWithDefault` function and cleaned up imports

### File List

- internal/cli/ai_config.go (NEW)
- internal/cli/ai_config_test.go (NEW)
- internal/cli/config_ai.go (NEW)
- internal/cli/config_ai_test.go (NEW)
- internal/cli/init.go (MODIFIED)
- internal/cli/init_test.go (MODIFIED)
- internal/cli/root.go (MODIFIED)
- internal/errors/errors.go (MODIFIED)
