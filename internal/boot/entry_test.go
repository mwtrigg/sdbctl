package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEntry(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test-entry.conf")

	content := `title Test Entry
version 5.15.0
machine-id abc123def456
sort-key test
options root=/dev/sda1 rw quiet
linux /test/linux
initrd /test/initrd
initrd /test/initrd-firmware
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}

	if entry.ID != "test-entry" {
		t.Errorf("expected ID 'test-entry', got '%s'", entry.ID)
	}
	if entry.Title != "Test Entry" {
		t.Errorf("expected title 'Test Entry', got '%s'", entry.Title)
	}
	if entry.Version != "5.15.0" {
		t.Errorf("expected version '5.15.0', got '%s'", entry.Version)
	}
	if entry.MachineID != "abc123def456" {
		t.Errorf("expected machine-id 'abc123def456', got '%s'", entry.MachineID)
	}
	if entry.SortKey != "test" {
		t.Errorf("expected sort-key 'test', got '%s'", entry.SortKey)
	}
	if entry.Linux != "/test/linux" {
		t.Errorf("expected linux '/test/linux', got '%s'", entry.Linux)
	}
	if len(entry.Initrds) != 2 {
		t.Fatalf("expected 2 initrds, got %d", len(entry.Initrds))
	}
	if entry.Initrds[0] != "/test/initrd" {
		t.Errorf("expected first initrd '/test/initrd', got '%s'", entry.Initrds[0])
	}
	if entry.Initrds[1] != "/test/initrd-firmware" {
		t.Errorf("expected second initrd '/test/initrd-firmware', got '%s'", entry.Initrds[1])
	}
	if len(entry.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(entry.Options))
	}
	if entry.Options[0] != "root=/dev/sda1 rw quiet" {
		t.Errorf("unexpected option: '%s'", entry.Options[0])
	}
}

func TestParseEntryMinimal(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "minimal.conf")

	content := `title Minimal
linux /vmlinuz
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}

	if entry.Title != "Minimal" {
		t.Errorf("expected title 'Minimal', got '%s'", entry.Title)
	}
	if entry.Linux != "/vmlinuz" {
		t.Errorf("expected linux '/vmlinuz', got '%s'", entry.Linux)
	}
}

func TestParseEntryComments(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "commented.conf")

	content := `# Comment at start
title Test
# Inline comment
linux /vmlinuz
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}

	if entry.Title != "Test" {
		t.Errorf("expected title 'Test', got '%s'", entry.Title)
	}
}

func TestParseEntries(t *testing.T) {
	tmpDir := t.TempDir()
	entriesDir := filepath.Join(tmpDir, "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"entry1.conf": "title Entry 1\nlinux /vmlinuz1\n",
		"entry2.conf": "title Entry 2\nlinux /vmlinuz2\n",
		"entry3.conf": "title Entry 3\nlinux /vmlinuz3\n",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(entriesDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := ParseEntries(entriesDir)
	if err != nil {
		t.Fatalf("ParseEntries failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestParseEntriesEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	entriesDir := filepath.Join(tmpDir, "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseEntries(entriesDir)
	if err != nil {
		t.Fatalf("ParseEntries failed: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseEntriesNonExistent(t *testing.T) {
	entries, err := ParseEntries("/nonexistent/path")
	if err != nil {
		t.Fatalf("ParseEntries should not error for non-existent dir: %v", err)
	}
	if entries != nil {
		t.Error("expected nil for non-existent dir")
	}
}

// TestParseTestdata parses the sample ESP layout shipped in testdata/ — a
// cheap regression fixture covering multi-initrd entries and an "efi"
// chainload entry.
func TestParseTestdata(t *testing.T) {
	esp := filepath.Join("..", "..", "testdata")

	cfg, err := ParseLoaderConf(filepath.Join(esp, "loader", "loader.conf"))
	if err != nil {
		t.Fatalf("sample loader.conf must parse: %v", err)
	}
	if len(cfg.Issues) > 0 {
		t.Errorf("sample loader.conf has issues: %v", cfg.Issues)
	}

	entries, err := ParseAllEntries(esp)
	if err != nil {
		t.Fatalf("sample entries must parse: %v", err)
	}

	var windows *BootEntry
	byID := map[string]*BootEntry{}
	for _, e := range entries {
		byID[e.ID] = e
		if e.Efi != "" {
			windows = e
		}
	}
	if windows == nil {
		t.Fatal("no efi-key entry found in testdata")
	}
	if len(windows.ImageKeys()) != 1 {
		t.Errorf("windows entry image keys: %v", windows.ImageKeys())
	}
	if got := byID["fedora"]; got == nil || len(got.Initrds) != 2 {
		t.Errorf("fedora entry should carry two initrds: %+v", got)
	}
	if !MatchEntryID(cfg.Default, byID["arch"].FullID()) {
		t.Errorf("sample default %q should select the arch entry", cfg.Default)
	}
}

func TestWriteEntry(t *testing.T) {
	entry := &BootEntry{
		Title:      "Test Write",
		Version:    "1.0.0",
		MachineID:  "testmachine123",
		SortKey:    "test",
		Linux:      "/boot/vmlinuz",
		Initrds:    []string{"/boot/initrd.img"},
		Options:    []string{"root=/dev/sda1", "quiet"},
		Devicetree: "/boot/dtb",
	}

	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test.conf")

	if err := entry.Write(entryPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}

	if parsed.Title != entry.Title {
		t.Errorf("title mismatch: got %s, want %s", parsed.Title, entry.Title)
	}
	if parsed.Linux != entry.Linux {
		t.Errorf("linux mismatch: got %s, want %s", parsed.Linux, entry.Linux)
	}
	if len(parsed.Initrds) != len(entry.Initrds) {
		t.Errorf("initrds length mismatch: got %d, want %d", len(parsed.Initrds), len(entry.Initrds))
	}
	if len(parsed.Options) != len(entry.Options) {
		t.Errorf("options length mismatch: got %d, want %d", len(parsed.Options), len(entry.Options))
	}
}

func TestDisplayTitle(t *testing.T) {
	tests := []struct {
		entry    BootEntry
		expected string
	}{
		{entry: BootEntry{Title: "Custom Title"}, expected: "Custom Title"},
		{entry: BootEntry{ID: "fallback-id"}, expected: "fallback-id"},
	}

	for _, tt := range tests {
		result := tt.entry.DisplayTitle()
		if result != tt.expected {
			t.Errorf("DisplayTitle() = %s, want %s", result, tt.expected)
		}
	}
}

func TestParseEntryPreservesRawLines(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "preserve-test.conf")

	content := `# Top-level comment
title My Entry
# Vendor extension
foo bar=baz
linux /vmlinuz
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}

	if entry.Title != "My Entry" {
		t.Errorf("expected title 'My Entry', got '%s'", entry.Title)
	}
	if entry.Linux != "/vmlinuz" {
		t.Errorf("expected linux '/vmlinuz', got '%s'", entry.Linux)
	}

	expectedRaw := []string{
		"# Top-level comment",
		"title My Entry",
		"# Vendor extension",
		"foo bar=baz",
		"linux /vmlinuz",
	}
	if len(entry.RawLines) != len(expectedRaw) {
		t.Fatalf("expected %d raw lines, got %d: %v", len(expectedRaw), len(entry.RawLines), entry.RawLines)
	}
	for i, exp := range expectedRaw {
		if entry.RawLines[i] != exp {
			t.Errorf("raw line %d: expected %q, got %q", i, exp, entry.RawLines[i])
		}
	}
}

func TestWriteEntryRoundTripPreservesRawLines(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "roundtrip.conf")

	original := `# My custom entry
title My Entry
# This comment must survive
grub-users-option somevalue
linux /vmlinuz
# Another trailing comment
`
	if err := os.WriteFile(entryPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}

	// Modify a known field
	entry.Title = "Updated Title"

	outPath := filepath.Join(tmpDir, "output.conf")
	if err := entry.Write(outPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	result, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	expected := `# My custom entry
title Updated Title
# This comment must survive
grub-users-option somevalue
linux /vmlinuz
# Another trailing comment
`
	if string(result) != expected {
		t.Errorf("round-trip output mismatch:\n--- got:\n%s\n--- want:\n%s", string(result), expected)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "atomic-test")

	if err := writeFileAtomic(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", string(data))
	}

	// Write with a different mode and verify new file uses 0644 fallback
	path2 := filepath.Join(tmpDir, "atomic-test-2")
	if err := writeFileAtomic(path2, []byte("content\n"), 0644); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}
	fi, err := os.Stat(path2)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0644 {
		t.Errorf("expected permissions 0644, got %o", fi.Mode().Perm())
	}

	// Overwrite preserving existing permissions
	path3 := filepath.Join(tmpDir, "atomic-test-3")
	if err := os.WriteFile(path3, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path3, []byte("overwritten\n"), 0644); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}
	fi, err = os.Stat(path3)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected permissions preserved as 0600, got %o", fi.Mode().Perm())
	}
	data, err = os.ReadFile(path3)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "overwritten\n" {
		t.Errorf("expected 'overwritten\\n', got %q", string(data))
	}
}

func TestParseEntryEFIKey(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "windows.conf")

	// A chainload entry for Windows, the way users create them for
	// dual-boot (SuperUser/Reddit: "Windows entry does not appear").
	content := `# Created by: sdbctl
title Windows Boot Manager
efi /EFI/Microsoft/Boot/bootmgfw.efi
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}
	if entry.Efi != "/EFI/Microsoft/Boot/bootmgfw.efi" {
		t.Errorf("efi: got %q", entry.Efi)
	}
	if keys := entry.ImageKeys(); len(keys) != 1 || keys[0] != "efi" {
		t.Errorf("ImageKeys: got %v, want [efi]", keys)
	}
	if entry.FullID() != "windows.conf" {
		t.Errorf("FullID: got %q", entry.FullID())
	}
}

func TestParseEntryUKIAndExtraKeys(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "image.conf")

	content := `title My OS
version 1.2.3
uki /myos/1.2.3/img.efi
profile 0
extra /myos/1.2.3/foo.cred
extra /myos/1.2.3/bar.sysext.raw
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("ParseEntry failed: %v", err)
	}
	if entry.UKI != "/myos/1.2.3/img.efi" {
		t.Errorf("uki: got %q", entry.UKI)
	}
	if entry.Profile != "0" {
		t.Errorf("profile: got %q", entry.Profile)
	}
	if len(entry.Extras) != 2 {
		t.Fatalf("extras: got %v", entry.Extras)
	}

	// Round-trip must keep both extra lines and the uki key.
	out := filepath.Join(tmpDir, "out.conf")
	if err := entry.Write(out); err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseEntry(out)
	if err != nil {
		t.Fatal(err)
	}
	if reparsed.UKI != entry.UKI || len(reparsed.Extras) != 2 {
		t.Errorf("round-trip lost uki/extra: %+v", reparsed)
	}
}

func TestWriteRoundTripRemovesClearedFields(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "clear.conf")

	content := `title My Entry
version 6.8.8-arch1-1
options root=UUID=deadbeef rw
linux /vmlinuz
initrd /initramfs-linux.img
initrd /second.img
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatal(err)
	}

	// Clearing fields in the UI sets them to "" — the saved file must lose
	// the corresponding lines, not keep the old values (regression).
	entry.Version = ""
	entry.Options = nil
	entry.Initrds = []string{entry.Initrds[0], ""} // Ctrl+N left empty

	out := filepath.Join(tmpDir, "out.conf")
	if err := entry.Write(out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "version") {
		t.Errorf("cleared version survived:\n%s", s)
	}
	if strings.Contains(s, "options") {
		t.Errorf("cleared options survived:\n%s", s)
	}
	if strings.Contains(s, "second.img") {
		t.Errorf("removed initrd survived:\n%s", s)
	}
	if strings.Count(s, "initrd ") != 1 {
		t.Errorf("expected exactly one initrd line:\n%s", s)
	}
	if !strings.Contains(s, "linux /vmlinuz") || !strings.Contains(s, "title My Entry") {
		t.Errorf("unexpected content:\n%s", s)
	}
}

func TestStripEntrySuffixAndFullID(t *testing.T) {
	if got := StripEntrySuffix("Arch.conf"); got != "Arch" {
		t.Errorf("StripEntrySuffix(Arch.conf) = %q", got)
	}
	if got := StripEntrySuffix("linux-6.9.efi"); got != "linux-6.9" {
		t.Errorf("StripEntrySuffix(linux-6.9.efi) = %q", got)
	}
	if got := StripEntrySuffix("no-suffix"); got != "no-suffix" {
		t.Errorf("StripEntrySuffix(no-suffix) = %q", got)
	}
	uki := &BootEntry{ID: "Fedora", IsUKI: true, FileSuffix: ".uki"}
	if uki.FullID() != "Fedora.uki" {
		t.Errorf("UKI FullID: got %q", uki.FullID())
	}
	conf := &BootEntry{ID: "arch", FileSuffix: ".conf"}
	if conf.FullID() != "arch.conf" {
		t.Errorf("Type1 FullID: got %q", conf.FullID())
	}
}
