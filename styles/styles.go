package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Adaptive colors for light/dark themes
	// primaryColor   = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#f0f0f0"}
	// secondaryColor = lipgloss.AdaptiveColor{Light: "#4a4a4a", Dark: "#d0d0d0"}
	AccentColor  = lipgloss.AdaptiveColor{Light: "#005f87", Dark: "#00afd7"}
	SuccessColor = lipgloss.AdaptiveColor{Light: "#008700", Dark: "#00d700"}
	WarningColor = lipgloss.AdaptiveColor{Light: "#af8700", Dark: "#ffd700"}
	ErrorColor   = lipgloss.AdaptiveColor{Light: "#ff7092", Dark: "#ff7092"}
	// highlightColor = lipgloss.AdaptiveColor{Light: "#ffd700", Dark: "#ffaf00"}
	// mutedColor     = lipgloss.AdaptiveColor{Light: "#6c757d", Dark: "#adb5bd"}
	Purple = lipgloss.AdaptiveColor{Light: "#6920e8", Dark: "#8454fc"}
)

var BtnStyleV2 = lipgloss.
	NewStyle().
	Transform(func(text string) string { return fmt.Sprintf("[ %s ]", text) })

var PositiveBtn = BtnStyleV2.
	Background(SuccessColor).
	Foreground(lipgloss.Color("#ffffff"))

var NegativeBtn = BtnStyleV2.
	Background(ErrorColor).
	Foreground(lipgloss.Color("#ffffff"))
