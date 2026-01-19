// Package git provides Git operations for ATLAS.
// This file provides branch naming and creation utilities.
package git

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// BranchConfig holds configuration for branch creation.
type BranchConfig struct {
	// Type is the branch type prefix (feat, fix, chore).
	Type string
	// BaseBranch is the branch to create from (default: main).
	BaseBranch string
	// Pattern is an optional custom naming pattern (default: "{type}/{name}").
	Pattern string
}

// BranchCreateOptions contains options for creating a branch.
type BranchCreateOptions struct {
	// WorkspaceName is the name of the workspace (used in branch name).
	WorkspaceName string
	// BranchType is the branch type prefix (feat, fix, chore).
	BranchType string
	// BaseBranch is the branch to create from (empty = use default from config).
	BaseBranch string
}

// BranchInfo contains information about a created branch.
type BranchInfo struct {
	// Name is the full branch name (e.g., "feat/auth-feature").
	Name string
	// BaseBranch is the branch this was created from.
	BaseBranch string
	// CreatedAt is when the branch was created.
	CreatedAt time.Time
}

// DefaultBranchPrefixes maps template types to their default branch prefixes.
// These follow conventional commit/branch naming standards.
//
//nolint:gochecknoglobals // Package-level constant-like mapping for branch prefixes
var DefaultBranchPrefixes = map[string]string{
	"bugfix":  "fix",
	"feature": "feat",
	"commit":  "chore",
	// Common aliases
	"fix":   "fix",
	"feat":  "feat",
	"chore": "chore",
}

// branchNameRegex is used to sanitize branch names.
// It matches any character that is NOT a lowercase letter, digit, or hyphen.
var branchNameRegex = regexp.MustCompile(`[^a-z0-9-]+`)

// SanitizeBranchName sanitizes a branch name by:
// - Converting to lowercase
// - Replacing non-alphanumeric characters with hyphens
// - Trimming leading/trailing hyphens
// - Collapsing consecutive hyphens
//
// Example: "My Feature!" -> "my-feature"
func SanitizeBranchName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Replace non-alphanumeric with hyphens
	name = branchNameRegex.ReplaceAllString(name, "-")
	// Collapse consecutive hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")
	return name
}

// GenerateBranchName creates a branch name from type and workspace name.
// The format is "{type}/{sanitized-workspace-name}".
//
// Example: GenerateBranchName("feat", "User Auth") -> "feat/user-auth"
func GenerateBranchName(branchType, workspaceName string) string {
	sanitized := SanitizeBranchName(workspaceName)
	if sanitized == "" {
		sanitized = "unnamed"
	}
	return fmt.Sprintf("%s/%s", branchType, sanitized)
}

// BranchExistsChecker is an interface for checking if a branch exists.
// This allows the unique name generation logic to be shared across packages.
type BranchExistsChecker interface {
	BranchExists(ctx context.Context, name string) (bool, error)
}

// GenerateUniqueBranchNameWithChecker generates a unique branch name using the provided checker.
// If the base name already exists, appends a timestamp suffix.
// This is a shared utility function for use by both BranchCreator and workspace packages.
func GenerateUniqueBranchNameWithChecker(ctx context.Context, checker BranchExistsChecker, baseName string) (string, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return "", err
	}

	exists, err := checker.BranchExists(ctx, baseName)
	if err != nil {
		return "", err
	}
	if !exists {
		return baseName, nil
	}

	// Branch exists, append timestamp suffix to create unique name
	uniqueName := fmt.Sprintf("%s-%s", baseName, time.Now().Format(constants.TimeFormatCompact))

	// Verify the new name doesn't also exist (extremely unlikely but possible)
	exists, err = checker.BranchExists(ctx, uniqueName)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("branch '%s' already exists and timestamp variant also exists: %w",
			baseName, atlaserrors.ErrBranchExists)
	}

	return uniqueName, nil
}

// ResolveBranchPrefix resolves a template type to its branch prefix.
// Uses DefaultBranchPrefixes for standard mappings.
// If templateType is not found, returns it as-is (allowing custom prefixes).
func ResolveBranchPrefix(templateType string) string {
	return ResolveBranchPrefixWithConfig(templateType, nil)
}

// ResolveBranchPrefixWithConfig resolves a template type to its branch prefix.
// First checks customPrefixes (from config), then falls back to DefaultBranchPrefixes.
// If templateType is not found in either, returns it as-is (allowing custom prefixes).
func ResolveBranchPrefixWithConfig(templateType string, customPrefixes map[string]string) string {
	// Normalize to lowercase for lookup
	key := strings.ToLower(templateType)

	// Check custom prefixes first (from config)
	if customPrefixes != nil {
		if prefix, ok := customPrefixes[key]; ok {
			return prefix
		}
	}

	// Fall back to built-in defaults
	if prefix, ok := DefaultBranchPrefixes[key]; ok {
		return prefix
	}

	// Return as-is for unknown types
	return key
}

// BranchCreatorService defines the interface for branch creation operations.
// This interface allows for mocking in tests and alternative implementations.
type BranchCreatorService interface {
	// Create creates a new branch with the given options.
	Create(ctx context.Context, opts BranchCreateOptions) (*BranchInfo, error)

	// GenerateUniqueBranchName generates a unique branch name.
	// If the base name already exists, appends a timestamp suffix.
	GenerateUniqueBranchName(ctx context.Context, baseName string) (string, error)
}

// BranchCreator creates branches with consistent naming.
// Implements BranchCreatorService interface.
type BranchCreator struct {
	runner         Runner
	customPrefixes map[string]string // Optional custom prefixes from config
}

// Ensure BranchCreator implements BranchCreatorService.
var _ BranchCreatorService = (*BranchCreator)(nil)

// BranchCreatorOption configures a BranchCreator.
type BranchCreatorOption func(*BranchCreator)

// WithCustomPrefixes sets custom branch prefixes from config.
// These take priority over DefaultBranchPrefixes when resolving branch types.
func WithCustomPrefixes(prefixes map[string]string) BranchCreatorOption {
	return func(bc *BranchCreator) {
		bc.customPrefixes = prefixes
	}
}

// NewBranchCreator creates a new BranchCreator with the given git runner
// and optional configuration options.
func NewBranchCreator(runner Runner, opts ...BranchCreatorOption) *BranchCreator {
	bc := &BranchCreator{runner: runner}
	for _, opt := range opts {
		opt(bc)
	}
	return bc
}

// GenerateUniqueBranchName generates a unique branch name.
// If the base name already exists, appends a timestamp suffix.
// Delegates to the shared GenerateUniqueBranchNameWithChecker function.
//
// Example: If "feat/auth" exists, returns "feat/auth-20251229-143022"
func (c *BranchCreator) GenerateUniqueBranchName(ctx context.Context, baseName string) (string, error) {
	return GenerateUniqueBranchNameWithChecker(ctx, c.runner, baseName)
}

// Create creates a new branch with the given options.
// It sanitizes the workspace name, generates a unique branch name,
// and creates the branch from the specified base branch.
func (c *BranchCreator) Create(ctx context.Context, opts BranchCreateOptions) (*BranchInfo, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate required fields
	if opts.WorkspaceName == "" {
		return nil, fmt.Errorf("workspace name cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	if opts.BranchType == "" {
		return nil, fmt.Errorf("branch type cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}

	// Resolve branch prefix from template type (using custom prefixes if configured)
	prefix := ResolveBranchPrefixWithConfig(opts.BranchType, c.customPrefixes)

	// Generate branch name
	baseBranchName := GenerateBranchName(prefix, opts.WorkspaceName)

	// Generate unique name (handles collision with existing branches)
	branchName, err := c.GenerateUniqueBranchName(ctx, baseBranchName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate unique branch name: %w", err)
	}

	// Create the branch from the specified base branch
	if err := c.runner.CreateBranch(ctx, branchName, opts.BaseBranch); err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	return &BranchInfo{
		Name:       branchName,
		BaseBranch: opts.BaseBranch,
		CreatedAt:  time.Now(),
	}, nil
}
