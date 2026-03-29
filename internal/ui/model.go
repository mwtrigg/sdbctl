package ui

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

type pane int

const (
	paneEntries pane = iota
	paneDetail
	paneLoader
)

type viewMode int

const (
	_ viewMode = iota
	viewMain
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
	showHelp   bool
	width      int
	height     int
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

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
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "k":
			if m.focused == paneEntries {
				if m.selected > 0 {
					m.selected--
					m.entry = m.entries[m.selected]
				}
			}
			return m, nil
		case "j":
			if m.focused == paneEntries {
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
				} else if m.focused == paneLoader {
					m.saveLoaderConf()
				}
			}
			return m, nil
		}

	case tea.KeyUp:
		if m.focused == paneEntries {
			if m.selected > 0 {
				m.selected--
				m.entry = m.entries[m.selected]
			}
		}
		return m, nil

	case tea.KeyDown:
		if m.focused == paneEntries {
			if m.selected < len(m.entries)-1 {
				m.selected++
				m.entry = m.entries[m.selected]
			}
		}
		return m, nil

	case tea.KeyTab:
		switch m.focused {
		case paneEntries:
			m.focused = paneDetail
		case paneDetail:
			m.focused = paneLoader
		case paneLoader:
			m.focused = paneEntries
		}
		return m, nil

	case tea.KeyShiftTab:
		switch m.focused {
		case paneEntries:
			m.focused = paneLoader
		case paneDetail:
			m.focused = paneEntries
		case paneLoader:
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
			} else if m.focused == paneLoader {
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
