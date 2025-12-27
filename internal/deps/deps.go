// Package deps is a TEMPORARY package to preserve Go module dependencies.
//
// IMPORTANT: DELETE THIS ENTIRE PACKAGE when the dependencies are actually used elsewhere.
//
// Why this exists:
// Story 1.1 requires core dependencies to be present in go.mod, but `go mod tidy`
// removes unused dependencies. This file imports all required dependencies to keep
// them in go.mod until they are properly integrated into the codebase.
//
// Dependencies preserved here:
//   - github.com/spf13/viper        → will be used by internal/config
//   - github.com/rs/zerolog         → will be used for structured logging throughout
//   - github.com/charmbracelet/huh  → will be used by internal/tui for forms
//   - github.com/charmbracelet/lipgloss → will be used by internal/tui for styling
//   - github.com/charmbracelet/bubbles  → will be used by internal/tui for widgets
//   - github.com/stretchr/testify   → will be used in *_test.go files
//
// When to delete:
//   - After Story 1.2+ when config package uses Viper
//   - After Story 2.x when TUI packages use Charm libraries
//   - After any story adds tests using testify
//
// TODO(cleanup): Remove this file when dependencies are properly integrated.
//
//nolint:testifylint // Blank imports are intentional here to preserve dependencies in go.mod
package deps

import (
	// TUI widgets (spinner, textinput, etc.) - will be used by internal/tui
	_ "github.com/charmbracelet/bubbles/spinner"
	// TUI interactive forms - will be used by internal/tui
	_ "github.com/charmbracelet/huh"
	// Terminal styling - will be used by internal/tui
	_ "github.com/charmbracelet/lipgloss"
	// Structured JSON logging - will be used throughout the application
	_ "github.com/rs/zerolog"
	// Config management - will be used by internal/config
	_ "github.com/spf13/viper"
	// Testing assertions - will be used in test files
	_ "github.com/stretchr/testify/assert"
)

// This file intentionally has no exported symbols.
// It exists solely to preserve dependencies in go.mod.
