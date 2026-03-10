package tui

import (
	"image/color"
	"os"
	"strings"
	"sync"
)

// hasDarkBackground caches the terminal background detection result.
//
//nolint:gochecknoglobals // Intentional package-level cache for background detection
var (
	hasDarkBackground     bool
	hasDarkBackgroundOnce sync.Once
)

// detectDarkBackground determines if the terminal has a dark background.
// Uses the COLORFGBG environment variable (format: "fg;bg"), where bg < 8
// typically indicates a dark background. Defaults to true (dark) when
// detection fails, matching lipgloss behavior.
func detectDarkBackground() bool {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg == "" {
		return true // default to dark
	}

	// COLORFGBG format: "foreground;background" (e.g., "15;0" = white on black)
	parts := strings.Split(colorfgbg, ";")
	if len(parts) < 2 {
		return true
	}

	bg := parts[len(parts)-1]
	// Background colors 0-6 are dark, 7+ are light (standard terminal convention)
	switch bg {
	case "0", "1", "2", "3", "4", "5", "6":
		return true
	default:
		return false
	}
}

// isDarkBackground returns the cached dark-background detection result.
func isDarkBackground() bool {
	hasDarkBackgroundOnce.Do(func() {
		hasDarkBackground = detectDarkBackground()
	})
	return hasDarkBackground
}

// AdaptiveColor selects between Light and Dark color variants based on
// the terminal background. Unlike compat.AdaptiveColor, this does NOT
// query the terminal via OSC 11, avoiding garbage characters in some
// terminal emulators (e.g., Wave).
//
// Detection uses the COLORFGBG environment variable, defaulting to dark
// background when unavailable.
type AdaptiveColor struct {
	Light color.Color
	Dark  color.Color
}

// RGBA implements the color.Color interface.
func (ac AdaptiveColor) RGBA() (r, g, b, a uint32) {
	if isDarkBackground() {
		return ac.Dark.RGBA()
	}
	return ac.Light.RGBA()
}
