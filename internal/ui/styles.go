package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Nord color palette
var (
	ColorBorderDim    = lipgloss.Color("#4C566A")
	ColorBorderFocus  = lipgloss.Color("#88C0D0")
	ColorKeyText      = lipgloss.Color("#81A1C1")
	ColorValueText    = lipgloss.Color("#ECEFF4")
	ColorSelected     = lipgloss.Color("#5E81AC")
	ColorSelectedText = lipgloss.Color("#ECEFF4")
	ColorHeaderBg     = lipgloss.Color("#2E3440")
	ColorHeaderText   = lipgloss.Color("#ECEFF4")
	ColorMeta         = lipgloss.Color("#616E88")
	ColorAmber        = lipgloss.Color("#EBCB8B")
	ColorError        = lipgloss.Color("#BF616A")
	ColorUKI          = lipgloss.Color("#A3BE8C") // Nord aurora green

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderDim)

	FocusBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderFocus)

	PaneTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBorderFocus).
			Padding(0, 1)

	PaneEditingStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAmber).
				Padding(0, 1)

	PaneContentStyle = lipgloss.NewStyle().
				Foreground(ColorValueText)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(ColorSelectedText).
				Background(ColorSelected)

	ItemStyle = lipgloss.NewStyle().
			Foreground(ColorValueText)

	KeyStyle = lipgloss.NewStyle().
			Foreground(ColorKeyText)

	ValueStyle = lipgloss.NewStyle().
			Foreground(ColorValueText)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMeta)

	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorError)

	DimValueStyle = lipgloss.NewStyle().
			Foreground(ColorMeta)

	FieldHelpStyle = lipgloss.NewStyle().
			Foreground(ColorMeta).
			Italic(true)

	RequiredStyle = lipgloss.NewStyle().
			Foreground(ColorAmber).
			Bold(true)

	UKIBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorUKI).
			Bold(true)

	WarnBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorAmber).
			Bold(true)

	DiffOldStyle   = lipgloss.NewStyle().Foreground(ColorError)
	DiffNewStyle   = lipgloss.NewStyle().Foreground(ColorUKI).Bold(true)
	DiffArrowStyle = lipgloss.NewStyle().Foreground(ColorMeta)

	HeaderTitleStyle = lipgloss.NewStyle().
				Foreground(ColorHeaderText).
				Background(ColorHeaderBg).
				Bold(true)

	HeaderMetaStyle = lipgloss.NewStyle().
				Foreground(ColorMeta).
				Background(ColorHeaderBg)

	OuterFrame = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderFocus).
			Padding(0, 0)

	HelpModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderFocus).
			Foreground(ColorValueText).
			Padding(1, 2)

	HelpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	AppVersion = "v1.0.0"
	AppRepo    = "https://codeberg.org/mwtrigg/sdboot-tui"
)
