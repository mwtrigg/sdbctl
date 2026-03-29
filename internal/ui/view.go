package ui

import (
	"github.com/charmbracelet/lipgloss"
)

func (m *model) View() string {
	header := m.header()

	leftColWidth := m.leftPaneWidth()

	leftColumn := lipgloss.JoinVertical(lipgloss.Top,
		m.entriesPane(leftColWidth),
		m.loaderPane(leftColWidth),
	)

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top,
		leftColumn,
		m.detailPane(),
	)

	helpBar := m.helpBar()

	appContent := lipgloss.JoinVertical(lipgloss.Top,
		header,
		mainArea,
		helpBar,
	)

	if m.showHelp {
		return m.helpOverlay()
	}

	return OuterFrame.Render(appContent)
}

func (m *model) leftPaneWidth() int {
	w := m.width
	if w <= 0 {
		w = 100
	}
	maxLeft := w / 6
	if maxLeft < 24 {
		maxLeft = 24
	}
	if maxLeft > 30 {
		maxLeft = 30
	}
	return maxLeft
}

func (m *model) helpOverlay() string {
	helpContent := lipgloss.JoinVertical(lipgloss.Top,
		HelpTitleStyle.Render(" sdboot-tui Help "),
		"",
		"  Navigation",
		"  ──────────────────────────────────────",
		"  ↑/↓ or j/k    Navigate boot entry list",
		"  ←/→ or Tab    Switch between panes",
		"  Shift+Tab     Previous pane",
		"",
		"  Actions",
		"  ──────────────────────────────────────",
		"  e             Toggle edit mode",
		"  Enter         Save changes (in edit mode)",
		"  Esc           Cancel editing",
		"  ?             Toggle this help",
		"  q             Quit",
		"",
		"  About",
		"  ──────────────────────────────────────",
		"  sdboot-tui "+AppVersion,
		"  "+AppRepo,
		"",
		"  Press ? or Esc to close",
	)

	modal := HelpModalStyle.Render(helpContent)

	w := m.width
	h := m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func (m *model) helpBar() string {
	switch m.focused {
	case paneEntries:
		if m.editing {
			return HelpStyle.Render(" ↑/↓: navigate  |  Tab: switch  |  Enter: save  |  Esc: cancel  |  ?: help")
		}
		return HelpStyle.Render(" ↑/↓: navigate  |  Tab: switch  |  e: edit  |  q: quit  |  ?: help")
	case paneDetail:
		if m.editing {
			return HelpStyle.Render(" e: stop  |  Tab: switch  |  Enter: save  |  Esc: cancel  |  ?: help")
		}
		return HelpStyle.Render(" e: edit  |  Tab: switch  |  q: quit  |  ?: help")
	case paneLoader:
		if m.editing {
			return HelpStyle.Render(" e: stop  |  Tab: switch  |  Enter: save  |  Esc: cancel  |  ?: help")
		}
		return HelpStyle.Render(" e: edit  |  Tab: switch  |  q: quit  |  ?: help")
	}
	return ""
}

func (m *model) paneTitleRow(title string, editing bool, width int) string {
	titleText := " " + title + " "
	if editing {
		titleText += "[EDITING] "
	}

	headerStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(DimmerGreen).
		Foreground(DimmerGreen).
		Padding(0, 1).
		Width(width)

	return headerStyle.Render(titleText)
}

func (m *model) header() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#1E3A5F")).
		Bold(true)

	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#88AACC")).
		Background(lipgloss.Color("#1E3A5F")).
		Bold(true)

	repoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#88AACC")).
		Background(lipgloss.Color("#1E3A5F"))

	headerContent := lipgloss.JoinHorizontal(lipgloss.Left,
		titleStyle.Render(" sdboot-tui "),
		versionStyle.Render(" "+AppVersion+" "),
		repoStyle.Render(" "+AppRepo),
	)

	headerBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(DimmerGreen).
		Margin(3).
		Width(m.width - 8).
		Padding(0, 1)

	return headerBox.Render(headerContent)
}
