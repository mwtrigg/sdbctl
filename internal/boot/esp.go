package boot

import (
	"fmt"
	"os"
	"path/filepath"
)

var espCandidates = []string{"/boot", "/boot/efi", "/efi"}

func FindESP() (string, error) {
	for _, path := range espCandidates {
		loaderDir := filepath.Join(path, "loader")
		if _, err := os.Stat(loaderDir); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("ESP not found in any of: %v", espCandidates)
}

func FindLoaderConf(espPath string) string {
	return filepath.Join(espPath, "loader", "loader.conf")
}

func FindEntriesDir(espPath string) string {
	return filepath.Join(espPath, "loader", "entries")
}

func DiscoverBoot() (*BootInfo, error) {
	espPath, err := FindESP()
	if err != nil {
		return nil, err
	}

	info := &BootInfo{
		ESPPath: espPath,
	}

	loaderConfPath := FindLoaderConf(espPath)
	if _, err := os.Stat(loaderConfPath); err == nil {
		cfg, err := ParseLoaderConf(loaderConfPath)
		if err != nil {
			return nil, fmt.Errorf("parsing loader.conf: %w", err)
		}
		info.LoaderConf = cfg
	}

	entriesDir := FindEntriesDir(espPath)
	entries, err := ParseEntries(entriesDir)
	if err != nil {
		return nil, fmt.Errorf("parsing entries: %w", err)
	}
	info.Entries = entries

	return info, nil
}

type BootInfo struct {
	ESPPath    string
	LoaderConf *LoaderConfig
	Entries    []*BootEntry
}
