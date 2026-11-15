package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Exit codes, documented in the man page.
const (
	ExitOK      = 0
	ExitFailure = 1 // an operation failed, or `check` found problems
	ExitUsage   = 2 // bad command line
)

// Invocation is what Main hands back to package main.
type Invocation struct {
	ExitCode  int
	LaunchTUI bool
	ESPPath   string
}

const usageText = `sdbctl — inspect and control systemd-boot from the command line

Usage:
  sdbctl [verb] [flags] [args]

Verbs:
  (no verb)          show boot status: what will boot, what is broken
  status             the same, explicitly
  list               one line per boot entry ( --ids | --json )
  boot <entry>       boot <entry> once on the next restart ( --reboot )
  default <entry>    set the persistent default entry ( or: default @saved )
  timeout <value>    set the menu timeout (seconds, menu-force, menu-hidden, menu-disabled)
  set <key> <value>  set any loader.conf option ( set <key> "" clears it )
  rm <entry>         delete a boot entry (snapshot first, so it is undoable)
  undo               restore the most recent snapshot
  snapshots          list snapshots
  check              diagnostics; exits 1 when problems were found
  edit               launch the full-screen editor (also: -e)
  mcp                serve the same operations over stdio as an MCP server
  completion <shell> print a shell completion script (bash, zsh, fish)
  version            print the version
  help               this text

Global flags:
  --esp PATH   boot directory to manage (default: /boot, /boot/efi, /efi)
  --json       machine-readable output (status, list, check, and all verbs)
  --dry-run    report what would change, change nothing
  --force      act on an entry that systemd-boot would hide
  --yes        skip confirmation prompts
  --quiet      suppress explanatory prose

Examples:
  sdbctl
  sdbctl boot fedora.conf --reboot
  sdbctl default @saved
  sdbctl list --json | jq -r '.entries[] | select(.hidden) | .fullId'
  sudo sdbctl rm old-kernel.conf && sdbctl undo

systemd-boot's own tools remain authoritative for installing and updating the
boot loader itself: use ` + "`bootctl install`" + ` and ` + "`bootctl update`" + `.
`

// verbSpecs describes the flags each verb accepts. Globals are handled
// separately so they can appear anywhere on the command line.
type verbSpecs struct {
	valueFlags map[string]bool // name -> appears in this verb
	boolFlags  map[string]bool
	minArgs    int
	maxArgs    int
	needsESP   bool
}

var verbSpec = map[string]verbSpecs{
	"status":     {needsESP: true},
	"list":       {boolFlags: map[string]bool{"ids": true}, needsESP: true},
	"boot":       {boolFlags: map[string]bool{"reboot": true}, minArgs: 1, maxArgs: 1, needsESP: true},
	"default":    {minArgs: 1, maxArgs: 1, needsESP: true},
	"timeout":    {minArgs: 1, maxArgs: 1, needsESP: true},
	"set":        {minArgs: 1, maxArgs: 2, needsESP: true},
	"rm":         {minArgs: 1, maxArgs: 1, needsESP: true},
	"undo":       {needsESP: true},
	"snapshots":  {needsESP: true},
	"check":      {needsESP: true},
	"edit":       {},
	"mcp":        {boolFlags: map[string]bool{"allow-write": true}},
	"completion": {minArgs: 1, maxArgs: 1},
	"version":    {},
	"help":       {},
}

// globals collects the flags accepted anywhere.
type globals struct {
	esp    string
	json   bool
	dryRun bool
	force  bool
	yes    bool
	quiet  bool
}

// splitGlobals pulls recognised global flags out of args, returning the
// remainder. Values may use --esp=path or --esp path.
func splitGlobals(args []string, g *globals) ([]string, error) {
	var rest []string
	tail := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		if tail {
			rest = append(rest, a)
			continue
		}
		switch {
		case a == "--":
			tail = true
		case a == "-e":
			rest = append(rest, "edit")
		case strings.HasPrefix(a, "-"):
			name := strings.TrimLeft(a, "-")
			value := ""
			withValue := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				value = name[eq+1:]
				name = name[:eq]
				withValue = true
			}
			switch name {
			case "esp":
				if !withValue {
					if i+1 >= len(args) {
						return nil, fmt.Errorf("--esp needs a path")
					}
					i++
					value = args[i]
				}
				g.esp = value
			case "json":
				g.json = true
			case "dry-run":
				g.dryRun = true
			case "force":
				g.force = true
			case "yes", "y":
				g.yes = true
			case "quiet", "q":
				g.quiet = true
			case "check":
				// Deprecated flag form of `sdbctl check`.
				rest = append([]string{"check"}, rest...)
			case "restore":
				rest = append([]string{"undo"}, rest...)
			case "list-backups":
				rest = append([]string{"snapshots"}, rest...)
			case "version", "V":
				rest = append([]string{"version"}, rest...)
			default:
				// Not a global: leave it for the verb parser, which reports
				// it as unknown if the verb does not accept it either.
				rest = append(rest, a)
			}
		default:
			rest = append(rest, a)
		}
	}
	return rest, nil
}

// Main runs the command line. It never touches a terminal except through the
// writers it is given, which keeps every verb testable.
func Main(args []string, stdout, stderr io.Writer) Invocation {
	g := &globals{}
	rest, err := splitGlobals(args, g)
	if err != nil {
		fmt.Fprintf(stderr, "sdbctl: %v\n", err)
		return Invocation{ExitCode: ExitUsage}
	}

	verb := "status"
	if len(rest) > 0 {
		verb = rest[0]
		rest = rest[1:]
	}

	spec, ok := verbSpec[verb]
	if !ok {
		if strings.HasPrefix(verb, "-") {
			fmt.Fprintf(stderr, "sdbctl: unknown flag %q\n", verb)
		} else {
			fmt.Fprintf(stderr, "sdbctl: unknown verb %q\n", verb)
		}
		fmt.Fprintf(stderr, "\nRun `sdbctl help` for usage.\n")
		return Invocation{ExitCode: ExitUsage}
	}

	// Verb-specific flags.
	var (
		idsOnly    bool
		reboot     bool
		allowWrite bool
	)
	positional, err := parseVerbFlags(verb, rest, &spec, &idsOnly, &reboot, &allowWrite)
	if err != nil {
		fmt.Fprintf(stderr, "sdbctl: %v\n", err)
		return Invocation{ExitCode: ExitUsage}
	}
	if len(positional) < spec.minArgs || len(positional) > spec.maxArgs {
		fmt.Fprintf(stderr, "sdbctl: %s takes %d argument(s), got %d\n",
			verb, spec.maxArgs, len(positional))
		return Invocation{ExitCode: ExitUsage}
	}

	switch verb {
	case "help":
		fmt.Fprint(stdout, usageText)
		return Invocation{ExitCode: ExitOK}
	case "version":
		fmt.Fprintf(stdout, "sdbctl %s\n", Version)
		return Invocation{ExitCode: ExitOK}
	case "completion":
		script, err := completionScript(positional[0])
		if err != nil {
			fmt.Fprintf(stderr, "sdbctl: %v\n", err)
			return Invocation{ExitCode: ExitUsage}
		}
		fmt.Fprint(stdout, script)
		return Invocation{ExitCode: ExitOK}
	case "edit":
		return Invocation{LaunchTUI: true, ESPPath: g.esp, ExitCode: ExitOK}
	case "mcp":
		return Invocation{ExitCode: serveMCP(os.Stdin, stdout, stderr, g, allowWrite)}
	}

	api, err := Resolve(g.esp)
	if err != nil {
		fmt.Fprintf(stderr, "sdbctl: %v\n", err)
		return Invocation{ExitCode: ExitFailure}
	}
	api.DryRun = g.dryRun
	api.AllowHidden = g.force

	switch verb {
	case "status":
		return renderStatus(api, stdout, g, false)
	case "check":
		return renderStatus(api, stdout, g, true)

	case "list":
		entries, err := api.List()
		if err != nil {
			fmt.Fprintf(stderr, "sdbctl: %v\n", err)
			return Invocation{ExitCode: ExitFailure}
		}
		if g.json {
			return emitJSON(stdout, map[string]any{"entries": entries})
		}
		if idsOnly {
			for _, e := range entries {
				fmt.Fprintln(stdout, e.FullID)
			}
			return Invocation{ExitCode: ExitOK}
		}
		writeEntryTable(stdout, entries)
		return Invocation{ExitCode: ExitOK}

	case "boot":
		res, err := api.Boot(positional[0], reboot)
		return afterMutation(api, stdout, stderr, g, res, err)

	case "default":
		res, err := api.SetDefault(positional[0])
		return afterMutation(api, stdout, stderr, g, res, err)

	case "timeout":
		res, err := api.SetKey("timeout", positional[0])
		return afterMutation(api, stdout, stderr, g, res, err)

	case "set":
		key := positional[0]
		value := ""
		if len(positional) > 1 {
			value = positional[1]
		} else {
			// `set <key>` with no value clears it; say so explicitly.
			if !g.yes && stdinIsTerminal() {
				fmt.Fprintf(stderr, "clear %s? [y/N] ", key)
				var answer string
				fmt.Scanln(&answer)
				if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
					return Invocation{ExitCode: ExitOK}
				}
			}
		}
		res, err := api.SetKey(key, value)
		return afterMutation(api, stdout, stderr, g, res, err)

	case "rm":
		if !g.yes && stdinIsTerminal() {
			fmt.Fprintf(stderr, "delete %s? [y/N] ", positional[0])
			var answer string
			fmt.Scanln(&answer)
			if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
				return Invocation{ExitCode: ExitOK}
			}
		}
		res, err := api.Remove(positional[0])
		return afterMutation(api, stdout, stderr, g, res, err)

	case "undo":
		res, err := api.Undo()
		return afterMutation(api, stdout, stderr, g, res, err)

	case "snapshots":
		snaps, err := api.Snapshots()
		if err != nil {
			fmt.Fprintf(stderr, "sdbctl: %v\n", err)
			return Invocation{ExitCode: ExitFailure}
		}
		if g.json {
			return emitJSON(stdout, map[string]any{"snapshots": snaps})
		}
		if len(snaps) == 0 {
			fmt.Fprintln(stdout, "no snapshots")
			return Invocation{ExitCode: ExitOK}
		}
		for _, s := range snaps {
			fmt.Fprintf(stdout, "%s  %s\n", s.ID, strings.Join(s.Files, ", "))
		}
		return Invocation{ExitCode: ExitOK}
	}

	fmt.Fprintf(stderr, "sdbctl: verb %q is not implemented\n", verb)
	return Invocation{ExitCode: ExitUsage}
}

// Version is overridden at build time via ldflags.
var Version = "dev"

func parseVerbFlags(verb string, args []string, spec *verbSpecs,
	idsOnly, reboot, allowWrite *bool) ([]string, error) {

	var positional []string
	tail := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if tail {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			tail = true
			continue
		}
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			return nil, fmt.Errorf("%s does not take a value for --%s", verb, name[:eq])
		}
		switch {
		case verb == "list" && name == "ids":
			*idsOnly = true
		case verb == "boot" && name == "reboot":
			*reboot = true
		case verb == "mcp" && name == "allow-write":
			*allowWrite = true
		default:
			return nil, fmt.Errorf("unknown flag %q for %s", a, verb)
		}
	}
	return positional, nil
}

func renderStatus(api *API, stdout io.Writer, g *globals, check bool) Invocation {
	st, err := api.Status()
	if err != nil {
		fmt.Fprintf(stdout, "sdbctl: %v\n", err)
		return Invocation{ExitCode: ExitFailure}
	}
	if g.json {
		code := ExitOK
		if check && !st.OK {
			code = ExitFailure
		}
		inv := emitJSON(stdout, st)
		inv.ExitCode = code
		return inv
	}
	writeStatus(stdout, st)
	if check {
		if st.OK {
			fmt.Fprintln(stdout, "\nno problems found")
			return Invocation{ExitCode: ExitOK}
		}
		fmt.Fprintf(stdout, "\n%d problem(s) found\n", countProblems(st))
		return Invocation{ExitCode: ExitFailure}
	}
	if !g.quiet && !st.OK {
		fmt.Fprintf(stdout, "\nRun `sdbctl check` for details, or `sdbctl edit` to fix entries.\n")
	}
	return Invocation{ExitCode: ExitOK}
}

func afterMutation(api *API, stdout, stderr io.Writer, g *globals, res *Result, err error) Invocation {
	if err != nil {
		fmt.Fprintf(stderr, "sdbctl: %v\n", err)
		return Invocation{ExitCode: ExitFailure}
	}
	if g.json {
		return emitJSON(stdout, res)
	}
	if res.Planned {
		fmt.Fprintf(stdout, "dry run: %s (nothing changed)\n", res.Summary)
	} else {
		fmt.Fprintf(stdout, "%s\n", res.Summary)
	}
	if res.Entry != nil {
		fmt.Fprintf(stdout, "  entry      %s  %s\n", res.Entry.FullID, res.Entry.Title)
	}
	if res.NextBoot != "" {
		fmt.Fprintf(stdout, "  next boot  %s\n", res.NextBoot)
	}
	if res.StatusLine != "" {
		fmt.Fprintf(stdout, "  %-10s %s\n", "loader.conf", res.StatusLine)
	}
	if res.Removed != "" {
		fmt.Fprintf(stdout, "  removed    %s\n", res.Removed)
	}
	if res.Restored != "" {
		fmt.Fprintf(stdout, "  restored   %s (%s)\n", res.Restored, res.StatusLine)
	}
	if res.Rebooted {
		fmt.Fprintln(stdout, "  rebooting now")
	}
	if res.Backup != "" && !g.quiet {
		fmt.Fprintf(stdout, "  snapshot   %s — roll back with `sdbctl undo`\n", res.Backup)
	}
	if res.Planned && res.Undo != "" {
		fmt.Fprintln(stdout, "  (dry run: no snapshot taken)")
	}
	return Invocation{ExitCode: ExitOK}
}

func emitJSON(w io.Writer, v any) Invocation {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "sdbctl: encoding JSON failed: %v\n", err)
		return Invocation{ExitCode: ExitFailure}
	}
	return Invocation{ExitCode: ExitOK}
}

// writeStatus renders the status report. Text and JSON both come from the
// same Status value, so they cannot drift apart.
func writeStatus(w io.Writer, st *Status) {
	fmt.Fprintf(w, "boot directory  %s\n", st.BootDir)
	if st.Xbootldr {
		fmt.Fprintf(w, "ESP             %s  (XBOOTLDR layout)\n", st.ESPDir)
	}
	if st.Loader.Path != "" {
		fmt.Fprintf(w, "loader          %s  (systemd-boot %s)\n", st.Loader.Path, st.Loader.Version)
	} else {
		fmt.Fprintln(w, "loader          not installed on this partition")
	}
	if st.Loader.PackageVersion != "" {
		fmt.Fprintf(w, "package         systemd-boot %s\n", st.Loader.PackageVersion)
	}
	if st.Default != "" {
		fmt.Fprintf(w, "default         %s\n", st.Default)
	}
	if st.Timeout != "" {
		fmt.Fprintf(w, "timeout         %s\n", st.Timeout)
	}
	if st.OneShot != "" {
		fmt.Fprintf(w, "next boot       %s  (one-shot)\n", st.OneShot)
	}
	if st.LastBooted != "" {
		fmt.Fprintf(w, "last booted     %s\n", st.LastBooted)
	}

	fmt.Fprintln(w)
	writeEntryTable(w, st.Entries)

	problems := st.Problems
	for _, p := range problems {
		fmt.Fprintf(w, "\n! %s\n", p)
	}
	if st.DefaultWarning != "" {
		fmt.Fprintf(w, "\n! %s\n", st.DefaultWarning)
	}
	for _, i := range st.ConfigIssues {
		fmt.Fprintf(w, "\n! %s\n", i)
	}

	hidden := 0
	for _, e := range st.Entries {
		if len(e.Issues) > 0 {
			hidden++
		}
	}
	if hidden > 0 {
		noun := "entries"
		if hidden == 1 {
			noun = "entry"
		}
		fmt.Fprintf(w, "\n%d %s with issues — `sdbctl list` marks them, `sdbctl check` explains them.\n", hidden, noun)
	}
}

func writeEntryTable(w io.Writer, entries []Entry) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "no boot entries")
		return
	}
	for _, e := range entries {
		marker := " "
		switch {
		case e.OneShot:
			marker = ">"
		case e.Default:
			marker = "*"
		case e.Hidden:
			marker = "!"
		case e.LastBooted:
			marker = "+"
		}
		fmt.Fprintf(w, "%s %-24s %-28s %s\n", marker, e.FullID, truncate(e.Title, 28), e.Version)
		if e.Hidden && len(e.Issues) > 0 {
			fmt.Fprintf(w, "    %s\n", e.Issues[0])
		}
	}
}

func countProblems(st *Status) int {
	n := len(st.Problems) + len(st.ConfigIssues)
	if st.DefaultWarning != "" {
		n++
	}
	for _, e := range st.Entries {
		n += len(e.Issues)
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}
