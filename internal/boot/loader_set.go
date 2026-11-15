package boot

import (
	"fmt"
	"strings"
)

// LoaderKeys is the set of loader.conf options that can be assigned by name,
// either through the TUI or `sdbctl set`.
var LoaderKeys = append([]string(nil), loaderKeyOrder...)

// SetKey assigns a known loader.conf key, validating the value the way
// systemd-boot would. An empty value clears the key, which removes its line
// on the next write. Unknown keys are refused rather than guessed at: silently
// writing a key the boot loader does not read is worse than refusing.
func (c *LoaderConfig) SetKey(key, value string) error {
	if !editableLoaderKeys[key] {
		return fmt.Errorf("unknown loader.conf key %q (known keys: %s)",
			key, strings.Join(LoaderKeys, ", "))
	}

	setBool := func(dst **bool) error {
		if value == "" {
			*dst = nil
			return nil
		}
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %q (expected yes or no)", key, value)
		}
		*dst = b
		return nil
	}
	setEnum := func(dst *string, allowed ...string) error {
		if value == "" {
			*dst = ""
			return nil
		}
		for _, a := range allowed {
			if value == a {
				*dst = value
				return nil
			}
		}
		return fmt.Errorf("invalid value for %s: %q (expected one of: %s)",
			key, value, strings.Join(allowed, ", "))
	}

	switch key {
	case "timeout":
		if value != "" {
			if _, err := parseTimeout(value); err != nil {
				return fmt.Errorf("invalid timeout: %q (expected seconds, menu-force, menu-hidden or menu-disabled)", value)
			}
		}
		c.Timeout = value
		return nil
	case "default":
		if err := validateEntryPattern(value); err != nil {
			return fmt.Errorf("invalid default: %w", err)
		}
		c.Default = value
		return nil
	case "preferred":
		if err := validateEntryPattern(value); err != nil {
			return fmt.Errorf("invalid preferred: %w", err)
		}
		c.Preferred = value
		return nil
	case "console-mode":
		if err := validateConsoleMode(value); err != nil {
			return err
		}
		c.ConsoleMode = value
		return nil
	case "log-level":
		if err := validateLogLevel(value); err != nil {
			return err
		}
		c.LogLevel = value
		return nil
	case "reboot-on-error":
		return setEnum(&c.RebootOnError, "yes", "no", "auto")
	case "secure-boot-enroll":
		return setEnum(&c.SecureBootEnroll, "if-safe", "manual", "off", "force")
	case "secure-boot-enroll-action":
		return setEnum(&c.SecureBootEnrollAction, "reboot", "shutdown")
	case "secure-boot-enroll-timeout-sec":
		if value != "" && value != "hidden" && !allDigits(value) {
			return fmt.Errorf("invalid value for %s: %q (expected seconds or \"hidden\")", key, value)
		}
		c.SecureBootEnrollTimeoutSec = value
	case "editor":
		return setBool(&c.Editor)
	case "auto-entries":
		return setBool(&c.AutoEntries)
	case "auto-firmware":
		return setBool(&c.AutoFirmware)
	case "auto-reboot":
		return setBool(&c.AutoReboot)
	case "auto-poweroff":
		return setBool(&c.AutoPowerOff)
	case "beep":
		return setBool(&c.Beep)
	case "reboot-for-bitlocker":
		return setBool(&c.RebootForBitlocker)
	}
	return fmt.Errorf("loader.conf key %q is not assignable", key)
}

// validateEntryPattern checks a default=/preferred= value: an entry
// identifier, a glob, or the @saved pseudo-entry.
func validateEntryPattern(value string) error {
	if value == "" {
		return nil
	}
	if DefaultSpecials[value] {
		return nil
	}
	if strings.ContainsAny(value, " \t\n\r") {
		return fmt.Errorf("must be an entry identifier or glob without whitespace")
	}
	return nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
