// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/signal"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/template/steps"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
	"github.com/mrz1836/atlas/internal/workspace"
)

// Workspace name generation constants.
const maxWorkspaceNameLen = 50

// Regex patterns for workspace name generation.
var (
	// nonAlphanumericRegex matches any character that is not a lowercase letter, digit, or hyphen.
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9-]+`)
	// multipleHyphensRegex matches consecutive hyphens.
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

// AddStartCommand adds the start command to the root command.
func AddStartCommand(root *cobra.Command) {
	root.AddCommand(newStartCmd())
}

// startOptions contains all options for the start command.
type startOptions struct {
	templateName  string
	workspaceName string
	agent         string
	model         string
	baseBranch    string
	useLocal      bool
	noInteractive bool
	verify        bool
	noVerify      bool
	dryRun        bool
}

// newStartCmd creates the start command.
func newStartCmd() *cobra.Command {
	var (
		templateName  string
		workspaceName string
		agent         string
		model         string
		baseBranch    string
		useLocal      bool
		noInteractive bool
		verify        bool
		noVerify      bool
		dryRun        bool
	)

	cmd := &cobra.Command{
		Use:   "start <description>",
		Short: "Start a new task with the given description",
		Long: `Start a new task by creating a workspace, selecting a template,
and beginning execution of the template steps.

Examples:
  atlas start "fix null pointer in parseConfig"
  atlas start "add retry logic to HTTP client" --template feature
  atlas start "update dependencies" --workspace deps-update --template commit
  atlas start "add new feature" --template feature --verify
  atlas start "quick fix" --template bugfix --no-verify
  atlas start "fix from develop" --template bugfix --branch develop
  atlas start "review changes" --template bugfix --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), cmd, cmd.OutOrStdout(), args[0], startOptions{
				templateName:  templateName,
				workspaceName: workspaceName,
				agent:         agent,
				model:         model,
				baseBranch:    baseBranch,
				useLocal:      useLocal,
				noInteractive: noInteractive,
				verify:        verify,
				noVerify:      noVerify,
				dryRun:        dryRun,
			})
		},
	}

	cmd.Flags().StringVarP(&templateName, "template", "t", "",
		"Template to use (bugfix, feature, commit)")
	cmd.Flags().StringVarP(&workspaceName, "workspace", "w", "",
		"Custom workspace name")
	cmd.Flags().StringVarP(&agent, "agent", "a", "",
		"AI agent/CLI to use (claude, gemini, codex)")
	cmd.Flags().StringVarP(&model, "model", "m", "",
		"AI model to use (claude: sonnet, opus, haiku; gemini: flash, pro; codex: codex, max, mini)")
	cmd.Flags().StringVarP(&baseBranch, "branch", "b", "",
		"Base branch to create workspace from (fetches from remote by default)")
	cmd.Flags().BoolVar(&useLocal, "use-local", false,
		"Prefer local branch over remote when both exist")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false,
		"Disable interactive prompts")
	cmd.Flags().BoolVar(&verify, "verify", false,
		"Enable AI verification step (cross-model validation)")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false,
		"Disable AI verification step")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Show what would happen without making changes")

	return cmd
}

// startContext holds shared state for the start command execution.
type startContext struct {
	ctx          context.Context //nolint:containedctx // context needed for error handling
	outputFormat string
	out          tui.Output
	w            io.Writer
}

// runStart executes the start command.
func runStart(ctx context.Context, cmd *cobra.Command, w io.Writer, description string, opts startOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Create signal handler for graceful shutdown on Ctrl+C
	sigHandler := signal.NewHandler(ctx)
	defer sigHandler.Stop()
	ctx = sigHandler.Context()

	logger := Logger()
	outputFormat := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)
	sc := &startContext{
		ctx:          ctx,
		outputFormat: outputFormat,
		out:          out,
		w:            w,
	}

	// Validate agent flag if provided
	if err := validateAgent(opts.agent); err != nil {
		return sc.handleError("", err)
	}

	// Validate model flag if provided
	if err := validateModel(opts.agent, opts.model); err != nil {
		return sc.handleError("", err)
	}

	// Validate verify flags - cannot use both
	if opts.verify && opts.noVerify {
		return sc.handleError("", atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: cannot use both --verify and --no-verify", atlaserrors.ErrConflictingFlags)))
	}

	// Validate we're in a git repository
	repoPath, err := findGitRepository(ctx) //nolint:contextcheck // context is properly checked and used
	if err != nil {
		return sc.handleError("", fmt.Errorf("not in a git repository: %w", err))
	}

	logger.Debug().Str("repo_path", repoPath).Msg("found git repository")

	// Load config for custom templates
	cfg, cfgErr := config.Load(ctx) //nolint:contextcheck // context is properly checked and used
	if cfgErr != nil {
		// Log warning but continue with defaults - don't fail task start for config issues
		logger.Error().Err(cfgErr).
			Str("project_config", config.ProjectConfigPath()).
			Msg("failed to load project config - falling back to defaults")
		cfg = config.DefaultConfig()
	}

	if cfgErr == nil {
		logConfigSources(cfg, logger)
	}

	// Load template registry with custom templates from config
	registry, err := template.NewRegistryWithConfig(repoPath, cfg.Templates.CustomTemplates)
	if err != nil {
		return sc.handleError("", fmt.Errorf("failed to load templates: %w", err))
	}

	// Select template
	tmpl, err := selectTemplate(ctx, registry, opts.templateName, opts.noInteractive, outputFormat) //nolint:contextcheck // context is properly checked and used
	if err != nil {
		return sc.handleError("", err)
	}

	logger.Debug().
		Str("template_name", tmpl.Name).
		Msg("template selected")

	// Determine workspace name
	wsName := opts.workspaceName
	if wsName == "" {
		wsName = generateWorkspaceName(description)
	} else {
		wsName = sanitizeWorkspaceName(wsName)
	}

	// Apply verify flag overrides to template (needed for dry-run too)
	applyVerifyOverrides(tmpl, opts.verify, opts.noVerify)

	// Handle dry-run mode - show what would happen without making changes
	if opts.dryRun {
		return runDryRun(ctx, sc, tmpl, description, wsName, cfg, logger) //nolint:contextcheck // context is properly checked and used
	}

	// Create and configure workspace
	ws, err := createWorkspace(ctx, sc, wsName, repoPath, tmpl.BranchPrefix, opts.baseBranch, opts.useLocal) //nolint:contextcheck // context is properly checked and used
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	logger.Info().
		Str("workspace_name", ws.Name).
		Str("branch", ws.Branch).
		Str("worktree_path", ws.WorktreePath).
		Msg("workspace created")

	// Start task execution
	t, err := startTaskExecution(ctx, ws, tmpl, description, opts.agent, opts.model, logger, out) //nolint:contextcheck // context is properly checked and used

	// Check if we were interrupted by Ctrl+C
	select {
	case <-sigHandler.Interrupted():
		return handleInterruption(ctx, sc, ws, t, logger, out) //nolint:contextcheck // context is properly checked and used
	default:
	}

	if err != nil {
		sc.handleTaskStartError(ctx, ws, repoPath, t, logger) //nolint:contextcheck // context is properly checked and used
		if t != nil {
			return displayTaskStatus(out, outputFormat, ws, t, err)
		}
		return sc.handleError(wsName, fmt.Errorf("failed to start task: %w", err))
	}

	logger.Info().
		Str("task_id", t.ID).
		Str("workspace_name", ws.Name).
		Str("template_name", tmpl.Name).
		Int("total_steps", len(t.Steps)).
		Msg("task started")

	return displayTaskStatus(out, outputFormat, ws, t, nil)
}

// logConfigSources logs which config sources were loaded and key metrics.
func logConfigSources(cfg *config.Config, logger zerolog.Logger) {
	// Determine which config files were loaded
	var sources []string
	var globalPathErr error
	var globalPath string
	globalPath, globalPathErr = config.GlobalConfigPath()
	if globalPathErr == nil {
		if _, statErr := os.Stat(globalPath); statErr == nil {
			sources = append(sources, "global")
		}
	}
	projectPath := config.ProjectConfigPath()
	if _, statErr := os.Stat(projectPath); statErr == nil {
		sources = append(sources, "project")
	}
	if len(sources) == 0 {
		sources = []string{"defaults"}
	}

	// Count validation commands
	validationCmds := len(cfg.Validation.Commands.Format) +
		len(cfg.Validation.Commands.Lint) +
		len(cfg.Validation.Commands.Test) +
		len(cfg.Validation.Commands.PreCommit) +
		len(cfg.Validation.Commands.CustomPrePR)

	// Log with sources and key metrics
	logger.Debug().
		Str("sources", strings.Join(sources, ",")).
		Str("agent", cfg.AI.Agent).
		Str("model", cfg.AI.Model).
		Int("custom_templates", len(cfg.Templates.CustomTemplates)).
		Int("required_workflows", len(cfg.CI.RequiredWorkflows)).
		Int("validation_cmds", validationCmds).
		Msg("config loaded")
}

// handleError handles errors based on output format.
func (sc *startContext) handleError(wsName string, err error) error {
	if sc.outputFormat == OutputJSON {
		return outputStartErrorJSON(sc.w, wsName, "", err.Error())
	}
	return err
}

// handleTaskStartError handles cleanup when task execution fails.
// Only cleans up workspace if the task was never created (t == nil).
// If the task exists, the workspace must be preserved for investigation and resume.
func (sc *startContext) handleTaskStartError(ctx context.Context, ws *domain.Workspace, repoPath string, t *domain.Task, logger zerolog.Logger) {
	// If task was created, workspace should be preserved for resume
	if t != nil {
		logger.Debug().
			Str("workspace_name", ws.Name).
			Str("task_id", t.ID).
			Str("task_status", string(t.Status)).
			Msg("preserving workspace for resume (task exists)")

		sc.updateWorkspaceStatusToPaused(ctx, ws, logger)
		return
	}

	// Only cleanup if task creation failed entirely (no task to preserve)
	logger.Debug().
		Str("workspace_name", ws.Name).
		Msg("task was never created, destroying workspace")
	cleanupErr := cleanupWorkspace(ctx, ws.Name, repoPath)
	if cleanupErr != nil {
		logger.Warn().Err(cleanupErr).
			Str("workspace_name", ws.Name).
			Msg("failed to cleanup workspace after task creation failure")
	}
}

// updateWorkspaceStatusToPaused updates the workspace status to paused to preserve it for resume.
func (sc *startContext) updateWorkspaceStatusToPaused(ctx context.Context, ws *domain.Workspace, logger zerolog.Logger) {
	ws.Status = constants.WorkspaceStatusPaused
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		logger.Error().Err(err).
			Str("workspace_name", ws.Name).
			Msg("CRITICAL: failed to create workspace store for pause update - workspace may not be resumable")
		return
	}

	updateErr := wsStore.Update(ctx, ws)
	if updateErr != nil {
		logger.Error().Err(updateErr).
			Str("workspace_name", ws.Name).
			Msg("CRITICAL: failed to persist workspace pause status - workspace may not be resumable")
		return
	}

	logger.Debug().
		Str("workspace_name", ws.Name).
		Str("workspace_status", string(ws.Status)).
		Msg("workspace status updated to paused for resume")

	// Verify worktree still exists after pause to detect race conditions
	if ws.WorktreePath != "" {
		if _, statErr := os.Stat(ws.WorktreePath); os.IsNotExist(statErr) {
			logger.Error().
				Str("workspace_name", ws.Name).
				Str("worktree_path", ws.WorktreePath).
				Msg("CRITICAL: worktree directory missing after pause - possible race condition or external deletion")
		}
	}
}

// handleInterruption handles graceful shutdown when user presses Ctrl+C.
// It saves the task and workspace state so the user can resume later.
func handleInterruption(ctx context.Context, sc *startContext, ws *domain.Workspace, t *domain.Task, logger zerolog.Logger, out tui.Output) error {
	logger.Info().
		Str("workspace_name", ws.Name).
		Str("task_id", safeTaskID(t)).
		Msg("received interrupt signal, initiating graceful shutdown")

	out.Warning("\n⚠ Interrupt received - saving state...")

	// Use a context without cancellation for cleanup since the original is canceled
	cleanupCtx := context.WithoutCancel(ctx)

	// Save interrupted task state
	if t != nil {
		saveInterruptedTaskState(cleanupCtx, ws, t, logger)
	}

	// Update workspace to paused
	sc.updateWorkspaceStatusToPaused(cleanupCtx, ws, logger)

	// Display summary
	displayInterruptionSummary(out, ws, t)

	return atlaserrors.ErrTaskInterrupted
}

// saveInterruptedTaskState saves the task state when interrupted by Ctrl+C.
func saveInterruptedTaskState(ctx context.Context, ws *domain.Workspace, t *domain.Task, logger zerolog.Logger) {
	// Transition task to interrupted status if it's running or validating
	if t.Status == constants.TaskStatusRunning || t.Status == constants.TaskStatusValidating {
		if err := task.Transition(ctx, t, constants.TaskStatusInterrupted, "user pressed Ctrl+C"); err != nil {
			logger.Error().Err(err).
				Str("task_id", t.ID).
				Str("from_status", string(t.Status)).
				Msg("failed to transition task to interrupted status")
		}
	}

	// Save task state
	taskStore, err := task.NewFileStore("")
	if err != nil {
		logger.Error().Err(err).Msg("failed to create task store for interrupted state save")
		return
	}

	if saveErr := taskStore.Update(ctx, ws.Name, t); saveErr != nil {
		logger.Error().Err(saveErr).
			Str("task_id", t.ID).
			Str("workspace_name", ws.Name).
			Msg("failed to save interrupted task state")
	} else {
		logger.Debug().
			Str("task_id", t.ID).
			Str("status", string(t.Status)).
			Int("current_step", t.CurrentStep).
			Msg("interrupted task state saved")
	}
}

// displayInterruptionSummary shows the user what happened and how to resume.
func displayInterruptionSummary(out tui.Output, ws *domain.Workspace, t *domain.Task) {
	out.Success("\n✓ Task state saved")
	out.Info("─────────────────────────────────────────")
	out.Info(fmt.Sprintf("📁 Workspace:    %s", ws.Name))
	out.Info(fmt.Sprintf("📁 Worktree:     %s", ws.WorktreePath))

	if t != nil {
		out.Info(fmt.Sprintf("📋 Task:         %s", t.ID))
		out.Info(fmt.Sprintf("📊 Status:       %s", t.Status))
		if t.CurrentStep < len(t.Steps) {
			out.Info(fmt.Sprintf("⏸ Stopped at:    Step %d/%d (%s)", t.CurrentStep+1, len(t.Steps), t.Steps[t.CurrentStep].Name))
		}
	}

	out.Info("")
	out.Info(fmt.Sprintf("▶ To resume:  atlas resume %s", ws.Name))
	out.Info("")
	out.Info("💡 Your workspace and all changes are preserved.")
}

// safeTaskID returns the task ID or a placeholder if task is nil.
func safeTaskID(t *domain.Task) string {
	if t == nil {
		return "(none)"
	}
	return t.ID
}

// createWorkspace creates a new workspace or uses an existing one (upsert behavior).
// If a workspace with the given name already exists and is active/paused, it will be reused.
// If a closed workspace with the same name exists, it will be automatically cleaned up and a new workspace created.
func createWorkspace(ctx context.Context, sc *startContext, wsName, repoPath, branchPrefix, baseBranch string, useLocal bool) (*domain.Workspace, error) {
	logger := Logger()

	// Create workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return nil, sc.handleError(wsName, fmt.Errorf("failed to create workspace store: %w", err))
	}

	// Create worktree runner
	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, repoPath, logger)
	if err != nil {
		return nil, sc.handleError(wsName, fmt.Errorf("failed to create worktree runner: %w", err))
	}

	// Create manager
	wsMgr := workspace.NewManager(wsStore, wtRunner, logger)

	// Check if workspace already exists (upsert behavior)
	existingWs, err := wsMgr.Get(ctx, wsName)
	if err == nil && existingWs != nil {
		// Check if workspace is closed (archived)
		if existingWs.Status == constants.WorkspaceStatusClosed {
			logger.Info().
				Str("workspace_name", wsName).
				Msg("workspace is closed, creating new workspace with same name")
			// Fall through to create new workspace
		} else {
			// Workspace exists and is active/paused - reuse it
			logger.Info().
				Str("workspace_name", wsName).
				Str("worktree_path", existingWs.WorktreePath).
				Str("status", string(existingWs.Status)).
				Msg("using existing workspace")
			return existingWs, nil
		}
	}

	// Workspace doesn't exist - create new
	ws, err := wsMgr.Create(ctx, workspace.CreateOptions{
		Name:       wsName,
		RepoPath:   repoPath,
		BranchType: branchPrefix,
		BaseBranch: baseBranch,
		UseLocal:   useLocal,
	})
	if err != nil {
		return nil, sc.handleError(wsName, fmt.Errorf("failed to create workspace: %w", err))
	}

	return ws, nil
}

// startTaskExecution creates and starts the task engine.
func startTaskExecution(ctx context.Context, ws *domain.Workspace, tmpl *domain.Template, description, agent, model string, logger zerolog.Logger, out tui.Output) (*domain.Task, error) {
	// Create task store and load config
	taskStore, cfg, err := setupTaskStoreAndConfig(ctx, logger)
	if err != nil {
		return nil, err
	}

	// Create notifiers
	notifier, stateNotifier := createNotifiers(cfg)

	// Create AI runner
	aiRunner := createAIRunner(cfg, logger)

	// Create git services
	gitRunner, smartCommitter, pusher, hubRunner, prDescGen, ciFailureHandler, err := createGitServices(ctx, ws.WorktreePath, cfg, aiRunner, logger)
	if err != nil {
		return nil, err
	}

	// Create executor registry
	execRegistry := createExecutorRegistry(ws.WorktreePath, taskStore, notifier, aiRunner, logger,
		smartCommitter, pusher, hubRunner, prDescGen, gitRunner, ciFailureHandler, cfg)

	// Create validation retry handler for automatic AI-assisted fixes
	validationRetryHandler := createValidationRetryHandler(aiRunner, cfg, logger)
	logger.Debug().
		Bool("handler_created", validationRetryHandler != nil).
		Bool("ai_retry_enabled", cfg.Validation.AIRetryEnabled).
		Int("max_retry_attempts", cfg.Validation.MaxAIRetryAttempts).
		Msg("validation retry handler status")

	// Create engine with progress callback
	engine := createEngine(ctx, taskStore, execRegistry, logger, stateNotifier, out, ws.Name, validationRetryHandler)

	// Apply agent and model overrides to template
	applyAgentModelOverrides(tmpl, agent, model)

	// Start task
	return startTask(ctx, engine, ws, tmpl, description, logger)
}

// setupTaskStoreAndConfig creates the task store and loads configuration.
func setupTaskStoreAndConfig(ctx context.Context, logger zerolog.Logger) (*task.FileStore, *config.Config, error) {
	taskStore, err := task.NewFileStore("")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create task store: %w", err)
	}

	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	return taskStore, cfg, nil
}

// createNotifiers creates UI and state change notifiers.
func createNotifiers(cfg *config.Config) (*tui.Notifier, *task.StateChangeNotifier) {
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)
	stateNotifier := task.NewStateChangeNotifier(task.NotificationConfig{
		BellEnabled: cfg.Notifications.Bell,
		Quiet:       false, // TODO: Pass quiet flag through when available
		Events:      cfg.Notifications.Events,
	})
	return notifier, stateNotifier
}

// createAIRunner creates and configures the AI runner with all supported agents.
func createAIRunner(cfg *config.Config, logger zerolog.Logger) ai.Runner {
	runnerRegistry := ai.NewRunnerRegistry()
	runnerRegistry.Register(domain.AgentClaude, ai.NewClaudeCodeRunner(&cfg.AI, nil))
	runnerRegistry.Register(domain.AgentGemini, ai.NewGeminiRunnerWithLogger(&cfg.AI, nil, logger))
	runnerRegistry.Register(domain.AgentCodex, ai.NewCodexRunner(&cfg.AI, nil))
	return ai.NewMultiRunner(runnerRegistry)
}

// createGitServices creates all git-related services.
func createGitServices(ctx context.Context, worktreePath string, cfg *config.Config, aiRunner ai.Runner, logger zerolog.Logger) (git.Runner, *git.SmartCommitRunner, *git.PushRunner, *git.CLIGitHubRunner, *git.AIDescriptionGenerator, *task.CIFailureHandler, error) {
	gitRunner, err := git.NewRunner(ctx, worktreePath)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to create git runner: %w", err)
	}

	// Resolve commit agent/model with fallback to global AI config
	commitAgent := cfg.SmartCommit.Agent
	if commitAgent == "" {
		commitAgent = cfg.AI.Agent
	}
	commitModel := cfg.SmartCommit.Model
	if commitModel == "" {
		commitModel = cfg.AI.Model
	}

	// Resolve PR description agent/model with fallback to global AI config
	prDescAgent := cfg.PRDescription.Agent
	if prDescAgent == "" {
		prDescAgent = cfg.AI.Agent
	}
	prDescModel := cfg.PRDescription.Model
	if prDescModel == "" {
		prDescModel = cfg.AI.Model
	}

	// Resolve smart commit timeout/retry settings with defaults
	commitTimeout := cfg.SmartCommit.Timeout
	if commitTimeout == 0 {
		commitTimeout = 30 * time.Second
	}
	commitMaxRetries := cfg.SmartCommit.MaxRetries
	if commitMaxRetries == 0 {
		commitMaxRetries = 2
	}
	commitRetryBackoffFactor := cfg.SmartCommit.RetryBackoffFactor
	if commitRetryBackoffFactor == 0 {
		commitRetryBackoffFactor = 1.5
	}

	smartCommitter := git.NewSmartCommitRunner(gitRunner, worktreePath, aiRunner,
		git.WithAgent(commitAgent),
		git.WithModel(commitModel),
		git.WithTimeout(commitTimeout),
		git.WithMaxRetries(commitMaxRetries),
		git.WithRetryBackoffFactor(commitRetryBackoffFactor),
		git.WithLogger(logger),
	)
	pusher := git.NewPushRunner(gitRunner)
	hubRunner := git.NewCLIGitHubRunner(worktreePath)
	prDescGen := git.NewAIDescriptionGenerator(aiRunner,
		git.WithAIDescAgent(prDescAgent),
		git.WithAIDescModel(prDescModel),
		git.WithAIDescLogger(logger),
	)
	ciFailureHandler := task.NewCIFailureHandler(hubRunner)

	return gitRunner, smartCommitter, pusher, hubRunner, prDescGen, ciFailureHandler, nil
}

// createExecutorRegistry creates the step executor registry with all dependencies.
func createExecutorRegistry(workDir string, taskStore *task.FileStore, notifier *tui.Notifier, aiRunner ai.Runner, logger zerolog.Logger, smartCommitter *git.SmartCommitRunner, pusher *git.PushRunner, hubRunner *git.CLIGitHubRunner, prDescGen *git.AIDescriptionGenerator, gitRunner git.Runner, ciFailureHandler *task.CIFailureHandler, cfg *config.Config) *steps.ExecutorRegistry {
	return steps.NewDefaultRegistry(steps.ExecutorDeps{
		WorkDir:                workDir,
		ArtifactSaver:          taskStore,
		Notifier:               notifier,
		AIRunner:               aiRunner,
		Logger:                 logger,
		SmartCommitter:         smartCommitter,
		Pusher:                 pusher,
		HubRunner:              hubRunner,
		PRDescriptionGenerator: prDescGen,
		GitRunner:              gitRunner,
		CIFailureHandler:       ciFailureHandler,
		BaseBranch:             cfg.Git.BaseBranch,
		CIConfig:               &cfg.CI,
		FormatCommands:         cfg.Validation.Commands.Format,
		LintCommands:           cfg.Validation.Commands.Lint,
		TestCommands:           cfg.Validation.Commands.Test,
		PreCommitCommands:      cfg.Validation.Commands.PreCommit,
	})
}

// createEngine creates the task engine with progress callback.
func createEngine(ctx context.Context, taskStore *task.FileStore, execRegistry *steps.ExecutorRegistry, _ zerolog.Logger, stateNotifier *task.StateChangeNotifier, out tui.Output, wsName string, validationRetryHandler *validation.RetryHandler) *task.Engine {
	engineCfg := task.DefaultEngineConfig()
	engineCfg.ProgressCallback = createProgressCallback(ctx, out, wsName)

	taskLogger := LoggerWithTaskStore(taskStore)
	opts := []task.EngineOption{
		task.WithNotifier(stateNotifier),
	}
	if validationRetryHandler != nil {
		opts = append(opts, task.WithValidationRetryHandler(validationRetryHandler))
	}
	return task.NewEngine(taskStore, execRegistry, engineCfg, taskLogger, opts...)
}

// createValidationRetryHandler creates the validation retry handler for automatic AI-assisted fixes.
func createValidationRetryHandler(aiRunner ai.Runner, cfg *config.Config, logger zerolog.Logger) *validation.RetryHandler {
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
		logger,
	)
}

// createProgressCallback creates the progress callback for UI feedback.
func createProgressCallback(_ context.Context, out tui.Output, _ string) func(task.StepProgressEvent) {
	logPathShown := false

	return func(event task.StepProgressEvent) {
		switch event.Type {
		case "start":
			handleProgressStart(out, event, &logPathShown)
		case "complete":
			handleProgressComplete(out, event)
		}
	}
}

// handleProgressStart handles the start event of a step progress.
func handleProgressStart(out tui.Output, event task.StepProgressEvent, logPathShown *bool) {
	// Show log path on first step start
	if !*logPathShown && event.TaskID != "" {
		logPath := fmt.Sprintf("~/.atlas/workspaces/%s/tasks/%s/task.log", event.WorkspaceName, event.TaskID)
		out.Info(fmt.Sprintf("Logs: %s", logPath))
		*logPathShown = true
	}

	// Print step start message (static, not animated spinner, to avoid conflicts with log output)
	msg := buildStepStartMessage(event)
	out.Info(msg)
}

// buildStepStartMessage builds the step start message based on the event.
func buildStepStartMessage(event task.StepProgressEvent) string {
	if event.Agent != "" && event.Model != "" {
		return fmt.Sprintf("Step %d/%d: %s (%s/%s)...", event.StepIndex+1, event.TotalSteps, event.StepName, event.Agent, event.Model)
	}
	return fmt.Sprintf("Step %d/%d: %s...", event.StepIndex+1, event.TotalSteps, event.StepName)
}

// handleProgressComplete handles the complete event of a step progress.
func handleProgressComplete(out tui.Output, event task.StepProgressEvent) {
	// Display completion message
	statusMsg := fmt.Sprintf("Step %d/%d: %s completed", event.StepIndex+1, event.TotalSteps, event.StepName)
	out.Success(statusMsg)

	// Display metrics for AI steps
	if event.Agent != "" && (event.DurationMs > 0 || event.NumTurns > 0 || event.FilesChangedCount > 0) {
		metrics := buildStepMetrics(event.DurationMs, event.NumTurns, event.FilesChangedCount)
		if metrics != "" {
			out.Info(fmt.Sprintf("  %s", metrics))
		}
	}

	// Display PR URL if present
	displayPRURL(out, event.Output)
}

// displayPRURL displays PR URLs from the output if present.
func displayPRURL(out tui.Output, output string) {
	if output != "" && strings.Contains(output, "Created PR #") {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
				out.URL(line, line)
			}
		}
	}
}

// applyAgentModelOverrides applies agent and model overrides to the template.
func applyAgentModelOverrides(tmpl *domain.Template, agent, model string) {
	if agent != "" {
		tmpl.DefaultAgent = domain.Agent(agent)
	}
	if model != "" {
		tmpl.DefaultModel = model
	}
}

// startTask starts the task execution and handles errors.
func startTask(ctx context.Context, engine *task.Engine, ws *domain.Workspace, tmpl *domain.Template, description string, logger zerolog.Logger) (*domain.Task, error) {
	t, err := engine.Start(ctx, ws.Name, ws.Branch, ws.WorktreePath, tmpl, description)
	if err != nil {
		logger.Error().Err(err).
			Str("workspace_name", ws.Name).
			Msg("task start failed")
		return t, err
	}
	return t, nil
}

// generateWorkspaceName creates a sanitized workspace name from description.
func generateWorkspaceName(description string) string {
	name := sanitizeWorkspaceName(description)

	// Handle empty result
	if name == "" {
		name = fmt.Sprintf("task-%s", time.Now().Format("20060102-150405"))
	}

	return name
}

// sanitizeWorkspaceName sanitizes a string for use as a workspace name.
func sanitizeWorkspaceName(input string) string {
	// Lowercase and replace spaces with hyphens
	name := strings.ToLower(input)
	name = strings.ReplaceAll(name, " ", "-")

	// Remove special characters
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Collapse multiple hyphens
	name = multipleHyphensRegex.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Truncate to max length
	if len(name) > maxWorkspaceNameLen {
		name = name[:maxWorkspaceNameLen]
		// Don't end with a hyphen
		name = strings.TrimRight(name, "-")
	}

	return name
}

// selectTemplate handles template selection based on flags and interactivity mode.
func selectTemplate(ctx context.Context, registry *template.Registry, templateName string, noInteractive bool, outputFormat string) (*domain.Template, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// If template specified via flag, use it directly
	if templateName != "" {
		tmpl, err := registry.Get(templateName)
		if err != nil {
			return nil, fmt.Errorf("template '%s' not found: %w", templateName, atlaserrors.ErrTemplateNotFound)
		}
		return tmpl, nil
	}

	// Non-interactive mode or JSON output requires template flag
	if noInteractive || outputFormat == OutputJSON || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("use --template to specify template: %w", atlaserrors.ErrTemplateRequired))
	}

	return selectTemplateInteractive(registry)
}

// selectTemplateInteractive displays an interactive template selection menu.
func selectTemplateInteractive(registry *template.Registry) (*domain.Template, error) {
	templates := registry.List()
	options := make([]huh.Option[string], 0, len(templates))
	for _, t := range templates {
		label := fmt.Sprintf("%s - %s", t.Name, t.Description)
		options = append(options, huh.NewOption(label, t.Name))
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a template").
				Description("Choose the workflow template for this task").
				Options(options...).
				Value(&selected),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("template selection canceled: %w", err)
	}

	return registry.Get(selected)
}

// handleWorkspaceConflict checks for existing workspace and handles conflicts.
func handleWorkspaceConflict(ctx context.Context, mgr *workspace.DefaultManager, wsName string, noInteractive bool, outputFormat string, out tui.Output, w io.Writer) (string, error) { //nolint:unparam // out reserved for future use
	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	exists, err := mgr.Exists(ctx, wsName)
	if err != nil {
		return "", fmt.Errorf("failed to check workspace existence: %w", err)
	}

	if !exists {
		return wsName, nil
	}

	// Workspace exists - handle conflict
	if noInteractive || outputFormat == OutputJSON {
		if outputFormat == OutputJSON {
			return "", outputStartErrorJSON(w, wsName, "", fmt.Sprintf("workspace '%s': %s", wsName, atlaserrors.ErrWorkspaceExists.Error()))
		}
		return "", atlaserrors.NewExitCode2Error(
			fmt.Errorf("workspace '%s': %w", wsName, atlaserrors.ErrWorkspaceExists))
	}

	// Check if we're in a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("workspace '%s': %w (use --workspace to specify a different name)", wsName, atlaserrors.ErrWorkspaceExists)
	}

	return resolveWorkspaceConflictInteractive(wsName, out)
}

// resolveWorkspaceConflictInteractive handles workspace conflict interactively.
func resolveWorkspaceConflictInteractive(wsName string, out tui.Output) (string, error) {
	action, err := promptWorkspaceConflict(wsName)
	if err != nil {
		return "", fmt.Errorf("failed to get user choice: %w", err)
	}

	switch action {
	case "resume":
		return "", atlaserrors.ErrResumeNotImplemented
	case "new":
		newName, err := promptNewWorkspaceName()
		if err != nil {
			return "", fmt.Errorf("failed to get new workspace name: %w", err)
		}
		return sanitizeWorkspaceName(newName), nil
	case "cancel":
		out.Info("Operation canceled")
		return "", atlaserrors.ErrOperationCanceled
	}

	return wsName, nil
}

// promptWorkspaceConflict prompts the user to resolve a workspace name conflict.
func promptWorkspaceConflict(name string) (string, error) {
	var action string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Workspace '%s' exists", name)).
				Description("What would you like to do?").
				Options(
					huh.NewOption("Resume existing workspace", "resume"),
					huh.NewOption("Use a different name", "new"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return "", err
	}

	return action, nil
}

// promptNewWorkspaceName prompts the user for a new workspace name.
func promptNewWorkspaceName() (string, error) {
	var name string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter new workspace name").
				Value(&name).
				Validate(validateWorkspaceName),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return "", err
	}

	return name, nil
}

// validateWorkspaceName validates a workspace name input.
func validateWorkspaceName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("name required: %w", atlaserrors.ErrEmptyValue)
	}
	return nil
}

// findGitRepository finds the git repository root from the current directory.
// Uses git rev-parse for accurate detection even in worktrees.
func findGitRepository(ctx context.Context) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	info, err := git.DetectRepo(ctx, cwd)
	if err != nil {
		return "", atlaserrors.ErrNotGitRepo
	}

	return info.WorktreePath, nil
}

// startResponse represents the JSON output for start operations.
type startResponse struct {
	Success   bool          `json:"success"`
	Workspace workspaceInfo `json:"workspace"`
	Task      taskInfo      `json:"task"`
	Error     string        `json:"error,omitempty"`
}

// workspaceInfo contains workspace details for JSON output.
type workspaceInfo struct {
	Name         string `json:"name"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktree_path"`
	Status       string `json:"status"`
}

// taskInfo contains task details for JSON output.
type taskInfo struct {
	ID           string `json:"task_id"`
	TemplateName string `json:"template_name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	CurrentStep  int    `json:"current_step"`
	TotalSteps   int    `json:"total_steps"`
}

// cleanupWorkspace removes a workspace after a failed task start.
// This calls Destroy() (complete removal), not Close() (archive).
func cleanupWorkspace(ctx context.Context, wsName, repoPath string) error {
	logger := Logger()
	logger.Debug().
		Str("workspace_name", wsName).
		Str("repo_path", repoPath).
		Msg("cleanupWorkspace called - will call Destroy() (not Close())")

	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, repoPath, logger)
	if err != nil {
		return fmt.Errorf("failed to create worktree runner: %w", err)
	}

	mgr := workspace.NewManager(wsStore, wtRunner, logger)
	return mgr.Destroy(ctx, wsName)
}

// isValidAgent checks if the agent name is valid.
func isValidAgent(agent string) bool {
	a := domain.Agent(agent)
	return a.IsValid()
}

// validateAgent checks if the agent name is valid.
func validateAgent(agent string) error {
	if agent == "" {
		return nil // Empty is valid (use default)
	}
	if !isValidAgent(agent) {
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: '%s' (must be one of claude, gemini, codex)", atlaserrors.ErrAgentNotFound, agent))
	}
	return nil
}

// isValidModelForAgent checks if the model name is valid for the given agent.
func isValidModelForAgent(agent, model string) bool {
	// If no agent specified, validate against Claude (default)
	a := domain.Agent(agent)
	if a == "" {
		a = domain.AgentClaude
	}

	// Check if model is in the agent's valid aliases
	for _, alias := range a.ModelAliases() {
		if model == alias {
			return true
		}
	}
	return false
}

// validateModel checks if the model name is valid for the given agent.
func validateModel(agent, model string) error {
	if model == "" {
		return nil // Empty is valid (use default)
	}

	// If agent not specified, check against all agents
	if agent == "" {
		// Accept models from either agent
		if isValidModelForAgent("claude", model) || isValidModelForAgent("gemini", model) {
			return nil
		}
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: '%s' (must be sonnet, opus, haiku for claude or flash, pro for gemini)", atlaserrors.ErrInvalidModel, model))
	}

	// Validate against specific agent
	if !isValidModelForAgent(agent, model) {
		a := domain.Agent(agent)
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: '%s' is not valid for agent '%s' (valid models: %v)", atlaserrors.ErrInvalidModel, model, agent, a.ModelAliases()))
	}
	return nil
}

// applyVerifyOverrides applies --verify or --no-verify flag overrides to the template.
// If neither flag is set, the template's default Verify setting is used.
// Also propagates VerifyModel from template to the verify step config, but only if
// the step doesn't have a different agent override (since VerifyModel may not be
// compatible with other agents).
func applyVerifyOverrides(tmpl *domain.Template, verify, noVerify bool) {
	// CLI flags override template defaults
	if verify {
		tmpl.Verify = true
	} else if noVerify {
		tmpl.Verify = false
	}

	// Update the verify step's Required field and model based on the template settings
	for i := range tmpl.Steps {
		if tmpl.Steps[i].Type == domain.StepTypeVerify {
			applyVerifyToStep(tmpl, &tmpl.Steps[i])
		}
	}
}

// applyVerifyToStep applies verify settings to a single verify step.
func applyVerifyToStep(tmpl *domain.Template, step *domain.StepDefinition) {
	step.Required = tmpl.Verify

	// Check if step has a different agent override
	stepHasDifferentAgent := stepHasDifferentAgent(step, tmpl.DefaultAgent)

	// Propagate VerifyModel from template to step config if applicable
	if shouldPropagateVerifyModel(tmpl.VerifyModel, stepHasDifferentAgent) {
		propagateVerifyModel(step, tmpl.VerifyModel)
	}
}

// stepHasDifferentAgent checks if the step has an agent override different from the default.
func stepHasDifferentAgent(step *domain.StepDefinition, defaultAgent domain.Agent) bool {
	if step.Config == nil {
		return false
	}

	stepAgent, ok := step.Config["agent"].(string)
	if !ok || stepAgent == "" {
		return false
	}

	return domain.Agent(stepAgent) != defaultAgent
}

// shouldPropagateVerifyModel determines if VerifyModel should be propagated to the step.
func shouldPropagateVerifyModel(verifyModel string, stepHasDifferentAgent bool) bool {
	return verifyModel != "" && !stepHasDifferentAgent
}

// propagateVerifyModel sets the VerifyModel on a step's config if not already set.
func propagateVerifyModel(step *domain.StepDefinition, verifyModel string) {
	if step.Config == nil {
		step.Config = make(map[string]any)
	}

	// Only set if not already configured in step
	if model, ok := step.Config["model"].(string); !ok || model == "" {
		step.Config["model"] = verifyModel
	}
}

// displayTaskStatus outputs the task status in the appropriate format.
func displayTaskStatus(out tui.Output, format string, ws *domain.Workspace, t *domain.Task, execErr error) error {
	if format == OutputJSON {
		resp := startResponse{
			Success: execErr == nil,
			Workspace: workspaceInfo{
				Name:         ws.Name,
				Branch:       ws.Branch,
				WorktreePath: ws.WorktreePath,
				Status:       string(ws.Status),
			},
			Task: taskInfo{
				ID:           t.ID,
				TemplateName: t.TemplateID,
				Description:  t.Description,
				Status:       string(t.Status),
				CurrentStep:  t.CurrentStep,
				TotalSteps:   len(t.Steps),
			},
		}
		if execErr != nil {
			resp.Error = execErr.Error()
		}
		return out.JSON(resp)
	}

	// TTY output
	out.Success(fmt.Sprintf("Task started: %s", t.ID))
	out.Info(fmt.Sprintf("  Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("  Branch:    %s", ws.Branch))
	out.Info(fmt.Sprintf("  Template:  %s", t.TemplateID))
	out.Info(fmt.Sprintf("  Status:    %s", t.Status))
	out.Info(fmt.Sprintf("  Progress:  Step %d/%d", t.CurrentStep+1, len(t.Steps)))

	if execErr != nil {
		out.Warning(fmt.Sprintf("Execution paused: %s", execErr.Error()))

		// Display manual fix instructions for validation failures
		if t.Status == constants.TaskStatusValidationFailed {
			tui.DisplayManualFixInstructions(out, t, ws)
		}
	}

	return nil
}

// outputStartErrorJSON outputs an error result as JSON.
func outputStartErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	resp := startResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: workspaceName,
		},
		Task: taskInfo{
			ID: taskID,
		},
		Error: errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// dryRunResponse represents the JSON output for dry-run mode.
type dryRunResponse struct {
	DryRun    bool                `json:"dry_run"`
	Template  string              `json:"template"`
	Workspace dryRunWorkspaceInfo `json:"workspace"`
	Steps     []dryRunStepInfo    `json:"steps"`
	Summary   dryRunSummary       `json:"summary"`
}

// dryRunWorkspaceInfo contains simulated workspace details.
type dryRunWorkspaceInfo struct {
	Name        string `json:"name"`
	Branch      string `json:"branch"`
	WouldCreate bool   `json:"would_create"`
}

// dryRunStepInfo contains information about what a step would do.
type dryRunStepInfo struct {
	Index       int            `json:"index"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required"`
	Status      string         `json:"status"`
	WouldDo     []string       `json:"would_do"`
	Config      map[string]any `json:"config,omitempty"`
}

// dryRunSummary contains summary information.
type dryRunSummary struct {
	TotalSteps           int      `json:"total_steps"`
	SideEffectsPrevented []string `json:"side_effects_prevented"`
}

// getSideEffectForStepType returns the side effect description for a given step type.
func getSideEffectForStepType(step domain.StepDefinition) string {
	switch step.Type {
	case domain.StepTypeAI:
		return "AI execution (file modifications)"
	case domain.StepTypeValidation:
		return "Validation commands (format may modify files)"
	case domain.StepTypeGit:
		if op, ok := step.Config["operation"].(string); ok {
			switch op {
			case "commit":
				return "Git commits"
			case "push":
				return "Git push to remote"
			case "create_pr":
				return "Pull request creation"
			default:
				return "Git operations"
			}
		}
		return "Git operations"
	case domain.StepTypeVerify:
		return "AI verification"
	case domain.StepTypeSDD:
		return "SDD generation"
	case domain.StepTypeCI:
		return "CI execution"
	case domain.StepTypeHuman:
		return ""
	default:
		return ""
	}
}

// runDryRun simulates task execution without making any changes.
func runDryRun(ctx context.Context, sc *startContext, tmpl *domain.Template, description, wsName string, cfg *config.Config, logger zerolog.Logger) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger.Debug().
		Str("template", tmpl.Name).
		Str("workspace", wsName).
		Msg("running dry-run simulation")

	// Create simulated workspace info
	simulatedBranch := fmt.Sprintf("%s%s", tmpl.BranchPrefix, wsName)

	// Create dry-run executor registry
	dryRunRegistry := steps.NewDryRunRegistry(steps.ExecutorDeps{
		WorkDir:           "(simulated)",
		BaseBranch:        cfg.Git.BaseBranch,
		FormatCommands:    cfg.Validation.Commands.Format,
		LintCommands:      cfg.Validation.Commands.Lint,
		TestCommands:      cfg.Validation.Commands.Test,
		PreCommitCommands: cfg.Validation.Commands.PreCommit,
		CIConfig:          &cfg.CI,
	})

	// Create simulated task for dry-run
	simulatedTask := &domain.Task{
		Description: description,
		TemplateID:  tmpl.Name,
		Metadata: map[string]any{
			"branch": simulatedBranch,
		},
	}

	// Collect step plans
	stepPlans := make([]dryRunStepInfo, 0, len(tmpl.Steps))
	var sideEffects []string

	for i, step := range tmpl.Steps {
		simulatedTask.CurrentStep = i

		// Get dry-run executor for this step type
		executor, err := dryRunRegistry.Get(step.Type)
		if err != nil {
			logger.Warn().Err(err).Str("step_type", string(step.Type)).Msg("no executor for step type")
			continue
		}

		// Execute dry-run (returns plan, no side effects)
		result, err := executor.Execute(ctx, simulatedTask, &step)
		if err != nil {
			return sc.handleError(wsName, fmt.Errorf("dry-run failed for step %s: %w", step.Name, err))
		}

		// Extract plan from result metadata
		var wouldDo []string
		var stepConfig map[string]any
		if result.Metadata != nil {
			if plan, ok := result.Metadata["plan"].(*steps.DryRunPlan); ok {
				wouldDo = plan.WouldDo
				stepConfig = plan.Config
			}
		}

		stepPlans = append(stepPlans, dryRunStepInfo{
			Index:       i,
			Name:        step.Name,
			Type:        string(step.Type),
			Description: step.Description,
			Required:    step.Required,
			Status:      "would_execute",
			WouldDo:     wouldDo,
			Config:      stepConfig,
		})

		// Track side effects that would occur
		if sideEffect := getSideEffectForStepType(step); sideEffect != "" {
			sideEffects = append(sideEffects, sideEffect)
		}
	}

	// Add workspace creation to side effects
	sideEffects = append([]string{"Workspace creation (git worktree)"}, sideEffects...)

	// Output results
	if sc.outputFormat == OutputJSON {
		return outputDryRunJSON(sc.w, tmpl.Name, wsName, simulatedBranch, stepPlans, sideEffects)
	}

	return outputDryRunTTY(sc.out, tmpl, wsName, simulatedBranch, stepPlans, sideEffects)
}

// outputDryRunJSON outputs the dry-run results as JSON.
func outputDryRunJSON(w io.Writer, templateName, wsName, branch string, stepPlans []dryRunStepInfo, sideEffects []string) error {
	resp := dryRunResponse{
		DryRun:   true,
		Template: templateName,
		Workspace: dryRunWorkspaceInfo{
			Name:        wsName,
			Branch:      branch,
			WouldCreate: true,
		},
		Steps: stepPlans,
		Summary: dryRunSummary{
			TotalSteps:           len(stepPlans),
			SideEffectsPrevented: sideEffects,
		},
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// outputDryRunTTY outputs the dry-run results for terminal display.
func outputDryRunTTY(out tui.Output, tmpl *domain.Template, wsName, branch string, stepPlans []dryRunStepInfo, sideEffects []string) error {
	// Header
	out.Info("=== DRY-RUN MODE ===")
	out.Info("Showing what would happen without making changes.\n")

	// Workspace info
	out.Info(fmt.Sprintf("[0/%d] Workspace Creation", len(stepPlans)))
	out.Info(fmt.Sprintf("      Name:   %s", wsName))
	out.Info(fmt.Sprintf("      Branch: %s", branch))
	out.Info("      Status: WOULD CREATE\n")

	// Step details
	for _, step := range stepPlans {
		requiredStr := ""
		if !step.Required {
			requiredStr = " (optional)"
		}
		out.Info(fmt.Sprintf("[%d/%d] %s Step: '%s'%s", step.Index+1, len(stepPlans), step.Type, step.Name, requiredStr))

		if step.Description != "" {
			out.Info(fmt.Sprintf("      Description: %s", step.Description))
		}

		if len(step.WouldDo) > 0 {
			out.Info("      Would:")
			for _, action := range step.WouldDo {
				out.Info(fmt.Sprintf("        - %s", action))
			}
		}

		out.Info(fmt.Sprintf("      Status: %s\n", step.Status))
	}

	// Summary
	out.Info("=== Summary ===")
	out.Info(fmt.Sprintf("Template: %s", tmpl.Name))
	out.Info(fmt.Sprintf("Steps: %d total", len(stepPlans)))
	out.Info("Side Effects Prevented:")
	for _, effect := range sideEffects {
		out.Info(fmt.Sprintf("  - %s", effect))
	}
	out.Info("")
	out.Success("Run without --dry-run to execute.")

	return nil
}

// buildStepMetrics formats step completion metrics for display.
// Returns a formatted string like "Duration: 2m 15s | Turns: 4 | Files: 3"
func buildStepMetrics(durationMs int64, numTurns, filesChangedCount int) string {
	var parts []string

	if durationMs > 0 {
		parts = append(parts, "Duration: "+formatDuration(durationMs))
	}
	if numTurns > 0 {
		parts = append(parts, fmt.Sprintf("Turns: %d", numTurns))
	}
	if filesChangedCount > 0 {
		parts = append(parts, fmt.Sprintf("Files: %d", filesChangedCount))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// formatDuration converts milliseconds to a human-readable duration string.
func formatDuration(ms int64) string {
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	secs := seconds % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, secs)
}
