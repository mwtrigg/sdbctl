package boot

import (
	"fmt"
	"os"
	"path/filepath"
)

type ValidationError struct {
	EntryID string
	Issues  []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %v", e.EntryID, e.Issues)
}

func ValidateEntry(entry *BootEntry, espPath string) *ValidationError {
	var issues []string

	if entry.Linux == "" {
		issues = append(issues, "missing linux field")
	} else {
		path := filepath.Join(espPath, entry.Linux)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, fmt.Sprintf("linux file not found: %s", entry.Linux))
			} else {
				issues = append(issues, fmt.Sprintf("cannot stat linux file: %v", err))
			}
		}
	}

	for _, initrd := range entry.Initrds {
		path := filepath.Join(espPath, initrd)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, fmt.Sprintf("initrd file not found: %s", initrd))
			} else {
				issues = append(issues, fmt.Sprintf("cannot stat initrd file: %v", err))
			}
		}
	}

	if entry.Devicetree != "" {
		path := filepath.Join(espPath, entry.Devicetree)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, fmt.Sprintf("devicetree file not found: %s", entry.Devicetree))
			} else {
				issues = append(issues, fmt.Sprintf("cannot stat devicetree file: %v", err))
			}
		}
	}

	for _, dto := range entry.DTOOverlays {
		path := filepath.Join(espPath, dto)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, fmt.Sprintf("devicetree-overlay file not found: %s", dto))
			} else {
				issues = append(issues, fmt.Sprintf("cannot stat devicetree-overlay file: %v", err))
			}
		}
	}

	if len(issues) > 0 {
		return &ValidationError{
			EntryID: entry.ID,
			Issues:  issues,
		}
	}

	return nil
}

func ValidateEntries(entries []*BootEntry, espPath string) []error {
	var errors []error

	for _, entry := range entries {
		if err := ValidateEntry(entry, espPath); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func ValidateLoaderConf(cfg *LoaderConfig) error {
	if cfg.Timeout != "" {
		if _, err := parseTimeout(cfg.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}

	if cfg.ConsoleMode != "" {
		if err := validateConsoleMode(cfg.ConsoleMode); err != nil {
			return err
		}
	}

	if cfg.LogLevel != "" {
		if err := validateLogLevel(cfg.LogLevel); err != nil {
			return err
		}
	}

	if cfg.RebootOnError != "" {
		switch cfg.RebootOnError {
		case "yes", "no", "auto":
		default:
			return fmt.Errorf("invalid reboot-on-error: %s (expected yes, no, or auto)", cfg.RebootOnError)
		}
	}

	return nil
}
