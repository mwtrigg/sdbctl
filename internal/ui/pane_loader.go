package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

func (m *model) loaderPane(width int) string {
	borderStyle := BorderStyle.Width(width)
	if m.focused == paneLoader {
		borderStyle = FocusBorderStyle.Width(width)
	}

	isEditing := m.editing && m.focused == paneLoader
	header := m.paneTitleRow("Loader", isEditing, width)

	var content strings.Builder

	if isEditing && m.draftLoader != nil {
		if len(m.editFields) == 0 {
			content.WriteString(FieldHelpStyle.Render(" (no editable fields)") + "\n")
		} else {
			for i, f := range m.editFields {
				label := f.label
				if f.required {
					label += "*"
				}
				keyLabel := KeyStyle.Render(label + ":")

				if i == m.editFieldIdx {
					inputWidth := width - len(label) - 4
					if inputWidth < 8 {
						inputWidth = 8
					}
					m.editInput.Width = inputWidth
					content.WriteString(fmt.Sprintf(" %s %s\n", keyLabel, m.editInput.View()))
				} else {
					val := f.get()
					var rendered string
					if val == "" {
						rendered = DimValueStyle.Render("(empty)")
					} else {
						rendered = DimValueStyle.Render(truncate(val, width-len(label)-4))
					}
					content.WriteString(fmt.Sprintf(" %s %s\n", keyLabel, rendered))
				}
			}

			// Validation error
			if m.validationErr != "" {
				content.WriteString("\n")
				content.WriteString(" " + ErrorStyle.Render("✗ "+m.validationErr) + "\n")
			}

			// Context help for the active field
			if m.editFieldIdx < len(m.editFields) {
				f := m.editFields[m.editFieldIdx]
				if f.help != "" {
					content.WriteString("\n")
					content.WriteString(KeyStyle.Render(strings.Repeat("─", width)) + "\n")
					for _, line := range wrapText(f.help, width-2) {
						content.WriteString(" " + FieldHelpStyle.Render(line) + "\n")
					}
					if f.required {
						content.WriteString(" " + RequiredStyle.Render("* required field") + "\n")
					}
				}
			}
		}
	} else {
		// View mode
		if m.loaderConf == nil {
			content.WriteString(" No loader.conf found\n")
		} else {
			fields := []struct {
				key   string
				value string
			}{
				{"timeout", m.loaderConf.Timeout},
				{"default", m.loaderConf.Default},
				{"preferred", m.loaderConf.Preferred},
				{"editor", boolToString(m.loaderConf.Editor)},
				{"console-mode", m.loaderConf.ConsoleMode},
				{"auto-entries", boolToString(m.loaderConf.AutoEntries)},
				{"auto-firmware", boolToString(m.loaderConf.AutoFirmware)},
				{"auto-reboot", boolToString(m.loaderConf.AutoReboot)},
				{"auto-poweroff", boolToString(m.loaderConf.AutoPowerOff)},
				{"beep", boolToString(m.loaderConf.Beep)},
				{"reboot-for-bitlocker", boolToString(m.loaderConf.RebootForBitlocker)},
				{"reboot-on-error", m.loaderConf.RebootOnError},
				{"secure-boot-enroll", m.loaderConf.SecureBootEnroll},
				{"secure-boot-enroll-action", m.loaderConf.SecureBootEnrollAction},
				{"secure-boot-enroll-timeout-sec", m.loaderConf.SecureBootEnrollTimeoutSec},
				{"log-level", m.loaderConf.LogLevel},
			}

			for _, f := range fields {
				if f.value != "" {
					content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render(f.key+":"), ValueStyle.Render(f.value)))
				}
			}

			for _, kv := range m.loaderConf.Extras {
				content.WriteString(fmt.Sprintf(" %s %s\n",
					KeyStyle.Render(kv.Key+":"), DimValueStyle.Render(kv.Value+"  (untouched)")))
			}

			if msg := boot.CheckDefaultEntry(m.loaderConf, m.entries); msg != "" {
				content.WriteString("\n")
				for _, line := range wrapText(msg, width-2) {
					content.WriteString(" " + ErrorStyle.Render("! "+line) + "\n")
				}
			}

			for _, issue := range m.loaderConf.Issues {
				content.WriteString(" " + ErrorStyle.Render("! "+issue) + "\n")
			}

			if m.loaderModTime != "" {
				content.WriteString("\n")
				content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("modified:"), HelpStyle.Render(m.loaderModTime)))
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
