// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// BuildInfo contains version information set at build time via ldflags.
type BuildInfo struct {
	// Version is the semantic version (e.g., "1.0.0").
	Version string
	// Commit is the git commit hash.
	Commit string
	// Date is the build date.
	Date string
}

// globalLogger stores the initialized logger for use by subcommands.
// This is set during PersistentPreRunE and should be accessed via Logger.
// This is a necessary global for CLI logger access across command handlers.
// Access is protected by globalLoggerMu for thread safety.
var (
	globalLogger   zerolog.Logger //nolint:gochecknoglobals // CLI logger requires global access
	globalLoggerMu sync.RWMutex   //nolint:gochecknoglobals // Protects globalLogger

	// globalLogFlags stores the verbose/quiet flags used to initialize the logger.
	// These are needed to create task-specific loggers with the same settings.
	globalLogFlags struct { //nolint:gochecknoglobals // CLI flags require global access
		verbose bool
		quiet   bool
	}
)

// Logger returns the initialized logger for use by subcommands.
//
// IMPORTANT: This function MUST only be called after the root command's
// PersistentPreRunE has executed. Calling it before initialization will
// return a zero-value logger that discards all log output.
//
// This function is safe for concurrent use.
//
// Typical usage is within a subcommand's Run/RunE function:
//
//	RunE: func(cmd *cobra.Command, args []string) error {
//	    logger := cli.Logger()
//	    logger.Info().Msg("executing command")
//	    ...
//	}
func Logger() zerolog.Logger {
	globalLoggerMu.RLock()
	defer globalLoggerMu.RUnlock()
	return globalLogger
}

// LoggerWithTaskStore returns a logger configured to persist task-specific logs.
// Log entries containing workspace_name and task_id fields will be written to
// the task's log file in addition to the console and global log.
//
// This function is safe for concurrent use.
func LoggerWithTaskStore(store TaskLogAppender) zerolog.Logger {
	// Read the global flags with lock protection
	globalLoggerMu.RLock()
	verbose := globalLogFlags.verbose
	quiet := globalLogFlags.quiet
	globalLoggerMu.RUnlock()

	// Call InitLoggerWithTaskStore without holding the lock to avoid deadlock
	return InitLoggerWithTaskStore(verbose, quiet, store)
}

// newRootCmd creates and returns the root command for the atlas CLI.
// This function-based approach avoids package-level globals, making the
// code more testable and avoiding gochecknoglobals linter warnings.
func newRootCmd(flags *GlobalFlags, info BuildInfo) *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:     "atlas",
		Short:   "ATLAS - AI Task Lifecycle Automation System",
		Long:    tui.RenderHeaderAuto(),
		Version: formatVersion(info),
		// Run displays help when the root command is invoked without subcommands.
		// This ensures PersistentPreRunE is called for flag validation.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Bind flags to Viper
			if err := BindGlobalFlags(v, cmd); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
			}

			// Validate output format
			if !IsValidOutputFormat(flags.Output) {
				return fmt.Errorf("%w: %q must be one of %v", errors.ErrInvalidOutputFormat, flags.Output, ValidOutputFormats())
			}

			// Initialize logger based on flags (protected by mutex for thread safety)
			globalLoggerMu.Lock()
			globalLogger = InitLogger(flags.Verbose, flags.Quiet)
			globalLogFlags.verbose = flags.Verbose
			globalLogFlags.quiet = flags.Quiet
			logger := globalLogger // Get a copy while holding the lock
			globalLoggerMu.Unlock()

			// Log verbose mode status (only visible when verbose is enabled)
			if flags.Verbose {
				logger.Debug().Msg("verbose mode enabled")
			}

			return nil
		},
		// SilenceUsage prevents printing usage on error
		// (we handle our own error messages)
		SilenceUsage: true,
	}

	// Add global flags
	AddGlobalFlags(cmd, flags)

	// Add subcommands
	AddInitCommand(cmd)
	AddConfigCommand(cmd)
	AddUpgradeCommand(cmd)
	AddWorkspaceCommand(cmd)
	AddStartCommand(cmd)
	AddStatusCommand(cmd)
	AddResumeCommand(cmd)
	AddAbandonCommand(cmd)
	AddValidateCommand(cmd)
	AddFormatCommand(cmd)
	AddLintCommand(cmd)
	AddTestCommand(cmd)
	AddApproveCommand(cmd)
	AddRejectCommand(cmd)
	AddCompletionCommand(cmd)
	AddHookCommand(cmd)
	AddCheckpointCommand(cmd)
	AddCleanupCommand(cmd)
	AddBacklogCommand(cmd)

	return cmd
}

// formatVersion creates the version string from build info.
func formatVersion(info BuildInfo) string {
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "none"
	}
	if info.Date == "" {
		info.Date = "unknown"
	}
	return fmt.Sprintf("%s (commit: %s, built: %s)", info.Version, info.Commit, info.Date)
}

// Execute runs the root command with the provided context and build info.
func Execute(ctx context.Context, info BuildInfo) error {
	flags := &GlobalFlags{}
	//nolint:contextcheck // Cobra command pattern uses cmd.Context() internally
	cmd := newRootCmd(flags, info)
	return cmd.ExecuteContext(ctx)
}
