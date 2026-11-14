package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

func (m *model) View() string {
	if m.err != nil {
		errText := ErrorStyle.Render(fmt.Sprintf(" Error: %s", m.err.Error()))
		w := m.width - 2
		if w < 20 {
			w = 20
		}
		return OuterFrame.Width(w).Render(lipgloss.JoinVertical(lipgloss.Top,
			m.header(),
			"\n",
			errText,
			"\n",
			HelpStyle.Render(" Press q to quit"),
		))
	}

	if m.entries == nil {
		loadingText := lipgloss.JoinHorizontal(lipgloss.Left,
			"  ",
			m.spinner.View(),
			"  Discovering boot entries...",
		)
		lw := m.width - 2
		if lw < 20 {
			lw = 20
		}
		return OuterFrame.Width(lw).Render(lipgloss.JoinVertical(lipgloss.Top,
			m.header(),
			"\n",
			PaneContentStyle.Render(loadingText),
		))
	}

	if m.confirming {
		return m.confirmOverlay()
	}

	if m.deleting {
		return m.deleteConfirmOverlay()
	}

	if m.showPreview {
		return m.previewOverlay()
	}

	if m.showDiag {
		return m.diagOverlay()
	}

	if m.showHelp {
		return m.helpOverlay()
	}

	leftColWidth := m.leftPaneWidth()

	leftColumn := lipgloss.JoinVertical(lipgloss.Top,
		m.entriesPane(leftColWidth),
		m.loaderPane(leftColWidth),
	)

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top,
		leftColumn,
		m.detailPane(leftColWidth),
	)

	appContent := lipgloss.JoinVertical(lipgloss.Top,
		m.header(),
		mainArea,
		m.helpBar(),
	)

	fw := m.width - 2
	if fw < 20 {
		fw = 20
	}
	return OuterFrame.Width(fw).Render(appContent)
}

func (m *model) leftPaneWidth() int {
	w := m.width
	if w <= 0 {
		w = 120
	}
	maxLeft := w / 4
	if maxLeft < 28 {
		maxLeft = 28
	}
	if maxLeft > 45 {
		maxLeft = 45
	}
	return maxLeft
}

func (m *model) confirmOverlay() string {
	var b strings.Builder
	b.WriteString(HelpTitleStyle.Render(" Save changes? ") + "\n\n")

	if len(m.confirmDiff) == 0 {
		b.WriteString(HelpStyle.Render("  (no changes)") + "\n")
	} else {
		for _, d := range m.confirmDiff {
			b.WriteString(fmt.Sprintf("  %s\n", KeyStyle.Render(d.field+":")))
			b.WriteString(fmt.Sprintf("    %s %s\n", DiffOldStyle.Render("-"), DiffOldStyle.Render(d.old)))
			b.WriteString(fmt.Sprintf("    %s %s\n", DiffNewStyle.Render("+"), DiffNewStyle.Render(d.new)))
		}
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("  [y] save   [any other key] back to editing"))

	modal := HelpModalStyle.Render(b.String())
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func (m *model) deleteConfirmOverlay() string {
	name := ""
	if m.entry != nil {
		name = m.entry.ID + ".conf"
	}
	content := lipgloss.JoinVertical(lipgloss.Top,
		HelpTitleStyle.Render(" Delete entry? "),
		"",
		fmt.Sprintf("  %s", ErrorStyle.Render(name)),
		"",
		HelpStyle.Render("  [y] delete   [any other key] cancel"),
	)
	modal := HelpModalStyle.Render(content)
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func (m *model) previewOverlay() string {
	title := m.previewTitle
	content := lipgloss.JoinVertical(lipgloss.Top,
		HelpTitleStyle.Render(title),
		"",
		PaneContentStyle.Render(m.previewContent),
		"",
		HelpStyle.Render(" Press any key to close"),
	)
	modal := HelpModalStyle.Render(content)
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

// diagOverlay renders the boot-environment diagnostics: partition layout,
// systemd-boot binaries on the ESP, loader.conf consistency and per-entry
// boot-menu issues.
func (m *model) diagOverlay() string {
	maxw := 78
	if m.width-6 < maxw && m.width > 0 {
		maxw = m.width - 6
	}
	if maxw < 40 {
		maxw = 40
	}

	var b strings.Builder
	add := func(style lipgloss.Style, label, value string) {
		fmt.Fprintf(&b, "%s %s\n", KeyStyle.Render(label+":"), style.Render(value))
	}
	wrap := func(prefix string, lines ...string) {
		for _, line := range lines {
			for i, w := range wrapText(line, maxw-4) {
				if i == 0 {
					b.WriteString(" " + prefix + w + "\n")
				} else {
					b.WriteString("    " + w + "\n")
				}
			}
		}
	}

	b.WriteString(HelpTitleStyle.Render(" Boot diagnostics ") + "\n\n")

	add(HelpStyle, "Entries on", m.loaderInfo.BootDir)
	if m.loaderInfo.XBOOTLDR {
		add(HelpStyle, "ESP at", m.loaderInfo.ESPDir+" (XBOOTLDR layout)")
	}
	if m.loaderInfo.LoaderPath != "" {
		add(HelpStyle, "Loader", m.loaderInfo.LoaderPath+" (systemd-boot "+m.loaderInfo.LoaderVersion+")")
	}
	if m.loaderInfo.FallbackPath != "" {
		add(HelpStyle, "Fallback", m.loaderInfo.FallbackPath)
	}
	if m.loaderInfo.PackageVersion != "" {
		add(HelpStyle, "Package", "systemd-boot "+m.loaderInfo.PackageVersion)
	}
	if m.machineID != "" {
		add(HelpStyle, "machine-id", m.machineID)
	}
	if m.oneShotEntry != "" {
		add(HelpStyle, "One-shot", m.oneShotEntry+" (boots once on next restart)")
	}
	if m.lastBooted != "" {
		add(HelpStyle, "Last booted", m.lastBooted)
	}

	if len(m.loaderInfo.Problems) > 0 {
		b.WriteString("\n" + ErrorStyle.Render(" Issues") + "\n")
		wrap("!", m.loaderInfo.Problems...)
	}

	if msg := boot.CheckDefaultEntry(m.loaderConf, m.entries); msg != "" {
		b.WriteString("\n")
		wrap("!", msg)
	}
	if m.loaderConf != nil && len(m.loaderConf.Issues) > 0 {
		wrap("!", m.loaderConf.Issues...)
	}

	var warnIDs []string
	for _, e := range m.entries {
		if len(m.entryWarnings[e.ID]) > 0 {
			warnIDs = append(warnIDs, e.ID)
		}
	}
	if len(warnIDs) > 0 {
		fmt.Fprintf(&b, "\n%s %s\n", ErrorStyle.Render("Entry issues"),
			HelpStyle.Render("(select an entry and press e/Tab to inspect)"))
		for _, id := range warnIDs {
			b.WriteString("  " + KeyStyle.Render(id+".conf") + "\n")
			wrap("   ! ", m.entryWarnings[id]...)
		}
	}

	b.WriteString("\n" + HelpStyle.Render(" Notes") + "\n")
	wrap("· ", m.loaderInfo.BootHints()...)

	b.WriteString("\n" + HelpStyle.Render(" Press any key to close"))

	modal := HelpModalStyle.Render(b.String())
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
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
		"  n             New boot entry",
		"  D             Delete selected entry",
		"  s             Set as default boot entry",
		"  o             Toggle one-shot (next boot only, EFI systems)",
		"  e             Toggle edit mode",
		"  Enter         Save changes (in edit mode)",
		"  Esc           Cancel editing / new entry",
		"  p             Preview raw file (entries or loader.conf)",
		"  d             Boot diagnostics overlay",
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
		if m.oneShotAvail {
			return HelpStyle.Render(" ↑/↓: navigate  |  n: new  |  D: delete  |  s: set default  |  o: one-shot  |  d: diagnostics  |  Tab: switch  |  q: quit  |  ?: help")
		}
		return HelpStyle.Render(" ↑/↓: navigate  |  n: new  |  D: delete  |  s: set default  |  d: diagnostics  |  Tab: switch  |  q: quit  |  ?: help")
	case paneDetail:
		if m.editing {
			return HelpStyle.Render(" ↑/↓: field  |  Tab: next/complete  |  Enter: save  |  Esc: cancel  |  ?: help")
		}
		if m.entry != nil && m.entry.IsUKI {
			return HelpStyle.Render(" p: show image info  |  Tab: switch  |  q: quit  |  ?: help")
		}
		return HelpStyle.Render(" e: edit  |  p: preview file  |  Tab: switch  |  q: quit  |  ?: help")
	case paneLoader:
		if m.editing {
			return HelpStyle.Render(" e: stop  |  Tab: switch  |  Enter: save  |  Esc: cancel  |  ?: help")
		}
		return HelpStyle.Render(" e: edit  |  p: preview file  |  d: diagnostics  |  Tab: switch  |  q: quit  |  ?: help")
	}
	return ""
}

// paneTitleRow renders a centered title line + separator, embedded directly
// inside the pane border — no nested box, no double-border gap.
func (m *model) paneTitleRow(title string, editing bool, width int) string {
	titleText := title
	if editing {
		titleText += "  " + PaneEditingStyle.Render("[EDITING]")
	}
	titleLine := PaneTitleStyle.Width(width).Align(lipgloss.Center).Render(titleText)
	sep := KeyStyle.Render(strings.Repeat("─", width))
	return lipgloss.JoinVertical(lipgloss.Top, titleLine, sep)
}

func (m *model) header() string {
	// OuterFrame border takes 2 chars per side. Header bg fills the rest flush.
	w := m.width - 4
	if w < 20 {
		w = 20
	}
	content := lipgloss.JoinHorizontal(lipgloss.Left,
		HeaderTitleStyle.Render(" sdboot-tui "),
		HeaderTitleStyle.Render(" "+AppVersion+" "),
		HeaderMetaStyle.Render(" "+AppRepo),
	)
	return lipgloss.NewStyle().
		Background(ColorHeaderBg).
		Width(w).
		Padding(0, 1).
		Render(content)
}
