package hook

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// DefaultMarkdownGenerator generates HOOK.md content from hook state.
type DefaultMarkdownGenerator struct{}

// NewMarkdownGenerator creates a new DefaultMarkdownGenerator.
func NewMarkdownGenerator() *DefaultMarkdownGenerator {
	return &DefaultMarkdownGenerator{}
}

// Generate creates the HOOK.md content from hook state.
func (g *DefaultMarkdownGenerator) Generate(hook *domain.Hook) ([]byte, error) {
	data := g.buildTemplateData(hook)

	tmpl, err := template.New("hookmd").Funcs(template.FuncMap{
		"formatTime":     formatTime,
		"formatDuration": formatDuration,
		"formatRelative": formatRelativeTime,
	}).Parse(hookMarkdownTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// templateData holds all data needed for the HOOK.md template.
type templateData struct {
	TaskID         string
	WorkspaceID    string
	State          string
	StateEmoji     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	HasCurrentStep bool
	CurrentStep    *stepData
	HasRecovery    bool
	Recovery       *recoveryData
	HasCheckpoints bool
	Checkpoints    []checkpointData
	HasReceipts    bool
	Receipts       []receiptData
	HasHistory     bool
	History        []historyData
}

type stepData struct {
	Name         string
	Index        int
	TotalSteps   int
	Attempt      int
	MaxAttempts  int
	WorkingOn    string
	FilesTouched []string
	LastOutput   string
	CheckpointID string
}

type recoveryData struct {
	RecommendedAction string
	ActionEmoji       string
	ActionLabel       string
	Reason            string
	CrashType         string
	LastKnownState    string
	WasValidating     bool
	ValidationCmd     string
	PartialOutput     string
	LastCheckpointID  string
}

type checkpointData struct {
	ID           string
	CreatedAt    time.Time
	Trigger      string
	TriggerEmoji string
	StepName     string
	Description  string
	GitBranch    string
	GitCommit    string
	GitDirty     bool
}

type receiptData struct {
	ID          string
	StepName    string
	Command     string
	ExitCode    int
	Duration    string
	CompletedAt time.Time
	Valid       bool
}

type historyData struct {
	Timestamp time.Time
	FromState string
	ToState   string
	Trigger   string
	StepName  string
}

func (g *DefaultMarkdownGenerator) buildTemplateData(hook *domain.Hook) *templateData {
	data := &templateData{
		TaskID:      hook.TaskID,
		WorkspaceID: hook.WorkspaceID,
		State:       string(hook.State),
		StateEmoji:  getStateEmoji(hook.State),
		CreatedAt:   hook.CreatedAt,
		UpdatedAt:   hook.UpdatedAt,
	}

	// Current step
	if hook.CurrentStep != nil {
		data.HasCurrentStep = true
		data.CurrentStep = &stepData{
			Name:         hook.CurrentStep.StepName,
			Index:        hook.CurrentStep.StepIndex,
			Attempt:      hook.CurrentStep.Attempt,
			MaxAttempts:  hook.CurrentStep.MaxAttempts,
			WorkingOn:    hook.CurrentStep.WorkingOn,
			FilesTouched: hook.CurrentStep.FilesTouched,
			LastOutput:   hook.CurrentStep.LastOutput,
			CheckpointID: hook.CurrentStep.CurrentCheckpointID,
		}
	}

	// Recovery context
	if hook.Recovery != nil {
		data.HasRecovery = true
		data.Recovery = &recoveryData{
			RecommendedAction: hook.Recovery.RecommendedAction,
			ActionEmoji:       getActionEmoji(hook.Recovery.RecommendedAction),
			ActionLabel:       getActionLabel(hook.Recovery.RecommendedAction),
			Reason:            hook.Recovery.Reason,
			CrashType:         hook.Recovery.CrashType,
			LastKnownState:    string(hook.Recovery.LastKnownState),
			WasValidating:     hook.Recovery.WasValidating,
			ValidationCmd:     hook.Recovery.ValidationCmd,
			PartialOutput:     hook.Recovery.PartialOutput,
			LastCheckpointID:  hook.Recovery.LastCheckpointID,
		}
	}

	// Checkpoints
	if len(hook.Checkpoints) > 0 {
		data.HasCheckpoints = true
		for _, cp := range hook.Checkpoints {
			data.Checkpoints = append(data.Checkpoints, checkpointData{
				ID:           cp.CheckpointID,
				CreatedAt:    cp.CreatedAt,
				Trigger:      string(cp.Trigger),
				TriggerEmoji: getTriggerEmoji(cp.Trigger),
				StepName:     cp.StepName,
				Description:  cp.Description,
				GitBranch:    cp.GitBranch,
				GitCommit:    cp.GitCommit,
				GitDirty:     cp.GitDirty,
			})
		}
	}

	// Receipts
	if len(hook.Receipts) > 0 {
		data.HasReceipts = true
		for _, r := range hook.Receipts {
			data.Receipts = append(data.Receipts, receiptData{
				ID:          r.ReceiptID,
				StepName:    r.StepName,
				Command:     r.Command,
				ExitCode:    r.ExitCode,
				Duration:    r.Duration,
				CompletedAt: r.CompletedAt,
				Valid:       r.Signature != "",
			})
		}
	}

	// History (last 10 events)
	if len(hook.History) > 0 {
		data.HasHistory = true
		start := 0
		if len(hook.History) > 10 {
			start = len(hook.History) - 10
		}
		for _, h := range hook.History[start:] {
			data.History = append(data.History, historyData{
				Timestamp: h.Timestamp,
				FromState: string(h.FromState),
				ToState:   string(h.ToState),
				Trigger:   h.Trigger,
				StepName:  h.StepName,
			})
		}
	}

	return data
}

func getStateEmoji(state domain.HookState) string {
	switch state {
	case domain.HookStateInitializing:
		return "🔄"
	case domain.HookStateStepPending:
		return "⏳"
	case domain.HookStateStepRunning:
		return "▶️"
	case domain.HookStateStepValidating:
		return "🔍"
	case domain.HookStateAwaitingHuman:
		return "👤"
	case domain.HookStateRecovering:
		return "🔧"
	case domain.HookStateCompleted:
		return "✅"
	case domain.HookStateFailed:
		return "❌"
	case domain.HookStateAbandoned:
		return "🚫"
	default:
		return "❓"
	}
}

func getActionEmoji(action string) string {
	switch action {
	case "retry_step":
		return "🔄"
	case "retry_from_checkpoint":
		return "⏪"
	case "skip_step":
		return "⏭️"
	case "manual":
		return "👤"
	default:
		return "❓"
	}
}

func getActionLabel(action string) string {
	switch action {
	case "retry_step":
		return "Retry Step"
	case "retry_from_checkpoint":
		return "Retry from Checkpoint"
	case "skip_step":
		return "Skip Step"
	case "manual":
		return "Manual Intervention"
	default:
		return action
	}
}

func getTriggerEmoji(trigger domain.CheckpointTrigger) string {
	switch trigger {
	case domain.CheckpointTriggerManual:
		return "✋"
	case domain.CheckpointTriggerCommit:
		return "💾"
	case domain.CheckpointTriggerPush:
		return "📤"
	case domain.CheckpointTriggerPR:
		return "🔀"
	case domain.CheckpointTriggerValidation:
		return "✓"
	case domain.CheckpointTriggerStepComplete:
		return "✅"
	case domain.CheckpointTriggerInterval:
		return "⏱️"
	default:
		return "📍"
	}
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05 MST")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

const hookMarkdownTemplate = `# ATLAS Task Recovery Hook

> ⚠️ **READ THIS FIRST** - This file contains your recovery context.

## Current State: {{.StateEmoji}} ` + "`{{.State}}`" + `

**Task:** {{.TaskID}}
**Workspace:** {{.WorkspaceID}}
**Created:** {{formatTime .CreatedAt}}
**Last Updated:** {{formatTime .UpdatedAt}} ({{formatRelative .UpdatedAt}})

---
{{if .HasCurrentStep}}
## Current Step

| Field | Value |
|-------|-------|
| Step Name | ` + "`{{.CurrentStep.Name}}`" + ` |
| Step Index | {{.CurrentStep.Index}} |
| Attempt | {{.CurrentStep.Attempt}}/{{.CurrentStep.MaxAttempts}} |
{{if .CurrentStep.WorkingOn}}| Working On | {{.CurrentStep.WorkingOn}} |{{end}}
{{if .CurrentStep.CheckpointID}}| Last Checkpoint | ` + "`{{.CurrentStep.CheckpointID}}`" + ` |{{end}}

{{if .CurrentStep.FilesTouched}}
### Files Touched
{{range .CurrentStep.FilesTouched}}
- ` + "`{{.}}`" + `
{{end}}
{{end}}

{{if .CurrentStep.LastOutput}}
### Last Output
` + "```" + `
{{.CurrentStep.LastOutput}}
` + "```" + `
{{end}}
{{end}}

---
{{if .HasRecovery}}
## 🚨 What To Do Now

### {{.Recovery.ActionEmoji}} {{.Recovery.ActionLabel}}

{{.Recovery.Reason}}

{{if eq .Recovery.RecommendedAction "retry_step"}}
**Action:** Re-run the current step from the beginning. This step is idempotent (safe to repeat).
{{else if eq .Recovery.RecommendedAction "retry_from_checkpoint"}}
**Action:** Resume from checkpoint ` + "`{{.Recovery.LastCheckpointID}}`" + `. Review changes since the checkpoint.
{{else if eq .Recovery.RecommendedAction "skip_step"}}
**Action:** Skip this step and proceed to the next one. The step was likely completing.
{{else if eq .Recovery.RecommendedAction "manual"}}
**Action:** Manual review is required. Check the files touched and git status before proceeding.

1. Run ` + "`git status`" + ` to see uncommitted changes
2. Review files touched: {{range $.CurrentStep.FilesTouched}}` + "`{{.}}`" + ` {{end}}
3. Decide whether to continue, rollback, or start fresh
{{end}}

{{if .Recovery.WasValidating}}
> ⚠️ **Note:** Crash occurred during validation (` + "`{{.Recovery.ValidationCmd}}`" + `)
{{end}}

{{if .Recovery.PartialOutput}}
### Partial Output
` + "```" + `
{{.Recovery.PartialOutput}}
` + "```" + `
{{end}}

---
{{end}}

## ❌ DO NOT

- **Do NOT** start the task from the beginning
- **Do NOT** repeat steps that are already completed (see below)
- **Do NOT** ignore the recovery recommendations
{{if .HasCheckpoints}}
- **Do NOT** create duplicate commits for work already checkpointed
{{end}}

---
{{if .HasReceipts}}
## Completed Steps (Validation Receipts)

| Step | Command | Exit | Duration | Verified |
|------|---------|------|----------|----------|
{{range .Receipts}}| {{.StepName}} | ` + "`{{.Command}}`" + ` | {{.ExitCode}} | {{.Duration}} | {{if .Valid}}✓{{else}}⚠{{end}} |
{{end}}

---
{{end}}
{{if .HasCheckpoints}}
## Checkpoint Timeline

| ID | Time | Trigger | Step | Description |
|----|------|---------|------|-------------|
{{range .Checkpoints}}| ` + "`{{.ID}}`" + ` | {{formatTime .CreatedAt}} | {{.TriggerEmoji}} {{.Trigger}} | {{.StepName}} | {{.Description}} |
{{end}}

---
{{end}}
{{if .HasHistory}}
## State History (Last 10)

| Time | From | To | Trigger |
|------|------|-----|---------|
{{range .History}}| {{formatTime .Timestamp}} | {{.FromState}} | {{.ToState}} | {{.Trigger}} |
{{end}}

---
{{end}}

## Troubleshooting

### Check git status
` + "```bash" + `
git status
git log --oneline -5
` + "```" + `

### Regenerate this file
` + "```bash" + `
atlas hook regenerate
` + "```" + `

### View full hook state
` + "```bash" + `
atlas hook export
` + "```" + `

---

*Generated by ATLAS Hook System*
`

// GenerateHookMarkdown is a convenience function to generate HOOK.md content.
func GenerateHookMarkdown(hook *domain.Hook) ([]byte, error) {
	gen := NewMarkdownGenerator()
	return gen.Generate(hook)
}

// TruncateString truncates a string to the specified length with ellipsis.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FormatFilesTouchedList formats a list of files for display.
func FormatFilesTouchedList(files []string, maxFiles int) string {
	if len(files) == 0 {
		return "(none)"
	}
	if len(files) <= maxFiles {
		return strings.Join(files, ", ")
	}
	return strings.Join(files[:maxFiles], ", ") + fmt.Sprintf(" (+%d more)", len(files)-maxFiles)
}
