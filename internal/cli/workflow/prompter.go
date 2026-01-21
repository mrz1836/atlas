// Package workflow provides workflow orchestration for ATLAS task execution.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// Prompter handles interactive user prompts.
type Prompter struct {
	out tui.Output
}

// NewPrompter creates a new Prompter.
func NewPrompter(out tui.Output) *Prompter {
	return &Prompter{out: out}
}

// SelectTemplate handles template selection based on flags and interactivity mode.
func (p *Prompter) SelectTemplate(ctx context.Context, registry *template.Registry, templateName string, noInteractive bool, outputFormat string) (*domain.Template, error) {
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
	if noInteractive || outputFormat == "json" || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("use --template to specify template: %w", atlaserrors.ErrTemplateRequired))
	}

	return p.selectTemplateInteractive(registry)
}

// ResolveWorkspaceConflict checks for existing workspace and handles conflicts.
func (p *Prompter) ResolveWorkspaceConflict(ctx context.Context, mgr *workspace.DefaultManager, wsName string, noInteractive bool, outputFormat string, w io.Writer) (string, error) {
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
	if noInteractive || outputFormat == "json" {
		if outputFormat == "json" {
			return "", outputStartErrorJSON(w, wsName, "", fmt.Sprintf("workspace '%s': %s", wsName, atlaserrors.ErrWorkspaceExists.Error()))
		}
		return "", atlaserrors.NewExitCode2Error(
			fmt.Errorf("workspace '%s': %w", wsName, atlaserrors.ErrWorkspaceExists))
	}

	// Check if we're in a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("workspace '%s': %w (use --workspace to specify a different name)", wsName, atlaserrors.ErrWorkspaceExists)
	}

	return p.resolveWorkspaceConflictInteractive(wsName)
}

// selectTemplateInteractive displays an interactive template selection menu.
func (p *Prompter) selectTemplateInteractive(registry *template.Registry) (*domain.Template, error) {
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
	).WithTheme(tui.AtlasTheme())

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("template selection canceled: %w", err)
	}

	return registry.Get(selected)
}

// resolveWorkspaceConflictInteractive handles workspace conflict interactively.
func (p *Prompter) resolveWorkspaceConflictInteractive(wsName string) (string, error) {
	action, err := p.promptWorkspaceConflict(wsName)
	if err != nil {
		return "", fmt.Errorf("failed to get user choice: %w", err)
	}

	switch action {
	case "resume":
		return "", atlaserrors.ErrResumeNotImplemented
	case "new":
		newName, err := p.promptNewWorkspaceName()
		if err != nil {
			return "", fmt.Errorf("failed to get new workspace name: %w", err)
		}
		return SanitizeWorkspaceName(newName), nil
	case "cancel":
		p.out.Info("Operation canceled")
		return "", atlaserrors.ErrOperationCanceled
	}

	return wsName, nil
}

// promptWorkspaceConflict prompts the user to resolve a workspace name conflict.
func (p *Prompter) promptWorkspaceConflict(name string) (string, error) {
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
	).WithTheme(tui.AtlasTheme())

	if err := form.Run(); err != nil {
		return "", err
	}

	return action, nil
}

// promptNewWorkspaceName prompts the user for a new workspace name.
func (p *Prompter) promptNewWorkspaceName() (string, error) {
	var name string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter new workspace name").
				Value(&name).
				Validate(ValidateWorkspaceName),
		),
	).WithTheme(tui.AtlasTheme())

	if err := form.Run(); err != nil {
		return "", err
	}

	return name, nil
}

// ValidateWorkspaceName validates a workspace name input.
func ValidateWorkspaceName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("name required: %w", atlaserrors.ErrEmptyValue)
	}
	return nil
}

// SanitizeWorkspaceName sanitizes a string for use as a workspace name.
// This is exported for use by other packages.
func SanitizeWorkspaceName(input string) string {
	return sanitizeWorkspaceName(input)
}

// errJSONOutput is a sentinel error for JSON output errors.
var errJSONOutput = errors.New("JSON output error")

// outputStartErrorJSON outputs an error result as JSON.
// This is a helper to maintain compatibility with the original API.
func outputStartErrorJSON(_ io.Writer, _, _, errMsg string) error {
	// This function delegates back to the cli package's implementation
	// via the startResponse type. For now, we keep a simple error return.
	return fmt.Errorf("%s: %w", errMsg, errJSONOutput)
}

// SelectTemplate is a standalone function for template selection.
// It creates a temporary prompter with a no-op output.
// This is primarily for testing and backwards compatibility.
func SelectTemplate(ctx context.Context, registry *template.Registry, templateName string, noInteractive bool, outputFormat string) (*domain.Template, error) {
	p := NewPrompter(tui.NewOutput(io.Discard, outputFormat))
	return p.SelectTemplate(ctx, registry, templateName, noInteractive, outputFormat)
}
