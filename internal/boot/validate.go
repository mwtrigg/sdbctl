package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ValidationError struct {
	EntryID string
	Issues  []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %v", e.EntryID, e.Issues)
}

// ValidationContext carries host facts needed to judge whether an entry would
// actually be shown and boot by systemd-boot on this machine. Empty optional
// fields simply skip the corresponding check.
type ValidationContext struct {
	ESPPath      string
	MachineID    string // contents of /etc/machine-id, "" when unknown
	Architecture string // EFI architecture token of this machine (x64, aa64, …)
	// LoaderMajor is the major version of the systemd-boot binary found on
	// the ESP (0 when unknown). Entry semantics differ between versions, so
	// warnings have to as well: verified against the systemd sources and an
	// OVMF boot,
	//   - "uki"/"uki-url" keys are only understood from v258;
	//   - an entry with two image keys is dropped from v258, while v255/v257
	//     let whichever key came last win;
	//   - "extra"/"profile" are only understood from v259.
	LoaderMajor int
}

// Version thresholds where type #1 entry semantics changed. Each one was
// read out of the systemd sources for that tag (v255, v257, v258, v259, v260,
// v261, main) and the conflict rule was confirmed by booting v255 under OVMF
// (see tools/bootoracle.py):
//
//	v255/v257  linux, initrd, efi, options, devicetree, architecture,
//	           title, version, machine-id, sort-key. No conflict check:
//	           whichever image key came last wins.
//	v258       + uki, uki-url; entries with two image keys are now dropped.
//	v259       + profile.
//	v261       + extra.
const (
	loaderMinUKI     = 258 // uki + uki-url keys, and dropping conflicting entries
	loaderMinProfile = 259 // profile key
	loaderMinExtra   = 261 // extra key
)

// loaderKnown reports whether the boot loader version is known.
func (ctx ValidationContext) loaderKnown() bool { return ctx.LoaderMajor > 0 }

// loaderAtLeast reports whether the installed boot loader is new enough.
// An unknown version is assumed to be current, so no warning is invented.
func (ctx ValidationContext) loaderAtLeast(v int) bool {
	return ctx.LoaderMajor == 0 || ctx.LoaderMajor >= v
}

// ValidateEntry checks a Type #1 entry the way systemd-boot would: entries
// with a missing image key, conflicting image keys, or missing referenced
// files are silently dropped from the boot menu. Warnings that do not hide
// the entry (machine-id mismatch, missing root=) are reported as issues too,
// since they are common causes of broken or unexpected boots.
func ValidateEntry(entry *BootEntry, ctx ValidationContext) *ValidationError {
	var issues []string

	// Exactly one of linux/efi/uki/uki-url must be present. What happens
	// otherwise depends on the boot loader version: v258+ marks the entry
	// bad and drops it, v255/v257 have no conflict check at all and simply
	// let whichever image key came last win.
	switch keys := entry.ImageKeys(); len(keys) {
	case 0:
		issues = append(issues, "no linux/efi/uki/uki-url key — systemd-boot ignores this entry")
	case 1:
	default:
		switch {
		case !ctx.loaderKnown():
			issues = append(issues, fmt.Sprintf(
				"conflicting image keys (%s) — systemd-boot 258 and newer drop this entry, "+
					"older versions boot whichever of them came last", strings.Join(keys, ", ")))
		case ctx.LoaderMajor >= loaderMinUKI:
			issues = append(issues, fmt.Sprintf(
				"conflicting image keys (%s) — systemd-boot %d drops this entry",
				strings.Join(keys, ", "), ctx.LoaderMajor))
		default:
			issues = append(issues, fmt.Sprintf(
				"conflicting image keys (%s) — systemd-boot %d boots the last of them, "+
					"not necessarily the one you meant (258 and newer drop the entry instead)",
				strings.Join(keys, ", "), ctx.LoaderMajor))
		}
	}

	// Keys that only exist in newer boot loaders: an entry that relies on
	// them is invisible or silently incomplete on older versions.
	if (entry.UKI != "" || entry.UKIUrl != "") && ctx.loaderKnown() && ctx.LoaderMajor < loaderMinUKI {
		issues = append(issues, fmt.Sprintf(
			"uki/uki-url is unknown to systemd-boot %d (needs %d) — entry hidden from boot menu",
			ctx.LoaderMajor, loaderMinUKI))
	}
	if entry.Profile != "" && ctx.loaderKnown() && ctx.LoaderMajor < loaderMinProfile {
		issues = append(issues, fmt.Sprintf("profile is ignored by systemd-boot %d (needs %d)",
			ctx.LoaderMajor, loaderMinProfile))
	}
	if len(entry.Extras) > 0 && ctx.loaderKnown() && ctx.LoaderMajor < loaderMinExtra {
		issues = append(issues, fmt.Sprintf(
			"extra files are ignored by systemd-boot %d (needs %d) — credentials/extimages will not be passed",
			ctx.LoaderMajor, loaderMinExtra))
	}

	// Referenced files must exist on the boot partition: systemd-boot hides
	// entries whose kernel/initrd/efi file cannot be opened. This is the most
	// common cause of stale entries from removed kernels or mistyped paths.
	// `soft` issues are worded without the "hidden" claim, for keys whose
	// handling differs or is absent depending on the boot loader version.
	checkFile := func(field, p string, soft bool) {
		if p == "" {
			return
		}
		full := filepath.Join(ctx.ESPPath, filepath.FromSlash(p))
		if _, err := os.Stat(full); err != nil {
			if os.IsNotExist(err) {
				if soft {
					issues = append(issues, fmt.Sprintf("%s file not found: %s", field, p))
				} else {
					issues = append(issues, fmt.Sprintf("%s file not found: %s (entry hidden from boot menu)", field, p))
				}
			} else {
				issues = append(issues, fmt.Sprintf("cannot stat %s file: %v", field, err))
			}
		}
	}
	checkFile("linux", entry.Linux, false)
	checkFile("efi", entry.Efi, false)
	checkFile("uki", entry.UKI, false)
	for _, initrd := range entry.Initrds {
		checkFile("initrd", initrd, false)
	}
	checkFile("devicetree", entry.Devicetree, false)
	for _, dto := range entry.DTOOverlays {
		// systemd-boot has not parsed devicetree-overlay in any release
		// checked (v255 through main), so a missing overlay file does not
		// hide the entry.
		checkFile("devicetree-overlay", dto, true)
	}
	for _, extra := range entry.Extras {
		// Only versions that understand the key open the file at all.
		checkFile("extra", extra, !ctx.loaderAtLeast(loaderMinExtra))
	}

	// An entry with a non-matching architecture token is hidden.
	if entry.Architecture != "" && ctx.Architecture != "" &&
		!strings.EqualFold(entry.Architecture, ctx.Architecture) {
		issues = append(issues, fmt.Sprintf(
			"architecture %q does not match this system (%s) — entry hidden from boot menu",
			entry.Architecture, ctx.Architecture))
	}

	// machine-id is used for sorting and title disambiguation; a foreign
	// machine-id usually means the entry was copied from another install.
	if entry.MachineID != "" && ctx.MachineID != "" && entry.MachineID != ctx.MachineID {
		issues = append(issues, fmt.Sprintf(
			"machine-id %s differs from this system — entry belongs to another OS installation",
			entry.MachineID))
	}

	// Kernel command line sanity. systemd falls back to discovering the root
	// partition via the GPT "root" partition type when no root= is given; if
	// the disk layout does not follow the Discoverable Partitions Spec the
	// boot hangs in the initramfs. This has bricked updates on Arch-family
	// distros using manual entries + dracut, so warn explicitly.
	if entry.Linux != "" && !hasOption(entry, "root") {
		issues = append(issues,
			"options have no root= — boot relies on systemd gpt-auto-discovery; "+
				"add root=UUID=… unless the root partition is Discoverable Partitions Spec compliant")
	}

	if len(issues) > 0 {
		return &ValidationError{
			EntryID: entry.ID,
			Issues:  issues,
		}
	}

	return nil
}

// hasOption reports whether any options line contains a parameter "name="
// (comparison is case-sensitive, kernel parameters are).
func hasOption(entry *BootEntry, name string) bool {
	prefix := name + "="
	for _, line := range entry.Options {
		for _, opt := range strings.Fields(line) {
			if strings.HasPrefix(opt, prefix) {
				return true
			}
		}
	}
	return false
}

func ValidateEntries(entries []*BootEntry, ctx ValidationContext) []error {
	var errors []error

	for _, entry := range entries {
		if err := ValidateEntry(entry, ctx); err != nil {
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

	if cfg.SecureBootEnroll != "" {
		switch cfg.SecureBootEnroll {
		case "if-safe", "manual", "off", "force":
		default:
			return fmt.Errorf("invalid secure-boot-enroll: %s (expected if-safe, manual, off, or force)", cfg.SecureBootEnroll)
		}
	}

	if cfg.SecureBootEnrollAction != "" {
		switch cfg.SecureBootEnrollAction {
		case "reboot", "shutdown":
		default:
			return fmt.Errorf("invalid secure-boot-enroll-action: %s (expected reboot or shutdown)", cfg.SecureBootEnrollAction)
		}
	}

	return nil
}

// DefaultSpecials are loader.conf default values handled by the boot loader
// itself rather than by matching against entry IDs.
var DefaultSpecials = map[string]bool{
	"@saved": true,
}

// MatchEntryID reports whether a loader.conf default=/preferred= glob pattern
// selects the given entry. systemd-boot compares the pattern against the full
// entry ID — the file name including the .conf/.efi suffix — case
// insensitively.
func MatchEntryID(pattern, fullID string) bool {
	if pattern == "" {
		return false
	}
	p, id := strings.ToLower(pattern), strings.ToLower(fullID)
	if !strings.ContainsAny(p, "*?[") {
		return p == id
	}
	ok, err := filepath.Match(p, id)
	return err == nil && ok
}

// CheckDefaultEntry verifies that the loader.conf default pattern selects at
// least one entry. It returns a warning message, or "" when everything looks
// fine. Patterns referencing no existing file are the classic "default boot
// entry is broken" case that bootctl(1) prints a WARNING for.
func CheckDefaultEntry(cfg *LoaderConfig, entries []*BootEntry) string {
	if cfg == nil || cfg.Default == "" || DefaultSpecials[cfg.Default] {
		return ""
	}
	for _, e := range entries {
		if e.IsUKI {
			continue // UKI auto-selection has its own ordering rules
		}
		if MatchEntryID(cfg.Default, e.FullID()) {
			return ""
		}
	}
	msg := fmt.Sprintf("default %q matches no boot entry — systemd-boot falls back to the newest entry", cfg.Default)
	if !strings.Contains(cfg.Default, ".") && !strings.ContainsAny(cfg.Default, "*?[") {
		msg += ` (entry IDs include the .conf suffix, e.g. "` + cfg.Default + `.conf")`
	}
	return msg
}
