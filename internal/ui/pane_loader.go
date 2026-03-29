package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) loaderPane(width int) string {
	borderStyle := BorderStyle.Width(width)
	if m.focused == paneLoader {
		borderStyle = FocusBorderStyle.Width(width)
	}

	header := m.paneTitleRow("loader.conf", m.editing && m.focused == paneLoader, width)

	var content strings.Builder
	if m.loaderConf == nil {
		content.WriteString(" No loader.conf found\n")
	} else {
		fields := []struct {
			key   string
			value string
		}{
			{"timeout", m.loaderConf.Timeout},
			{"default", m.loaderConf.Default},
			{"editor", boolToString(m.loaderConf.Editor)},
			{"console-mode", m.loaderConf.ConsoleMode},
			{"auto-entries", boolToString(m.loaderConf.AutoEntries)},
			{"auto-firmware", boolToString(m.loaderConf.AutoFirmware)},
		}

		for _, f := range fields {
			if f.value != "" {
				content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render(f.key+":"), ValueStyle.Render(f.value)))
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
