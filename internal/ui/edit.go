package ui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// fieldDesc describes one editable field in a boot entry or loader config.
type fieldDesc struct {
	label    string      // key name shown in the UI
	get      func() string  // reads current draft value
	set      func(string)   // writes value back to draft
	isPath   bool           // enables ESP tab completion on Tab key
	required bool           // blocks save if empty
	help     string         // context help shown at the bottom of the pane
}

// espPathsLoaded carries the result of the async ESP filesystem crawl.
type espPathsLoaded struct{ paths []string }

// crawlESP asynchronously walks the ESP directory and returns all file paths.
func crawlESP(espPath string) tea.Cmd {
	return func() tea.Msg {
		var paths []string
		_ = filepath.WalkDir(espPath, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(espPath, p)
			if relErr != nil {
				return nil
			}
			paths = append(paths, "/"+rel)
			return nil
		})
		return espPathsLoaded{paths}
	}
}

// enterEntryEditMode begins editing the currently selected boot entry.
func (m *model) enterEntryEditMode() tea.Cmd {
	if m.entry == nil {
		return nil
	}
	draft := *m.entry
	draft.Initrds = append([]string(nil), m.entry.Initrds...)
	draft.Options = append([]string(nil), m.entry.Options...)
	draft.DTOOverlays = append([]string(nil), m.entry.DTOOverlays...)
	m.draftEntry = &draft
	m.buildEntryFields()
	return m.resetEditInput()
}

// enterLoaderEditMode begins editing the loader configuration.
func (m *model) enterLoaderEditMode() tea.Cmd {
	if m.loaderConf == nil {
		return nil
	}
	draft := *m.loaderConf
	m.draftLoader = &draft
	m.buildLoaderFields()
	return m.resetEditInput()
}

func (m *model) resetEditInput() tea.Cmd {
	m.editInput = textinput.New()
	m.editInput.Focus()
	m.editFieldIdx = 0
	m.validationErr = ""
	m.completions = nil
	m.showCompletions = false
	m.editing = true
	m.loadFieldIntoInput()
	return textinput.Blink
}

// buildEntryFields defines the editable fields for a BootEntry.
func (m *model) buildEntryFields() {
	e := m.draftEntry

	fields := []fieldDesc{
		{
			label:    "title",
			get:      func() string { return e.Title },
			set:      func(v string) { e.Title = strings.TrimSpace(v) },
			required: true,
			help:     "Human-readable name shown in the boot menu.",
		},
		{
			label: "version",
			get:   func() string { return e.Version },
			set:   func(v string) { e.Version = strings.TrimSpace(v) },
			help:  "Version string for display and entry sorting.",
		},
		{
			label: "sort-key",
			get:   func() string { return e.SortKey },
			set:   func(v string) { e.SortKey = strings.TrimSpace(v) },
			help:  "Key used to order entries in the menu. Entries with the same sort-key are grouped.",
		},
		{
			label:    "linux",
			get:      func() string { return e.Linux },
			set:      func(v string) { e.Linux = strings.TrimSpace(v) },
			isPath:   true,
			required: true,
			help:     "Kernel image path relative to ESP root (e.g. /vmlinuz-linux). Tab for completion.",
		},
	}

	// One field per initrd entry.
	for i := range e.Initrds {
		idx := i
		label := "initrd"
		if len(e.Initrds) > 1 {
			label = fmt.Sprintf("initrd[%d]", idx)
		}
		fields = append(fields, fieldDesc{
			label:  label,
			get:    func() string { return e.Initrds[idx] },
			set:    func(v string) { e.Initrds[idx] = strings.TrimSpace(v) },
			isPath: true,
			help:   "Initial RAM disk path relative to ESP root. Tab for completion.",
		})
	}

	// One field per options line.
	for i := range e.Options {
		idx := i
		label := "options"
		if len(e.Options) > 1 {
			label = fmt.Sprintf("options[%d]", idx)
		}
		fields = append(fields, fieldDesc{
			label: label,
			get:   func() string { return e.Options[idx] },
			set:   func(v string) { e.Options[idx] = v },
			help:  "Kernel command line parameters (e.g. root=UUID=... rw quiet).",
		})
	}

	// Devicetree (if set).
	if e.Devicetree != "" {
		fields = append(fields, fieldDesc{
			label:  "devicetree",
			get:    func() string { return e.Devicetree },
			set:    func(v string) { e.Devicetree = strings.TrimSpace(v) },
			isPath: true,
			help:   "Device tree blob path relative to ESP root. ARM/RISC-V systems only.",
		})
	}

	m.editFields = fields
}

func (m *model) buildLoaderFields() {
	lc := m.draftLoader

	boolGet := func(b *bool) func() string {
		return func() string {
			if b == nil {
				return ""
			}
			if *b {
				return "yes"
			}
			return "no"
		}
	}
	boolSet := func(pp **bool) func(string) {
		return func(v string) {
			v = strings.ToLower(strings.TrimSpace(v))
			b := v == "yes" || v == "true" || v == "on" || v == "1" || v == "y"
			*pp = &b
		}
	}

	m.editFields = []fieldDesc{
		{
			label:    "timeout",
			get:      func() string { return lc.Timeout },
			set:      func(v string) { lc.Timeout = strings.TrimSpace(v) },
			required: false,
			help:     "Seconds before auto-boot. 0=immediate. 'menu-force'=always show menu. 'menu-hidden'=no countdown.",
		},
		{
			label:    "default",
			get:      func() string { return lc.Default },
			set:      func(v string) { lc.Default = strings.TrimSpace(v) },
			required: false,
			help:     "Default entry filename or glob (e.g. 'arch.conf'). '@saved' uses last-booted entry.",
		},
		{
			label:    "editor",
			get:      boolGet(lc.Editor),
			set:      boolSet(&lc.Editor),
			required: false,
			help:     "Allow kernel cmdline editing at boot prompt (yes/no). Disable on shared/secure systems.",
		},
		{
			label:    "console-mode",
			get:      func() string { return lc.ConsoleMode },
			set:      func(v string) { lc.ConsoleMode = strings.TrimSpace(v) },
			required: false,
			help:     "EFI console resolution: 0=80x25, 1=80x50, 2=highest, auto, max, or keep.",
		},
		{
			label:    "auto-entries",
			get:      boolGet(lc.AutoEntries),
			set:      boolSet(&lc.AutoEntries),
			required: false,
			help:     "Auto-add entries for other EFI bootloaders found on the ESP (yes/no).",
		},
		{
			label:    "auto-firmware",
			get:      boolGet(lc.AutoFirmware),
			set:      boolSet(&lc.AutoFirmware),
			required: false,
			help:     "Show 'Reboot into firmware interface' option in the menu (yes/no).",
		},
	}
}

func (m *model) loadFieldIntoInput() {
	if len(m.editFields) == 0 || m.editFieldIdx >= len(m.editFields) {
		return
	}
	m.editInput.SetValue(m.editFields[m.editFieldIdx].get())
	m.editInput.CursorEnd()
	m.completions = nil
	m.showCompletions = false
}

func (m *model) saveInputToField() {
	if m.editFieldIdx < len(m.editFields) {
		m.editFields[m.editFieldIdx].set(m.editInput.Value())
	}
}

func (m *model) editNextField() {
	m.saveInputToField()
	if len(m.editFields) > 0 {
		m.editFieldIdx = (m.editFieldIdx + 1) % len(m.editFields)
	}
	m.loadFieldIntoInput()
}

func (m *model) editPrevField() {
	m.saveInputToField()
	if len(m.editFields) > 0 {
		m.editFieldIdx = (m.editFieldIdx - 1 + len(m.editFields)) % len(m.editFields)
	}
	m.loadFieldIntoInput()
}

// diffLine represents a single changed field for the confirmation overlay.
type diffLine struct {
	field string
	old   string
	new   string
}

// prepareCommit validates the draft and, if valid, builds the diff and
// enters the confirming state. The actual write happens in finalizeCommit.
func (m *model) prepareCommit() {
	m.saveInputToField()

	// Validate required fields.
	for _, f := range m.editFields {
		if f.required && strings.TrimSpace(f.get()) == "" {
			m.validationErr = f.label + " is required"
			return
		}
	}
	m.validationErr = ""

	m.confirmDiff = m.buildDiff()
	m.confirming = true
}

// finalizeCommit writes the confirmed draft to disk.
func (m *model) finalizeCommit() {
	m.editing = false
	m.confirming = false
	m.editFields = nil
	m.completions = nil
	m.showCompletions = false
	m.confirmDiff = nil

	if m.focused == paneDetail {
		m.entry = m.draftEntry
		m.entries[m.selected] = m.draftEntry // keep slice in sync
		m.draftEntry = nil
		m.saveEntry()
		m.refreshEntryMeta()
	} else {
		m.loaderConf = m.draftLoader
		m.draftLoader = nil
		m.saveLoaderConf()
		m.refreshLoaderMeta()
	}
}

// buildDiff compares the original and draft structs and returns changed fields.
func (m *model) buildDiff() []diffLine {
	var diff []diffLine

	add := func(field, old, new string) {
		if old != new {
			diff = append(diff, diffLine{field, old, new})
		}
	}
	boolStr := func(b *bool) string {
		if b == nil {
			return ""
		}
		if *b {
			return "yes"
		}
		return "no"
	}

	if m.focused == paneDetail && m.entry != nil && m.draftEntry != nil {
		e, d := m.entry, m.draftEntry
		add("title", e.Title, d.Title)
		add("version", e.Version, d.Version)
		add("sort-key", e.SortKey, d.SortKey)
		add("linux", e.Linux, d.Linux)
		add("initrd", strings.Join(e.Initrds, ", "), strings.Join(d.Initrds, ", "))
		add("options", strings.Join(e.Options, " "), strings.Join(d.Options, " "))
		add("devicetree", e.Devicetree, d.Devicetree)
	} else if m.focused == paneLoader && m.loaderConf != nil && m.draftLoader != nil {
		l, d := m.loaderConf, m.draftLoader
		add("timeout", l.Timeout, d.Timeout)
		add("default", l.Default, d.Default)
		add("editor", boolStr(l.Editor), boolStr(d.Editor))
		add("console-mode", l.ConsoleMode, d.ConsoleMode)
		add("auto-entries", boolStr(l.AutoEntries), boolStr(d.AutoEntries))
		add("auto-firmware", boolStr(l.AutoFirmware), boolStr(d.AutoFirmware))
	}

	return diff
}

func (m *model) cancelEdit() {
	m.draftEntry = nil
	m.draftLoader = nil
	m.editFields = nil
	m.completions = nil
	m.showCompletions = false
	m.validationErr = ""
	m.editing = false
	m.confirming = false
	m.confirmDiff = nil
}

// handleEditKey routes keyboard input while in edit mode.
func (m *model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEscape:
		m.cancelEdit()
		return m, nil
	case tea.KeyEnter:
		m.prepareCommit()
		return m, nil
	case tea.KeyUp:
		m.editPrevField()
		return m, nil
	case tea.KeyDown:
		m.editNextField()
		return m, nil
	case tea.KeyTab:
		if len(m.editFields) > 0 && m.editFields[m.editFieldIdx].isPath {
			m.tabComplete()
		} else {
			m.editNextField()
		}
		return m, nil
	case tea.KeyShiftTab:
		m.editPrevField()
		return m, nil
	case tea.KeyRunes:
		if msg.String() == "?" {
			m.showHelp = !m.showHelp
			return m, nil
		}
	}
	// Forward all other input (printable chars, backspace, etc.) to the textinput.
	m.showCompletions = false
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(msg)
	return m, cmd
}

// tabComplete performs path completion on the current input value.
func (m *model) tabComplete() {
	prefix := m.editInput.Value()
	matches := m.matchESPPaths(prefix)
	if len(matches) == 0 {
		return
	}
	if m.showCompletions && len(m.completions) > 0 {
		m.compIdx = (m.compIdx + 1) % len(m.completions)
		m.editInput.SetValue(m.completions[m.compIdx])
		m.editInput.CursorEnd()
		return
	}
	if len(matches) == 1 {
		m.editInput.SetValue(matches[0])
		m.editInput.CursorEnd()
		return
	}
	common := commonPrefix(matches)
	if len(common) > len(prefix) {
		m.editInput.SetValue(common)
		m.editInput.CursorEnd()
	}
	m.completions = matches
	m.compIdx = 0
	m.showCompletions = true
}

func (m *model) matchESPPaths(prefix string) []string {
	if prefix == "" {
		return nil
	}
	lower := strings.ToLower(prefix)
	var out []string
	for _, p := range m.espPaths {
		if strings.HasPrefix(strings.ToLower(p), lower) {
			out = append(out, p)
		}
	}
	return out
}

func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}
