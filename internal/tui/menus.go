// Package tui provides terminal user interface components for ATLAS.
//
// This file provides the interactive menu system using Charm Huh for consistent,
// intuitive interfaces at all user decision points.
//
// # Interactive Menu Functions (AC: #1)
//
// Four primary functions are provided for user interaction:
//   - Select: Single selection from a list of options
//   - Confirm: Yes/no confirmation prompts
//   - Input: Single-line text input
//   - TextArea: Multi-line text input
//
// # Styling (AC: #2)
//
// All menus use the established style system from styles.go, with a custom ATLAS
// theme that maps ColorPrimary, ColorSuccess, ColorWarning, and ColorError to
// appropriate Huh form states.
//
// # Keyboard Navigation (AC: #3, #4)
//
// Menus support standard navigation: arrow keys, Enter to select, q/Esc to cancel.
// Key hints are displayed at the bottom of menus when enabled.
//
// # Terminal Adaptation (AC: #5, #6)
//
// Menus respect terminal width and adapt accordingly. They work across common
// terminal emulators including tmux, iTerm2, Terminal.app, and VS Code terminal.
package tui

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Terminal layout constants.
const (
	// TerminalEdgeMargin is the number of characters to leave between
	// menu content and the terminal edge for visual padding.
	TerminalEdgeMargin = 4

	// MinMenuWidth is the minimum usable width for menu content.
	// Menus narrower than this become difficult to read and use.
	MinMenuWidth = 40
)

// ErrMenuCanceled is an alias for errors.ErrMenuCanceled for package-local use.
// Returns when the user cancels a menu operation by pressing q or Escape.
var ErrMenuCanceled = atlaserrors.ErrMenuCanceled

// KeyHints is the standard key hint string displayed below interactive menus (AC: #4).
const KeyHints = "[↑↓] Navigate  [enter] Select  [q] Cancel"

// Option represents a selectable menu option (AC: #1).
type Option struct {
	// Label is the display text shown to the user.
	Label string
	// Description is optional help text shown below the label.
	Description string
	// Value is the value returned when this option is selected.
	Value string
}

// MenuConfig holds configuration for menu components (AC: #5, #6).
type MenuConfig struct {
	// Width is the maximum width for the menu. If 0, adapts to terminal width.
	Width int
	// Accessible enables accessible mode for screen readers (AC: #6).
	Accessible bool
	// ShowKeyHints controls whether key hints are displayed (AC: #4).
	ShowKeyHints bool
}

// MenuConfigOption is a functional option for configuring MenuConfig.
type MenuConfigOption func(*MenuConfig)

// WithMenuWidth sets the menu width.
func WithMenuWidth(width int) MenuConfigOption {
	return func(c *MenuConfig) {
		c.Width = width
	}
}

// WithMenuAccessible enables or disables accessible mode.
func WithMenuAccessible(enabled bool) MenuConfigOption {
	return func(c *MenuConfig) {
		c.Accessible = enabled
	}
}

// WithMenuKeyHints enables or disables key hints display.
func WithMenuKeyHints(show bool) MenuConfigOption {
	return func(c *MenuConfig) {
		c.ShowKeyHints = show
	}
}

// NewMenuConfig creates a MenuConfig with sensible defaults from styles.go.
// It automatically detects accessible mode from the ACCESSIBLE environment variable.
// Use functional options to customize: NewMenuConfig(WithMenuWidth(80), WithMenuKeyHints(false))
func NewMenuConfig(opts ...MenuConfigOption) *MenuConfig {
	// Check for accessible mode from environment (AC: #6)
	_, accessible := os.LookupEnv("ACCESSIBLE")

	c := &MenuConfig{
		Width:        DefaultBoxWidth, // From styles.go
		Accessible:   accessible,
		ShowKeyHints: true,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithWidth returns a new MenuConfig with the specified width.
func (c *MenuConfig) WithWidth(width int) *MenuConfig {
	return &MenuConfig{
		Width:        width,
		Accessible:   c.Accessible,
		ShowKeyHints: c.ShowKeyHints,
	}
}

// WithAccessible returns a new MenuConfig with accessible mode enabled/disabled.
func (c *MenuConfig) WithAccessible(enabled bool) *MenuConfig {
	return &MenuConfig{
		Width:        c.Width,
		Accessible:   enabled,
		ShowKeyHints: c.ShowKeyHints,
	}
}

// WithKeyHints returns a new MenuConfig with key hints enabled/disabled.
func (c *MenuConfig) WithKeyHints(show bool) *MenuConfig {
	return &MenuConfig{
		Width:        c.Width,
		Accessible:   c.Accessible,
		ShowKeyHints: show,
	}
}

// adaptWidth returns an appropriate menu width based on terminal size (AC: #5).
// It respects the maxWidth constraint while adapting to narrower terminals.
func adaptWidth(maxWidth int) int {
	// Try to get terminal width
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		// Fallback to provided max width
		if maxWidth <= 0 {
			return DefaultBoxWidth
		}
		return maxWidth
	}

	// Leave some margin from terminal edge for visual padding
	availableWidth := width - TerminalEdgeMargin

	// Use the smaller of maxWidth and available terminal width
	if maxWidth > 0 && maxWidth < availableWidth {
		return maxWidth
	}

	// Ensure minimum usable width
	if availableWidth < MinMenuWidth {
		return MinMenuWidth
	}

	return availableWidth
}

// runFormWithConfig creates and runs a form with the given field and config.
// It handles common setup (theme, width, accessibility) and error handling.
// The errorContext parameter is used to wrap errors with descriptive context.
func runFormWithConfig(field huh.Field, cfg *MenuConfig, errorContext string) error {
	// Check if we're running in a terminal environment
	// This prevents tests from hanging when TUI code is called without a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ErrMenuCanceled
	}

	CheckNoColor()

	width := adaptWidth(cfg.Width)

	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(AtlasTheme()).
		WithWidth(width).
		WithAccessible(cfg.Accessible).
		WithShowHelp(cfg.ShowKeyHints)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrMenuCanceled
		}
		return fmt.Errorf("%s: %w", errorContext, err)
	}

	return nil
}

// AtlasTheme returns a custom Huh theme using ATLAS colors from styles.go (AC: #2, #9).
// Uses AdaptiveColor for proper light/dark terminal support.
func AtlasTheme() *huh.Theme {
	// Check color support (NO_COLOR handling)
	CheckNoColor()

	// Start with base theme and customize
	t := huh.ThemeBase()

	// Map ColorPrimary to focused state (uses AdaptiveColor for light/dark support)
	t.Focused.Base = t.Focused.Base.BorderForeground(ColorPrimary)
	t.Focused.Title = t.Focused.Title.Foreground(ColorPrimary)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(ColorPrimary)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(ColorPrimary)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(ColorPrimary)

	// Map ColorSuccess to selected/completed state
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(ColorSuccess)

	// Map ColorError to error/validation failed state
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(ColorError)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(ColorError)

	// Map ColorMuted to unfocused/help text state
	t.Blurred.Base = t.Blurred.Base.BorderForeground(ColorMuted)
	t.Blurred.Title = t.Blurred.Title.Foreground(ColorMuted)
	t.Focused.Description = t.Focused.Description.Foreground(ColorMuted)
	t.Help.Ellipsis = t.Help.Ellipsis.Foreground(ColorMuted)

	return t
}

// Select presents a single-selection menu and returns the selected value (AC: #1, #2, #3).
// Returns ErrMenuCanceled if user presses q or Esc.
func Select(title string, options []Option) (string, error) {
	return SelectWithConfig(title, options, NewMenuConfig())
}

// SelectWithConfig presents a single-selection menu with custom configuration.
func SelectWithConfig(title string, options []Option, cfg *MenuConfig) (string, error) {
	if len(options) == 0 {
		return "", atlaserrors.ErrNoMenuOptions
	}

	// Convert options to huh.Option format (AC: #1)
	// Huh library doesn't support option-level descriptions natively,
	// so we include the description in the label if present for better UX
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		label := opt.Label
		if opt.Description != "" {
			label = opt.Label + " - " + opt.Description
		}
		huhOptions[i] = huh.NewOption(label, opt.Value)
	}

	var selected string

	selectField := huh.NewSelect[string]().
		Title(title).
		Options(huhOptions...).
		Value(&selected)

	if err := runFormWithConfig(selectField, cfg, "select menu failed"); err != nil {
		return "", err
	}

	return selected, nil
}

// Confirm presents a yes/no confirmation prompt (AC: #1, #2, #3).
// Returns the user's choice or ErrMenuCanceled if canceled.
func Confirm(message string, defaultYes bool) (bool, error) {
	return ConfirmWithConfig(message, defaultYes, NewMenuConfig())
}

// ConfirmWithConfig presents a confirmation prompt with custom configuration.
func ConfirmWithConfig(message string, defaultYes bool, cfg *MenuConfig) (bool, error) {
	var confirmed bool
	if defaultYes {
		confirmed = true
	}

	confirmField := huh.NewConfirm().
		Title(message).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed)

	if err := runFormWithConfig(confirmField, cfg, "confirm prompt failed"); err != nil {
		return false, err
	}

	return confirmed, nil
}

// Input presents a single-line text input prompt (AC: #1, #2, #3).
// Returns the entered text or ErrMenuCanceled if canceled.
func Input(prompt, defaultValue string) (string, error) {
	return InputWithConfig(prompt, defaultValue, NewMenuConfig())
}

// InputWithConfig presents an input prompt with custom configuration.
func InputWithConfig(prompt, defaultValue string, cfg *MenuConfig) (string, error) {
	var value string
	if defaultValue != "" {
		value = defaultValue
	}

	inputField := huh.NewInput().
		Title(prompt).
		Value(&value)

	if err := runFormWithConfig(inputField, cfg, "input prompt failed"); err != nil {
		return "", err
	}

	return value, nil
}

// InputWithValidation presents an input prompt with a validation function.
func InputWithValidation(prompt, defaultValue string, validate func(string) error) (string, error) {
	return InputWithValidationConfig(prompt, defaultValue, validate, NewMenuConfig())
}

// InputWithValidationConfig presents an input prompt with validation and custom config.
func InputWithValidationConfig(prompt, defaultValue string, validate func(string) error, cfg *MenuConfig) (string, error) {
	var value string
	if defaultValue != "" {
		value = defaultValue
	}

	inputField := huh.NewInput().
		Title(prompt).
		Value(&value).
		Validate(validate)

	if err := runFormWithConfig(inputField, cfg, "validated input prompt failed"); err != nil {
		return "", err
	}

	return value, nil
}

// TextArea presents a multi-line text input prompt (AC: #1, #2, #3).
// Returns the entered text or ErrMenuCanceled if canceled.
func TextArea(prompt, placeholder string) (string, error) {
	return TextAreaWithConfig(prompt, placeholder, NewMenuConfig())
}

// TextAreaWithConfig presents a text area with custom configuration.
func TextAreaWithConfig(prompt, placeholder string, cfg *MenuConfig) (string, error) {
	var value string

	textField := huh.NewText().
		Title(prompt).
		Placeholder(placeholder).
		Value(&value)

	if err := runFormWithConfig(textField, cfg, "text area failed"); err != nil {
		return "", err
	}

	return value, nil
}

// TextAreaWithLimit presents a text area with a character limit.
func TextAreaWithLimit(prompt, placeholder string, charLimit int) (string, error) {
	return TextAreaWithLimitConfig(prompt, placeholder, charLimit, NewMenuConfig())
}

// TextAreaWithLimitConfig presents a text area with character limit and custom config.
func TextAreaWithLimitConfig(prompt, placeholder string, charLimit int, cfg *MenuConfig) (string, error) {
	var value string

	textField := huh.NewText().
		Title(prompt).
		Placeholder(placeholder).
		CharLimit(charLimit).
		Value(&value)

	if err := runFormWithConfig(textField, cfg, "text area with limit failed"); err != nil {
		return "", err
	}

	return value, nil
}
