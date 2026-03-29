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

		style := ItemStyle.Width(width)
		if i == m.selected {
			style = SelectedItemStyle.Width(width)
		}
		items.WriteString(style.Render(truncate(title, width)) + "\n")
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
