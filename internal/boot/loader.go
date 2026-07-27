package boot

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type LoaderConfig struct {
	Timeout       string
	Default       string
	Editor        *bool
	ConsoleMode   string
	AutoEntries   *bool
	AutoFirmware  *bool
	AutoReboot    *bool
	AutoPowerOff  *bool
	Beep          *bool
	RebootOnError string
	LogLevel      string
}

var boolTrue = true
var boolFalse = false

func defaultLoaderConfig() *LoaderConfig {
	return &LoaderConfig{
		Editor:       &boolTrue,
		AutoEntries:  &boolTrue,
		AutoFirmware: &boolTrue,
	}
}

func ParseLoaderConf(path string) (*LoaderConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading loader.conf: %w", err)
	}

	cfg := defaultLoaderConfig()

	for lineNum, line := range strings.Split(string(data), "\n") {
		lineNum++
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first run of whitespace (space or tab).
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			return nil, fmt.Errorf("%s:%d: malformed line: %s", path, lineNum, line)
		}
		key := line[:idx]
		value := strings.TrimSpace(line[idx:])

		switch key {
		case "timeout":
			cfg.Timeout = value
		case "default":
			cfg.Default = value
		case "editor":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid editor value: %w", path, lineNum, err)
			}
			cfg.Editor = b
		case "console-mode":
			cfg.ConsoleMode = value
		case "auto-entries":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid auto-entries value: %w", path, lineNum, err)
			}
			cfg.AutoEntries = b
		case "auto-firmware":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid auto-firmware value: %w", path, lineNum, err)
			}
			cfg.AutoFirmware = b
		case "auto-reboot":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid auto-reboot value: %w", path, lineNum, err)
			}
			cfg.AutoReboot = b
		case "auto-poweroff":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid auto-poweroff value: %w", path, lineNum, err)
			}
			cfg.AutoPowerOff = b
		case "beep":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid beep value: %w", path, lineNum, err)
			}
			cfg.Beep = b
		case "reboot-on-error":
			cfg.RebootOnError = value
		case "log-level":
			cfg.LogLevel = value
		default:
			return nil, fmt.Errorf("%s:%d: unknown option: %s", path, lineNum, key)
		}
	}

	return cfg, nil
}

func (c *LoaderConfig) Write(path string) error {
	var lines []string

	if c.Timeout != "" {
		lines = append(lines, fmt.Sprintf("timeout %s", c.Timeout))
	}
	if c.Default != "" {
		lines = append(lines, fmt.Sprintf("default %s", c.Default))
	}
	if c.Editor != nil {
		lines = append(lines, fmt.Sprintf("editor %s", formatBool(*c.Editor)))
	}
	if c.ConsoleMode != "" {
		lines = append(lines, fmt.Sprintf("console-mode %s", c.ConsoleMode))
	}
	if c.AutoEntries != nil {
		lines = append(lines, fmt.Sprintf("auto-entries %s", formatBool(*c.AutoEntries)))
	}
	if c.AutoFirmware != nil {
		lines = append(lines, fmt.Sprintf("auto-firmware %s", formatBool(*c.AutoFirmware)))
	}
	if c.AutoReboot != nil {
		lines = append(lines, fmt.Sprintf("auto-reboot %s", formatBool(*c.AutoReboot)))
	}
	if c.AutoPowerOff != nil {
		lines = append(lines, fmt.Sprintf("auto-poweroff %s", formatBool(*c.AutoPowerOff)))
	}
	if c.Beep != nil {
		lines = append(lines, fmt.Sprintf("beep %s", formatBool(*c.Beep)))
	}
	if c.RebootOnError != "" {
		lines = append(lines, fmt.Sprintf("reboot-on-error %s", c.RebootOnError))
	}
	if c.LogLevel != "" {
		lines = append(lines, fmt.Sprintf("log-level %s", c.LogLevel))
	}

	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func parseBool(s string) (*bool, error) {
	s = strings.ToLower(s)
	switch s {
	case "yes", "y", "true", "t", "on", "1":
		return &boolTrue, nil
	case "no", "n", "false", "f", "off", "0":
		return &boolFalse, nil
	default:
		return nil, fmt.Errorf("invalid boolean: %s", s)
	}
}

func formatBool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func parseTimeout(s string) (int, error) {
	if s == "" || s == "menu-disabled" || s == "menu-hidden" {
		return 0, nil
	}
	if s == "menu-force" {
		return -1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout: %s", s)
	}
	return n, nil
}

var validConsoleMode = regexp.MustCompile(`^(0|1|2|auto|max|keep)$`)

func validateConsoleMode(s string) error {
	if s == "" {
		return nil
	}
	if !validConsoleMode.MatchString(s) {
		return fmt.Errorf("invalid console-mode: %s (expected 0, 1, 2, auto, max, or keep)", s)
	}
	return nil
}

var validLogLevel = regexp.MustCompile(`^(emerg|alert|crit|err|warning|notice|info|debug)$`)

func validateLogLevel(s string) error {
	if s == "" {
		return nil
	}
	if !validLogLevel.MatchString(s) {
		return fmt.Errorf("invalid log-level: %s", s)
	}
	return nil
}
