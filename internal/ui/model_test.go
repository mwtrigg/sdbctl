package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
)

func newBootTestdata(t *testing.T) string {
	t.Helper()
	esp := t.TempDir()
	entries := filepath.Join(esp, "loader", "entries")
	if err := os.MkdirAll(entries, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(esp, "loader", "loader.conf"),
		[]byte("# managed\nsecure-boot-enroll if-safe\nkeyfile /loader/keys/auto/KEYFILE.key\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entries, "arch.conf"),
		[]byte("title Arch\nlinux /vmlinuz\ninitrd /initrd\noptions root=/dev/sda1 rw\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entries, "windows.conf"),
		[]byte("title Windows\nefi /EFI/Microsoft/Boot/bootmgfw.efi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(esp, "EFI"), 0755); err != nil {
		t.Fatal(err)
	}
	return esp
}

func loadedModel(t *testing.T) *model {
	t.Helper()
	esp := newBootTestdata(t)
	m := New(WithESPPath(esp))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	entries, err := boot.ParseAllEntries(esp)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := boot.ParseLoaderConf(filepath.Join(esp, "loader", "loader.conf"))
	if err != nil {
		t.Fatal(err)
	}
	m.Update(bootLoaded{espPath: esp, loaderConf: cfg, entries: entries, archToken: boot.LocalArchToken()})
	m.Update(espPathsLoaded{paths: []string{"/vmlinuz", "/initrd"}})
	return m
}

func TestViewRendersWithBLSExtras(t *testing.T) {
	m := loadedModel(t)
	out := m.View()
	if !strings.Contains(out, "Arch") || !strings.Contains(out, "Windows") {
		t.Fatalf("entries missing from view:\n%s", out)
	}
	// Unknown-but-preserved loader.conf key must be shown.
	if !strings.Contains(out, "keyfile") {
		t.Errorf("unknown loader key missing:\n%s", out)
	}
}

func TestSetDefaultWritesFullID(t *testing.T) {
	m := loadedModel(t)
	// Select windows.conf entry (index 0 or 1 depends on dir order; find it).
	for i, e := range m.entries {
		if e.ID == "windows" {
			m.selected = i
			m.entry = e
		}
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.loaderConf.Default != "windows.conf" {
		t.Errorf("default after 's': %q, want windows.conf", m.loaderConf.Default)
	}
	data, err := os.ReadFile(filepath.Join(m.espPath, "loader", "loader.conf"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "default windows.conf") {
		t.Errorf("saved loader.conf missing default:\n%s", s)
	}
	// Distro comment and unknown key must survive the 's' write path —
	// this used to rewrite the whole file and drop them.
	if !strings.Contains(s, "# managed") || !strings.Contains(s, "keyfile") {
		t.Errorf("saved loader.conf lost preserved lines:\n%s", s)
	}
	// Star badge now uses glob matching.
	for _, e := range m.entries {
		if e.ID == "windows" && !m.isDefaultEntry(e) {
			t.Error("default badge not shown on windows entry")
		}
	}
}

func TestDiagnosticsOverlayAndArrows(t *testing.T) {
	m := loadedModel(t)
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.showDiag {
		t.Fatal("d did not open diagnostics")
	}
	out := m.View()
	// The windows entry points at a file not on this ESP.
	if !strings.Contains(out, "bootmgfw.efi") {
		t.Errorf("diagnostics missing entry issues:\n%s", out)
	}
	// Any key closes it.
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.showDiag {
		t.Error("diagnostics did not close")
	}

	before := m.focused
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.focused == before {
		t.Error("right arrow did not switch panes")
	}
}

func TestRealDiscoveryAgainstTestdata(t *testing.T) {
	esp := filepath.Join("..", "..", "testdata")
	m := New(WithESPPath(esp))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 50})

	msg := discoverBoot(esp)
	if e, isErr := msg.(errMsg); isErr {
		t.Fatalf("discovery failed: %v", e.err)
	}
	m.Update(msg)

	if m.err != nil {
		t.Fatalf("model error after load: %v", m.err)
	}
	if len(m.entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(m.entries))
	}
	out := m.View()
	for _, want := range []string{"Arch Linux", "Windows Boot Manager", "Loader"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
	// testdata kernel/initrd paths do not exist on disk, so entries must be
	// flagged and diagnostics must render without panicking.
	if len(m.entryWarnings) == 0 {
		t.Error("expected validation warnings for nonexistent testdata files")
	}
	m.showDiag = true
	if !strings.Contains(m.View(), "diagnostics") && !strings.Contains(m.View(), "Diagnostics") {
		t.Error("diagnostics overlay did not render")
	}
}

func TestUndoRestoresPreviousEntryContent(t *testing.T) {
	m := loadedModel(t)

	// Edit the arch entry's title and save it.
	for i, e := range m.entries {
		if e.ID == "arch" {
			m.selected = i
			m.entry = e
		}
	}
	m.enterEntryEditMode()
	m.editInput.SetValue("Arch Renamed") // as if the user typed it
	m.prepareCommit()
	m.finalizeCommit()

	data, err := os.ReadFile(filepath.Join(m.espPath, "loader", "entries", "arch.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Arch Renamed") {
		t.Fatalf("edit not saved:\n%s", data)
	}

	// Undo must restore the previous content.
	m.undo()
	data, err = os.ReadFile(filepath.Join(m.espPath, "loader", "entries", "arch.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Arch Renamed") {
		t.Errorf("undo did not revert the edit:\n%s", data)
	}
	if !strings.Contains(string(data), "title Arch") {
		t.Errorf("undo lost the original title:\n%s", data)
	}
}

func TestDeleteIsUndoable(t *testing.T) {
	m := loadedModel(t)
	for i, e := range m.entries {
		if e.ID == "windows" {
			m.selected = i
			m.entry = e
		}
	}
	m.confirmDelete()

	path := filepath.Join(m.espPath, "loader", "entries", "windows.conf")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("entry was not deleted")
	}

	m.undo()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("undo did not restore the deleted entry: %v", err)
	}
}
