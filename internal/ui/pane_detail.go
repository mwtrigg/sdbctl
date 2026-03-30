package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) detailPane(leftColWidth int) string {
	detailWidth := m.width - leftColWidth - 6 // 6 = outer thick border + both pane borders
	if detailWidth < 20 {
		detailWidth = 20
	}

	isEditing := m.editing && m.focused == paneDetail

	borderStyle := BorderStyle.Width(detailWidth)
	if m.focused == paneDetail {
		borderStyle = FocusBorderStyle.Width(detailWidth)
	}

	header := m.paneTitleRow("Entry Details", isEditing, detailWidth)

	var content strings.Builder

	if isEditing && m.draftEntry != nil {
		content.WriteString(fmt.Sprintf(" ID: %s\n\n", m.draftEntry.ID))

		if len(m.editFields) == 0 {
			content.WriteString(FieldHelpStyle.Render(" No editable fields — implement buildEntryFields() in edit.go") + "\n")
		} else {
			for i, f := range m.editFields {
				label := f.label
				if f.required {
					label += "*"
				}
				keyLabel := KeyStyle.Render(label + ":")

				if i == m.editFieldIdx {
					inputWidth := detailWidth - len(label) - 4
					if inputWidth < 10 {
						inputWidth = 10
					}
					m.editInput.Width = inputWidth
					content.WriteString(fmt.Sprintf(" %s %s\n", keyLabel, m.editInput.View()))
				} else {
					val := f.get()
					var rendered string
					if val == "" {
						rendered = DimValueStyle.Render("(empty)")
					} else {
						rendered = DimValueStyle.Render(truncate(val, detailWidth-len(label)-4))
					}
					content.WriteString(fmt.Sprintf(" %s %s\n", keyLabel, rendered))
				}
			}

			// Tab completion candidates
			if m.showCompletions && len(m.completions) > 0 {
				content.WriteString("\n")
				content.WriteString(HelpStyle.Render(" Tab: cycle  Shift+Tab: reverse") + "\n")
				maxShow := 5
				if len(m.completions) < maxShow {
					maxShow = len(m.completions)
				}
				for i := 0; i < maxShow; i++ {
					line := truncate(m.completions[i], detailWidth-3)
					if i == m.compIdx {
						content.WriteString(" " + SelectedItemStyle.Width(detailWidth-2).Render(line) + "\n")
					} else {
						content.WriteString(" " + HelpStyle.Render(line) + "\n")
					}
				}
				if len(m.completions) > maxShow {
					content.WriteString(HelpStyle.Render(fmt.Sprintf("  …%d more", len(m.completions)-maxShow)) + "\n")
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
					content.WriteString(KeyStyle.Render(strings.Repeat("─", detailWidth)) + "\n")
					for _, line := range wrapText(f.help, detailWidth-2) {
						content.WriteString(" " + FieldHelpStyle.Render(line) + "\n")
					}
					if f.isPath {
						content.WriteString(" " + FieldHelpStyle.Render("Tab: path completion") + "\n")
					}
					if strings.HasPrefix(f.label, "initrd") || strings.HasPrefix(f.label, "devicetree-overlay") {
						content.WriteString(" " + FieldHelpStyle.Render("Ctrl+N: add another line") + "\n")
					}
					if f.required {
						content.WriteString(" " + RequiredStyle.Render("* required field") + "\n")
					}
				}
			}
		}
	} else {
		// View mode
		if m.entry == nil {
			content.WriteString(" Select an entry from the list\n")
		} else if m.entry.IsUKI {
			content.WriteString(fmt.Sprintf(" ID: %s\n\n", m.entry.ID))
			content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("type:"), UKIBadgeStyle.Render("Unified Kernel Image")))
			content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("efi:"), ValueStyle.Render(m.entry.EFIPath)))
			content.WriteString("\n")
			content.WriteString(" " + HelpStyle.Render("Kernel, initrd, and cmdline are") + "\n")
			content.WriteString(" " + HelpStyle.Render("embedded in the EFI binary.") + "\n")
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

			if len(m.entry.DTOOverlays) > 0 {
				content.WriteString("\n")
				for _, dto := range m.entry.DTOOverlays {
					content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("devicetree-overlay:"), ValueStyle.Render(dto)))
				}
			}

			if m.oneShotAvail && m.oneShotEntry == m.entry.ID {
				content.WriteString("\n")
				content.WriteString(" " + OneShotBadgeStyle.Render("one-shot: boots once on next restart") + "\n")
			}

			if warns := m.entryWarnings[m.entry.ID]; len(warns) > 0 {
				content.WriteString("\n")
				for _, w := range warns {
					content.WriteString(" " + WarnBadgeStyle.Render("! ") + ErrorStyle.Render(w) + "\n")
				}
			}

			if m.entryModTime != "" {
				content.WriteString("\n")
				content.WriteString(fmt.Sprintf(" %s %s\n", KeyStyle.Render("modified:"), HelpStyle.Render(m.entryModTime)))
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
