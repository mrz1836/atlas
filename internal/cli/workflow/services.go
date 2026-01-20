// Package workflow provides workflow orchestration for ATLAS task execution.
package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/hook"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/template/steps"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
)

// GitServices holds all git-related services created for task execution.
// This struct reduces the number of return values from CreateGitServices.
type GitServices struct {
	Runner           git.Runner
	SmartCommitter   *git.SmartCommitRunner
	Pusher           *git.PushRunner
	HubRunner        *git.CLIGitHubRunner
	PRDescGen        *git.AIDescriptionGenerator
	CIFailureHandler *task.CIFailureHandler
}

// RegistryDeps holds all dependencies needed to create an ExecutorRegistry.
// This struct reduces the number of parameters to CreateExecutorRegistry.
type RegistryDeps struct {
	WorkDir          string
	TaskStore        *task.FileStore
	Notifier         *tui.Notifier
	AIRunner         ai.Runner
	Logger           zerolog.Logger
	GitServices      *GitServices
	Config           *config.Config
	ProgressCallback func(event interface{})
}

// ServiceFactory creates all services needed for task execution.
type ServiceFactory struct {
	logger zerolog.Logger
}

// NewServiceFactory creates a new ServiceFactory.
func NewServiceFactory(logger zerolog.Logger) *ServiceFactory {
	return &ServiceFactory{logger: logger}
}

// CreateTaskStore creates a new task file store.
func (f *ServiceFactory) CreateTaskStore() (*task.FileStore, error) {
	taskStore, err := task.NewFileStore("")
	if err != nil {
		return nil, fmt.Errorf("failed to create task store: %w", err)
	}
	return taskStore, nil
}

// CreateConfig loads configuration with fallback to defaults.
func (f *ServiceFactory) CreateConfig(ctx context.Context) (*config.Config, error) {
	cfg, err := config.Load(ctx)
	if err != nil {
		f.logger.Warn().Err(err).Msg("failed to load config, using default settings")
		return config.DefaultConfig(), nil
	}
	return cfg, nil
}

// SetupTaskStoreAndConfig creates the task store and loads configuration.
func (f *ServiceFactory) SetupTaskStoreAndConfig(ctx context.Context) (*task.FileStore, *config.Config, error) {
	taskStore, err := f.CreateTaskStore()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := f.CreateConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	return taskStore, cfg, nil
}

// CreateNotifiers creates UI and state change notifiers.
func (f *ServiceFactory) CreateNotifiers(cfg *config.Config) (*tui.Notifier, *task.StateChangeNotifier) {
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)
	stateNotifier := task.NewStateChangeNotifier(task.NotificationConfig{
		BellEnabled: cfg.Notifications.Bell,
		Quiet:       false, // TODO: Pass quiet flag through when available
		Events:      cfg.Notifications.Events,
	})
	return notifier, stateNotifier
}

// CreateAIRunner creates and configures the AI runner with all supported agents.
func (f *ServiceFactory) CreateAIRunner(cfg *config.Config) ai.Runner {
	runnerRegistry := ai.NewRunnerRegistry()
	runnerRegistry.Register(domain.AgentClaude, ai.NewClaudeCodeRunner(&cfg.AI, nil))
	runnerRegistry.Register(domain.AgentGemini, ai.NewGeminiRunner(&cfg.AI, nil, ai.WithGeminiLogger(f.logger)))
	runnerRegistry.Register(domain.AgentCodex, ai.NewCodexRunner(&cfg.AI, nil))
	return ai.NewMultiRunner(runnerRegistry)
}

// GitConfig holds resolved git configuration settings.
type GitConfig struct {
	CommitAgent         string
	CommitModel         string
	CommitTimeout       time.Duration
	CommitMaxRetries    int
	CommitBackoffFactor float64
	PRDescAgent         string
	PRDescModel         string
}

// CreateGitServices creates all git-related services and returns them in a GitServices struct.
func (f *ServiceFactory) CreateGitServices(ctx context.Context, worktreePath string, _ *config.Config, aiRunner ai.Runner, gitCfg GitConfig) (*GitServices, error) {
	gitRunner, err := git.NewRunner(ctx, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create git runner: %w", err)
	}

	smartCommitter := git.NewSmartCommitRunner(gitRunner, worktreePath, aiRunner,
		git.WithAgent(gitCfg.CommitAgent),
		git.WithModel(gitCfg.CommitModel),
		git.WithTimeout(gitCfg.CommitTimeout),
		git.WithMaxRetries(gitCfg.CommitMaxRetries),
		git.WithRetryBackoffFactor(gitCfg.CommitBackoffFactor),
		git.WithLogger(f.logger),
	)
	pusher := git.NewPushRunner(gitRunner)
	hubRunner := git.NewCLIGitHubRunner(worktreePath)
	prDescGen := git.NewAIDescriptionGenerator(aiRunner,
		git.WithAIDescAgent(gitCfg.PRDescAgent),
		git.WithAIDescModel(gitCfg.PRDescModel),
		git.WithAIDescLogger(f.logger),
	)
	ciFailureHandler := task.NewCIFailureHandler(hubRunner)

	return &GitServices{
		Runner:           gitRunner,
		SmartCommitter:   smartCommitter,
		Pusher:           pusher,
		HubRunner:        hubRunner,
		PRDescGen:        prDescGen,
		CIFailureHandler: ciFailureHandler,
	}, nil
}

// CreateExecutorRegistry creates the step executor registry with all dependencies.
func (f *ServiceFactory) CreateExecutorRegistry(deps RegistryDeps) *steps.ExecutorRegistry {
	return steps.NewDefaultRegistry(steps.ExecutorDeps{
		WorkDir:                deps.WorkDir,
		ArtifactSaver:          deps.TaskStore,
		Notifier:               deps.Notifier,
		AIRunner:               deps.AIRunner,
		Logger:                 deps.Logger,
		SmartCommitter:         deps.GitServices.SmartCommitter,
		Pusher:                 deps.GitServices.Pusher,
		HubRunner:              deps.GitServices.HubRunner,
		PRDescriptionGenerator: deps.GitServices.PRDescGen,
		GitRunner:              deps.GitServices.Runner,
		CIFailureHandler:       deps.GitServices.CIFailureHandler,
		BaseBranch:             deps.Config.Git.BaseBranch,
		CIConfig:               &deps.Config.CI,
		FormatCommands:         deps.Config.Validation.Commands.Format,
		LintCommands:           deps.Config.Validation.Commands.Lint,
		TestCommands:           deps.Config.Validation.Commands.Test,
		PreCommitCommands:      deps.Config.Validation.Commands.PreCommit,
		ProgressCallback:       deps.ProgressCallback,
	})
}

// EngineDeps holds dependencies for creating a task engine.
type EngineDeps struct {
	TaskStore              *task.FileStore
	ExecRegistry           *steps.ExecutorRegistry
	Logger                 zerolog.Logger
	StateNotifier          *task.StateChangeNotifier
	ProgressCallback       func(task.StepProgressEvent)
	ValidationRetryHandler *validation.RetryHandler
	HookManager            task.HookManager
}

// CreateEngine creates the task engine with progress callback.
func (f *ServiceFactory) CreateEngine(deps EngineDeps, cfg *config.Config) *task.Engine {
	engineCfg := task.DefaultEngineConfig()
	engineCfg.ProgressCallback = deps.ProgressCallback

	opts := []task.EngineOption{
		task.WithNotifier(deps.StateNotifier),
	}
	if deps.ValidationRetryHandler != nil {
		opts = append(opts, task.WithValidationRetryHandler(deps.ValidationRetryHandler))
	}
	if deps.HookManager != nil {
		opts = append(opts, task.WithHookManager(deps.HookManager))
	}

	// Pass validation commands from config to engine for retry operations
	opts = append(opts, task.WithValidationCommands(
		cfg.Validation.Commands.Format,
		cfg.Validation.Commands.Lint,
		cfg.Validation.Commands.Test,
		cfg.Validation.Commands.PreCommit,
	))

	return task.NewEngine(deps.TaskStore, deps.ExecRegistry, engineCfg, deps.Logger, opts...)
}

// CreateValidationRetryHandler creates the validation retry handler for automatic AI-assisted fixes.
func (f *ServiceFactory) CreateValidationRetryHandler(aiRunner ai.Runner, cfg *config.Config) *validation.RetryHandler {
	if !cfg.Validation.AIRetryEnabled {
		return nil
	}

	// Create validation executor for retry
	executor := validation.NewExecutorWithRunner(validation.DefaultTimeout, &validation.DefaultCommandRunner{})

	// Create retry handler with config
	return validation.NewRetryHandlerFromConfig(
		aiRunner,
		executor,
		cfg.Validation.AIRetryEnabled,
		cfg.Validation.MaxAIRetryAttempts,
		f.logger,
	)
}

// CreateHookManager creates the hook manager for crash recovery and checkpointing.
// Returns nil if creation fails (hooks are optional, non-blocking).
func (f *ServiceFactory) CreateHookManager(cfg *config.Config, logger zerolog.Logger) task.HookManager {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		f.logger.Warn().Err(err).Msg("failed to get home directory, hooks disabled")
		return nil
	}

	basePath := filepath.Join(homeDir, constants.AtlasHome)

	// Create markdown generator for HOOK.md
	mdGen := hook.NewMarkdownGenerator()

	// Create file store with optional markdown generator
	hookStore := hook.NewFileStore(basePath,
		hook.WithMarkdownGenerator(mdGen),
		hook.WithLogger(&logger),
	)

	// Create and return manager
	return hook.NewManager(hookStore, &cfg.Hooks,
		hook.WithManagerLogger(logger),
	)
}

// Standalone wrapper functions for testing and backwards compatibility.

// CreateValidationRetryHandler is a standalone function for creating a validation retry handler.
// It creates a temporary service factory with a no-op logger.
// This is primarily for testing and backwards compatibility.
func CreateValidationRetryHandler(aiRunner ai.Runner, cfg *config.Config) *validation.RetryHandler {
	f := NewServiceFactory(zerolog.Nop())
	return f.CreateValidationRetryHandler(aiRunner, cfg)
}

// CreateNotifiers is a standalone function for creating notifiers.
// It creates a temporary service factory with a no-op logger.
// This is primarily for testing and backwards compatibility.
func CreateNotifiers(cfg *config.Config) (*tui.Notifier, *task.StateChangeNotifier) {
	f := NewServiceFactory(zerolog.Nop())
	return f.CreateNotifiers(cfg)
}

// CreateAIRunner is a standalone function for creating an AI runner.
// It creates a temporary service factory with a no-op logger.
// This is primarily for testing and backwards compatibility.
func CreateAIRunner(cfg *config.Config) ai.Runner {
	f := NewServiceFactory(zerolog.Nop())
	return f.CreateAIRunner(cfg)
}
