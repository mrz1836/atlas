// Package backlog provides discovery management for the ATLAS work backlog feature.
//
// The backlog captures issues discovered during AI-assisted development that
// cannot be addressed in the current task scope. Each discovery is stored as
// an individual YAML file in .atlas/backlog/ to enable frictionless capture
// and zero merge conflicts.
package backlog

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// SchemaVersion is the current schema version for discovery files.
const SchemaVersion = "1.0"

// Status represents the lifecycle state of a discovery.
type Status string

const (
	// StatusPending indicates a discovery that has not yet been acted upon.
	StatusPending Status = "pending"
	// StatusPromoted indicates a discovery that has been promoted to a task.
	StatusPromoted Status = "promoted"
	// StatusDismissed indicates a discovery that has been dismissed.
	StatusDismissed Status = "dismissed"
)

// ValidStatuses returns all valid status values.
func ValidStatuses() []Status {
	return []Status{StatusPending, StatusPromoted, StatusDismissed}
}

// IsValid checks if the status is a valid value.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusPromoted, StatusDismissed:
		return true
	default:
		return false
	}
}

// Category classifies the type of discovery.
type Category string

// Category constants define the valid classification types for discoveries.
const (
	CategoryBug             Category = "bug"
	CategorySecurity        Category = "security"
	CategoryPerformance     Category = "performance"
	CategoryMaintainability Category = "maintainability"
	CategoryTesting         Category = "testing"
	CategoryDocumentation   Category = "documentation"
)

// ValidCategories returns all valid category values.
func ValidCategories() []Category {
	return []Category{
		CategoryBug,
		CategorySecurity,
		CategoryPerformance,
		CategoryMaintainability,
		CategoryTesting,
		CategoryDocumentation,
	}
}

// IsValid checks if the category is a valid value.
func (c Category) IsValid() bool {
	switch c {
	case CategoryBug, CategorySecurity, CategoryPerformance,
		CategoryMaintainability, CategoryTesting, CategoryDocumentation:
		return true
	default:
		return false
	}
}

// Severity indicates the priority level of a discovery.
type Severity string

// Severity constants define the valid priority levels for discoveries.
const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// ValidSeverities returns all valid severity values.
func ValidSeverities() []Severity {
	return []Severity{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
}

// IsValid checks if the severity is a valid value.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

// Discovery represents a captured issue or observation found during development.
type Discovery struct {
	SchemaVersion string    `yaml:"schema_version"`
	ID            string    `yaml:"id"`
	Title         string    `yaml:"title"`
	Status        Status    `yaml:"status"`
	Content       Content   `yaml:"content"`
	Location      *Location `yaml:"location,omitempty"`
	Context       Context   `yaml:"context"`
	Lifecycle     Lifecycle `yaml:"lifecycle,omitempty"`
}

// Content holds the discovery details.
type Content struct {
	Description string   `yaml:"description,omitempty"`
	Category    Category `yaml:"category"`
	Severity    Severity `yaml:"severity"`
	Tags        []string `yaml:"tags,omitempty"`
}

// Location identifies where in code the discovery was found.
type Location struct {
	File string `yaml:"file,omitempty"`
	Line int    `yaml:"line,omitempty"`
}

// Context captures when/who/where the discovery was made.
type Context struct {
	DiscoveredAt time.Time   `yaml:"discovered_at"`
	DuringTask   string      `yaml:"discovered_during_task,omitempty"`
	DiscoveredBy string      `yaml:"discovered_by"`
	Git          *GitContext `yaml:"git,omitempty"`
}

// GitContext holds git repository state at discovery time.
type GitContext struct {
	Branch string `yaml:"branch,omitempty"`
	Commit string `yaml:"commit,omitempty"`
}

// Lifecycle tracks status transitions.
type Lifecycle struct {
	PromotedToTask  string `yaml:"promoted_to_task,omitempty"`
	DismissedReason string `yaml:"dismissed_reason,omitempty"`
}

// Validation constants and patterns.
const (
	MaxTitleLength = 200
	MaxTagLength   = 50
	MaxTags        = 10
	MinIDLength    = 10 // "disc-" + 6 alphanumeric
)

var (
	// idPattern matches the discovery ID format: disc-<6 alphanumeric chars>
	idPattern = regexp.MustCompile(`^disc-[a-z0-9]{6}$`)
	// tagPattern matches valid tag format: starts with alphanumeric, contains only alphanumeric, hyphens, underscores
	tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// ValidateID checks if the discovery ID is valid.
func (d *Discovery) ValidateID() error {
	if d.ID == "" {
		return fmt.Errorf("%w: id is required", atlaserrors.ErrInvalidDiscoveryID)
	}
	if !idPattern.MatchString(d.ID) {
		return fmt.Errorf("%w: must match pattern disc-[a-z0-9]{6}, got %q", atlaserrors.ErrInvalidDiscoveryID, d.ID)
	}
	return nil
}

// ValidateTitle checks if the discovery title is valid.
func (d *Discovery) ValidateTitle() error {
	title := strings.TrimSpace(d.Title)
	if title == "" {
		return fmt.Errorf("%w: title is required", atlaserrors.ErrEmptyValue)
	}
	if len(d.Title) > MaxTitleLength {
		return fmt.Errorf("%w: title exceeds %d characters", atlaserrors.ErrValueOutOfRange, MaxTitleLength)
	}
	return nil
}

// ValidateCategory checks if the category is valid.
func (d *Discovery) ValidateCategory() error {
	if !d.Content.Category.IsValid() {
		return fmt.Errorf("%w: %q is not a valid category", atlaserrors.ErrInvalidArgument, d.Content.Category)
	}
	return nil
}

// ValidateSeverity checks if the severity is valid.
func (d *Discovery) ValidateSeverity() error {
	if !d.Content.Severity.IsValid() {
		return fmt.Errorf("%w: %q is not a valid severity", atlaserrors.ErrInvalidArgument, d.Content.Severity)
	}
	return nil
}

// ValidateTags checks if the tags are valid.
func (d *Discovery) ValidateTags() error {
	if len(d.Content.Tags) > MaxTags {
		return fmt.Errorf("%w: cannot have more than %d tags", atlaserrors.ErrValueOutOfRange, MaxTags)
	}
	for _, tag := range d.Content.Tags {
		if len(tag) == 0 {
			return fmt.Errorf("%w: tag cannot be empty", atlaserrors.ErrEmptyValue)
		}
		if len(tag) > MaxTagLength {
			return fmt.Errorf("%w: tag %q exceeds %d characters", atlaserrors.ErrValueOutOfRange, tag, MaxTagLength)
		}
		if !tagPattern.MatchString(tag) {
			return fmt.Errorf("%w: tag %q must match pattern [a-z0-9][a-z0-9_-]*", atlaserrors.ErrInvalidArgument, tag)
		}
	}
	return nil
}

// ValidateLocation checks if the location is valid.
func (d *Discovery) ValidateLocation() error {
	if d.Location == nil {
		return nil // Location is optional
	}
	// If line is provided, file must also be provided
	if d.Location.Line > 0 && d.Location.File == "" {
		return fmt.Errorf("%w: file is required when line is specified", atlaserrors.ErrInvalidArgument)
	}
	// Line must be positive if provided
	if d.Location.Line < 0 {
		return fmt.Errorf("%w: line must be a positive integer", atlaserrors.ErrValueOutOfRange)
	}
	return nil
}

// ValidateStatus checks if the status is valid.
func (d *Discovery) ValidateStatus() error {
	if !d.Status.IsValid() {
		return fmt.Errorf("%w: %q", atlaserrors.ErrInvalidDiscoveryStatus, d.Status)
	}
	return nil
}

// Validate performs full validation of the discovery.
func (d *Discovery) Validate() error {
	if err := d.ValidateID(); err != nil {
		return err
	}
	if err := d.ValidateTitle(); err != nil {
		return err
	}
	if err := d.ValidateStatus(); err != nil {
		return err
	}
	if err := d.ValidateCategory(); err != nil {
		return err
	}
	if err := d.ValidateSeverity(); err != nil {
		return err
	}
	if err := d.ValidateTags(); err != nil {
		return err
	}
	if err := d.ValidateLocation(); err != nil {
		return err
	}
	// Context validation
	if d.Context.DiscoveredBy == "" {
		return fmt.Errorf("%w: discovered_by is required", atlaserrors.ErrEmptyValue)
	}
	if d.Context.DiscoveredAt.IsZero() {
		return fmt.Errorf("%w: discovered_at is required", atlaserrors.ErrEmptyValue)
	}
	// Lifecycle validation
	if d.Status == StatusPromoted && d.Lifecycle.PromotedToTask == "" {
		return fmt.Errorf("%w: promoted_to_task is required when status is promoted", atlaserrors.ErrInvalidArgument)
	}
	if d.Status == StatusDismissed && d.Lifecycle.DismissedReason == "" {
		return fmt.Errorf("%w: dismissed_reason is required when status is dismissed", atlaserrors.ErrInvalidArgument)
	}
	return nil
}

// Filter specifies criteria for listing discoveries.
type Filter struct {
	Status   *Status   // nil = all statuses
	Category *Category // nil = all categories
	Severity *Severity // nil = all severities
	Limit    int       // 0 = unlimited
}

// Match returns true if discovery matches filter criteria.
func (f Filter) Match(d *Discovery) bool {
	if f.Status != nil && d.Status != *f.Status {
		return false
	}
	if f.Category != nil && d.Content.Category != *f.Category {
		return false
	}
	if f.Severity != nil && d.Content.Severity != *f.Severity {
		return false
	}
	return true
}

// PromoteOptions configures how a discovery is promoted to a task.
type PromoteOptions struct {
	// Template overrides the auto-detected template.
	// If empty, the template is determined by MapCategoryToTemplate.
	Template string

	// Agent overrides the default AI agent for AI-assisted promotion.
	Agent string

	// Model overrides the default AI model for AI-assisted promotion.
	Model string

	// UseAI enables AI-assisted analysis for optimal task configuration.
	// When true, the AIPromoter is used to determine template and description.
	UseAI bool

	// DryRun, when true, returns the promotion result without actually
	// promoting the discovery or creating a task.
	DryRun bool

	// TaskID allows providing a pre-existing task ID (for backward compatibility).
	// When set, no new task is created; the discovery is linked to this task.
	TaskID string

	// WorkspaceName overrides the auto-generated workspace name.
	WorkspaceName string

	// Description overrides the auto-generated task description.
	Description string
}

// PromoteResult contains the outcome of a promotion operation.
type PromoteResult struct {
	// Discovery is the promoted discovery with updated status.
	Discovery *Discovery

	// TaskID is the ID of the created or linked task.
	TaskID string

	// WorkspaceName is the name of the created workspace.
	WorkspaceName string

	// BranchName is the git branch name for the task.
	BranchName string

	// TemplateName is the template used for the task.
	TemplateName string

	// Description is the task description.
	Description string

	// DryRun indicates if this was a dry-run operation.
	DryRun bool

	// AIAnalysis contains the AI analysis if UseAI was enabled.
	AIAnalysis *AIAnalysis
}
