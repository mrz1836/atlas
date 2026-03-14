package workflow

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/daemon"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/validation"
	"github.com/mrz1836/atlas/internal/workspace"
)

// DaemonTaskExecutor implements daemon.TaskExecutor using the task engine layer.
// It bridges daemon TaskJob metadata to the full workspace / git / engine setup
// that is normally done by the CLI start and resume commands.
type DaemonTaskExecutor struct {
	logger    zerolog.Logger
	cfg       *config.Config
	logWriter *daemon.LogWriter // set by daemon after Redis connects
}

// NewDaemonTaskExecutor creates a DaemonTaskExecutor.
func NewDaemonTaskExecutor(cfg *config.Config, logger zerolog.Logger) *DaemonTaskExecutor {
	return &DaemonTaskExecutor{
		logger: logger,
		cfg:    cfg,
	}
}

// SetLogWriter injects the Redis log writer. Called by the daemon after Redis connects.
func (e *DaemonTaskExecutor) SetLogWriter(lw *daemon.LogWriter) {
	e.logWriter = lw
}

// Execute starts or resumes a task based on whether job.EngineTaskID is set.
func (e *DaemonTaskExecutor) Execute(ctx context.Context, job daemon.TaskJob) (string, string, error) {
	if job.EngineTaskID != "" {
		return e.resume(ctx, job)
	}
	return e.start(ctx, job)
}

// Abandon transitions a paused/failed task to abandoned state in the engine task store.
func (e *DaemonTaskExecutor) Abandon(ctx context.Context, job daemon.TaskJob, reason string) error {
	if job.EngineTaskID == "" || job.Workspace == "" {
		return nil // Nothing to abandon — engine task not yet created.
	}

	services := NewServiceFactory(e.logger).WithRepoPath(job.RepoPath)
	taskStore, err := services.CreateTaskStore()
	if err != nil {
		return fmt.Errorf("abandon: create task store: %w", err)
	}

	t, err := taskStore.Get(ctx, job.Workspace, job.EngineTaskID)
	if err != nil {
		return fmt.Errorf("abandon: get task %s: %w", job.EngineTaskID, err)
	}

	if transErr := task.Transition(ctx, t, constants.TaskStatusAbandoned, reason); transErr != nil {
		return fmt.Errorf("abandon: transition task: %w", transErr)
	}

	if updErr := taskStore.Update(ctx, job.Workspace, t); updErr != nil {
		return fmt.Errorf("abandon: update task: %w", updErr)
	}
	return nil
}

// start creates a new workspace and begins task execution.
func (e *DaemonTaskExecutor) start(ctx context.Context, job daemon.TaskJob) (string, string, error) {
	services := NewServiceFactory(e.logger).WithRepoPath(job.RepoPath)

	taskStore, err := services.CreateTaskStore()
	if err != nil {
		return "", "", fmt.Errorf("start: setup services: %w", err)
	}
	cfg := e.cfg

	wsName := job.Workspace
	if wsName == "" {
		wsName = GenerateWorkspaceName(job.Description)
	}

	worktreePath, branch, err := e.provisionWorkspace(ctx, job, wsName, cfg)
	if err != nil {
		return "", "", fmt.Errorf("start: provision workspace: %w", err)
	}

	eng, err := e.buildEngine(ctx, services, worktreePath, taskStore, cfg, job.TaskID)
	if err != nil {
		return "", "", fmt.Errorf("start: build engine: %w", err)
	}

	tmpl, err := e.resolveTemplate(job, cfg)
	if err != nil {
		return "", "", fmt.Errorf("start: resolve template: %w", err)
	}

	ApplyAgentModelOverrides(tmpl, job.Agent, job.Model)
	ApplyVerifyOverrides(tmpl, job.Verify, job.NoVerify)

	t, execErr := eng.Start(ctx, wsName, branch, worktreePath, tmpl, job.Description, "")
	if execErr != nil && t == nil {
		return "", "", execErr
	}

	finalStatus := ""
	engineTaskID := ""
	if t != nil {
		finalStatus = string(t.Status)
		engineTaskID = t.ID
	}
	return engineTaskID, finalStatus, execErr
}

// resume continues a paused or error-state task.
func (e *DaemonTaskExecutor) resume(ctx context.Context, job daemon.TaskJob) (string, string, error) {
	services := NewServiceFactory(e.logger).WithRepoPath(job.RepoPath)

	taskStore, err := services.CreateTaskStore()
	if err != nil {
		return "", "", fmt.Errorf("resume: setup services: %w", err)
	}
	cfg := e.cfg

	t, err := taskStore.Get(ctx, job.Workspace, job.EngineTaskID)
	if err != nil {
		return "", "", fmt.Errorf("resume: get task %s: %w", job.EngineTaskID, err)
	}

	// Apply approval metadata before resuming.
	if job.ApprovalChoice != "" || job.RejectFeedback != "" {
		if metaErr := applyApprovalMetadata(ctx, taskStore, t, job); metaErr != nil {
			return "", "", metaErr
		}
	}

	// Reconstruct workspace paths from persisted task metadata.
	worktreePath, _ := t.Metadata["worktree_dir"].(string)

	eng, err := e.buildEngine(ctx, services, worktreePath, taskStore, cfg, job.TaskID)
	if err != nil {
		return "", "", fmt.Errorf("resume: build engine: %w", err)
	}

	tmpl, err := e.resolveTemplate(job, cfg)
	if err != nil {
		return "", "", fmt.Errorf("resume: resolve template: %w", err)
	}

	ApplyCLIOverridesFromTask(t, tmpl)

	resumeErr := eng.Resume(ctx, t, tmpl)
	return job.EngineTaskID, string(t.Status), resumeErr
}

// applyApprovalMetadata stores approval choice and reject feedback in the task's
// metadata map and persists the update to the task store.
func applyApprovalMetadata(ctx context.Context, taskStore *task.FileStore, t *domain.Task, job daemon.TaskJob) error {
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	if job.ApprovalChoice != "" {
		t.Metadata["step_approval_choice"] = job.ApprovalChoice
	}
	if job.RejectFeedback != "" {
		t.Metadata["reject_feedback"] = job.RejectFeedback
	}
	if updErr := taskStore.Update(ctx, job.Workspace, t); updErr != nil {
		return fmt.Errorf("resume: update task metadata: %w", updErr)
	}
	return nil
}

// logStreamWriter adapts daemon.LogWriter to io.Writer for streaming live
// validation command output to the Redis log stream.
type logStreamWriter struct {
	logWriter *daemon.LogWriter
	ctx       context.Context //nolint:containedctx // carries context for async writes
	taskID    string
}

// Write implements io.Writer. Each call writes non-empty trimmed lines as
// debug-level log entries to the Redis log stream.
func (w *logStreamWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	_ = w.logWriter.Write(w.ctx, w.taskID, daemon.LogEntry{
		Level:   "debug",
		Message: line,
		Step:    "validate",
		Source:  "command",
	})
	return len(p), nil
}

// Ensure logStreamWriter implements io.Writer.
var _ io.Writer = (*logStreamWriter)(nil)

// buildEngine creates a fully-wired task engine for the given worktree path.
func (e *DaemonTaskExecutor) buildEngine(
	ctx context.Context,
	services *ServiceFactory,
	worktreePath string,
	taskStore *task.FileStore,
	cfg *config.Config,
	taskID string,
) (*task.Engine, error) {
	hookManager := services.CreateHookManager(cfg, e.logger)
	_, stateNotifier := services.CreateNotifiers(cfg)
	aiRunner := services.CreateAIRunner(cfg)

	gitCfgResolved := resolveGitCfgFromConfig(cfg)
	gitServices, err := services.CreateGitServices(ctx, worktreePath, cfg, aiRunner, gitCfgResolved)
	if err != nil {
		return nil, fmt.Errorf("create git services: %w", err)
	}

	validationRetryHandler := services.CreateValidationRetryHandler(aiRunner, cfg)

	var liveOut io.Writer
	if e.logWriter != nil {
		liveOut = &logStreamWriter{logWriter: e.logWriter, ctx: ctx, taskID: taskID}
	}

	execRegistry := services.CreateExecutorRegistry(RegistryDeps{
		WorkDir:                    worktreePath,
		TaskStore:                  taskStore,
		Notifier:                   nil, // no TUI in daemon mode
		AIRunner:                   aiRunner,
		Logger:                     e.logger,
		GitServices:                gitServices,
		Config:                     cfg,
		ValidationProgressCallback: e.makeValidationProgressCallback(ctx, taskID),
		ValidationLiveOutput:       liveOut,
	})

	return services.CreateEngine(EngineDeps{
		TaskStore:              taskStore,
		ExecRegistry:           execRegistry,
		Logger:                 e.logger,
		StateNotifier:          stateNotifier,
		ValidationRetryHandler: validationRetryHandler,
		HookManager:            hookManager,
		ProgressCallback:       e.makeProgressCallback(ctx, taskID),
	}, cfg), nil
}

// makeProgressCallback returns a StepProgressCallback that writes step events to Redis.
// Returns nil when no logWriter is configured.
func (e *DaemonTaskExecutor) makeProgressCallback(ctx context.Context, taskID string) task.StepProgressCallback {
	if e.logWriter == nil {
		return nil
	}
	return func(ev task.StepProgressEvent) {
		var msg string
		level := "info"
		switch ev.Type {
		case "start":
			msg = fmt.Sprintf("[step %d/%d] starting: %s (%s)", ev.StepIndex+1, ev.TotalSteps, ev.StepName, ev.StepType)
		case "complete":
			if ev.Status != "" && ev.Status != "completed" {
				level = "warn"
			}
			msg = fmt.Sprintf("[step %d/%d] %s: %s (%dms)", ev.StepIndex+1, ev.TotalSteps, ev.StepName, ev.Status, ev.DurationMs)
		case "progress":
			if ev.SubStep != "" {
				msg = fmt.Sprintf("[step %d/%d] %s: %s", ev.StepIndex+1, ev.TotalSteps, ev.StepName, ev.SubStep)
			}
		default:
			msg = fmt.Sprintf("[step %d/%d] %s: %s", ev.StepIndex+1, ev.TotalSteps, ev.StepName, ev.Type)
		}
		if msg == "" {
			return
		}
		_ = e.logWriter.Write(ctx, taskID, daemon.LogEntry{
			Level:   level,
			Message: msg,
			Step:    ev.StepName,
			Source:  "engine",
		})
	}
}

// makeValidationProgressCallback returns a validation progress callback that writes to Redis.
// Returns nil when no logWriter is configured.
func (e *DaemonTaskExecutor) makeValidationProgressCallback(ctx context.Context, taskID string) func(step, status string, info *validation.ProgressInfo) {
	if e.logWriter == nil {
		return nil
	}
	return func(step, status string, _ *validation.ProgressInfo) {
		level := "info"
		if status == "failed" {
			level = "warn"
		}
		_ = e.logWriter.Write(ctx, taskID, daemon.LogEntry{
			Level:   level,
			Message: fmt.Sprintf("validation: %s %s", step, status),
			Step:    "validate",
			Source:  "engine",
		})
	}
}

// provisionWorkspace creates or reuses a workspace and returns worktreePath and branch.
func (e *DaemonTaskExecutor) provisionWorkspace(
	ctx context.Context,
	job daemon.TaskJob,
	wsName string,
	cfg *config.Config,
) (worktreePath, branch string, err error) {
	wsStore, err := workspace.NewRepoScopedFileStore(job.RepoPath)
	if err != nil {
		return "", "", fmt.Errorf("create workspace store: %w", err)
	}

	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, job.RepoPath, e.logger)
	if err != nil {
		return "", "", fmt.Errorf("create worktree runner: %w", err)
	}

	wsMgr := workspace.NewManager(wsStore, wtRunner, e.logger)

	createOpts := workspace.CreateOptions{
		Name:       wsName,
		RepoPath:   job.RepoPath,
		BranchType: "feature",
		BaseBranch: cfg.Git.BaseBranch,
		UseLocal:   job.UseLocal,
	}
	if job.Branch != "" {
		// Use the specified base branch instead of the config default.
		createOpts.BaseBranch = job.Branch
	}
	if job.TargetBranch != "" {
		// Checkout an existing branch directly (mutually exclusive with BranchType/BaseBranch).
		createOpts.ExistingBranch = job.TargetBranch
		createOpts.BranchType = ""
		createOpts.BaseBranch = ""
	}

	ws, err := wsMgr.Create(ctx, createOpts)
	if err != nil {
		return "", "", fmt.Errorf("create workspace %q: %w", wsName, err)
	}
	return ws.WorktreePath, ws.Branch, nil
}

// resolveTemplate loads and returns the named template from the registry.
func (e *DaemonTaskExecutor) resolveTemplate(job daemon.TaskJob, cfg *config.Config) (*domain.Template, error) {
	registry, err := template.NewRegistryWithConfig(job.RepoPath, cfg.Templates.CustomTemplates)
	if err != nil {
		return nil, fmt.Errorf("create template registry: %w", err)
	}

	tmplName := job.Template
	if tmplName == "" {
		tmplName = "task"
	}

	tmpl, err := registry.Get(tmplName)
	if err != nil {
		return nil, fmt.Errorf("get template %q: %w", tmplName, err)
	}
	return tmpl, nil
}

// resolveGitCfgFromConfig converts config git settings to GitConfig with fallbacks.
func resolveGitCfgFromConfig(cfg *config.Config) GitConfig {
	return GitConfig{
		CommitAgent:         coalesce(cfg.SmartCommit.Agent, cfg.AI.Agent),
		CommitModel:         coalesce(cfg.SmartCommit.Model, cfg.AI.Model),
		PRDescAgent:         coalesce(cfg.PRDescription.Agent, cfg.AI.Agent),
		PRDescModel:         coalesce(cfg.PRDescription.Model, cfg.AI.Model),
		CommitTimeout:       cfg.SmartCommit.Timeout,
		CommitMaxRetries:    cfg.SmartCommit.MaxRetries,
		CommitBackoffFactor: cfg.SmartCommit.RetryBackoffFactor,
	}
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
