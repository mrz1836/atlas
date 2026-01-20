// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProgressBar_CreatesBar(t *testing.T) {
	t.Parallel()
	bar := NewProgressBar(40)
	require.NotNil(t, bar)
	assert.Equal(t, 40, bar.Width())
}

func TestProgressBar_Render_VariousPercentages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		percent float64
		wantLen int // minimum expected length
	}{
		{
			name:    "0 percent",
			percent: 0.0,
			wantLen: 1, // at least something rendered
		},
		{
			name:    "25 percent",
			percent: 0.25,
			wantLen: 1,
		},
		{
			name:    "50 percent",
			percent: 0.50,
			wantLen: 1,
		},
		{
			name:    "75 percent",
			percent: 0.75,
			wantLen: 1,
		},
		{
			name:    "100 percent",
			percent: 1.0,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bar := NewProgressBar(40)
			result := bar.Render(tt.percent)
			assert.GreaterOrEqual(t, len(result), tt.wantLen, "bar should render content")
		})
	}
}

func TestProgressBar_Render_ClampsNegative(t *testing.T) {
	t.Parallel()
	bar := NewProgressBar(40)
	result := bar.Render(-0.5)
	// Should not panic and should render something
	assert.NotEmpty(t, result)
}

func TestProgressBar_Render_ClampsOver100(t *testing.T) {
	t.Parallel()
	bar := NewProgressBar(40)
	result := bar.Render(1.5)
	// Should not panic and should render something
	assert.NotEmpty(t, result)
}

func TestProgressBar_WidthAdaptation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		width int
	}{
		{"narrow", 20},
		{"standard", 40},
		{"wide", 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bar := NewProgressBar(tt.width)
			assert.Equal(t, tt.width, bar.Width())
			result := bar.Render(0.5)
			assert.NotEmpty(t, result)
		})
	}
}

func TestProgressBar_SetWidth(t *testing.T) {
	t.Parallel()
	bar := NewProgressBar(40)
	assert.Equal(t, 40, bar.Width())

	bar.SetWidth(60)
	assert.Equal(t, 60, bar.Width())
}

func TestProgressBar_WithWidthOption(t *testing.T) {
	t.Parallel()
	bar := NewProgressBar(40, WithWidth(60))
	assert.Equal(t, 60, bar.Width())
}

func TestProgressBar_NoColor(t *testing.T) {
	// Cannot use t.Parallel() - test uses t.Setenv
	t.Setenv("NO_COLOR", "1")

	bar := NewProgressBar(40)
	result := bar.Render(0.5)

	// Should still render without panic
	assert.NotEmpty(t, result)
}

func TestFormatStepCounter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		current  int
		total    int
		expected string
	}{
		{"basic", 3, 7, "3/7"},
		{"first", 1, 5, "1/5"},
		{"last", 7, 7, "7/7"},
		{"zero", 0, 10, "0/10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FormatStepCounter(tt.current, tt.total)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatStepWithName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		current  int
		total    int
		stepName string
		expected string
	}{
		{"with name", 3, 7, "Validating", "3/7 Validating"},
		{"empty name", 3, 7, "", "3/7"},
		{"long name", 1, 5, "AI Processing", "1/5 AI Processing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FormatStepWithName(tt.current, tt.total, tt.stepName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultStepNameLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		stepType string
		expected string
	}{
		// Step types
		{"ai", "AI Processing"},
		{"validation", "Validating"},
		{"git", "Git Operations"},
		{"github", "GitHub"},
		{"ci", "CI/CD"},
		{"approval", "Awaiting Approval"},
		{"complete", "Complete"},
		// Status values (for when called with task status)
		{"running", "Running"},
		{"validating", "Validating"},
		{"validation_failed", "Validation Failed"},
		{"awaiting_approval", "Awaiting Approval"},
		{"completed", "Complete"},
		{"pending", "Pending"},
		{"rejected", "Rejected"},
		{"abandoned", "Abandoned"},
		{"gh_failed", "GitHub Failed"},
		{"ci_failed", "CI Failed"},
		{"ci_timeout", "CI Timeout"},
		// Unknown returns raw type
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.stepType, func(t *testing.T) {
			t.Parallel()
			result := defaultStepNameLookup(tt.stepType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		taskCount int
		expected  DensityMode
	}{
		{"0 tasks", 0, DensityExpanded},
		{"1 task", 1, DensityExpanded},
		{"5 tasks (boundary)", 5, DensityExpanded},
		{"6 tasks (over boundary)", 6, DensityCompact},
		{"10 tasks", 10, DensityCompact},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mode := DetermineMode(tt.taskCount)
			assert.Equal(t, tt.expected, mode)
		})
	}
}

func TestProgressRowCompact(t *testing.T) {
	t.Parallel()
	row := ProgressRow{
		Name:        "auth",
		Percent:     0.40,
		CurrentStep: 3,
		TotalSteps:  7,
		StepName:    "Validating",
	}

	result := ProgressRowCompact(row, 20)

	assert.Contains(t, result, "40%")
	assert.Contains(t, result, "3/7")
	assert.Contains(t, result, "auth")
}

func TestProgressRowCompact_LongStepName(t *testing.T) {
	t.Parallel()
	row := ProgressRow{
		Name:        "workspace",
		Percent:     0.50,
		CurrentStep: 2,
		TotalSteps:  4,
		StepName:    "VeryLongStepNameThatShouldBeTruncated",
	}

	result := ProgressRowCompact(row, 20)

	// Step name should be truncated to 9 chars + ellipsis
	assert.Contains(t, result, "VeryLongS…")
}

func TestProgressRowCompact_NoStepName(t *testing.T) {
	t.Parallel()
	row := ProgressRow{
		Name:        "workspace",
		Percent:     0.50,
		CurrentStep: 2,
		TotalSteps:  4,
		StepName:    "",
	}

	result := ProgressRowCompact(row, 20)

	assert.Contains(t, result, "50%")
	assert.Contains(t, result, "2/4")
	assert.Contains(t, result, "workspace")
}

func TestProgressRowExpanded(t *testing.T) {
	t.Parallel()
	row := ProgressRow{
		Name:        "auth",
		Percent:     0.40,
		CurrentStep: 3,
		TotalSteps:  7,
		StepName:    "Validating",
		Duration:    "2m 15s",
	}

	result := ProgressRowExpanded(row, 30, 12)

	// Should contain two lines
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 2)

	// Line 1: name + bar + percent
	assert.Contains(t, lines[0], "auth")
	assert.Contains(t, lines[0], "40%")

	// Line 2: step info + duration
	assert.Contains(t, lines[1], "Step 3/7 Validating")
	assert.Contains(t, lines[1], "2m 15s")
}

func TestProgressRowExpanded_NoDuration(t *testing.T) {
	t.Parallel()
	row := ProgressRow{
		Name:        "auth",
		Percent:     0.40,
		CurrentStep: 3,
		TotalSteps:  7,
		StepName:    "Validating",
		Duration:    "",
	}

	result := ProgressRowExpanded(row, 30, 12)

	lines := strings.Split(result, "\n")
	require.Len(t, lines, 2)

	// Line 2 should not contain bullet separator
	assert.NotContains(t, lines[1], "•")
}

func TestProgressRowExpanded_LongName(t *testing.T) {
	t.Parallel()
	row := ProgressRow{
		Name:        "very-long-workspace-name",
		Percent:     0.50,
		CurrentStep: 1,
		TotalSteps:  3,
	}

	result := ProgressRowExpanded(row, 30, 12)

	// Name should be truncated with ellipsis
	assert.Contains(t, result, "very-long-w…")
}

func TestNewProgressDashboard_AutoMode(t *testing.T) {
	t.Parallel()
	// 5 rows -> expanded
	rows5 := make([]ProgressRow, 5)
	for i := range rows5 {
		rows5[i] = ProgressRow{Name: "test", Percent: 0.5}
	}
	pd5 := NewProgressDashboard(rows5)
	assert.Equal(t, DensityExpanded, pd5.Mode())

	// 6 rows -> compact
	rows6 := make([]ProgressRow, 6)
	for i := range rows6 {
		rows6[i] = ProgressRow{Name: "test", Percent: 0.5}
	}
	pd6 := NewProgressDashboard(rows6)
	assert.Equal(t, DensityCompact, pd6.Mode())
}

func TestNewProgressDashboard_ManualModeOverride(t *testing.T) {
	t.Parallel()
	rows := make([]ProgressRow, 10) // Would normally be compact
	for i := range rows {
		rows[i] = ProgressRow{Name: "test", Percent: 0.5}
	}

	pd := NewProgressDashboard(rows, WithDensityMode(DensityExpanded))
	assert.Equal(t, DensityExpanded, pd.Mode())
}

func TestProgressDashboard_Render_Empty(t *testing.T) {
	t.Parallel()
	pd := NewProgressDashboard([]ProgressRow{})

	var buf bytes.Buffer
	err := pd.Render(&buf)

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestProgressDashboard_Render_SingleRow(t *testing.T) {
	t.Parallel()
	rows := []ProgressRow{
		{Name: "auth", Percent: 0.40, CurrentStep: 3, TotalSteps: 7},
	}

	pd := NewProgressDashboard(rows, WithTermWidth(80))

	var buf bytes.Buffer
	err := pd.Render(&buf)

	require.NoError(t, err)
	assert.NotEmpty(t, buf.String())
	assert.Contains(t, buf.String(), "auth")
}

func TestProgressDashboard_Render_MultipleRowsExpanded(t *testing.T) {
	t.Parallel()
	rows := []ProgressRow{
		{Name: "auth", Percent: 0.40, CurrentStep: 3, TotalSteps: 7},
		{Name: "payment", Percent: 0.85, CurrentStep: 6, TotalSteps: 7},
	}

	pd := NewProgressDashboard(rows, WithTermWidth(100))

	var buf bytes.Buffer
	err := pd.Render(&buf)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "auth")
	assert.Contains(t, output, "payment")
	// Should have blank lines between expanded rows
	assert.Greater(t, strings.Count(output, "\n"), 4)
}

func TestProgressDashboard_Render_MultipleRowsCompact(t *testing.T) {
	t.Parallel()
	rows := make([]ProgressRow, 7)
	for i := range rows {
		rows[i] = ProgressRow{
			Name:        "ws" + string(rune('0'+i)),
			Percent:     float64(i) * 0.15,
			CurrentStep: i,
			TotalSteps:  7,
		}
	}

	pd := NewProgressDashboard(rows, WithTermWidth(100))

	var buf bytes.Buffer
	err := pd.Render(&buf)

	require.NoError(t, err)
	output := buf.String()

	// In compact mode, each row is one line
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 7)
}

func TestProgressDashboard_Rows(t *testing.T) {
	t.Parallel()
	rows := []ProgressRow{
		{Name: "test", Percent: 0.5},
	}
	pd := NewProgressDashboard(rows)

	result := pd.Rows()
	assert.Equal(t, rows, result)

	// Verify it's a copy
	result[0].Name = "modified"
	assert.Equal(t, "test", pd.Rows()[0].Name)
}

func TestProgressDashboard_WidthAdaptation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		width     int
		wantWidth int // expected bar width
	}{
		{"narrow terminal", 60, 20},
		{"standard terminal", 80, 40},
		{"medium terminal", 100, 40},
		{"wide terminal", 140, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pd := &ProgressDashboard{
				width: tt.width,
			}
			barWidth := pd.calculateBarWidth()
			assert.Equal(t, tt.wantWidth, barWidth)
		})
	}
}

func TestProgressDashboard_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("0 tasks", func(t *testing.T) {
		t.Parallel()
		pd := NewProgressDashboard([]ProgressRow{})
		var buf bytes.Buffer
		err := pd.Render(&buf)
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})

	t.Run("1 task", func(t *testing.T) {
		t.Parallel()
		pd := NewProgressDashboard([]ProgressRow{{Name: "single", Percent: 0.5}})
		assert.Equal(t, DensityExpanded, pd.Mode())
	})

	t.Run("negative percentage", func(t *testing.T) {
		t.Parallel()
		pd := NewProgressDashboard([]ProgressRow{{Name: "neg", Percent: -0.5}})
		var buf bytes.Buffer
		err := pd.Render(&buf)
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String())
	})

	t.Run("percentage over 100", func(t *testing.T) {
		t.Parallel()
		pd := NewProgressDashboard([]ProgressRow{{Name: "over", Percent: 1.5}})
		var buf bytes.Buffer
		err := pd.Render(&buf)
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String())
	})
}

func TestStepProgress_Struct(t *testing.T) {
	t.Parallel()
	sp := StepProgress{
		Current:  3,
		Total:    7,
		StepName: "Validating",
	}

	assert.Equal(t, 3, sp.Current)
	assert.Equal(t, 7, sp.Total)
	assert.Equal(t, "Validating", sp.StepName)
}

func TestTruncateToRuneWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected string
	}{
		{"short string unchanged", "hello", 10, "hello"},
		{"exact length unchanged", "hello", 5, "hello"},
		{"truncate ASCII", "hello world", 5, "hello"},
		{"empty string", "", 5, ""},
		{"zero width", "hello", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := truncateToRuneWidth(tt.input, tt.maxWidth)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildProgressRowsFromStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rows     []StatusRow
		expected int // expected number of progress rows
	}{
		{
			name:     "empty rows",
			rows:     []StatusRow{},
			expected: 0,
		},
		{
			name: "only running tasks included",
			rows: []StatusRow{
				{Workspace: "running-ws", Status: "running", CurrentStep: 3, TotalSteps: 7},
				{Workspace: "completed-ws", Status: "completed", CurrentStep: 7, TotalSteps: 7},
			},
			expected: 1,
		},
		{
			name: "running and validating included",
			rows: []StatusRow{
				{Workspace: "running-ws", Status: "running", CurrentStep: 3, TotalSteps: 7},
				{Workspace: "validating-ws", Status: "validating", CurrentStep: 5, TotalSteps: 7},
				{Workspace: "pending-ws", Status: "pending", CurrentStep: 0, TotalSteps: 5},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := BuildProgressRowsFromStatus(tt.rows)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestBuildProgressRowsFromStatus_ProgressCalculation(t *testing.T) {
	t.Parallel()
	rows := []StatusRow{
		{Workspace: "test-ws", Status: "running", CurrentStep: 3, TotalSteps: 6},
	}

	result := BuildProgressRowsFromStatus(rows)

	require.Len(t, result, 1)
	assert.Equal(t, "test-ws", result[0].Name)
	assert.InDelta(t, 0.5, result[0].Percent, 0.01) // 3/6 = 0.5
	assert.Equal(t, 3, result[0].CurrentStep)
	assert.Equal(t, 6, result[0].TotalSteps)
	assert.Equal(t, "Running", result[0].StepName) // Now properly mapped
}
