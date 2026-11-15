package cli

import (
	"fmt"
	"strings"
)

// Recommendation is a suggested next step: the command to run, why, and what
// it would do. The MCP server returns these instead of acting, so an agent
// can plan a boot change without being able to perform one.
type Recommendation struct {
	// Intent is a short label for what the recommendation does.
	Intent string `json:"intent"`
	// Command is the command line to run, ready to copy.
	Command string `json:"command"`
	// Why explains the reasoning in one or two sentences.
	Why string `json:"why"`
	// Effect describes what running it would change, if anything.
	Effect string `json:"effect,omitempty"`
	// Mutating is true when running the command changes the system.
	Mutating bool `json:"mutating"`
}

// Recommend turns a stated goal into concrete sdbctl commands, grounded in
// the machine's actual state. It never changes anything itself.
func (a *API) Recommend(goal, entry string) ([]Recommendation, error) {
	g := strings.ToLower(strings.TrimSpace(goal))
	recs := []Recommendation{}

	// The current state, so the advice can be specific.
	st, err := a.Status()
	if err != nil {
		return nil, err
	}

	// Resolve the entry the user means, if they named one and it exists.
	resolved := ""
	if entry != "" {
		if e, err := a.find(entry); err == nil {
			resolved = e.FullID
		}
	}
	if resolved == "" {
		// Fall back to the only entry the goal could refer to.
		for _, e := range st.Entries {
			if strings.Contains(g, strings.ToLower(e.ID)) || strings.Contains(g, strings.ToLower(e.Title)) {
				resolved = e.FullID
				break
			}
		}
	}

	add := func(intent, command, why, effect string, mutating bool) {
		recs = append(recs, Recommendation{
			Intent: intent, Command: command, Why: why, Effect: effect, Mutating: mutating,
		})
	}

	switch {
	case containsAny(g, "default", "persistent", "always boot", "normally boot", "permanent"):
		if resolved == "" {
			add("choose an entry",
				"sdbctl list --ids",
				"No entry was named, so list the available ones first.",
				"", false)
		} else {
			add("set the default",
				fmt.Sprintf("sudo sdbctl default %s", resolved),
				fmt.Sprintf("%s becomes the entry the boot loader picks without being told. "+
					"The full ID including the .conf suffix is what loader.conf matches against.", resolved),
				"rewrites default= in loader.conf", true)
			add("or remember the last choice",
				"sudo sdbctl default @saved",
				"With @saved the boot loader boots whichever entry you picked last in the menu.",
				"rewrites default= in loader.conf", true)
		}

	case containsAny(g, "one-shot", "once", "next boot", "next time", "temporar",
		"this time", "flip", "switch to", "go to", "instead of"):
		if resolved == "" {
			add("choose an entry",
				"sdbctl list --ids",
				"No entry was named, so list the available ones first.",
				"", false)
		} else {
			add("boot once",
				fmt.Sprintf("sudo sdbctl boot %s --reboot", resolved),
				fmt.Sprintf("%s boots once on the next restart and the machine restarts into it.", resolved),
				"sets LoaderEntryOneShot; the boot loader clears it after one boot", true)
		}

	case containsAny(g, "timeout", "menu", "wait", "countdown"):
		add("set the timeout",
			"sudo sdbctl timeout 5",
			fmt.Sprintf("The menu currently waits %s seconds before booting the default entry. "+
				"Use menu-force to always show the menu, or menu-disabled to hide it entirely.",
				orDefault(st.Timeout, "an unset number of")),
			"rewrites timeout= in loader.conf", true)

	case containsAny(g, "undo", "rollback", "roll back", "revert", "restore"):
		if len(st.Backups) == 0 {
			add("nothing to roll back",
				"sdbctl snapshots",
				"No snapshots have been taken yet, so there is nothing to restore.",
				"", false)
		} else {
			add("roll back",
				"sudo sdbctl undo",
				fmt.Sprintf("The most recent snapshot is %s, covering %s.",
					st.Backups[0].ID, strings.Join(st.Backups[0].Files, ", ")),
				"restores the snapshotted file", true)
		}

	case containsAny(g, "delete", "remove", "rm "):
		if resolved == "" {
			add("choose an entry",
				"sdbctl list --ids",
				"No entry was named, so list the available ones first.",
				"", false)
		} else {
			add("delete an entry",
				fmt.Sprintf("sudo sdbctl rm %s", resolved),
				"The file is snapshotted first, so this can be undone with `sdbctl undo`.",
				"deletes loader/entries/"+resolved, true)
		}

	case containsAny(g, "hidden", "missing", "not show", "doesn't show", "why", "broken", "fail"):
		for _, e := range st.Entries {
			if !e.Hidden {
				continue
			}
			add("fix a hidden entry",
				fmt.Sprintf("sdbctl list --json | jq '.entries[] | select(.id==\"%s\")'", e.ID),
				fmt.Sprintf("%s is hidden from the boot menu: %s", e.FullID, strings.Join(e.Issues, "; ")),
				"", false)
		}
		add("see everything",
			"sdbctl check",
			"The full diagnostics: loader binaries and versions, loader.conf problems, and the reason each entry is hidden.",
			"", false)
		add("edit it",
			"sudo sdbctl edit",
			"The editor shows the same information per entry and can fix the fields.",
			"", false)

	case containsAny(g, "windows", "dual boot", "dual-boot"):
		add("add a Windows entry",
			"sudo sdbctl edit",
			"Windows is booted through an `efi` entry pointing at "+
				"/EFI/Microsoft/Boot/bootmgfw.efi. That file has to exist on the same partition "+
				"as the entry: copy the EFI/Microsoft folder from the Windows partition, or let "+
				"`auto-entries` detect it.",
			"", false)

	default:
		add("look around",
			"sdbctl status",
			"Shows what will boot, what is set for next time, and what is broken.",
			"", false)
		add("list the options",
			"sdbctl list",
			"One line per entry; entries systemd-boot will hide are marked with !.",
			"", false)
		add("check for problems",
			"sdbctl check",
			"Full diagnostics, and it exits 1 when something is wrong.",
			"", false)
	}

	return recs, nil
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
