package boot

// Report is a snapshot of the boot configuration and the problems found in
// it. It is the shared data source for the in-app diagnostics overlay, the
// CLI (`sdbctl status` / `sdbctl check`) and `sdbctl mcp`, so all three agree
// on what "a problem" means. The Report deliberately carries no formatting:
// rendering lives in internal/cli.
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
		LoaderMajor:  r.Loader.MajorVersion(),
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
