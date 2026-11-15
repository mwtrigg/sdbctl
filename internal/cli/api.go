// Package cli implements sdbctl's command surface. It is deliberately split
// in two layers:
//
//   - api.go  — every operation, with typed, JSON-tagged results and no
//     printing. This is the contract shared by the CLI, `sdbctl mcp` and the
//     qemu oracle, so all three agree by construction.
//   - cli.go  — argument parsing, rendering and exit codes.
//
// Nothing in here touches a terminal, so the whole surface is testable with a
// temp directory and no root.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mwtrigg/sdbctl/internal/boot"
)

// Entry is one boot loader entry as the outside world sees it.
type Entry struct {
	ID         string   `json:"id"`
	FullID     string   `json:"fullId"`
	Title      string   `json:"title"`
	Version    string   `json:"version,omitempty"`
	ImageKeys  []string `json:"imageKeys,omitempty"`
	Linux      string   `json:"linux,omitempty"`
	Efi        string   `json:"efi,omitempty"`
	UKI        string   `json:"uki,omitempty"`
	Options    string   `json:"options,omitempty"`
	UkiImage   bool     `json:"ukiImage"`
	Default    bool     `json:"isDefault"`
	OneShot    bool     `json:"isOneShot"`
	LastBooted bool     `json:"isLastBooted"`
	Hidden     bool     `json:"hidden"`
	Issues     []string `json:"issues,omitempty"`
}

// Loader describes the boot loader binaries found on the partition.
type Loader struct {
	Path           string `json:"path,omitempty"`
	Version        string `json:"version,omitempty"`
	PackageVersion string `json:"packageVersion,omitempty"`
	Fallback       string `json:"fallback,omitempty"`
	FallbackOK     bool   `json:"fallbackIsLoader"`
}

// Snapshot is one backup taken before a change.
type Snapshot struct {
	ID    string   `json:"id"`
	Files []string `json:"files"`
}

// Status is the full picture: what is installed, what will boot, what is
// broken. Returned by both `sdbctl status` and `sdbctl check`.
type Status struct {
	BootDir        string     `json:"bootDir"`
	ESPDir         string     `json:"espDir,omitempty"`
	Xbootldr       bool       `json:"xbootldr"`
	Architecture   string     `json:"architecture,omitempty"`
	MachineID      string     `json:"machineId,omitempty"`
	Loader         Loader     `json:"loader"`
	Default        string     `json:"default,omitempty"`
	Timeout        string     `json:"timeout,omitempty"`
	OneShot        string     `json:"oneShot,omitempty"`
	LastBooted     string     `json:"lastBooted,omitempty"`
	Entries        []Entry    `json:"entries"`
	Problems       []string   `json:"problems,omitempty"`
	ConfigIssues   []string   `json:"configIssues,omitempty"`
	DefaultWarning string     `json:"defaultWarning,omitempty"`
	Hints          []string   `json:"hints,omitempty"`
	Backups        []Snapshot `json:"backups,omitempty"`
	HiddenEntries  int        `json:"hiddenEntries"`
	OK             bool       `json:"ok"`
}

// Result is what a mutating verb reports.
type Result struct {
	Changed bool   `json:"changed"`
	Summary string `json:"summary"`
	Backup  string `json:"backup,omitempty"`
	// Undo tells the user (or an agent) how to roll this back.
	Undo       string   `json:"undo,omitempty"`
	Entry      *Entry   `json:"entry,omitempty"`
	NextBoot   string   `json:"nextBoot,omitempty"`
	Rebooted   bool     `json:"rebooted,omitempty"`
	Removed    string   `json:"removed,omitempty"`
	Restored   string   `json:"restored,omitempty"`
	Planned    bool     `json:"planned,omitempty"`
	StatusLine string   `json:"statusLine,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// API runs operations against one boot directory.
type API struct {
	ESP string
	// DryRun reports what would change without touching anything.
	DryRun bool
	// AllowHidden permits acting on entries systemd-boot would hide
	// (--force), which is what you want when fixing a machine that will
	// not boot.
	AllowHidden bool
}

// Resolve finds the boot directory: the explicit flag, else the standard
// candidates, always validated so the verbs get a clear error instead of an
// empty listing.
func Resolve(espFlag string) (*API, error) {
	esp := espFlag
	if esp == "" {
		found, err := boot.FindESP()
		if err != nil {
			return nil, fmt.Errorf("no boot directory found: %w "+
				"(pass --esp /path/to/esp)", err)
		}
		esp = found
	}
	fi, err := os.Stat(esp)
	if err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", esp)
	}
	return &API{ESP: esp}, nil
}

// load reads the current state into a Status.
func (a *API) load() (*Status, error) {
	r, err := boot.BuildReport(a.ESP)
	if err != nil {
		return nil, err
	}

	st := &Status{
		BootDir:        r.BootDir,
		ESPDir:         r.ESPDir,
		Xbootldr:       r.XBOOTLDR,
		Architecture:   r.ArchToken,
		MachineID:      r.MachineID,
		OneShot:        r.OneShot,
		LastBooted:     r.LastBooted,
		Problems:       r.Loader.Problems,
		Hints:          r.Loader.BootHints(),
		DefaultWarning: r.DefaultWarn,
		Loader: Loader{
			Path:           r.Loader.LoaderPath,
			Version:        r.Loader.LoaderVersion,
			PackageVersion: r.Loader.PackageVersion,
			Fallback:       r.Loader.FallbackPath,
			FallbackOK:     r.Loader.FallbackIsLoader,
		},
	}
	if r.LoaderConf != nil {
		st.Default = r.LoaderConf.Default
		st.Timeout = r.LoaderConf.Timeout
		st.ConfigIssues = r.LoaderConf.Issues
	}

	for _, e := range r.Entries {
		issues := r.EntryIssues[e.ID]
		ent := Entry{
			ID:         e.ID,
			FullID:     e.FullID(),
			Title:      e.DisplayTitle(),
			Version:    e.Version,
			ImageKeys:  e.ImageKeys(),
			Linux:      e.Linux,
			Efi:        e.Efi,
			UKI:        e.UKI,
			Options:    strings.Join(e.Options, " "),
			UkiImage:   e.IsUKI,
			Hidden:     boot.EntryHidden(issues),
			Issues:     issues,
			OneShot:    r.OneShot != "" && r.OneShot == e.ID,
			LastBooted: r.LastBooted != "" && r.LastBooted == e.ID,
		}
		if r.LoaderConf != nil && r.LoaderConf.Default != "" {
			if boot.DefaultSpecials[r.LoaderConf.Default] {
				ent.Default = ent.LastBooted
			} else {
				ent.Default = boot.MatchEntryID(r.LoaderConf.Default, ent.FullID)
			}
		}
		if ent.Hidden {
			st.HiddenEntries++
		}
		st.Entries = append(st.Entries, ent)
	}
	if st.Entries == nil {
		st.Entries = []Entry{}
	}

	for _, b := range r.Backups {
		st.Backups = append(st.Backups, Snapshot{ID: b.ID, Files: b.Files})
	}
	st.OK = r.OK()
	return st, nil
}

// Status returns the full report.
func (a *API) Status() (*Status, error) { return a.load() }

// List returns just the entries.
func (a *API) List() ([]Entry, error) {
	st, err := a.load()
	if err != nil {
		return nil, err
	}
	return st.Entries, nil
}

// find locates one entry by id, full id, or glob, mirroring how systemd-boot
// itself resolves default=/one-shot values.
func (a *API) find(spec string) (*Entry, error) {
	entries, err := a.List()
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(spec)

	var exact []Entry
	for _, e := range entries {
		if strings.EqualFold(e.FullID, spec) || strings.EqualFold(e.ID, spec) {
			exact = append(exact, e)
		}
	}
	if len(exact) == 1 {
		return &exact[0], nil
	}
	if len(exact) > 1 {
		return nil, fmt.Errorf("%q is ambiguous", spec)
	}

	// No literal match: try it as a glob, the way loader.conf default= works.
	var globbed []Entry
	for _, e := range entries {
		if boot.MatchEntryID(lower, strings.ToLower(e.FullID)) {
			globbed = append(globbed, e)
		}
	}
	switch len(globbed) {
	case 1:
		return &globbed[0], nil
	case 0:
		return nil, fmt.Errorf("no boot entry matches %q", spec)
	default:
		names := make([]string, 0, len(globbed))
		for _, e := range globbed {
			names = append(names, e.FullID)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("%q matches %d entries (%s) — be more specific",
			spec, len(names), strings.Join(names, ", "))
	}
}

// mutate snapshots the affected files, then applies fn. On --dry-run nothing
// is touched and the result says so.
func (a *API) mutate(relPaths []string, summary string, fn func(*Result) error) (*Result, error) {
	res := &Result{Summary: summary}
	if a.DryRun {
		res.Planned = true
		res.Changed = false
		res.Undo = "sdbctl undo"
		return res, nil
	}

	backups, err := boot.CreateBackup(a.ESP, relPaths)
	if err != nil {
		return nil, fmt.Errorf("backup failed, nothing was changed: %w", err)
	}
	if backups != nil {
		res.Backup = backups.ID
		res.Undo = "sdbctl undo"
	}
	if err := fn(res); err != nil {
		return nil, err
	}
	res.Changed = true
	return res, nil
}

// Boot sets the entry to boot once on the next restart.
func (a *API) Boot(spec string, reboot bool) (*Result, error) {
	if !boot.OneShotAvailable() {
		return nil, fmt.Errorf("EFI variables are not available (%s): "+
			"boot from an EFI system, or bind-mount /sys into the chroot",
			"/sys/firmware/efi/efivars")
	}
	e, err := a.find(spec)
	if err != nil {
		return nil, err
	}
	if e.Hidden && !a.AllowHidden {
		return nil, fmt.Errorf("%s is hidden from the boot menu: %s — pass --force to boot it anyway",
			e.FullID, strings.Join(e.Issues, "; "))
	}

	res, err := a.mutate(nil, "one-shot set", func(r *Result) error {
		if err := boot.SetOneShot(e.FullID); err != nil {
			return fmt.Errorf("writing LoaderEntryOneShot: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if e.Hidden {
		res.Warnings = append(res.Warnings,
			e.FullID+" is hidden from the boot menu: "+strings.Join(e.Issues, "; "))
	}
	res.Entry = e
	res.NextBoot = e.Title

	if reboot && !a.DryRun {
		if err := rebootNow(); err != nil {
			return res, fmt.Errorf("one-shot was set, but rebooting failed: %w", err)
		}
		res.Rebooted = true
	}
	return res, nil
}

// SetDefault makes an entry (or @saved) the persistent default.
func (a *API) SetDefault(spec string) (*Result, error) {
	token := spec
	var e *Entry
	if !boot.DefaultSpecials[spec] {
		found, err := a.find(spec)
		if err != nil {
			return nil, err
		}
		e = found
		token = found.FullID
	}

	cfg, err := a.config()
	if err != nil {
		return nil, err
	}
	if err := cfg.SetKey("default", token); err != nil {
		return nil, err
	}

	res, err := a.mutate([]string{loaderRel}, "default set", func(r *Result) error {
		return a.writeConfig(cfg)
	})
	if err != nil {
		return nil, err
	}
	res.Entry = e
	res.StatusLine = "default " + token
	return res, nil
}

// SetKey assigns any loader.conf option, e.g. timeout or auto-entries.
func (a *API) SetKey(key, value string) (*Result, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, err
	}
	if err := cfg.SetKey(key, value); err != nil {
		return nil, err
	}
	res, err := a.mutate([]string{loaderRel}, "loader.conf updated", func(r *Result) error {
		return a.writeConfig(cfg)
	})
	if err != nil {
		return nil, err
	}
	if value == "" {
		res.StatusLine = key + " cleared"
	} else {
		res.StatusLine = key + " = " + value
	}
	return res, nil
}

// Remove deletes a Type #1 entry file. The snapshot makes it undoable.
func (a *API) Remove(spec string) (*Result, error) {
	e, err := a.find(spec)
	if err != nil {
		return nil, err
	}
	if e.UkiImage {
		return nil, fmt.Errorf("%s is a unified kernel image: remove it with your "+
			"package manager or kernel-install, not here", e.FullID)
	}
	rel := filepath.ToSlash(filepath.Join("loader", "entries", e.ID+".conf"))

	res, err := a.mutate([]string{rel}, "entry deleted", func(r *Result) error {
		return os.Remove(filepath.Join(a.ESP, filepath.FromSlash(rel)))
	})
	if err != nil {
		return nil, err
	}
	res.Removed = e.FullID
	res.Entry = e
	return res, nil
}

// Undo restores the most recent snapshot.
func (a *API) Undo() (*Result, error) {
	b, err := boot.LatestBackup(a.ESP)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("no snapshots in %s", boot.BackupRoot(a.ESP))
	}
	if a.DryRun {
		return &Result{Summary: "restore " + b.ID, Planned: true, Restored: b.ID}, nil
	}

	// Snapshot the current state first, so the undo is itself undoable.
	if _, err := boot.CreateBackup(a.ESP, b.Files); err != nil {
		return nil, fmt.Errorf("backup failed, nothing was changed: %w", err)
	}
	if err := boot.RestoreBackup(a.ESP, *b); err != nil {
		return nil, err
	}
	return &Result{
		Changed:    true,
		Summary:    "restored",
		Restored:   b.ID,
		StatusLine: strings.Join(b.Files, ", "),
		Undo:       "sdbctl undo",
	}, nil
}

// Snapshots lists available backups, newest first.
func (a *API) Snapshots() ([]Snapshot, error) {
	backups, err := boot.ListBackups(a.ESP)
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, 0, len(backups))
	for _, b := range backups {
		out = append(out, Snapshot{ID: b.ID, Files: b.Files})
	}
	return out, nil
}

// config reads loader.conf, or returns a fresh default configuration when the
// file does not exist yet (a freshly installed boot loader).
func (a *API) config() (*boot.LoaderConfig, error) {
	cfg, err := boot.ParseLoaderConf(filepath.Join(a.ESP, loaderRel))
	if err != nil {
		if os.IsNotExist(err) {
			return boot.NewLoaderConfig(), nil
		}
		return nil, err
	}
	if cfg == nil {
		return boot.NewLoaderConfig(), nil
	}
	return cfg, nil
}

const loaderRel = "loader/loader.conf"

// writeConfig persists loader.conf.
func (a *API) writeConfig(cfg *boot.LoaderConfig) error {
	return cfg.Write(filepath.Join(a.ESP, loaderRel))
}

func rebootNow() error {
	if path, err := exec.LookPath("systemctl"); err == nil {
		return exec.Command(path, "reboot").Run()
	}
	if path, err := exec.LookPath("reboot"); err == nil {
		return exec.Command(path).Run()
	}
	return fmt.Errorf("neither systemctl nor reboot is available")
}
