package main

import "github.com/charmbracelet/lipgloss"

var (
	colorYellow  = lipgloss.Color("3")
	colorCyan    = lipgloss.Color("6")
	colorGreen   = lipgloss.Color("2")
	colorRed     = lipgloss.Color("1")
	colorMagenta = lipgloss.Color("5")
	colorGrey    = lipgloss.Color("8")
	colorAccent  = lipgloss.Color("12")

	colorHighlight = lipgloss.AdaptiveColor{Light: "254", Dark: "237"}
)

var (
	styleDim      = lipgloss.NewStyle().Foreground(colorGrey)
	styleStruck   = lipgloss.NewStyle().Foreground(colorGrey).Strikethrough(true)
	styleUnread   = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	stylePending  = lipgloss.NewStyle().Foreground(colorYellow)
	styleGood     = lipgloss.NewStyle().Foreground(colorGreen)
	styleBad      = lipgloss.NewStyle().Foreground(colorRed)
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleHeading  = lipgloss.NewStyle().Bold(true).Underline(true)
	styleTab      = lipgloss.NewStyle().Padding(0, 1).Foreground(colorGrey)
	styleTabFocus = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("0")).Background(colorAccent)
	styleError    = lipgloss.NewStyle().Foreground(colorRed)
	styleDetail   = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(colorAccent)
)

func stateStyle(state PullRequestState) lipgloss.Style {
	switch state {
	case PullRequestOpen:
		return styleGood
	case PullRequestDraft:
		return stylePending
	case PullRequestMerged:
		return lipgloss.NewStyle().Foreground(colorMagenta)
	default:
		return styleBad
	}
}
