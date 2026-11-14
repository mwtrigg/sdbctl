package boot

import (
	"fmt"
	"sort"
	"strings"
)

// Report is a snapshot of the boot configuration and the problems found in
// it. It backs both the in-app diagnostics overlay and the non-interactive
// `sdboot-tui --check` mode, so both report the same things.
type Report struct {
	BootDir   string
	ESPDir    string
	XBOOTLDR  bool
	MachineID string
	ArchToken string

	Loader     LoaderStatus
	LoaderConf *LoaderConfig

	Entries     []*BootEntry
	EntryIssues map[string][]string
	DefaultWarn string

	OneShot    string
	LastBooted string

	Backups []Backup
}

// BuildReport inspects a boot directory and collects everything the tool
// knows how to check.
func BuildReport(bootDir string) (*Report, error) {
	r := &Report{
		BootDir:     bootDir,
		MachineID:   ReadMachineID(),
		ArchToken:   LocalArchToken(),
		EntryIssues: map[string][]string{},
	}

	r.Loader = InspectLoader(bootDir)
	r.ESPDir = r.Loader.ESPDir
	r.XBOOTLDR = r.Loader.XBOOTLDR

	if cfg, err := ParseLoaderConf(FindLoaderConf(bootDir)); err == nil {
		r.LoaderConf = cfg
	}

	entries, err := ParseAllEntries(bootDir)
	if err != nil {
		return nil, err
	}
	r.Entries = entries

	ctx := ValidationContext{
		ESPPath:      bootDir,
		MachineID:    r.MachineID,
		Architecture: r.ArchToken,
	}
	for _, e := range entries {
		if e.IsUKI {
			continue
		}
		if verr := ValidateEntry(e, ctx); verr != nil {
			r.EntryIssues[e.ID] = verr.Issues
		}
	}

	if r.LoaderConf != nil {
		r.DefaultWarn = CheckDefaultEntry(r.LoaderConf, entries)
	}

	if OneShotAvailable() {
		if id, err := GetOneShot(); err == nil && id != "" {
			r.OneShot = StripEntrySuffix(id)
		}
		if id, err := GetLastBooted(); err == nil && id != "" {
			r.LastBooted = StripEntrySuffix(id)
		}
	}

	if backups, err := ListBackups(bootDir); err == nil {
		r.Backups = backups
	}

	return r, nil
}

// IssueCount returns the number of problems found: loader-level problems,
// loader.conf issues, a broken default pattern and per-entry issues.
func (r *Report) IssueCount() int {
	n := len(r.Loader.Problems)
	if r.LoaderConf != nil {
		n += len(r.LoaderConf.Issues)
	}
	if r.DefaultWarn != "" {
		n++
	}
	for _, issues := range r.EntryIssues {
		n += len(issues)
	}
	return n
}

// OK reports whether everything checks out.
func (r *Report) OK() bool {
	return r.IssueCount() == 0
}

// Text renders the report as plain text (no ANSI), suitable for a terminal
// or a CI log.
func (r *Report) Text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "sdboot-tui diagnostics\n")
	fmt.Fprintf(&b, "  boot directory : %s\n", r.BootDir)
	if r.XBOOTLDR {
		fmt.Fprintf(&b, "  ESP           : %s (XBOOTLDR layout)\n", r.ESPDir)
	}
	fmt.Fprintf(&b, "  machine-id    : %s\n", orDash(r.MachineID))
	fmt.Fprintf(&b, "  architecture  : %s\n", orDash(r.ArchToken))
	if r.Loader.LoaderPath != "" {
		fmt.Fprintf(&b, "  loader        : %s (systemd-boot %s)\n", r.Loader.LoaderPath, r.Loader.LoaderVersion)
	} else {
		fmt.Fprintf(&b, "  loader        : not found\n")
	}
	if r.Loader.FallbackPath != "" {
		fmt.Fprintf(&b, "  fallback      : %s\n", r.Loader.FallbackPath)
	}
	if r.Loader.PackageVersion != "" {
		fmt.Fprintf(&b, "  package       : systemd-boot %s\n", r.Loader.PackageVersion)
	}
	if r.OneShot != "" {
		fmt.Fprintf(&b, "  one-shot      : %s\n", r.OneShot)
	}
	if r.LastBooted != "" {
		fmt.Fprintf(&b, "  last booted   : %s\n", r.LastBooted)
	}

	if r.LoaderConf != nil {
		fmt.Fprintf(&b, "\nloader.conf\n")
		fmt.Fprintf(&b, "  default       : %s\n", orDash(r.LoaderConf.Default))
		fmt.Fprintf(&b, "  timeout       : %s\n", orDash(r.LoaderConf.Timeout))
		fmt.Fprintf(&b, "  entries       : %d\n", len(r.Entries))
		for _, kv := range r.LoaderConf.Extras {
			fmt.Fprintf(&b, "  (kept)        : %s %s\n", kv.Key, kv.Value)
		}
		for _, issue := range r.LoaderConf.Issues {
			fmt.Fprintf(&b, "  ! %s\n", issue)
		}
	}

	if r.DefaultWarn != "" {
		fmt.Fprintf(&b, "\n! %s\n", r.DefaultWarn)
	}

	if len(r.EntryIssues) > 0 {
		fmt.Fprintf(&b, "\nentries\n")
		ids := make([]string, 0, len(r.EntryIssues))
		for id := range r.EntryIssues {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(&b, "  %s.conf\n", id)
			for _, issue := range r.EntryIssues[id] {
				fmt.Fprintf(&b, "    ! %s\n", issue)
			}
		}
	}

	if len(r.Loader.Problems) > 0 {
		fmt.Fprintf(&b, "\nboot loader\n")
		for _, p := range r.Loader.Problems {
			fmt.Fprintf(&b, "  ! %s\n", p)
		}
	}

	if len(r.Loader.BootHints()) > 0 {
		fmt.Fprintf(&b, "\nnotes\n")
		for _, h := range r.Loader.BootHints() {
			fmt.Fprintf(&b, "  · %s\n", h)
		}
	}

	if len(r.Backups) > 0 {
		fmt.Fprintf(&b, "\nbackups\n")
		for i, bk := range r.Backups {
			if i >= 5 {
				fmt.Fprintf(&b, "  … %d older\n", len(r.Backups)-5)
				break
			}
			fmt.Fprintf(&b, "  %s\n", bk.Describe())
		}
	}

	n := r.IssueCount()
	if n == 0 {
		fmt.Fprintf(&b, "\nno problems found\n")
	} else {
		fmt.Fprintf(&b, "\n%d problem(s) found\n", n)
	}

	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
