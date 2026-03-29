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
}

func ParseEntry(path string) (*BootEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading entry: %w", err)
	}

	entry := &BootEntry{
		ID: strings.TrimSuffix(filepath.Base(path), ".conf"),
	}

	for lineNum, line := range strings.Split(string(data), "\n") {
		lineNum++
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			if parts[0] != "" {
				return nil, fmt.Errorf("%s:%d: malformed line: %s", path, lineNum, line)
			}
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

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
			return nil, fmt.Errorf("%s:%d: unknown option: %s", path, lineNum, key)
		}
	}

	return entry, nil
}

func (e *BootEntry) Write(path string) error {
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

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
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

		uki := &BootEntry{
			ID:    strings.TrimSuffix(entry.Name(), ".efi"),
			Title: strings.TrimSuffix(entry.Name(), ".efi"),
		}
		result = append(result, uki)
	}

	return result, nil
}
