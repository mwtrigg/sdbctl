package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	// editing state
	draftEntry      *boot.BootEntry
	draftLoader     *boot.LoaderConfig
	editInput       textinput.Model
	editFields      []fieldDesc
	editFieldIdx    int
	espPaths        []string
	completions     []string
	compIdx         int
	showCompletions bool
	validationErr   string
	// file preview overlay
	showPreview    bool
	previewTitle   string
	previewContent string
	// cached file metadata
	entryModTime  string
	loaderModTime string
}

type Option func(*model)

func WithESPPath(path string) Option {
	return func(m *model) {
		m.espPath = path
	}
}

func New(opts ...Option) *model {
	m := &model{
		spinner:   spinner.New(),
		editInput: textinput.New(),
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

type errMsg struct{ err error }

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
		m.refreshEntryMeta()
		m.refreshLoaderMeta()
		// Kick off async ESP path crawl for tab completion.
		return m, crawlESP(msg.espPath)

	case espPathsLoaded:
		m.espPaths = msg.paths
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward non-key messages to textinput (cursor blink) and spinner.
	var cmds []tea.Cmd
	if m.editing {
		var inputCmd tea.Cmd
		m.editInput, inputCmd = m.editInput.Update(msg)
		cmds = append(cmds, inputCmd)
	}
	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	cmds = append(cmds, spinnerCmd)
	return m, tea.Batch(cmds...)
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Close preview overlay on any key.
	if m.showPreview {
		m.showPreview = false
		return m, nil
	}

	if m.editing {
		return m.handleEditKey(msg)
	}

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
		case "k":
			if m.focused == paneEntries && m.selected > 0 {
				m.selected--
				m.entry = m.entries[m.selected]
				m.refreshEntryMeta()
			}
		case "j":
			if m.focused == paneEntries && m.selected < len(m.entries)-1 {
				m.selected++
				m.entry = m.entries[m.selected]
				m.refreshEntryMeta()
			}
		case "e":
			if m.focused == paneDetail {
				return m, m.enterEntryEditMode()
			} else if m.focused == paneLoader {
				return m, m.enterLoaderEditMode()
			}
		case "p":
			if m.focused == paneDetail && m.entry != nil {
				m.loadEntryPreview()
				m.showPreview = true
			} else if m.focused == paneLoader && m.loaderConf != nil {
				m.loadLoaderPreview()
				m.showPreview = true
			}
		}
		return m, nil

	case tea.KeyUp:
		if m.focused == paneEntries && m.selected > 0 {
			m.selected--
			m.entry = m.entries[m.selected]
			m.refreshEntryMeta()
		}

	case tea.KeyDown:
		if m.focused == paneEntries && m.selected < len(m.entries)-1 {
			m.selected++
			m.entry = m.entries[m.selected]
			m.refreshEntryMeta()
		}

	case tea.KeyTab:
		switch m.focused {
		case paneEntries:
			m.focused = paneDetail
		case paneDetail:
			m.focused = paneLoader
		case paneLoader:
			m.focused = paneEntries
		}

	case tea.KeyShiftTab:
		switch m.focused {
		case paneEntries:
			m.focused = paneLoader
		case paneDetail:
			m.focused = paneEntries
		case paneLoader:
			m.focused = paneDetail
		}

	case tea.KeyEscape:
		if m.showHelp {
			m.showHelp = false
		}
	}

	return m, nil
}

func (m *model) refreshEntryMeta() {
	if m.entry == nil || m.espPath == "" {
		m.entryModTime = ""
		return
	}
	path := filepath.Join(m.espPath, "loader", "entries", m.entry.ID+".conf")
	if info, err := os.Stat(path); err == nil {
		m.entryModTime = info.ModTime().Format("2006-01-02 15:04")
	} else {
		m.entryModTime = ""
	}
}

func (m *model) loadEntryPreview() {
	if m.entry == nil {
		return
	}
	path := filepath.Join(m.espPath, "loader", "entries", m.entry.ID+".conf")
	m.previewTitle = " " + m.entry.ID + ".conf "
	data, err := os.ReadFile(path)
	if err != nil {
		m.previewContent = fmt.Sprintf("error reading file: %v", err)
		return
	}
	m.previewContent = string(data)
}

func (m *model) loadLoaderPreview() {
	path := filepath.Join(m.espPath, "loader", "loader.conf")
	m.previewTitle = " loader.conf "
	data, err := os.ReadFile(path)
	if err != nil {
		m.previewContent = fmt.Sprintf("error reading file: %v", err)
		return
	}
	m.previewContent = string(data)
}

func (m *model) refreshLoaderMeta() {
	if m.espPath == "" {
		m.loaderModTime = ""
		return
	}
	path := filepath.Join(m.espPath, "loader", "loader.conf")
	if info, err := os.Stat(path); err == nil {
		m.loaderModTime = info.ModTime().Format("2006-01-02 15:04")
	} else {
		m.loaderModTime = ""
	}
}

func (m *model) saveEntry() {
	if m.entry == nil {
		return
	}
	path := filepath.Join(m.espPath, "loader", "entries", m.entry.ID+".conf")
	if err := m.entry.Write(path); err != nil {
		m.err = err
	}
}

func (m *model) saveLoaderConf() {
	path := filepath.Join(m.espPath, "loader", "loader.conf")
	if err := m.loaderConf.Write(path); err != nil {
		m.err = err
	}
}
