# Story 1.5: Create Configuration Framework

Status: done

## Story

As a **developer**,
I want **a configuration package with layered precedence loading**,
So that **configuration can be overridden at CLI, environment, project, and global levels**.

## Acceptance Criteria

1. **Given** the domain package exists **When** I implement `internal/config/` **Then** `config.go` contains the main `Config` struct with nested configs:
   - `AIConfig` for AI settings
   - `GitConfig` for Git settings
   - `WorktreeConfig` for worktree settings
   - `CIConfig` for CI settings
   - `TemplatesConfig` for template settings
   - `ValidationConfig` for validation settings

2. **Given** the config package is being implemented **When** I create `load.go` **Then** it implements `Load(ctx context.Context) (*Config, error)` with precedence:
   1. CLI flags (passed in)
   2. Environment variables (ATLAS_* prefix)
   3. Project config (.atlas/config.yaml)
   4. Global config (~/.atlas/config.yaml)
   5. Built-in defaults

3. **Given** the config package is being implemented **When** I create `validate.go` **Then** it implements config validation with meaningful error messages

4. **Given** the config package is complete **When** I test Viper integration **Then** YAML parsing works correctly for config files

5. **Given** the config package is complete **When** I test environment variables **Then** ATLAS_AI_MODEL maps to ai.model correctly

6. **Given** the config package is complete **When** I run tests **Then** tests verify precedence order is correct (higher priority overrides lower)

## Tasks / Subtasks

- [x] Task 1: Create config.go with Config struct and nested configs (AC: #1)
  - [x] Create `internal/config/config.go`
  - [x] Define `Config` struct with all nested config types
  - [x] Define `AIConfig` struct with fields: Model, APIKeyEnvVar, Timeout, MaxTurns
  - [x] Define `GitConfig` struct with fields: BaseBranch, AutoProceedGit, Remote
  - [x] Define `WorktreeConfig` struct with fields: BaseDir, NamingSuffix
  - [x] Define `CIConfig` struct with fields: Timeout, PollInterval, RequiredWorkflows
  - [x] Define `TemplatesConfig` struct with fields: DefaultTemplate, CustomTemplates
  - [x] Define `ValidationConfig` struct with fields: Commands, Timeout, ParallelExecution
  - [x] Define `NotificationsConfig` struct with fields: Bell, Events
  - [x] Use yaml tags for all fields (NOT json - config uses YAML)
  - [x] Add comprehensive documentation for each type and field
  - [x] Verify imports follow architecture rules

- [x] Task 2: Create defaults.go with default values (AC: #1, #2)
  - [x] Create `internal/config/defaults.go`
  - [x] Define `DefaultConfig()` function returning config with sensible defaults
  - [x] Set default AI model to "sonnet"
  - [x] Set default API key env var to "ANTHROPIC_API_KEY"
  - [x] Set default AI timeout to 30 minutes (from constants)
  - [x] Set default max turns to 10
  - [x] Set default git base branch to "main"
  - [x] Set default CI timeout to 30 minutes (from constants)
  - [x] Set default CI poll interval to 2 minutes (from constants)
  - [x] Set default validation timeout to 5 minutes
  - [x] Set default notifications bell to true
  - [x] Document why each default was chosen

- [x] Task 3: Create load.go with precedence loading (AC: #2, #4, #5)
  - [x] Create `internal/config/load.go`
  - [x] Implement `Load(ctx context.Context) (*Config, error)` function
  - [x] Create and configure Viper instance (DO NOT use global viper)
  - [x] Set config file name to "config" and type to "yaml"
  - [x] Add search paths: `.atlas/` (project), `~/.atlas/` (global)
  - [x] Configure environment variable prefix "ATLAS" with AutomaticEnv
  - [x] Configure env var key replacer (e.g., ATLAS_AI_MODEL -> ai.model)
  - [x] Load defaults first, then merge config files, then env vars
  - [x] Unmarshal into Config struct using Viper
  - [x] Call validation before returning
  - [x] Implement `LoadWithOverrides(ctx context.Context, overrides *Config) (*Config, error)` for CLI flags
  - [x] Handle file not found gracefully (not an error, use defaults)

- [x] Task 4: Create validate.go with config validation (AC: #3)
  - [x] Create `internal/config/validate.go`
  - [x] Implement `Validate(cfg *Config) error` function
  - [x] Validate AI timeout is positive
  - [x] Validate max turns is between 1 and 100
  - [x] Validate CI timeout is positive
  - [x] Validate CI poll interval is reasonable (1s - 10m)
  - [x] Validate git base branch is not empty
  - [x] Return wrapped errors with field names for clear debugging
  - [x] Use errors.Wrap from internal/errors for wrapping

- [x] Task 5: Create paths.go for config file paths (AC: #2)
  - [x] Create `internal/config/paths.go`
  - [x] Implement `GlobalConfigDir() (string, error)` returning ~/.atlas
  - [x] Implement `ProjectConfigDir() string` returning .atlas
  - [x] Implement `GlobalConfigPath() (string, error)` returning ~/.atlas/config.yaml
  - [x] Implement `ProjectConfigPath() string` returning .atlas/config.yaml
  - [x] Use constants.AtlasHome for directory name
  - [x] Handle home directory expansion correctly using os.UserHomeDir

- [x] Task 6: Create comprehensive tests (AC: #6)
  - [x] Create `internal/config/config_test.go`
  - [x] Test DefaultConfig returns valid config
  - [x] Test Config YAML serialization/deserialization
  - [x] Test validation catches invalid values
  - [x] Test validation passes for valid config
  - [x] Create `internal/config/load_test.go`
  - [x] Test Load returns defaults when no config file exists
  - [x] Test Load reads project config file correctly
  - [x] Test Load reads global config file correctly
  - [x] Test precedence: project config overrides global
  - [x] Test precedence: env vars override config files
  - [x] Test env var mapping (ATLAS_AI_MODEL -> ai.model)
  - [x] Test LoadWithOverrides applies CLI overrides
  - [x] Use t.TempDir() for isolated test directories
  - [x] Use table-driven tests for validation scenarios

- [x] Task 7: Remove .gitkeep and validate (AC: all)
  - [x] Remove `internal/config/.gitkeep`
  - [x] Run `go build ./...` to verify compilation
  - [x] Run `magex format:fix` to format code
  - [x] Run `magex lint` to verify linting passes (must have 0 issues)
  - [x] Run `magex test` to verify tests pass

## Dev Notes

### Critical Architecture Requirements

**This package is critical for the entire application configuration system!** All other packages will rely on config for their settings. Any mistakes here will affect the entire codebase.

#### Package Rules (CRITICAL - ENFORCE STRICTLY)

From architecture.md:
- **internal/config** → CAN import `internal/constants`, `internal/errors`, and standard library
- **internal/config** → MUST NOT import `internal/domain` or other internal packages
- All packages CAN import config

#### Config Precedence (CRITICAL)

From Architecture Document:
```
Precedence (highest to lowest):
1. CLI flags (passed in via LoadWithOverrides)
2. Environment variables (ATLAS_* prefix)
3. Project config (.atlas/config.yaml)
4. Global config (~/.atlas/config.yaml)
5. Built-in defaults
```

Each higher level COMPLETELY overrides the lower level for the same key.

#### Viper Integration Pattern (CRITICAL)

From Viper v1.21 best practices:
```go
// ✅ CORRECT - Create new Viper instance (not global)
func Load(ctx context.Context) (*Config, error) {
    v := viper.New()

    // Set defaults first
    v.SetDefault("ai.model", "sonnet")
    v.SetDefault("ai.timeout", constants.DefaultAITimeout)

    // Configure file search
    v.SetConfigName("config")
    v.SetConfigType("yaml")
    v.AddConfigPath(".atlas")
    v.AddConfigPath("$HOME/.atlas")

    // Configure environment
    v.SetEnvPrefix("ATLAS")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()

    // Read config (ignore file not found)
    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, errors.Wrap(err, "failed to read config")
        }
    }

    // Unmarshal
    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, errors.Wrap(err, "failed to unmarshal config")
    }

    return &cfg, nil
}

// ❌ WRONG - Using global viper
func Load(ctx context.Context) (*Config, error) {
    viper.SetDefault("ai.model", "sonnet")  // DON'T use global viper
}
```

#### Config Struct Pattern

From Architecture Document - use YAML tags (not JSON tags):
```go
// ✅ CORRECT - YAML tags for config files
type Config struct {
    AI           AIConfig           `yaml:"ai"`
    Git          GitConfig          `yaml:"git"`
    Worktree     WorktreeConfig     `yaml:"worktree"`
    CI           CIConfig           `yaml:"ci"`
    Templates    TemplatesConfig    `yaml:"templates"`
    Validation   ValidationConfig   `yaml:"validation"`
    Notifications NotificationsConfig `yaml:"notifications"`
}

type AIConfig struct {
    Model       string        `yaml:"model"`
    APIKeyEnvVar string       `yaml:"api_key_env_var"`
    Timeout     time.Duration `yaml:"timeout"`
    MaxTurns    int           `yaml:"max_turns"`
}

// ❌ WRONG - JSON tags for config
type Config struct {
    AI AIConfig `json:"ai"`  // DON'T use json tags for YAML config
}
```

#### Environment Variable Mapping

The env key replacer transforms nested keys:
- `ATLAS_AI_MODEL` → `ai.model`
- `ATLAS_AI_TIMEOUT` → `ai.timeout`
- `ATLAS_GIT_BASE_BRANCH` → `git.base_branch`
- `ATLAS_CI_POLL_INTERVAL` → `ci.poll_interval`

#### Default Values from Architecture/Constants

| Setting | Default | Source |
|---------|---------|--------|
| ai.model | "sonnet" | Architecture requirement |
| ai.api_key_env_var | "ANTHROPIC_API_KEY" | Architecture requirement |
| ai.timeout | 30m | constants.DefaultAITimeout |
| ai.max_turns | 10 | Architecture requirement |
| git.base_branch | "main" | Common convention |
| git.remote | "origin" | Git default |
| git.auto_proceed_git | true | For automation |
| ci.timeout | 30m | constants.DefaultCITimeout |
| ci.poll_interval | 2m | constants.CIPollInterval |
| validation.timeout | 5m | Reasonable default |
| validation.parallel_execution | true | Performance |
| notifications.bell | true | UX requirement |

### Previous Story Intelligence

From Story 1-4 (domain) completion:
- Use `yaml` tags for config structs (different from domain which uses `json` tags)
- All code must pass `magex format:fix`, `magex lint`, `magex test`
- Table-driven tests are the preferred pattern
- Comprehensive documentation is required for all exported items
- Import from constants for shared values like timeouts

### Git Commit Patterns

Recent commits follow conventional commits format:
- `feat(domain): add centralized domain types package`
- `feat(errors): add centralized error handling package`

For this story, use:
- `feat(config): add configuration framework with layered precedence`

### Project Structure Notes

Current structure after Story 1-4:
```
internal/
├── constants/
│   ├── constants.go      # File/dir/timeout/retry constants
│   ├── status.go         # TaskStatus, WorkspaceStatus types
│   ├── paths.go          # Path-related constants
│   └── status_test.go    # Tests
├── errors/
│   ├── errors.go         # Sentinel errors
│   ├── wrap.go           # Wrap utility
│   ├── user.go           # User-facing formatting
│   └── errors_test.go    # Tests
├── domain/
│   ├── task.go           # Task, Step, StepResult
│   ├── workspace.go      # Workspace, TaskRef
│   ├── template.go       # Template, StepDefinition
│   ├── ai.go             # AIRequest, AIResult
│   ├── status.go         # Re-exports from constants
│   └── domain_test.go    # Tests
├── config/
│   └── .gitkeep          # Will be replaced by actual files
└── ...
```

After this story:
```
internal/
├── constants/
│   └── ... (unchanged)
├── errors/
│   └── ... (unchanged)
├── domain/
│   └── ... (unchanged)
├── config/
│   ├── config.go         # Config struct and nested types
│   ├── defaults.go       # DefaultConfig function
│   ├── load.go           # Load and LoadWithOverrides functions
│   ├── validate.go       # Validation logic
│   ├── paths.go          # Config path helpers
│   ├── config_test.go    # Config struct tests
│   └── load_test.go      # Load function tests
└── ...
```

### Config File Examples

**Global config (~/.atlas/config.yaml):**
```yaml
ai:
  model: opus
  timeout: 45m
  max_turns: 15

git:
  base_branch: main
  auto_proceed_git: false

notifications:
  bell: true
  events:
    - awaiting_approval
    - validation_failed
```

**Project config (.atlas/config.yaml):**
```yaml
# Project-specific overrides
ai:
  model: sonnet  # Override global opus for this project

validation:
  commands:
    - magex format:fix
    - magex lint
    - magex test
  timeout: 10m
```

### Testing Strategy

**Precedence Tests (CRITICAL):**
```go
func TestLoad_Precedence(t *testing.T) {
    // Create temp dirs for global and project config
    homeDir := t.TempDir()
    projectDir := t.TempDir()

    // Write global config with ai.model = "opus"
    globalConfig := filepath.Join(homeDir, ".atlas", "config.yaml")
    os.MkdirAll(filepath.Dir(globalConfig), 0755)
    os.WriteFile(globalConfig, []byte("ai:\n  model: opus"), 0644)

    // Write project config with ai.model = "sonnet"
    projectConfig := filepath.Join(projectDir, ".atlas", "config.yaml")
    os.MkdirAll(filepath.Dir(projectConfig), 0755)
    os.WriteFile(projectConfig, []byte("ai:\n  model: sonnet"), 0644)

    // Load config - project should override global
    cfg, err := LoadFromPaths(ctx, projectConfig, globalConfig)
    require.NoError(t, err)
    assert.Equal(t, "sonnet", cfg.AI.Model, "project config should override global")
}

func TestLoad_EnvOverridesConfigFile(t *testing.T) {
    t.Setenv("ATLAS_AI_MODEL", "haiku")

    // Create config file with model = "opus"
    // ...

    cfg, err := Load(ctx)
    require.NoError(t, err)
    assert.Equal(t, "haiku", cfg.AI.Model, "env var should override config file")
}
```

### Viper Version Note

Using Viper v1.21.0 (already in go.mod). Key features:
- Uses maintained YAML library (go.yaml.in/yaml/v3)
- Supports mapstructure v2
- Case-insensitive key handling
- Automatic environment variable binding

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Config Package]
- [Source: _bmad-output/planning-artifacts/architecture.md#Configuration Sources & Precedence]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns & Consistency Rules]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.5]
- [Source: _bmad-output/project-context.md#Package Import Rules]
- [Source: github.com/spf13/viper - Go Packages](https://pkg.go.dev/github.com/spf13/viper)

### Validation Commands (MUST RUN BEFORE COMPLETION)

```bash
magex format:fix    # Format code
magex lint          # Must pass with 0 issues
magex test          # Must pass all tests
go build ./...      # Must compile
```

### Anti-Patterns to Avoid

```go
// ❌ NEVER: Use global viper instance
viper.SetDefault("ai.model", "sonnet")  // DON'T
func Load() { viper.ReadInConfig() }     // DON'T

// ✅ DO: Create new viper instance
func Load(ctx context.Context) (*Config, error) {
    v := viper.New()
    v.SetDefault("ai.model", "sonnet")
    // ...
}

// ❌ NEVER: Use json tags for config
type Config struct {
    AI AIConfig `json:"ai"`  // DON'T
}

// ✅ DO: Use yaml tags for config
type Config struct {
    AI AIConfig `yaml:"ai"`
}

// ❌ NEVER: Import domain package
import "github.com/mrz1836/atlas/internal/domain"  // DON'T

// ✅ DO: Only import constants, errors, and std lib
import "github.com/mrz1836/atlas/internal/constants"
import "github.com/mrz1836/atlas/internal/errors"

// ❌ NEVER: Hardcode config values
timeout := 30 * time.Minute  // DON'T - use constants

// ✅ DO: Use constants for default values
timeout := constants.DefaultAITimeout

// ❌ NEVER: Ignore context parameter
func Load() (*Config, error) { ... }  // DON'T - missing ctx

// ✅ DO: Accept context as first parameter
func Load(ctx context.Context) (*Config, error) { ... }

// ❌ NEVER: Panic on config errors
if err != nil {
    panic(err)  // DON'T
}

// ✅ DO: Return wrapped errors
if err != nil {
    return nil, errors.Wrap(err, "failed to load config")
}
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

- Fixed mapstructure decode hook signature issue by using proper mapstructure.ComposeDecodeHookFunc
- Added mapstructure tags alongside yaml tags for proper Viper unmarshaling
- Fixed linting issues: replaced dynamic errors with static sentinel errors, used errors.As for type assertions
- Fixed file permissions in tests (0o600 instead of 0o644, 0o750 for directories)

### Completion Notes List

- Implemented complete configuration framework with layered precedence loading
- All 7 config sections implemented: AI, Git, Worktree, CI, Templates, Validation, Notifications
- Load function uses Viper with proper instance (not global)
- Environment variables with ATLAS_ prefix properly mapped (ATLAS_AI_MODEL -> ai.model)
- Comprehensive validation with meaningful error messages using static sentinel errors
- All tests pass including precedence, env var mapping, and duration parsing tests
- Added config-related sentinel errors to internal/errors package
- Full linting compliance (0 issues)

### File List

- internal/config/config.go (new)
- internal/config/defaults.go (new)
- internal/config/load.go (new)
- internal/config/validate.go (new)
- internal/config/paths.go (new)
- internal/config/config_test.go (new)
- internal/config/load_test.go (new)
- internal/errors/errors.go (modified - added config sentinel errors)
- internal/config/.gitkeep (deleted)
- _bmad-output/implementation-artifacts/sprint-status.yaml (modified)

## Senior Developer Review (AI)

**Review Date:** 2025-12-27
**Reviewer:** Claude Opus 4.5 (Code Review Workflow)
**Outcome:** CHANGES APPLIED

### Issues Found and Fixed

| Severity | Issue | Resolution |
|----------|-------|------------|
| HIGH | Load() only read ONE config file (project OR global), not both with proper merging | Refactored Load() to use loadGlobalConfig() and loadProjectConfig() helper functions that properly merge configs using ReadInConfig for global then MergeInConfig for project |
| MEDIUM | sprint-status.yaml modified but not in File List | Added to File List |
| MEDIUM | Bool override limitation undocumented | Added comprehensive documentation to applyOverrides() explaining how CLI should handle boolean flags |
| MEDIUM | Context parameter usage undocumented | Added documentation explaining why context is not used for cancellation in config loading |

### Code Quality Improvements

- Extracted loadGlobalConfig() and loadProjectConfig() helper functions for better readability
- Added getGlobalConfigPathIfExists() and fileExists() utility functions
- Added test TestLoad_MergesGlobalAndProjectConfigs to verify merging behavior
- All changes pass magex format:fix, magex lint (0 issues), magex test

### Remaining LOW Items (not blocking)

- defaults.go:82 and load.go hardcode 5*time.Minute instead of using a constant
- Missing test for GlobalConfigDir error case when os.UserHomeDir() fails
- Missing test for LoadFromPaths with both paths as empty strings

## Change Log

- 2025-12-27: Code review fixes - Load() now properly merges global and project configs (Story 1.5)
- 2025-12-27: Implemented configuration framework with layered precedence loading (Story 1.5)

