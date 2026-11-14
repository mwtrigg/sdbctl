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

type model struct {
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
	// save confirmation
	confirming  bool
	confirmDiff []diffLine
	// new / delete state
	creating bool
	deleting bool
	// file preview overlay
	showPreview    bool
	previewTitle   string
	previewContent string
	// diagnostics overlay
	showDiag bool
	// cached file metadata
	entryModTime  string
	loaderModTime string
	// per-entry boot menu issues (entry ID → issues list)
	entryWarnings map[string][]string
	// one-shot boot state
	oneShotEntry string // entry token currently set for one-shot, or ""
	oneShotAvail bool   // whether EFI variable interface is accessible
	// lastBackup is the snapshot taken before the most recent change, so the
	// change can be undone.
	lastBackup *boot.Backup
	// undoing is set while the undo confirmation is pending.
	undoing bool
	// host context for validation and diagnostics
	machineID  string // local /etc/machine-id, "" if unknown
	archToken  string // local EFI architecture token
	lastBooted string // entry token of the last @saved boot, or ""
	loaderInfo boot.LoaderStatus
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
	entries, err := boot.ParseAllEntries(espPath)
	if err != nil {
		return errMsg{err}
	}
	if entries == nil {
		entries = []*boot.BootEntry{}
	}

	msg := bootLoaded{
		espPath:    espPath,
		loaderConf: cfg,
		entries:    entries,
		machineID:  boot.ReadMachineID(),
		archToken:  boot.LocalArchToken(),
		loaderInfo: boot.InspectLoader(espPath),
	}
	if boot.OneShotAvailable() {
		if id, err := boot.GetOneShot(); err == nil {
			msg.oneShot = boot.StripEntrySuffix(id)
		}
		if id, err := boot.GetLastBooted(); err == nil {
			msg.lastBooted = boot.StripEntrySuffix(id)
		}
	}
	return msg
}

type bootLoaded struct {
	espPath    string
	loaderConf *boot.LoaderConfig
	entries    []*boot.BootEntry
	machineID  string
	archToken  string
	loaderInfo boot.LoaderStatus
	oneShot    string
	lastBooted string
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
		m.machineID = msg.machineID
		m.archToken = msg.archToken
		m.loaderInfo = msg.loaderInfo
		m.oneShotEntry = msg.oneShot
		m.lastBooted = msg.lastBooted
		if len(m.entries) > 0 {
			m.entry = m.entries[0]
		}
		m.refreshEntryMeta()
		m.refreshLoaderMeta()
		m.revalidateEntries()
		m.oneShotAvail = boot.OneShotAvailable()
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

	// Close diagnostics overlay on any key.
	if m.showDiag {
		m.showDiag = false
		return m, nil
	}

	// Confirmation prompt: y to save, anything else cancels back to edit.
	if m.confirming {
		switch msg.String() {
		case "y", "Y":
			m.finalizeCommit()
		default:
			m.confirming = false // back to editing
		}
		return m, nil
	}

	// Delete confirmation: y to confirm, anything else cancels.
	if m.deleting {
		switch msg.String() {
		case "y", "Y":
			m.confirmDelete()
		default:
			m.deleting = false
		}
		return m, nil
	}

	// Undo confirmation: y to restore the last snapshot.
	if m.undoing {
		switch msg.String() {
		case "y", "Y":
			m.undoing = false
			m.undo()
		default:
			m.undoing = false
		}
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
		case "n":
			if m.focused == paneEntries {
				return m, m.enterNewEntryMode()
			}
		case "D":
			if m.focused == paneEntries && m.entry != nil && !m.entry.IsUKI {
				m.deleting = true
			}
		case "d":
			m.showDiag = true
		case "u":
			if m.focused == paneEntries || m.focused == paneLoader {
				m.undoing = true
			}
		case "o":
			if m.focused == paneEntries && m.entry != nil && m.oneShotAvail {
				m.toggleOneShot()
			}
		case "s":
			if m.focused == paneEntries && m.entry != nil && m.loaderConf != nil {
				m.setDefaultEntry()
			}
		case "e":
			if m.focused == paneDetail && m.entry != nil && !m.entry.IsUKI {
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

	case tea.KeyRight:
		switch m.focused {
		case paneEntries:
			m.focused = paneDetail
		case paneDetail:
			m.focused = paneLoader
		case paneLoader:
			m.focused = paneEntries
		}

	case tea.KeyLeft:
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

// entryConfPath returns the on-disk path of an entry file, for either
// Type #1 (.conf) or Type #2 (UKI) entries. Returns "" for new entries that
// have no ID yet.
func (m *model) entryConfPath(e *boot.BootEntry) string {
	if e == nil || m.espPath == "" {
		return ""
	}
	if e.IsUKI {
		suffix := e.FileSuffix
		if suffix == "" {
			suffix = ".efi"
		}
		return filepath.Join(m.espPath, "EFI", "Linux", e.ID+suffix)
	}
	if e.ID == "" {
		return ""
	}
	return filepath.Join(m.espPath, "loader", "entries", e.ID+".conf")
}

func (m *model) refreshEntryMeta() {
	path := m.entryConfPath(m.entry)
	if path == "" {
		m.entryModTime = ""
		return
	}
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
	path := m.entryConfPath(m.entry)
	m.previewTitle = " " + filepath.Base(path) + " "
	if m.entry.IsUKI {
		// A UKI is a PE binary, not a text file: show metadata instead.
		info, err := os.Stat(path)
		if err != nil {
			m.previewContent = fmt.Sprintf("error reading file: %v", err)
			return
		}
		m.previewContent = fmt.Sprintf(
			"Unified Kernel Image (EFI binary, no text preview)\n\npath: %s\nsize: %d bytes\nmodified: %s\n\nkernel, initrd and cmdline are embedded in this file.",
			path, info.Size(), info.ModTime().Format("2006-01-02 15:04"))
		return
	}
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

func (m *model) revalidateEntries() {
	warnings := make(map[string][]string)
	ctx := boot.ValidationContext{
		ESPPath:      m.espPath,
		MachineID:    m.machineID,
		Architecture: m.archToken,
	}
	for _, e := range m.entries {
		if e.IsUKI {
			continue
		}
		if verr := boot.ValidateEntry(e, ctx); verr != nil {
			warnings[e.ID] = verr.Issues
		}
	}
	m.entryWarnings = warnings
}

// backupBefore snapshots the given boot-directory-relative files so that the
// upcoming change can be undone. It returns false (and records the error) if
// the snapshot failed, in which case the caller must not modify anything.
func (m *model) backupBefore(relPaths ...string) bool {
	b, err := boot.CreateBackup(m.espPath, relPaths)
	if err != nil {
		m.err = fmt.Errorf("backup failed, nothing was changed: %w", err)
		return false
	}
	if b != nil {
		m.lastBackup = b
	}
	return true
}

// undo restores the most recent backup, which reverts the last change made
// through this tool (an edited entry, a deleted entry, an edited
// loader.conf). The state before the undo is snapshotted again, so undoing is
// itself reversible.
func (m *model) undo() {
	b, err := boot.LatestBackup(m.espPath)
	if err != nil {
		m.err = err
		return
	}
	if b == nil {
		m.err = fmt.Errorf("no backups to restore")
		return
	}

	// Snapshot the current state first so the undo can be undone.
	current := make([]string, 0, len(b.Files))
	for _, rel := range b.Files {
		current = append(current, rel)
	}
	if !m.backupBefore(current...) {
		return
	}

	if err := boot.RestoreBackup(m.espPath, *b); err != nil {
		m.err = err
		return
	}
	m.reload()
}

// reload re-reads the boot configuration from disk (after an undo or an
// external change).
func (m *model) reload() {
	entries, err := boot.ParseAllEntries(m.espPath)
	if err != nil {
		m.err = err
		return
	}
	cfg, _ := boot.ParseLoaderConf(boot.FindLoaderConf(m.espPath))

	// Keep the selection if the entry still exists.
	selectedID := ""
	if m.entry != nil {
		selectedID = m.entry.ID
	}
	m.entries = entries
	m.loaderConf = cfg
	m.selected = 0
	m.entry = nil
	for i, e := range entries {
		if e.ID == selectedID {
			m.selected = i
			m.entry = e
			break
		}
	}
	if m.entry == nil && len(entries) > 0 {
		m.entry = entries[0]
	}
	m.refreshEntryMeta()
	m.refreshLoaderMeta()
	m.revalidateEntries()
}

func (m *model) saveEntry() {
	if m.entry == nil {
		return
	}
	path := filepath.Join(m.espPath, "loader", "entries", m.entry.ID+".conf")
	if !m.backupBefore("loader/entries/" + m.entry.ID + ".conf") {
		return
	}
	if err := m.entry.Write(path); err != nil {
		m.err = err
		return
	}
	m.revalidateEntries()
}

// isDefaultEntry reports whether the loader.conf default pattern selects the
// given entry. systemd-boot globs the pattern (case-insensitively) against
// the full entry ID including the .conf/.efi suffix. With default @saved the
// effective default is the entry booted last, which we know from the
// LoaderEntryLastBooted EFI variable.
func (m *model) isDefaultEntry(e *boot.BootEntry) bool {
	if m.loaderConf == nil || m.loaderConf.Default == "" {
		return false
	}
	d := m.loaderConf.Default
	if boot.DefaultSpecials[d] {
		return m.lastBooted != "" && m.lastBooted == e.ID
	}
	return boot.MatchEntryID(d, e.FullID())
}

func (m *model) setDefaultEntry() {
	if m.entry == nil || m.loaderConf == nil {
		return
	}
	// systemd-boot matches default= against the full entry ID, i.e. the
	// file name including the .conf (Type #1) or .efi/.uki (Type #2) suffix.
	m.loaderConf.Default = m.entry.FullID()
	m.saveLoaderConf()
	m.refreshLoaderMeta()
}

func (m *model) toggleOneShot() {
	if m.entry == nil {
		return
	}
	if m.oneShotEntry == m.entry.ID {
		// Already set for this entry — clear it.
		if err := boot.ClearOneShot(); err != nil {
			m.err = err
			return
		}
		m.oneShotEntry = ""
	} else {
		// The variable is glob-matched against full entry IDs by the boot
		// loader, so it must carry the .conf/.efi suffix to match anything.
		if err := boot.SetOneShot(m.entry.FullID()); err != nil {
			m.err = err
			return
		}
		m.oneShotEntry = m.entry.ID
	}
}

func (m *model) saveLoaderConf() {
	if m.loaderConf == nil {
		return
	}
	path := filepath.Join(m.espPath, "loader", "loader.conf")
	if !m.backupBefore("loader/loader.conf") {
		return
	}
	if err := m.loaderConf.Write(path); err != nil {
		m.err = err
	}
}
