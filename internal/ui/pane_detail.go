package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) detailPane() string {
	borderStyle := BorderStyle
	if m.focused == paneDetail {
		borderStyle = FocusBorderStyle
	}

	header := m.paneTitleRow("Entry Details", m.editing, 0)

	var content strings.Builder
	if m.entry == nil {
		content.WriteString(" Select an entry from the list\n")
	} else {
		content.WriteString(fmt.Sprintf(" ID: %s\n\n", m.entry.ID))

		fields := []struct {
			key   string
			value string
		}{
			{"title", m.entry.Title},
			{"version", m.entry.Version},
			{"machine-id", m.entry.MachineID},
			{"sort-key", m.entry.SortKey},
			{"architecture", m.entry.Architecture},
			{"linux", m.entry.Linux},
		}

		for _, f := range fields {
			if f.value != "" {
				content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render(f.key+":"), ValueStyle.Render(f.value)))
			}
		}

		if len(m.entry.Initrds) > 0 {
			content.WriteString("\n")
			for _, initrd := range m.entry.Initrds {
				content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("initrd:"), ValueStyle.Render(initrd)))
			}
		}

		if len(m.entry.Options) > 0 {
			content.WriteString("\n")
			for _, opt := range m.entry.Options {
				content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("options:"), ValueStyle.Render(opt)))
			}
		}
	}

	return borderStyle.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			header,
			content.String(),
		),
	)
}
