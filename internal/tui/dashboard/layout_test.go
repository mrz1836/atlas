package dashboard_test

import (
	"strings"
	"testing"

	"github.com/mrz1836/atlas/internal/tui/dashboard"
)

func TestLayout_LeftWidth_Narrow(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(60, 24) // narrow < 80
	got := l.LeftWidth()
	if got != 60 {
		t.Errorf("narrow LeftWidth() = %d, want 60", got)
	}
}

func TestLayout_RightWidth_Narrow(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(60, 24)
	got := l.RightWidth()
	if got != 0 {
		t.Errorf("narrow RightWidth() = %d, want 0", got)
	}
}

func TestLayout_LeftWidth_Standard(t *testing.T) {
	t.Parallel()
	// Width=100, SplitRatio=0.30 → left = int(99*0.30) = 29
	l := dashboard.NewLayout(100, 24)
	got := l.LeftWidth()
	totalMinusDivider := float64(100 - 1)
	want := int(totalMinusDivider * 0.30)
	if got != want {
		t.Errorf("standard LeftWidth() = %d, want %d", got, want)
	}
}

func TestLayout_RightWidth_Standard(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(100, 24)
	left := l.LeftWidth()
	right := l.RightWidth()
	// left + 1 (divider) + right should equal total width
	if left+1+right != 100 {
		t.Errorf("left(%d) + divider(1) + right(%d) = %d, want 100", left, right, left+1+right)
	}
}

func TestLayout_LeftWidth_Wide(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(160, 40) // wide >= 120
	got := l.LeftWidth()
	totalMinusDivider := float64(160 - 1)
	want := int(totalMinusDivider * 0.30)
	if got != want {
		t.Errorf("wide LeftWidth() = %d, want %d", got, want)
	}
}

func TestLayout_RightWidth_Wide(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(160, 40)
	left := l.LeftWidth()
	right := l.RightWidth()
	if left+1+right != 160 {
		t.Errorf("left(%d) + divider(1) + right(%d) = %d, want 160", left, right, left+1+right)
	}
}

func TestLayout_ContentHeight(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		height int
		want   int
	}{
		{"standard 24", 24, 24 - 3 - 1}, // 20
		{"tall 40", 40, 40 - 3 - 1},     // 36
		{"very short 5", 5, 1},          // clamped to 1
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := dashboard.NewLayout(100, tt.height)
			got := l.ContentHeight()
			if got != tt.want {
				t.Errorf("ContentHeight() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLayout_Render_Narrow(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(60, 10)
	h := l.ContentHeight()

	leftContent := strings.Repeat("L\n", h)
	rightContent := strings.Repeat("R\n", h)

	result := l.Render(leftContent, rightContent)

	// In narrow mode, no divider should appear.
	if strings.Contains(result, "│") {
		t.Error("narrow Render() should not contain divider '│'")
	}

	// Should not contain right content.
	if strings.Contains(result, "R") {
		t.Error("narrow Render() should not contain right pane content")
	}
}

func TestLayout_Render_Standard(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(100, 10)
	h := l.ContentHeight()

	leftLines := make([]string, h)
	rightLines := make([]string, h)
	for i := range leftLines {
		leftLines[i] = "L"
		rightLines[i] = "R"
	}

	result := l.Render(
		strings.Join(leftLines, "\n"),
		strings.Join(rightLines, "\n"),
	)

	// Divider must be present.
	if !strings.Contains(result, "│") {
		t.Error("standard Render() should contain divider '│'")
	}

	// Both sides should appear.
	if !strings.Contains(result, "L") {
		t.Error("standard Render() should contain left pane content")
	}
	if !strings.Contains(result, "R") {
		t.Error("standard Render() should contain right pane content")
	}

	// Row count should equal ContentHeight.
	rows := strings.Split(result, "\n")
	if len(rows) != h {
		t.Errorf("standard Render() row count = %d, want %d", len(rows), h)
	}
}

func TestLayout_Render_Wide(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(160, 10)
	h := l.ContentHeight()

	leftLines := make([]string, h)
	rightLines := make([]string, h)
	for i := range leftLines {
		leftLines[i] = "L"
		rightLines[i] = "R"
	}

	result := l.Render(
		strings.Join(leftLines, "\n"),
		strings.Join(rightLines, "\n"),
	)

	if !strings.Contains(result, "│") {
		t.Error("wide Render() should contain divider '│'")
	}
	rows := strings.Split(result, "\n")
	if len(rows) != h {
		t.Errorf("wide Render() row count = %d, want %d", len(rows), h)
	}
}

func TestLayout_Render_EmptyContent(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(100, 10)
	// Should not panic with empty strings.
	result := l.Render("", "")
	if result == "" {
		t.Error("Render() with empty content should return non-empty string (spaces)")
	}
}

func TestLayout_CustomSplitRatio(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(100, 24)
	l.SplitRatio = 0.50

	left := l.LeftWidth()
	right := l.RightWidth()

	if left+1+right != 100 {
		t.Errorf("50/50 split: left(%d) + 1 + right(%d) = %d, want 100", left, right, left+1+right)
	}
	// With 0.50 ratio the sides should be roughly equal.
	diff := left - right
	if diff < -5 || diff > 5 {
		t.Errorf("50/50 split: left=%d right=%d differ by more than 5", left, right)
	}
}
