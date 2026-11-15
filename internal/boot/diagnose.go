package boot

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// efiArchTokens maps Go architectures to the EFI architecture names used in
// systemd-boot binary file names and the BLS "architecture" key.
var efiArchTokens = map[string]string{
	"amd64":   "x64",
	"arm64":   "aa64",
	"386":     "ia32",
	"riscv64": "riscv64",
	"loong64": "loongarch64",
}

// LocalArchToken returns the EFI architecture token for this machine, or ""
// if the architecture is unknown to systemd-boot's vocabulary.
func LocalArchToken() string {
	return efiArchTokens[runtime.GOARCH]
}

// ReadMachineID returns the trimmed contents of /etc/machine-id, or "" when
// the file is missing or unreadable (e.g. in an installer environment).
func ReadMachineID() string {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// efiMagicPrefix is the start of the ".sdmagic" PE section systemd-boot
// embeds: "#### LoaderInfo: systemd-boot <version> ####". bootctl compares
// installed and packaged loader binaries the same way.
const efiMagicPrefix = "#### LoaderInfo: systemd-boot "

// loaderVersionFromEFI reads an EFI binary and extracts the embedded
// systemd-boot version string, or "" if the binary is not a systemd-boot
// image (or cannot be read). Files are capped at 16 MiB.
func loaderVersionFromEFI(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 16<<20 {
		return ""
	}
	i := bytes.Index(data, []byte(efiMagicPrefix))
	if i < 0 {
		return ""
	}
	rest := data[i+len(efiMagicPrefix):]
	j := bytes.Index(rest, []byte(" ####"))
	if j < 0 {
		return ""
	}
	return string(rest[:j])
}

// statFold returns the directory entry in dir whose name equals name
// case-insensitively, or "". FAT32 (the ESP filesystem) is case-insensitive,
// while test directories and case-sensitive mounts may store either form, so
// all existence checks go through this.
func statFold(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// LoaderStatus summarizes what actually sits on the boot partition: where the
// systemd-boot binaries are, which versions, and whether they are in sync
// with the packaged ones. It backs the diagnostics overlay ("d").
type LoaderStatus struct {
	// BootDir is the partition root the loader directory was found in
	// (the value of -esp or the auto-discovered candidate).
	BootDir string

	// XBOOTLDR is true when a separate ESP (containing EFI/) was found
	// alongside BootDir, i.e. BootDir is an Extended Boot Loader partition.
	XBOOTLDR bool
	ESPDir   string

	LoaderPath    string // e.g. /EFI/systemd/systemd-bootx64.efi (ESP-relative)
	LoaderVersion string

	FallbackPath     string // e.g. /EFI/BOOT/BOOTX64.EFI (ESP-relative)
	FallbackIsLoader bool   // fallback binary is systemd-boot, not e.g. Microsoft's copy

	PackagePath    string // /usr/lib/systemd/boot/efi/… (empty on non-systemd installs)
	PackageVersion string

	// Problems lists diagnostic messages; empty means all good.
	Problems []string
}

// InspectLoader examines bootDir (the directory that holds loader/) for the
// systemd-boot binaries and compares them with the packaged copies.
func InspectLoader(bootDir string) LoaderStatus {
	st := LoaderStatus{BootDir: bootDir}

	// A separate ESP: any standard candidate other than bootDir that has an
	// EFI/ directory. (bootctl/UEFI only ever reads one ESP per disk.)
	for _, cand := range espCandidates {
		if cand == bootDir {
			continue
		}
		if fi, err := os.Stat(filepath.Join(cand, "EFI")); err == nil && fi.IsDir() {
			st.XBOOTLDR = true
			st.ESPDir = cand
			break
		}
	}

	efiDir := bootDir
	if st.XBOOTLDR {
		efiDir = st.ESPDir // the loader binaries themselves live on the ESP
	}

	token := LocalArchToken()

	if p := statFold(filepath.Join(efiDir, "EFI", "systemd"), "systemd-boot"+token+".efi"); p != "" {
		st.LoaderPath = "/EFI/systemd/" + filepath.Base(p)
		st.LoaderVersion = loaderVersionFromEFI(p)
	}

	fallbackName := "BOOT" + strings.ToUpper(token) + ".EFI"
	if p := statFold(filepath.Join(efiDir, "EFI", "BOOT"), fallbackName); p != "" {
		st.FallbackPath = "/EFI/BOOT/" + filepath.Base(p)
		st.FallbackIsLoader = loaderVersionFromEFI(p) != ""
	}

	st.PackagePath = filepath.Join("/usr/lib/systemd/boot/efi", "systemd-boot"+token+".efi")
	if _, err := os.Stat(st.PackagePath); err != nil {
		st.PackagePath = ""
	} else {
		st.PackageVersion = loaderVersionFromEFI(st.PackagePath)
	}

	// Diagnostic messages, ordered roughly by severity.
	if st.LoaderPath == "" && st.FallbackPath == "" {
		st.Problems = append(st.Problems,
			"No systemd-boot binary found on the ESP. Run `sudo bootctl install`.")
	} else if st.LoaderPath == "" {
		st.Problems = append(st.Problems,
			"systemd-boot is only present at the fallback path ("+st.FallbackPath+
				"). Firmware boot-order resets or Windows updates can wipe it; run `sudo bootctl install`.")
	}
	if st.FallbackPath == "" && st.LoaderPath != "" {
		st.Problems = append(st.Problems,
			"No fallback \\EFI\\BOOT entry — if Windows Update re-registers itself as the default OS, "+
				"recovery may need firmware setup. `sudo bootctl install` restores it.")
	} else if st.FallbackPath != "" && !st.FallbackIsLoader && st.LoaderPath != "" {
		st.Problems = append(st.Problems,
			st.FallbackPath+" is not a systemd-boot binary (overwritten, e.g. by a Windows update). "+
				"systemd-boot still boots via the firmware entry, but `sudo bootctl install` restores the fallback.")
	}
	if st.LoaderVersion != "" && st.PackageVersion != "" && st.LoaderVersion != st.PackageVersion {
		st.Problems = append(st.Problems,
			"ESP loader is systemd-boot "+st.LoaderVersion+", the installed package ships "+
				st.PackageVersion+". Run `sudo bootctl update`.")
	}

	return st
}

// MajorVersion returns the systemd major version of the boot loader binary
// found on the ESP (0 when unknown), e.g. "255.4-1ubuntu8.15" → 255.
func (st LoaderStatus) MajorVersion() int {
	return parseMajorVersion(st.LoaderVersion)
}

// parseMajorVersion extracts the leading integer from a systemd version
// string ("259.1", "255.4-1ubuntu8", "258-rc3"). Returns 0 when unparseable.
func parseMajorVersion(v string) int {
	n := 0
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// BootHints returns environment notes worth surfacing in diagnostics, e.g.
// the XBOOTLDR/fast-boot caveat that makes perfectly valid entries vanish on
// some firmware.
func (st LoaderStatus) BootHints() []string {
	var hints []string
	if st.XBOOTLDR {
		hints = append(hints,
			"Entries live on the XBOOTLDR partition ("+st.BootDir+"), the ESP is "+st.ESPDir+
				". Some firmware skips non-ESP partitions when 'fast boot' is enabled: if entries "+
				"do not appear in the boot menu, disable fast boot in firmware setup.")
	}
	hints = append(hints,
		"systemd-boot can only load files from the partition its entries live on. "+
			"Entries for another OS (e.g. Windows on a different disk) must reference files "+
			"copied onto this partition, or rely on auto-detection via auto-entries.")
	return hints
}
