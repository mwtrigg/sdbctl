package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

type pane int

const (
	paneList pane = iota
	paneDetail
	paneLoaderConf
)

type viewMode int

const (
	_ viewMode = iota
	viewMain
)

var (
	AppStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#1E3A5F")).
			Padding(0, 1).
			Width(100)

	VersionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88AACC")).
			Background(lipgloss.Color("#1E3A5F"))

	RepoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88AACC")).
			Background(lipgloss.Color("#1E3A5F"))

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444"))

	FocusBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#33FF33"))

	PaneHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#33FF33")).
			Background(lipgloss.Color("#001100")).
			Padding(0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#001100")).
			Background(lipgloss.Color("#33FF33")).
			Width(35)

	ItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33CC33")).
			Width(35)

	KeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#229922"))

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33FF33"))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33FF33")).
			Background(lipgloss.Color("#001100"))
)

const (
	AppVersion = "v0.1.0"
	AppRepo    = "https://codeberg.org/mwtrigg/sdboot-tui"
)

type model struct {
	view       viewMode
	focused    pane
	espPath    string
	entries    []*boot.BootEntry
	loaderConf *boot.LoaderConfig
	selected   int
	entry      *boot.BootEntry
	editing    bool
	err        error
	spinner    spinner.Model
}

type Option func(*model)

func WithESPPath(path string) Option {
	return func(m *model) {
		m.espPath = path
	}
}

func New(opts ...Option) *model {
	m := &model{
		spinner: spinner.New(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			return discoverBoot(m.espPath)
		},
	)
}

func discoverBoot(overridePath string) tea.Msg {
	espPath := overridePath
	var err error
	if espPath == "" {
		espPath, err = boot.FindESP()
		if err != nil {
			return errMsg{err}
		}
	}

	cfg, _ := boot.ParseLoaderConf(boot.FindLoaderConf(espPath))
	entries, _ := boot.ParseEntries(boot.FindEntriesDir(espPath))

	return bootLoaded{espPath, cfg, entries}
}

type bootLoaded struct {
	espPath    string
	loaderConf *boot.LoaderConfig
	entries    []*boot.BootEntry
}

type errMsg struct {
	err error
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case bootLoaded:
		m.espPath = msg.espPath
		m.loaderConf = msg.loaderConf
		m.entries = msg.entries
		if len(m.entries) > 0 {
			m.entry = m.entries[0]
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyRunes:
		switch keyStr {
		case "q":
			return m, tea.Quit
		case "k":
			if m.focused == paneList {
				if m.selected > 0 {
					m.selected--
					m.entry = m.entries[m.selected]
				}
			}
			return m, nil
		case "j":
			if m.focused == paneList {
				if m.selected < len(m.entries)-1 {
					m.selected++
					m.entry = m.entries[m.selected]
				}
			}
			return m, nil
		case "e":
			if m.focused == paneDetail {
				m.editing = !m.editing
			}
			return m, nil
		case "b", "escape":
			if m.editing {
				m.editing = false
			}
			return m, nil
		case "enter":
			if m.editing {
				if m.focused == paneDetail {
					m.saveEntry()
				} else if m.focused == paneLoaderConf {
					m.saveLoaderConf()
				}
			}
			return m, nil
		}

	case tea.KeyUp:
		if m.focused == paneList {
			if m.selected > 0 {
				m.selected--
				m.entry = m.entries[m.selected]
			}
		}
		return m, nil

	case tea.KeyDown:
		if m.focused == paneList {
			if m.selected < len(m.entries)-1 {
				m.selected++
				m.entry = m.entries[m.selected]
			}
		}
		return m, nil

	case tea.KeyTab:
		switch m.focused {
		case paneList:
			m.focused = paneDetail
		case paneDetail:
			m.focused = paneLoaderConf
		case paneLoaderConf:
			m.focused = paneList
		}
		return m, nil

	case tea.KeyShiftTab:
		switch m.focused {
		case paneList:
			m.focused = paneLoaderConf
		case paneDetail:
			m.focused = paneList
		case paneLoaderConf:
			m.focused = paneDetail
		}
		return m, nil

	case tea.KeyEscape:
		if m.editing {
			m.editing = false
		}
		return m, nil

	case tea.KeyEnter:
		if m.editing {
			if m.focused == paneDetail {
				m.saveEntry()
			} else if m.focused == paneLoaderConf {
				m.saveLoaderConf()
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *model) saveEntry() {
	if m.entry == nil {
		return
	}
	path := filepath.Join(m.espPath, "loader", "entries", m.entry.ID+".conf")
	if err := m.entry.Write(path); err != nil {
		m.err = err
		return
	}
	m.editing = false
}

func (m *model) saveLoaderConf() {
	path := filepath.Join(m.espPath, "loader", "loader.conf")
	if err := m.loaderConf.Write(path); err != nil {
		m.err = err
		return
	}
	m.editing = false
}

func (m *model) View() string {
	header := m.header()

	leftColumn := lipgloss.JoinVertical(lipgloss.Top,
		m.listPane(),
		m.loaderConfPane(),
	)

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top,
		leftColumn,
		m.detailPane(),
	)

	helpBar := m.helpBar()

	return lipgloss.JoinVertical(lipgloss.Top,
		header,
		mainArea,
		helpBar,
	)
}

func (m *model) helpBar() string {
	switch m.focused {
	case paneList:
		if m.editing {
			return HelpStyle.Render(" ↑/↓: navigate  |  Tab: switch pane  |  Enter: save  |  Esc: cancel")
		}
		return HelpStyle.Render(" ↑/↓: navigate  |  Tab: switch pane  |  e: edit  |  q: quit")
	case paneDetail:
		if m.editing {
			return HelpStyle.Render(" e: stop  |  Tab: switch pane  |  Enter: save  |  Esc: cancel")
		}
		return HelpStyle.Render(" e: edit  |  Tab: switch pane  |  q: quit")
	case paneLoaderConf:
		if m.editing {
			return HelpStyle.Render(" e: stop  |  Tab: switch pane  |  Enter: save  |  Esc: cancel")
		}
		return HelpStyle.Render(" e: edit  |  Tab: switch pane  |  q: quit")
	}
	return ""
}

func (m *model) header() string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#1E3A5F")).
		Bold(true).
		Padding(0, 1)

	return headerStyle.Render(" sdboot-tui " + AppVersion + " " + AppRepo)
}

func (m *model) listPane() string {
	focusStyle := lipgloss.NewStyle()
	if m.focused == paneList {
		focusStyle = FocusBorderStyle
	}

	header := PaneHeaderStyle.Render(" Boot Entries ")

	var items strings.Builder
	for i, entry := range m.entries {
		title := entry.DisplayTitle()
		if entry.Version != "" {
			title = fmt.Sprintf("%s (%s)", title, entry.Version)
		}

		style := ItemStyle
		if i == m.selected {
			style = SelectedItemStyle
		}
		items.WriteString(style.Render(truncate(title, 35)) + "\n")
	}

	if len(m.entries) == 0 {
		items.WriteString(ItemStyle.Render(" No entries ") + "\n")
	}

	content := items.String()

	return focusStyle.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			header,
			content,
		),
	)
}

func (m *model) detailPane() string {
	focusStyle := lipgloss.NewStyle()
	if m.focused == paneDetail {
		focusStyle = FocusBorderStyle
	}

	headerText := " Entry Details "
	if m.editing {
		headerText = " Entry Details [EDITING] "
	}
	header := PaneHeaderStyle.Render(headerText)

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

	return focusStyle.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			header,
			content.String(),
		),
	)
}

func (m *model) loaderConfPane() string {
	focusStyle := lipgloss.NewStyle()
	if m.focused == paneLoaderConf {
		focusStyle = FocusBorderStyle
	}

	headerText := " loader.conf "
	if m.editing && m.focused == paneLoaderConf {
		headerText = " loader.conf [EDITING] "
	}
	header := PaneHeaderStyle.Render(headerText)

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

	return focusStyle.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			header,
			content.String(),
		),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + "…"
}

func boolToString(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "yes"
	}
	return "no"
}
