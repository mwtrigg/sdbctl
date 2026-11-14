package boot

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeESP(t *testing.T) string {
	t.Helper()
	esp := t.TempDir()
	for _, p := range []string{"/vmlinuz", "/initramfs-linux.img", "/EFI/Microsoft/Boot/bootmgfw.efi"} {
		full := filepath.Join(esp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return esp
}

func entryIssues(t *testing.T, e *BootEntry, ctx ValidationContext) []string {
	t.Helper()
	verr := ValidateEntry(e, ctx)
	if verr == nil {
		return nil
	}
	return verr.Issues
}

func TestValidateEntryHiddenWithoutImageKey(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp}

	issues := entryIssues(t, &BootEntry{ID: "broken", Title: "Broken"}, ctx)
	if len(issues) != 1 || !strings.Contains(issues[0], "ignores this entry") {
		t.Fatalf("expected ignored-entry issue, got %v", issues)
	}
}

func TestValidateEntryConflictingImageKeys(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp}

	e := &BootEntry{ID: "dual", Linux: "/vmlinuz", Efi: "/EFI/Microsoft/Boot/bootmgfw.efi"}
	issues := entryIssues(t, e, ctx)
	found := false
	for _, i := range issues {
		if strings.Contains(i, "conflicting image keys") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected conflicting image keys issue, got %v", issues)
	}
}

func TestValidateEntryEFIEntryNeedsNoRoot(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp}

	// Windows chainload entry: valid, files present, must produce no issues.
	e := &BootEntry{ID: "windows", Efi: "/EFI/Microsoft/Boot/bootmgfw.efi"}
	if issues := entryIssues(t, e, ctx); issues != nil {
		t.Fatalf("valid efi entry flagged: %v", issues)
	}

	// Missing target file (multi-disk dual boot without copied files) —
	// systemd-boot drops the entry silently; we must say so.
	e2 := &BootEntry{ID: "win2", Efi: "/EFI/Microsoft/Boot/bootmgfw.efi"}
	ctx2 := ValidationContext{ESPPath: t.TempDir()}
	issues := entryIssues(t, e2, ctx2)
	if len(issues) != 1 || !strings.Contains(issues[0], "hidden from boot menu") {
		t.Fatalf("expected hidden-entry issue, got %v", issues)
	}
}

func TestValidateEntryMissingKernelHides(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp}

	e := &BootEntry{ID: "stale", Linux: "/vmlinuz", Options: []string{"root=/dev/sda2 rw"}}
	if issues := entryIssues(t, e, ctx); issues != nil {
		t.Fatalf("clean entry flagged: %v", issues)
	}

	e2 := &BootEntry{ID: "stale", Linux: "/vmlinuz-removed", Options: []string{"root=/dev/sda2 rw"}}
	issues := entryIssues(t, e2, ctx)
	if len(issues) != 1 || !strings.Contains(issues[0], "linux file not found") {
		t.Fatalf("expected missing kernel issue, got %v", issues)
	}
}

func TestValidateEntryMissingRootWarning(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp}

	// The EndeavourOS June-2026 failure mode: entry regenerated without
	// root= falls back to gpt-auto-discovery and can hang the boot.
	e := &BootEntry{ID: "noroot", Linux: "/vmlinuz", Initrds: []string{"/initramfs-linux.img"}}
	issues := entryIssues(t, e, ctx)
	if len(issues) != 1 || !strings.Contains(issues[0], "root=") {
		t.Fatalf("expected missing root= issue, got %v", issues)
	}

	e.Options = []string{"root=UUID=1234 rw"}
	if issues := entryIssues(t, e, ctx); issues != nil {
		t.Fatalf("entry with root= flagged: %v", issues)
	}
}

func TestValidateEntryArchitectureMismatch(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp, Architecture: "x64"}

	e := &BootEntry{ID: "arm", Linux: "/vmlinuz", Options: []string{"root=/dev/sda2"}, Architecture: "aa64"}
	issues := entryIssues(t, e, ctx)
	if len(issues) != 1 || !strings.Contains(issues[0], "hidden from boot menu") {
		t.Fatalf("expected arch-hidden issue, got %v", issues)
	}

	e.Architecture = "X64" // case-insensitive per spec
	if issues := entryIssues(t, e, ctx); issues != nil {
		t.Fatalf("case-insensitive arch flagged: %v", issues)
	}
}

func TestValidateEntryMachineIDMismatch(t *testing.T) {
	esp := writeFakeESP(t)
	ctx := ValidationContext{ESPPath: esp, MachineID: "aabbccddeeff00112233445566778899"}

	e := &BootEntry{ID: "other", Linux: "/vmlinuz", Options: []string{"root=/dev/sda2"},
		MachineID: "99887766554433221100ffeeddccbbaa"}
	issues := entryIssues(t, e, ctx)
	if len(issues) != 1 || !strings.Contains(issues[0], "another OS installation") {
		t.Fatalf("expected machine-id note, got %v", issues)
	}

	e.MachineID = ctx.MachineID
	if issues := entryIssues(t, e, ctx); issues != nil {
		t.Fatalf("matching machine-id flagged: %v", issues)
	}
}

func TestMatchEntryID(t *testing.T) {
	tests := []struct {
		pattern, id string
		want        bool
	}{
		{"arch.conf", "arch.conf", true},
		{"ARCH.CONF", "arch.conf", true}, // case-insensitive
		{"arch", "arch.conf", false},     // suffix matters — classic footgun
		{"arch*", "arch-lts.conf", true}, // glob
		{"linux-6.?0.efi", "linux-6.90.efi", true},
		{"*", "anything.conf", true},
		{"", "arch.conf", false},
	}
	for _, tt := range tests {
		if got := MatchEntryID(tt.pattern, tt.id); got != tt.want {
			t.Errorf("MatchEntryID(%q, %q) = %v, want %v", tt.pattern, tt.id, got, tt.want)
		}
	}
}

func TestCheckDefaultEntry(t *testing.T) {
	entries := []*BootEntry{
		{ID: "arch", FileSuffix: ".conf"},
		{ID: "fedora", FileSuffix: ".conf"},
	}

	if msg := CheckDefaultEntry(&LoaderConfig{Default: "arch.conf"}, entries); msg != "" {
		t.Errorf("valid default flagged: %q", msg)
	}
	if msg := CheckDefaultEntry(&LoaderConfig{Default: "@saved"}, entries); msg != "" {
		t.Errorf("@saved flagged: %q", msg)
	}

	// The "default arch" mistake: no suffix, matches nothing.
	msg := CheckDefaultEntry(&LoaderConfig{Default: "arch"}, entries)
	if msg == "" {
		t.Fatal("expected warning for suffixless default")
	}
	if !strings.Contains(msg, "arch.conf") {
		t.Errorf("expected suffix hint in %q", msg)
	}

	// Glob matching nothing.
	if msg := CheckDefaultEntry(&LoaderConfig{Default: "gone-*"}, entries); msg == "" {
		t.Error("expected warning for non-matching glob")
	}
}

func TestValidateLoaderConfNewKeys(t *testing.T) {
	cfg := &LoaderConfig{SecureBootEnroll: "sometimes"}
	if err := ValidateLoaderConf(cfg); err == nil {
		t.Error("expected invalid secure-boot-enroll to fail")
	}
	cfg.SecureBootEnroll = "if-safe"
	cfg.SecureBootEnrollAction = "reboot"
	if err := ValidateLoaderConf(cfg); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestLoaderVersionFromEFI(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "systemd-bootx64.efi")
	data := make([]byte, 64)
	copy(data, []byte("#### LoaderInfo: systemd-boot 257.5 ####garbage"))
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	if v := loaderVersionFromEFI(p); v != "257.5" {
		t.Errorf("version: got %q", v)
	}

	other := filepath.Join(tmp, "bootmgfw.efi")
	if err := os.WriteFile(other, []byte("MZ nothing here"), 0644); err != nil {
		t.Fatal(err)
	}
	if v := loaderVersionFromEFI(other); v != "" {
		t.Errorf("non-loader binary got version %q", v)
	}
}

func TestStatFold(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "BOOTX64.EFI"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := statFold(tmp, "bootx64.efi"); got == "" {
		t.Error("case-insensitive lookup failed")
	}
	if got := statFold(tmp, "ia64.efi"); got != "" {
		t.Errorf("unexpected match: %q", got)
	}
}

func TestLocalArchTokenMatchesGoArch(t *testing.T) {
	switch runtime.GOARCH {
	case "amd64":
		if LocalArchToken() != "x64" {
			t.Errorf("amd64 token: %q", LocalArchToken())
		}
	case "arm64":
		if LocalArchToken() != "aa64" {
			t.Errorf("arm64 token: %q", LocalArchToken())
		}
	}
}
