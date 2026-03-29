package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	DimmerGreen = lipgloss.Color("#229922")
	BrightGreen = lipgloss.Color("#33FF33")

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(DimmerGreen)

	FocusBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(BrightGreen)

	PaneTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(DimmerGreen).
			Padding(0, 1)

	PaneEditingStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFAA00")).
			Padding(0, 1)

	PaneContentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33CC33"))

	SelectedItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#001100")).
			Background(lipgloss.Color("#33FF33"))

	ItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33CC33"))

	KeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#229922"))

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33FF33"))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	OuterFrame = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("#33FF33")).
			Padding(0, 0)

	HelpModalStyle = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("#33FF33")).
			Foreground(lipgloss.Color("#33FF33")).
			Padding(1, 2)

	HelpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	AppVersion = "v0.1.0"
	AppRepo    = "https://codeberg.org/mwtrigg/sdboot-tui"
)
