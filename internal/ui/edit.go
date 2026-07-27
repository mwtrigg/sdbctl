package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

// fieldDesc describes one editable field in a boot entry or loader config.
type fieldDesc struct {
	label    string              // key name shown in the UI
	get      func() string       // reads current draft value
	set      func(string)        // writes value back to draft
	isPath   bool                // enables ESP tab completion on Tab key
	required bool                // blocks save if empty
	validate func(string) string // returns error message, or "" if valid
	help     string              // context help shown at the bottom of the pane
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

// enterNewEntryMode creates a blank boot entry, appends it to m.entries, selects
// it, and enters edit mode so the user can fill in all fields including the ID.
func (m *model) enterNewEntryMode() tea.Cmd {
	newEntry := &boot.BootEntry{}
	m.entries = append(m.entries, newEntry)
	m.selected = len(m.entries) - 1
	m.entry = newEntry
	m.draftEntry = newEntry
	m.focused = paneDetail
	m.creating = true
	m.buildEntryFields()
	return m.resetEditInput()
}

// confirmDelete removes the selected entry's .conf file and updates the slice.
func (m *model) confirmDelete() {
	if m.entry == nil || m.entry.IsUKI {
		return
	}
	path := filepath.Join(m.espPath, "loader", "entries", m.entry.ID+".conf")
	if err := os.Remove(path); err != nil {
		m.err = err
		return
	}
	m.entries = append(m.entries[:m.selected], m.entries[m.selected+1:]...)
	if len(m.entries) == 0 {
		m.selected = 0
		m.entry = nil
	} else {
		if m.selected >= len(m.entries) {
			m.selected = len(m.entries) - 1
		}
		m.entry = m.entries[m.selected]
	}
	m.refreshEntryMeta()
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

	var fields []fieldDesc

	if m.creating {
		fields = append(fields, fieldDesc{
			label:    "id",
			get:      func() string { return e.ID },
			set:      func(v string) { e.ID = strings.TrimSpace(v) },
			required: true,
			validate: validateEntryID,
			help:     "Filename without .conf (e.g. arch-linux). Written to loader/entries/<id>.conf on the ESP. No spaces or path separators. Note: UKI entries (.efi) cannot be created here — they are discovered automatically from EFI/Linux/.",
		})
	}

	fields = append(fields, []fieldDesc{
		{
			label:    "title",
			get:      func() string { return e.Title },
			set:      func(v string) { e.Title = strings.TrimSpace(v) },
			required: true,
			validate: validateNoNewlines,
			help:     "Human-readable name shown in the boot menu.",
		},
		{
			label:    "version",
			get:      func() string { return e.Version },
			set:      func(v string) { e.Version = strings.TrimSpace(v) },
			validate: validateNoNewlines,
			help:     "Version string for display and entry sorting.",
		},
		{
			label:    "sort-key",
			get:      func() string { return e.SortKey },
			set:      func(v string) { e.SortKey = strings.TrimSpace(v) },
			validate: validateNoNewlines,
			help:     "Key used to order entries in the menu. Entries with the same sort-key are grouped.",
		},
		{
			label:    "linux",
			get:      func() string { return e.Linux },
			set:      func(v string) { e.Linux = strings.TrimSpace(v) },
			isPath:   true,
			required: true,
			validate: validateESPPath,
			help:     "Kernel image path relative to ESP root (e.g. /vmlinuz-linux). Tab for completion.",
		},
	}...)

	// One field per initrd entry.
	for i := range e.Initrds {
		idx := i
		label := "initrd"
		if len(e.Initrds) > 1 {
			label = fmt.Sprintf("initrd[%d]", idx)
		}
		fields = append(fields, fieldDesc{
			label:    label,
			get:      func() string { return e.Initrds[idx] },
			set:      func(v string) { e.Initrds[idx] = strings.TrimSpace(v) },
			isPath:   true,
			validate: validateESPPath,
			help:     "Initial RAM disk path relative to ESP root. Tab for completion.",
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
	fields = append(fields, fieldDesc{
		label:    "machine-id",
		get:      func() string { return e.MachineID },
		set:      func(v string) { e.MachineID = strings.TrimSpace(v) },
		validate: validateMachineID,
		help:     "32-character lowercase hex machine identifier. Leave empty to omit.",
	})

	fields = append(fields, fieldDesc{
		label:    "architecture",
		get:      func() string { return e.Architecture },
		set:      func(v string) { e.Architecture = strings.TrimSpace(v) },
		validate: validateArchitecture,
		help:     "Override boot architecture: x64, aa64, ia32, riscv64, loongarch64. Usually omitted.",
	})

	fields = append(fields, fieldDesc{
		label:    "devicetree",
		get:      func() string { return e.Devicetree },
		set:      func(v string) { e.Devicetree = strings.TrimSpace(v) },
		isPath:   true,
		validate: validateESPPath,
		help:     "Device tree blob path relative to ESP root. ARM/RISC-V systems only.",
	})

	for i := range e.DTOOverlays {
		idx := i
		label := fmt.Sprintf("devicetree-overlay[%d]", idx)
		fields = append(fields, fieldDesc{
			label:    label,
			get:      func() string { return e.DTOOverlays[idx] },
			set:      func(v string) { e.DTOOverlays[idx] = strings.TrimSpace(v) },
			isPath:   true,
			validate: validateESPPath,
			help:     "Device tree overlay path relative to ESP root.",
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
			validate: validateTimeout,
			help:     "Seconds before auto-boot. 0=immediate. 'menu-force'=always show menu. 'menu-hidden'=no countdown.",
		},
		{
			label: "default",
			get:   func() string { return lc.Default },
			set:   func(v string) { lc.Default = strings.TrimSpace(v) },
			help:  "Default entry filename or glob (e.g. 'arch.conf'). '@saved' uses last-booted entry.",
		},
		{
			label:    "editor",
			get:      boolGet(lc.Editor),
			set:      boolSet(&lc.Editor),
			validate: validateBoolField,
			help:     "Allow kernel cmdline editing at boot prompt (yes/no). Disable on shared/secure systems.",
		},
		{
			label:    "console-mode",
			get:      func() string { return lc.ConsoleMode },
			set:      func(v string) { lc.ConsoleMode = strings.TrimSpace(v) },
			validate: validateConsoleMode,
			help:     "EFI console resolution: 0=80x25, 1=80x50, 2=highest, auto, max, or keep.",
		},
		{
			label:    "auto-entries",
			get:      boolGet(lc.AutoEntries),
			set:      boolSet(&lc.AutoEntries),
			validate: validateBoolField,
			help:     "Auto-add entries for other EFI bootloaders found on the ESP (yes/no).",
		},
		{
			label:    "auto-firmware",
			get:      boolGet(lc.AutoFirmware),
			set:      boolSet(&lc.AutoFirmware),
			validate: validateBoolField,
			help:     "Show 'Reboot into firmware interface' option in the menu (yes/no).",
		},
		{
			label:    "auto-reboot",
			get:      boolGet(lc.AutoReboot),
			set:      boolSet(&lc.AutoReboot),
			validate: validateBoolField,
			help:     "Automatically reboot after menu timeout with no input (yes/no).",
		},
		{
			label:    "auto-poweroff",
			get:      boolGet(lc.AutoPowerOff),
			set:      boolSet(&lc.AutoPowerOff),
			validate: validateBoolField,
			help:     "Automatically power off after menu timeout with no input (yes/no).",
		},
		{
			label:    "beep",
			get:      boolGet(lc.Beep),
			set:      boolSet(&lc.Beep),
			validate: validateBoolField,
			help:     "Beep on boot menu navigation (yes/no). Requires a PC speaker.",
		},
		{
			label:    "reboot-on-error",
			get:      func() string { return lc.RebootOnError },
			set:      func(v string) { lc.RebootOnError = strings.TrimSpace(v) },
			validate: validateRebootOnError,
			help:     "Action on boot failure: yes (reboot), no (halt), auto (firmware default).",
		},
		{
			label:    "log-level",
			get:      func() string { return lc.LogLevel },
			set:      func(v string) { lc.LogLevel = strings.TrimSpace(v) },
			validate: validateLogLevelField,
			help:     "Logging verbosity: emerg, alert, crit, err, warning, notice, info, or debug.",
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

	// Validate all fields: required check, then per-field validator.
	for _, f := range m.editFields {
		v := f.get()
		if f.required && strings.TrimSpace(v) == "" {
			m.validationErr = f.label + " is required"
			return
		}
		if f.validate != nil {
			if msg := f.validate(v); msg != "" {
				m.validationErr = f.label + ": " + msg
				return
			}
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
		if m.creating {
			path := filepath.Join(m.espPath, "loader", "entries", m.draftEntry.ID+".conf")
			if err := m.draftEntry.Write(path); err != nil {
				m.err = err
			}
			m.creating = false
		} else {
			m.saveEntry()
		}
		m.draftEntry = nil
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
		add("machine-id", e.MachineID, d.MachineID)
		add("architecture", e.Architecture, d.Architecture)
		add("devicetree", e.Devicetree, d.Devicetree)
		add("devicetree-overlay", strings.Join(e.DTOOverlays, ", "), strings.Join(d.DTOOverlays, ", "))
	} else if m.focused == paneLoader && m.loaderConf != nil && m.draftLoader != nil {
		l, d := m.loaderConf, m.draftLoader
		add("timeout", l.Timeout, d.Timeout)
		add("default", l.Default, d.Default)
		add("editor", boolStr(l.Editor), boolStr(d.Editor))
		add("console-mode", l.ConsoleMode, d.ConsoleMode)
		add("auto-entries", boolStr(l.AutoEntries), boolStr(d.AutoEntries))
		add("auto-firmware", boolStr(l.AutoFirmware), boolStr(d.AutoFirmware))
		add("auto-reboot", boolStr(l.AutoReboot), boolStr(d.AutoReboot))
		add("auto-poweroff", boolStr(l.AutoPowerOff), boolStr(d.AutoPowerOff))
		add("beep", boolStr(l.Beep), boolStr(d.Beep))
		add("reboot-on-error", l.RebootOnError, d.RebootOnError)
		add("log-level", l.LogLevel, d.LogLevel)
	}

	return diff
}

func (m *model) cancelEdit() {
	if m.creating {
		// Remove the unsaved placeholder entry.
		m.entries = m.entries[:len(m.entries)-1]
		if len(m.entries) > 0 {
			m.selected = len(m.entries) - 1
			m.entry = m.entries[m.selected]
		} else {
			m.selected = 0
			m.entry = nil
		}
		m.creating = false
	}
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
	case tea.KeyCtrlN:
		// Append a new line to the initrd or devicetree-overlay list.
		if m.focused == paneDetail && m.draftEntry != nil {
			m.appendListField()
			return m, nil
		}
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

// appendListField adds a new blank element to the initrd or devicetree-overlay
// list, rebuilds the field list, and positions the cursor on the new field.
// It is a no-op if the active field is not one of those two types.
func (m *model) appendListField() {
	if m.editFieldIdx >= len(m.editFields) {
		return
	}
	m.saveInputToField()
	label := m.editFields[m.editFieldIdx].label

	e := m.draftEntry
	switch {
	case strings.HasPrefix(label, "initrd"):
		e.Initrds = append(e.Initrds, "")
		m.buildEntryFields()
		// Position on the newly appended initrd field.
		for i, f := range m.editFields {
			if strings.HasPrefix(f.label, "initrd") {
				m.editFieldIdx = i
			}
		}
	case strings.HasPrefix(label, "devicetree-overlay"):
		e.DTOOverlays = append(e.DTOOverlays, "")
		m.buildEntryFields()
		for i, f := range m.editFields {
			if strings.HasPrefix(f.label, "devicetree-overlay") {
				m.editFieldIdx = i
			}
		}
	default:
		return
	}
	m.loadFieldIntoInput()
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

// validateEntryID checks that s is a valid boot entry filename component:
// no path separators, no spaces, no newlines, no dots at the start, non-empty after trim.
func validateEntryID(s string) string {
	if s == "" {
		return "" // handled by required check
	}
	if strings.ContainsAny(s, "/ \\ \t") {
		return "must not contain spaces or path separators"
	}
	if strings.ContainsAny(s, "\x00") {
		return "must not contain null bytes"
	}
	if strings.ContainsAny(s, "\n\r") {
		return "must not contain newlines"
	}
	if strings.HasPrefix(s, ".") {
		return "must not start with a dot"
	}
	return ""
}

// validateESPPath checks that s is a valid absolute ESP-relative path:
// must start with /, no spaces, no newlines, no null bytes, no .. traversal.
func validateESPPath(s string) string {
	if s == "" {
		return "" // handled by required check
	}
	if !strings.HasPrefix(s, "/") {
		return "must be an absolute path starting with /"
	}
	if strings.ContainsAny(s, " \t\x00") {
		return "must not contain spaces or null bytes"
	}
	if strings.ContainsAny(s, "\n\r") {
		return "must not contain newlines"
	}
	for _, part := range strings.Split(s, "/") {
		if part == ".." {
			return "must not contain .. path traversal"
		}
	}
	return ""
}

// validateNoNewlines rejects values containing newlines or null bytes, used
// for single-line text fields like title, version, sort-key.
func validateNoNewlines(s string) string {
	if strings.ContainsAny(s, "\n\r\x00") {
		return "must not contain newlines or null bytes"
	}
	return ""
}

// validateTimeout mirrors the loader.conf spec: a non-negative integer, or one
// of the special tokens menu-force / menu-hidden.
func validateTimeout(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case "menu-force", "menu-hidden":
		return ""
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return "must be a number of seconds, \"menu-force\", or \"menu-hidden\""
		}
	}
	return ""
}

// validateConsoleMode mirrors the loader.conf spec.
func validateConsoleMode(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case "0", "1", "2", "auto", "max", "keep":
		return ""
	}
	return "must be 0, 1, 2, auto, max, or keep"
}

// validateBoolField accepts yes/no and common synonyms.
func validateBoolField(s string) string {
	if s == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "no", "true", "false", "on", "off", "1", "0", "y", "n":
		return ""
	}
	return "must be yes or no"
}

// validateMachineID accepts a 32-char lowercase hex string or empty.
func validateMachineID(s string) string {
	if s == "" {
		return ""
	}
	if len(s) != 32 {
		return "must be exactly 32 hex characters"
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "must be lowercase hex characters only"
		}
	}
	return ""
}

// validateArchitecture accepts known systemd-boot architecture tokens or empty.
func validateArchitecture(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case "x64", "aa64", "ia32", "riscv64", "loongarch64":
		return ""
	}
	return "must be one of: x64, aa64, ia32, riscv64, loongarch64"
}

// validateRebootOnError mirrors the loader.conf reboot-on-error spec.
func validateRebootOnError(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case "yes", "no", "auto":
		return ""
	}
	return "must be yes, no, or auto"
}

// validateLogLevelField mirrors the loader.conf log-level spec.
func validateLogLevelField(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case "emerg", "alert", "crit", "err", "warning", "notice", "info", "debug":
		return ""
	}
	return "must be one of: emerg, alert, crit, err, warning, notice, info, debug"
}
