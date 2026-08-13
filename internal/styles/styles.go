// Package styles centralizes the aiworker-cli look: a rounded-border,
// box-drawing-heavy terminal UI that adapts to the terminal width instead of
// wrapping badly.
package styles

import "github.com/charmbracelet/lipgloss"

// Palette
var (
	ColorAccent  = lipgloss.Color("#00D9C0") // teal - brand accent
	ColorGood    = lipgloss.Color("#32CD32")
	ColorBad     = lipgloss.Color("#FF4C4C")
	ColorWarn    = lipgloss.Color("#FFB347")
	ColorDim     = lipgloss.Color("241")
	ColorText    = lipgloss.Color("#EEEEEE")
	ColorBorder  = lipgloss.Color("#3A3A3A")
	ColorBorderF = lipgloss.Color("#00D9C0")
)

var (
	Title = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

	Subtitle = lipgloss.NewStyle().Foreground(ColorDim).Italic(true)

	Container = lipgloss.NewStyle().Margin(1, 2)

	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	PanelFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderF).
			Padding(0, 1)

	ItemName = lipgloss.NewStyle().Bold(true).Foreground(ColorText)

	ItemNameSelected = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

	ItemDesc = lipgloss.NewStyle().Foreground(ColorDim)

	Help = lipgloss.NewStyle().
		Foreground(ColorDim).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(ColorBorder).
		PaddingTop(1).
		MarginTop(1)

	Success = lipgloss.NewStyle().Bold(true).Foreground(ColorGood)
	Error   = lipgloss.NewStyle().Bold(true).Foreground(ColorBad)
	Warn    = lipgloss.NewStyle().Bold(true).Foreground(ColorWarn)
	Dim     = lipgloss.NewStyle().Foreground(ColorDim)

	Label = lipgloss.NewStyle().Foreground(ColorDim).Width(20)

	InputPrompt = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
)

// Dot renders a filled/empty status dot.
func Dot(on bool) string {
	if on {
		return Success.Render("●")
	}
	return Dim.Render("○")
}

// Check renders a pass/fail glyph.
func Check(ok bool) string {
	if ok {
		return Success.Render("✓")
	}
	return Error.Render("✗")
}
