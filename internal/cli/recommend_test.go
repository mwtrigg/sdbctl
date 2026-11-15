package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recommend(t *testing.T, esp, goal, entry string) []Recommendation {
	t.Helper()
	api, err := Resolve(esp)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := api.Recommend(goal, entry)
	if err != nil {
		t.Fatalf("Recommend(%q) failed: %v", goal, err)
	}
	return recs
}

func TestRecommendIntents(t *testing.T) {
	esp := fakeESP(t)

	// Match on the whole recommendation, not just the command line: the
	// reasoning is part of what the caller gets.
	commands := func(recs []Recommendation) string {
		var b strings.Builder
		for _, r := range recs {
			b.WriteString(r.Command + " — " + r.Why + " " + r.Effect + "\n")
		}
		return b.String()
	}

	// Take a snapshot first, so "undo" has something to roll back to.
	if code, _, errb := run(t, esp, "--yes", "rm", "old-kernel"); code != ExitOK {
		t.Fatalf("could not create a snapshot: %s", errb)
	}

	cases := []struct {
		goal   string
		entry  string
		wants  string
		absent string
	}{
		{goal: "boot fedora once", wants: "sudo sdbctl boot fedora.conf --reboot"},
		{goal: "boot the other distro next time", entry: "fedora", wants: "sdbctl boot fedora.conf"},
		{goal: "make fedora the default", wants: "sudo sdbctl default fedora.conf"},
		{goal: "always boot fedora", entry: "fedora", wants: "sdbctl default fedora.conf"},
		{goal: "increase the menu timeout", wants: "sudo sdbctl timeout 5"},
		{goal: "undo my last change", wants: "sudo sdbctl undo"},
		{goal: "delete the lts kernel entry", entry: "arch-lts", wants: "sudo sdbctl rm arch-lts.conf"},
		{goal: "why is my entry not showing up", wants: "sdbctl check"},
		{goal: "set up windows dual boot", wants: "efi"},
		{goal: "hello", wants: "sdbctl status"},
	}

	for _, tc := range cases {
		out := commands(recommend(t, esp, tc.goal, tc.entry))
		if !strings.Contains(out, tc.wants) {
			t.Errorf("goal %q: expected a recommendation containing %q, got:\n%s", tc.goal, tc.wants, out)
		}
	}
}

func TestRecommendGroundsInRealState(t *testing.T) {
	esp := fakeESP(t)

	// The hidden entries must be named, with the reason.
	out := ""
	for _, r := range recommend(t, esp, "why is my entry hidden", "") {
		out += r.Command + " " + r.Why + "\n"
	}
	for _, want := range []string{"old-kernel.conf", "windows.conf"} {
		if !strings.Contains(out, want) {
			t.Errorf("hidden entry %s not mentioned:\n%s", want, out)
		}
	}

	// The timeout advice quotes the timeout that is actually configured.
	out = ""
	for _, r := range recommend(t, esp, "change the timeout", "") {
		out += r.Why + "\n"
	}
	if !strings.Contains(out, "5 seconds") {
		t.Errorf("timeout advice should quote the current timeout:\n%s", out)
	}
}

func TestRecommendChangesNothing(t *testing.T) {
	esp := fakeESP(t)

	before := map[string]string{}
	for _, rel := range []string{"loader/loader.conf", "loader/entries/fedora.conf"} {
		data, err := os.ReadFile(filepath.Join(esp, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		before[rel] = string(data)
	}

	for _, goal := range []string{
		"boot fedora once", "make fedora the default", "increase the timeout",
		"undo my change", "delete fedora",
	} {
		recommend(t, esp, goal, "")
	}

	for rel, want := range before {
		data, err := os.ReadFile(filepath.Join(esp, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s disappeared: %v", rel, err)
		}
		if string(data) != want {
			t.Errorf("%s changed while only recommending", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(esp, "loader", "sdbctl-backups")); err == nil {
		t.Error("recommending must not take snapshots")
	}
}
