package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type BootEntry struct {
	ID           string
	Title        string
	Version      string
	MachineID    string
	SortKey      string
	Linux        string
	Efi          string // "efi" key: arbitrary EFI program (e.g. Windows Boot Manager)
	UKI          string // "uki" key: unified kernel image stored outside EFI/Linux/
	UKIUrl       string // "uki-url" key: network-booted UKI (RFC 3986 URI or ":name.efi")
	Profile      string // "profile" key: profile number of a multi-profile UKI
	Initrds      []string
	Options      []string
	Devicetree   string
	DTOOverlays  []string
	Extras       []string // "extra" keys: .cred/.sysext.raw/.confext.raw/.addon.efi companion files
	Architecture string
	IsUKI        bool     // true for entries discovered from EFI/Linux/*.efi or *.uki
	FileSuffix   string   // ".conf" for Type #1, ".efi"/".uki" for Type #2
	EFIPath      string   // set for UKI entries: /EFI/Linux/<id>.efi
	RawLines     []string // original lines preserved for round-trip fidelity
}

// knownEntryKeys are the Boot Loader Specification Type #1 keys this tool
// understands. Lines with any other key are preserved verbatim on write.
// See: https://uapi-group.org/specifications/specs/boot_loader_specification/
var knownEntryKeys = map[string]bool{
	"title": true, "version": true, "machine-id": true, "sort-key": true,
	"linux": true, "initrd": true, "options": true, "devicetree": true,
	"devicetree-overlay": true, "architecture": true,
	"efi": true, "uki": true, "uki-url": true, "profile": true, "extra": true,
}

// entryKeyOrder is the canonical order in which fields are written.
var entryKeyOrder = []string{
	"title", "version", "machine-id", "sort-key", "architecture",
	"efi", "uki", "uki-url", "profile",
	"linux", "initrd", "options",
	"devicetree", "devicetree-overlay", "extra",
}

// FullID returns the entry identifier exactly as systemd-boot knows it: the
// file name including its ".conf"/".efi"/".uki" suffix. loader.conf default=
// patterns and the LoaderEntryOneShot/Default EFI variables are matched
// (case-insensitively, as globs) against this identifier.
func (e *BootEntry) FullID() string {
	if e.FileSuffix != "" {
		return e.ID + e.FileSuffix
	}
	if e.IsUKI {
		return e.ID + ".efi"
	}
	return e.ID + ".conf"
}

// StripEntrySuffix removes a boot entry file suffix from an identifier,
// returning the "entry token" (e.g. "arch.conf" → "arch"). Matching is
// case-insensitive.
func StripEntrySuffix(id string) string {
	lower := strings.ToLower(id)
	for _, suffix := range []string{".conf", ".efi", ".uki"} {
		if strings.HasSuffix(lower, suffix) {
			return id[:len(id)-len(suffix)]
		}
	}
	return id
}

// ImageKeys returns which of the mutually exclusive image source keys
// (linux, efi, uki, uki-url) are set on this entry. systemd-boot drops
// entries with none or with more than one of these keys.
func (e *BootEntry) ImageKeys() []string {
	var keys []string
	if e.Linux != "" {
		keys = append(keys, "linux")
	}
	if e.Efi != "" {
		keys = append(keys, "efi")
	}
	if e.UKI != "" {
		keys = append(keys, "uki")
	}
	if e.UKIUrl != "" {
		keys = append(keys, "uki-url")
	}
	return keys
}

func ParseEntry(path string) (*BootEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading entry: %w", err)
	}

	entry := &BootEntry{
		ID:         strings.TrimSuffix(filepath.Base(path), ".conf"),
		FileSuffix: ".conf",
	}

	content := strings.TrimRight(string(data), "\n")
	for lineNum, line := range strings.Split(content, "\n") {
		lineNum++
		trimmed := strings.TrimSpace(line)

		entry.RawLines = append(entry.RawLines, trimmed)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Split on first run of whitespace (space or tab).
		idx := strings.IndexAny(trimmed, " \t")
		if idx < 0 {
			return nil, fmt.Errorf("%s:%d: malformed line: %s", path, lineNum, trimmed)
		}
		key := trimmed[:idx]
		value := strings.TrimSpace(trimmed[idx:])

		switch key {
		case "title":
			entry.Title = value
		case "version":
			entry.Version = value
		case "machine-id":
			entry.MachineID = value
		case "sort-key":
			entry.SortKey = value
		case "linux":
			entry.Linux = value
		case "efi":
			entry.Efi = value
		case "uki":
			entry.UKI = value
		case "uki-url":
			entry.UKIUrl = value
		case "profile":
			entry.Profile = value
		case "initrd":
			entry.Initrds = append(entry.Initrds, value)
		case "options":
			entry.Options = append(entry.Options, value)
		case "devicetree":
			entry.Devicetree = value
		case "devicetree-overlay":
			entry.DTOOverlays = append(entry.DTOOverlays, strings.Fields(value)...)
		case "extra":
			entry.Extras = append(entry.Extras, value)
		case "architecture":
			entry.Architecture = value
		default:
			// Unrecognized keys are preserved in RawLines.
		}
	}

	return entry, nil
}

func (e *BootEntry) Write(path string) error {
	var data []byte
	if len(e.RawLines) > 0 {
		data = e.writeWithRawLines()
	} else {
		data = e.writeStandard()
	}
	return writeFileAtomic(path, data, 0644)
}

func (e *BootEntry) writeStandard() []byte {
	var lines []string
	for _, key := range entryKeyOrder {
		for _, v := range e.valueMap()[key] {
			lines = append(lines, fmt.Sprintf("%s %s", key, v))
		}
	}
	return []byte(strings.Join(trimTrailingBlanks(lines), "\n") + "\n")
}

func (e *BootEntry) writeWithRawLines() []byte {
	vals := e.valueMap()
	used := make(map[string]int)
	var lines []string

	for _, raw := range e.RawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, raw)
			continue
		}
		idx := strings.IndexAny(trimmed, " \t")
		if idx < 0 {
			lines = append(lines, raw)
			continue
		}
		key := trimmed[:idx]

		if !knownEntryKeys[key] {
			// Vendor extensions and unknown keys are kept verbatim.
			lines = append(lines, raw)
			continue
		}
		vv := vals[key]
		i := used[key]
		used[key] = i + 1
		if i < len(vv) {
			lines = append(lines, key+" "+vv[i])
		}
		// Extra original lines for a key whose value was cleared (or whose
		// list shrank) are dropped.
	}

	for _, key := range entryKeyOrder {
		vv := vals[key]
		for i := used[key]; i < len(vv); i++ {
			lines = append(lines, key+" "+vv[i])
		}
	}

	return []byte(strings.Join(trimTrailingBlanks(lines), "\n") + "\n")
}

// trimTrailingBlanks drops blank lines from the end of a file. Parsing trims
// trailing newlines before splitting, so without this the writer would keep
// re-adding one and write(parse(x)) would not be a fixed point.
func trimTrailingBlanks(lines []string) []string {
	i := len(lines)
	for i > 0 && strings.TrimSpace(lines[i-1]) == "" {
		i--
	}
	return lines[:i]
}

// valueMap returns the current field values keyed by BLS key name. Cleared
// fields and empty list elements are represented by short/absent value lists
// so that writing removes their original lines instead of keeping them.
func (e *BootEntry) valueMap() map[string][]string {
	m := make(map[string][]string, len(entryKeyOrder))
	for _, key := range entryKeyOrder {
		m[key] = nil
	}
	set := func(key, v string) {
		if v != "" {
			m[key] = []string{v}
		}
	}
	set("title", e.Title)
	set("version", e.Version)
	set("machine-id", e.MachineID)
	set("sort-key", e.SortKey)
	set("architecture", e.Architecture)
	set("efi", e.Efi)
	set("uki", e.UKI)
	set("uki-url", e.UKIUrl)
	set("profile", e.Profile)
	set("linux", e.Linux)
	set("devicetree", e.Devicetree)
	m["initrd"] = nonEmptyStrings(e.Initrds)
	m["options"] = nonEmptyStrings(e.Options)
	m["devicetree-overlay"] = nonEmptyStrings(e.DTOOverlays)
	m["extra"] = nonEmptyStrings(e.Extras)
	return m
}

func nonEmptyStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// writeFileAtomic writes data to path atomically by creating a temp file in the
// same directory, syncing it, and renaming over the target. If the target file
// already exists its permissions are preserved; otherwise 0644 is used.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path))
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}

	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	// Sync the parent directory so the rename is durable on non-journaled
	// filesystems (e.g. FAT32, commonly used for the ESP).
	parent, parentErr := os.Open(dir)
	if parentErr == nil {
		parent.Sync()
		parent.Close()
	}
	return nil
}

func (e *BootEntry) DisplayTitle() string {
	if e.Title != "" {
		return e.Title
	}
	return e.ID
}

func ParseEntries(entriesDir string) ([]*BootEntry, error) {
	entries, err := os.ReadDir(entriesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading entries directory: %w", err)
	}

	var result []*BootEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}

		path := filepath.Join(entriesDir, entry.Name())
		bootEntry, err := ParseEntry(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		result = append(result, bootEntry)
	}

	return result, nil
}

func ParseAllEntries(espPath string) ([]*BootEntry, error) {
	var all []*BootEntry

	loaderDir := filepath.Join(espPath, "loader", "entries")
	entries, err := ParseEntries(loaderDir)
	if err != nil {
		return nil, err
	}
	all = append(all, entries...)

	efiLinuxDir := filepath.Join(espPath, "EFI", "Linux")
	ukis, err := parseUKIs(efiLinuxDir)
	if err != nil {
		return nil, err
	}
	all = append(all, ukis...)

	return all, nil
}

// ukiExtensions are the file suffixes systemd-boot reads unified kernel
// images from in EFI/Linux/. ".uki" support was added in systemd v257.
var ukiExtensions = []string{".efi", ".uki"}

func parseUKIs(dir string) ([]*BootEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading UKI directory: %w", err)
	}

	var result []*BootEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		suffix := ""
		for _, ext := range ukiExtensions {
			if strings.HasSuffix(lower, ext) {
				suffix = ext
				break
			}
		}
		if suffix == "" {
			continue
		}
		base := name[:len(name)-len(suffix)]

		uki := &BootEntry{
			ID:         base,
			Title:      base,
			IsUKI:      true,
			FileSuffix: suffix,
			EFIPath:    "/EFI/Linux/" + name,
		}
		result = append(result, uki)
	}

	return result, nil
}
