package boot

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// LoaderConfig mirrors loader.conf(5). Field coverage follows the current
// systemd documentation (systemd ≥ 259). The parser is deliberately lenient,
// mirroring systemd-boot itself: unparseable or unknown lines are recorded in
// Issues and preserved verbatim on round-trip writes, never fatal. That way a
// distro-provided loader.conf carrying options this tool does not know about
// (e.g. "keyfile" on older systemd, or future additions) is still displayed
// and edited safely.
type LoaderConfig struct {
	Timeout                    string
	Default                    string
	Preferred                  string
	Editor                     *bool
	ConsoleMode                string
	AutoEntries                *bool
	AutoFirmware               *bool
	AutoReboot                 *bool
	AutoPowerOff               *bool
	Beep                       *bool
	RebootForBitlocker         *bool
	RebootOnError              string
	SecureBootEnroll           string
	SecureBootEnrollAction     string
	SecureBootEnrollTimeoutSec string
	LogLevel                   string

	// Extras holds known-but-not-editable or unknown key/value pairs in file
	// order, purely for display; raw lines keep them intact on disk.
	Extras []ExtrasKV

	RawLines []string
	Issues   []string
}

// ExtrasKV is a loader.conf line whose key this tool does not edit.
type ExtrasKV struct {
	Key   string
	Value string
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

	content := strings.TrimRight(string(data), "\n")
	for lineNum, raw := range strings.Split(content, "\n") {
		lineNum++
		cfg.RawLines = append(cfg.RawLines, raw)

		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first run of whitespace (space or tab).
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			cfg.Issues = append(cfg.Issues, fmt.Sprintf("line %d: malformed, ignored by systemd-boot: %s", lineNum, line))
			continue
		}
		key := line[:idx]
		value := strings.TrimSpace(line[idx:])

		issue := cfg.apply(key, value)
		if issue != "" {
			cfg.Issues = append(cfg.Issues, fmt.Sprintf("line %d: %s", lineNum, issue))
		}
	}

	return cfg, nil
}

// apply sets the field for key, returning an issue message when the value is
// invalid (the value is then left at its default) or "" on success. Unknown
// keys become Extras entries and return "".
func (c *LoaderConfig) apply(key, value string) string {
	parseB := func(field **bool) string {
		b, err := parseBool(value)
		if err != nil {
			return fmt.Sprintf("invalid boolean for %s: %q (systemd-boot ignores this line)", key, value)
		}
		*field = b
		return ""
	}

	switch key {
	case "timeout":
		c.Timeout = value
	case "default":
		c.Default = value
	case "preferred":
		c.Preferred = value
	case "editor":
		return parseB(&c.Editor)
	case "console-mode":
		c.ConsoleMode = value
	case "auto-entries":
		return parseB(&c.AutoEntries)
	case "auto-firmware":
		return parseB(&c.AutoFirmware)
	case "auto-reboot":
		return parseB(&c.AutoReboot)
	case "auto-poweroff":
		return parseB(&c.AutoPowerOff)
	case "beep":
		return parseB(&c.Beep)
	case "reboot-for-bitlocker":
		return parseB(&c.RebootForBitlocker)
	case "reboot-on-error":
		c.RebootOnError = value
	case "secure-boot-enroll":
		c.SecureBootEnroll = value
	case "secure-boot-enroll-action":
		c.SecureBootEnrollAction = value
	case "secure-boot-enroll-timeout-sec":
		c.SecureBootEnrollTimeoutSec = value
	case "log-level":
		c.LogLevel = value
	default:
		c.Extras = append(c.Extras, ExtrasKV{Key: key, Value: value})
	}
	return ""
}

// editableKeys are the loader.conf keys represented by struct fields; they
// round-trip through editing. Anything else is preserved verbatim.
var editableLoaderKeys = map[string]bool{
	"timeout": true, "default": true, "preferred": true, "editor": true,
	"console-mode": true, "auto-entries": true, "auto-firmware": true,
	"auto-reboot": true, "auto-poweroff": true, "beep": true,
	"reboot-for-bitlocker": true, "reboot-on-error": true,
	"secure-boot-enroll": true, "secure-boot-enroll-action": true,
	"secure-boot-enroll-timeout-sec": true, "log-level": true,
}

// loaderKeyOrder is the canonical write order (mirrors loader.conf(5)).
var loaderKeyOrder = []string{
	"timeout", "default", "preferred", "editor", "console-mode",
	"auto-entries", "auto-firmware", "auto-reboot", "auto-poweroff", "beep",
	"reboot-for-bitlocker", "reboot-on-error",
	"secure-boot-enroll", "secure-boot-enroll-action", "secure-boot-enroll-timeout-sec",
	"log-level",
}

func (c *LoaderConfig) Write(path string) error {
	var data []byte
	if len(c.RawLines) > 0 {
		data = c.writeWithRawLines()
	} else {
		data = c.writeStandard()
	}
	return writeFileAtomic(path, data, 0644)
}

func (c *LoaderConfig) writeStandard() []byte {
	var lines []string
	for _, kv := range c.valueMap() {
		lines = append(lines, kv.Key+" "+kv.Value)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (c *LoaderConfig) writeWithRawLines() []byte {
	vals := c.valueMap()
	valIdx := make(map[string]int, len(vals))
	for i, kv := range vals {
		valIdx[kv.Key] = i
	}
	used := make(map[string]int)
	var lines []string

	for _, raw := range c.RawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, raw)
			continue
		}
		idx := strings.IndexAny(trimmed, " \t")
		if idx < 0 {
			// Malformed line: keep verbatim (systemd-boot ignores it too).
			lines = append(lines, raw)
			continue
		}
		key := trimmed[:idx]
		if !editableLoaderKeys[key] {
			// Unknown key: keep verbatim.
			lines = append(lines, raw)
			continue
		}

		i, has := valIdx[key]
		n := used[key]
		used[key] = n + 1
		// vals[key] lists at most one value (loader.conf keys are
		// single-valued); a cleared field is absent from valIdx, and the
		// original line is then dropped.
		if has && n == 0 {
			lines = append(lines, key+" "+vals[i].Value)
		}
	}

	// Append values for fields that had no line in the original file.
	for _, kv := range vals {
		if used[kv.Key] == 0 {
			lines = append(lines, kv.Key+" "+kv.Value)
		}
	}

	return []byte(strings.Join(lines, "\n") + "\n")
}

// valueMap returns non-empty current values in canonical key order. Bool
// fields are only emitted when explicitly set.
func (c *LoaderConfig) valueMap() []ExtrasKV {
	str := func(key, v string) (ExtrasKV, bool) {
		if v == "" {
			return ExtrasKV{}, false
		}
		return ExtrasKV{key, v}, true
	}
	boo := func(key string, v *bool) (ExtrasKV, bool) {
		if v == nil {
			return ExtrasKV{}, false
		}
		return ExtrasKV{key, formatBool(*v)}, true
	}

	var out []ExtrasKV
	add := func(kv ExtrasKV, ok bool) {
		if ok {
			out = append(out, kv)
		}
	}
	for _, key := range loaderKeyOrder {
		switch key {
		case "timeout":
			add(str(key, c.Timeout))
		case "default":
			add(str(key, c.Default))
		case "preferred":
			add(str(key, c.Preferred))
		case "editor":
			add(boo(key, c.Editor))
		case "console-mode":
			add(str(key, c.ConsoleMode))
		case "auto-entries":
			add(boo(key, c.AutoEntries))
		case "auto-firmware":
			add(boo(key, c.AutoFirmware))
		case "auto-reboot":
			add(boo(key, c.AutoReboot))
		case "auto-poweroff":
			add(boo(key, c.AutoPowerOff))
		case "beep":
			add(boo(key, c.Beep))
		case "reboot-for-bitlocker":
			add(boo(key, c.RebootForBitlocker))
		case "reboot-on-error":
			add(str(key, c.RebootOnError))
		case "secure-boot-enroll":
			add(str(key, c.SecureBootEnroll))
		case "secure-boot-enroll-action":
			add(str(key, c.SecureBootEnrollAction))
		case "secure-boot-enroll-timeout-sec":
			add(str(key, c.SecureBootEnrollTimeoutSec))
		case "log-level":
			add(str(key, c.LogLevel))
		}
	}
	return out
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
