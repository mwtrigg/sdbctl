package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) entriesPane(width int) string {
	borderStyle := BorderStyle.Width(width)
	if m.focused == paneEntries {
		borderStyle = FocusBorderStyle.Width(width)
	}

	header := m.paneTitleRow("Boot Entries", false, width)

	var items strings.Builder
	for i, entry := range m.entries {
		title := entry.DisplayTitle()
		badge := ""
		if entry.IsUKI {
			badge = UKIBadgeStyle.Render("[UKI] ")
		} else if len(m.entryWarnings[entry.ID]) > 0 {
			badge = WarnBadgeStyle.Render("[!] ")
		}

		style := ItemStyle.Width(width)
		if i == m.selected {
			style = SelectedItemStyle.Width(width)
		}
		line := badge + style.Render(truncate(title, width-lipgloss.Width(badge)))
		items.WriteString(line + "\n")
	}

	if len(m.entries) == 0 {
		items.WriteString(ItemStyle.Width(width).Render(" No entries ") + "\n")
	}

	content := items.String()

	return borderStyle.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			header,
			content,
		),
	)
}
