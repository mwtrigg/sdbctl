package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

// consoleMode describes one EFI text-mode option.
type consoleMode struct {
	token string // loader.conf value
	label string // human label
	cols  int    // character columns (0 = runtime/unknown)
	rows  int    // character rows  (0 = runtime/unknown)
}

// consoleModes lists all loader.conf console-mode values in display order.
var consoleModes = []consoleMode{
	{token: "0", label: "Mode 0", cols: 80, rows: 25},
	{token: "1", label: "Mode 1", cols: 80, rows: 50},
	{token: "2", label: "Mode 2 (highest text)", cols: 0, rows: 0},
	{token: "auto", label: "auto", cols: 0, rows: 0},
	{token: "max", label: "max (highest graphics)", cols: 0, rows: 0},
	{token: "keep", label: "keep (unchanged)", cols: 0, rows: 0},
}

// resolutionPreviewOverlay renders the console-mode preview modal.
func (m *model) resolutionPreviewOverlay() string {
	cm := consoleModes[m.resPreviewIdx]

	var b strings.Builder

	// Header
	b.WriteString(HelpTitleStyle.Render(" Console-Mode Preview ") + "\n\n")

	// Mode selector
	var modeSelector strings.Builder
	for i, c := range consoleModes {
		if i == m.resPreviewIdx {
			modeSelector.WriteString(SelectedItemStyle.Render(" " + c.label + " "))
		} else {
			modeSelector.WriteString(HelpStyle.Render(" " + c.label + " "))
		}
		if i < len(consoleModes)-1 {
			modeSelector.WriteString(HelpStyle.Render("│"))
		}
	}
	b.WriteString(" " + modeSelector.String() + "\n\n")

	// Dimension description
	if cm.cols > 0 {
		b.WriteString(fmt.Sprintf("  %s  %s\n\n",
			KeyStyle.Render("dimensions:"),
			ValueStyle.Render(fmt.Sprintf("%d×%d characters", cm.cols, cm.rows)),
		))
	} else {
		b.WriteString(fmt.Sprintf("  %s  %s\n\n",
			KeyStyle.Render("dimensions:"),
			HelpStyle.Render("determined at runtime by firmware"),
		))
	}

	// Mock boot screen
	if cm.cols > 0 {
		b.WriteString(renderBootScreen(cm.cols, cm.rows, m.entries, m.loaderConf, m.isDefaultEntry))
	} else {
		b.WriteString(renderBootScreenRuntime(cm.token, m.entries, m.loaderConf, m.isDefaultEntry))
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("  ←/→: cycle modes   any other key: close"))

	modal := HelpModalStyle.Render(b.String())
	w, h := m.width, m.height
	if w <= 0 {
		w = 120
	}
	if h <= 0 {
		h = 40
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

// renderBootScreen draws a mock systemd-boot screen at the given character dimensions.
func renderBootScreen(cols, rows int, entries []*boot.BootEntry, lc *boot.LoaderConfig, isDefault func(string) bool) string {
	// Cap preview to a reasonable size so it fits in the modal.
	previewCols := cols
	if previewCols > 76 {
		previewCols = 76
	}
	previewRows := rows
	if previewRows > 30 {
		previewRows = 30
	}

	// Entry list width: entries box is ~60% of screen, centered.
	listWidth := previewCols * 60 / 100
	if listWidth < 30 {
		listWidth = 30
	}
	if listWidth > previewCols-4 {
		listWidth = previewCols - 4
	}

	// How many entries fit: subtract 2 border rows, title row, separator, timeout row.
	chrome := 6 // top border, title, sep, bottom border, nav hint, timeout
	maxVisible := previewRows - chrome
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Build the screen lines.
	var lines []string

	// Top border
	lines = append(lines, DimValueStyle.Render("┌"+strings.Repeat("─", previewCols-2)+"┐"))

	// Vertical padding to center the list box.
	listBoxHeight := 2 + min(len(entries), maxVisible) // border lines + entries
	topPad := (previewRows - listBoxHeight - 4) / 2    // 4 = title + sep + hint + timeout
	if topPad < 1 {
		topPad = 1
	}
	for i := 0; i < topPad; i++ {
		lines = append(lines, screenRow("", previewCols))
	}

	// Title
	title := "Please select a boot entry:"
	lines = append(lines, screenRow(centerText(title, previewCols-2), previewCols))
	lines = append(lines, screenRow("", previewCols))

	// Entry list box — centered horizontally.
	leftPad := (previewCols - listWidth) / 2
	pad := strings.Repeat(" ", leftPad)
	lines = append(lines, DimValueStyle.Render("│")+
		pad+DimValueStyle.Render("┌"+strings.Repeat("─", listWidth-2)+"┐")+
		strings.Repeat(" ", previewCols-2-leftPad-listWidth)+DimValueStyle.Render("│"))

	visible := entries
	if len(visible) > maxVisible {
		visible = visible[:maxVisible]
	}
	for _, e := range visible {
		entryTitle := e.DisplayTitle()
		marker := "  "
		style := DimValueStyle
		if isDefault(e.ID) {
			marker = "★ "
			style = ValueStyle
		}
		row := marker + truncate(entryTitle, listWidth-4)
		row = fmt.Sprintf(" %-*s ", listWidth-4, row)
		lines = append(lines, DimValueStyle.Render("│")+
			pad+DimValueStyle.Render("│")+style.Render(row)+DimValueStyle.Render("│")+
			strings.Repeat(" ", previewCols-2-leftPad-listWidth)+DimValueStyle.Render("│"))
	}
	if len(entries) > maxVisible {
		more := fmt.Sprintf("  … %d more", len(entries)-maxVisible)
		lines = append(lines, DimValueStyle.Render("│")+
			pad+DimValueStyle.Render("│")+HelpStyle.Render(fmt.Sprintf(" %-*s ", listWidth-4, more))+DimValueStyle.Render("│")+
			strings.Repeat(" ", previewCols-2-leftPad-listWidth)+DimValueStyle.Render("│"))
	}

	lines = append(lines, DimValueStyle.Render("│")+
		pad+DimValueStyle.Render("└"+strings.Repeat("─", listWidth-2)+"┘")+
		strings.Repeat(" ", previewCols-2-leftPad-listWidth)+DimValueStyle.Render("│"))

	// Nav hint
	navHint := "↑↓: navigate   Enter: boot   e: edit cmdline"
	lines = append(lines, screenRow(centerText(navHint, previewCols-2), previewCols))

	// Timeout line
	timeoutLine := ""
	if lc != nil && lc.Timeout != "" && lc.Timeout != "0" {
		timeoutLine = fmt.Sprintf("Booting in %ss, press any key to stop.", lc.Timeout)
	}
	lines = append(lines, screenRow(centerText(timeoutLine, previewCols-2), previewCols))

	// Fill remaining rows
	for len(lines) < previewRows-1 {
		lines = append(lines, screenRow("", previewCols))
	}

	// Bottom border
	lines = append(lines, DimValueStyle.Render("└"+strings.Repeat("─", previewCols-2)+"┘"))

	// Annotate dimensions
	dimensionLabel := fmt.Sprintf(" ↑ mock at %d×%d (actual: %d×%d) ", previewCols, len(lines), cols, rows)
	return strings.Join(lines, "\n") + "\n" +
		HelpStyle.Render(centerText(dimensionLabel, previewCols))
}

func renderBootScreenRuntime(token string, entries []*boot.BootEntry, lc *boot.LoaderConfig, isDefault func(string) bool) string {
	desc := map[string]string{
		"2":    "Firmware picks the highest available text mode.",
		"auto": "systemd-boot selects a comfortable text mode.",
		"max":  "Firmware picks the highest available resolution (may be graphical).",
		"keep": "Console mode is not changed from what firmware set.",
	}
	note := desc[token]
	var b strings.Builder
	b.WriteString("  " + HelpStyle.Render(note) + "\n\n")
	b.WriteString("  " + HelpStyle.Render("Exact dimensions depend on firmware and display hardware.") + "\n")
	b.WriteString("  " + HelpStyle.Render("Entries that will appear:") + "\n\n")
	for _, e := range entries {
		marker := "  "
		if isDefault(e.ID) {
			marker = "★ "
		}
		b.WriteString("  " + ValueStyle.Render(marker+e.DisplayTitle()) + "\n")
	}
	return b.String()
}

func screenRow(content string, cols int) string {
	return DimValueStyle.Render("│") + fmt.Sprintf("%-*s", cols-2, content) + DimValueStyle.Render("│")
}

func centerText(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	pad := (width - len(s)) / 2
	return strings.Repeat(" ", pad) + s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
