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
	Initrds      []string
	Options      []string
	Devicetree   string
	DTOOverlays  []string
	Architecture string
	IsUKI        bool     // true for entries discovered from EFI/Linux/*.efi
	EFIPath      string   // set for UKI entries: /EFI/Linux/<id>.efi
	RawLines     []string // original lines preserved for round-trip fidelity
}

func ParseEntry(path string) (*BootEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading entry: %w", err)
	}

	entry := &BootEntry{
		ID: strings.TrimSuffix(filepath.Base(path), ".conf"),
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
		case "initrd":
			entry.Initrds = append(entry.Initrds, value)
		case "options":
			entry.Options = append(entry.Options, value)
		case "devicetree":
			entry.Devicetree = value
		case "devicetree-overlay":
			entry.DTOOverlays = append(entry.DTOOverlays, strings.Fields(value)...)
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

	if e.Title != "" {
		lines = append(lines, fmt.Sprintf("title %s", e.Title))
	}
	if e.Version != "" {
		lines = append(lines, fmt.Sprintf("version %s", e.Version))
	}
	if e.MachineID != "" {
		lines = append(lines, fmt.Sprintf("machine-id %s", e.MachineID))
	}
	if e.SortKey != "" {
		lines = append(lines, fmt.Sprintf("sort-key %s", e.SortKey))
	}
	if e.Architecture != "" {
		lines = append(lines, fmt.Sprintf("architecture %s", e.Architecture))
	}
	if e.Linux != "" {
		lines = append(lines, fmt.Sprintf("linux %s", e.Linux))
	}
	for _, initrd := range e.Initrds {
		lines = append(lines, fmt.Sprintf("initrd %s", initrd))
	}
	for _, opt := range e.Options {
		lines = append(lines, fmt.Sprintf("options %s", opt))
	}
	if e.Devicetree != "" {
		lines = append(lines, fmt.Sprintf("devicetree %s", e.Devicetree))
	}
	for _, dto := range e.DTOOverlays {
		lines = append(lines, fmt.Sprintf("devicetree-overlay %s", dto))
	}

	return []byte(strings.Join(lines, "\n") + "\n")
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

		vv, ok := vals[key]
		if !ok {
			lines = append(lines, raw)
			continue
		}
		i := used[key]
		if i < len(vv) {
			lines = append(lines, key+" "+vv[i])
			used[key] = i + 1
		}
	}

	for key, vv := range vals {
		for i := used[key]; i < len(vv); i++ {
			lines = append(lines, key+" "+vv[i])
		}
	}

	return []byte(strings.Join(lines, "\n") + "\n")
}

func (e *BootEntry) valueMap() map[string][]string {
	m := make(map[string][]string)
	if e.Title != "" {
		m["title"] = []string{e.Title}
	}
	if e.Version != "" {
		m["version"] = []string{e.Version}
	}
	if e.MachineID != "" {
		m["machine-id"] = []string{e.MachineID}
	}
	if e.SortKey != "" {
		m["sort-key"] = []string{e.SortKey}
	}
	if e.Architecture != "" {
		m["architecture"] = []string{e.Architecture}
	}
	if e.Linux != "" {
		m["linux"] = []string{e.Linux}
	}
	if len(e.Initrds) > 0 {
		m["initrd"] = e.Initrds
	}
	if len(e.Options) > 0 {
		m["options"] = e.Options
	}
	if e.Devicetree != "" {
		m["devicetree"] = []string{e.Devicetree}
	}
	if len(e.DTOOverlays) > 0 {
		m["devicetree-overlay"] = e.DTOOverlays
	}
	return m
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
	return os.Rename(tmpPath, path)
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".efi") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".efi")
		uki := &BootEntry{
			ID:      id,
			Title:   id,
			IsUKI:   true,
			EFIPath: "/EFI/Linux/" + entry.Name(),
		}
		result = append(result, uki)
	}

	return result, nil
}
