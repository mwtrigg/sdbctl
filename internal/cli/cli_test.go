package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwtrigg/sdbctl/internal/boot"
)

// fakeESP builds a boot directory exercising the interesting cases: healthy
// entries, an entry whose kernel is gone, a chainload entry whose EFI file is
// missing, a UKI, and a loader.conf carrying a comment plus an option this
// tool does not know about.
func fakeESP(t *testing.T) string {
	t.Helper()
	esp := t.TempDir()
	entries := filepath.Join(esp, "loader", "entries")
	for _, d := range []string{entries, filepath.Join(esp, "EFI", "Linux")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	write(t, filepath.Join(esp, "loader", "loader.conf"),
		"# managed by the installer\nsecure-boot-enroll if-safe\nkeyfile /loader/keys/auto/KEYFILE.key\ntimeout 5\ndefault arch.conf\n")
	write(t, filepath.Join(esp, "vmlinuz-linux"), "kernel")
	write(t, filepath.Join(esp, "initramfs-linux.img"), "initrd")
	write(t, filepath.Join(esp, "vmlinuz-fedora"), "kernel")
	write(t, filepath.Join(entries, "arch.conf"),
		"title Arch Linux\nlinux /vmlinuz-linux\ninitrd /initramfs-linux.img\noptions root=UUID=dead-beef rw\n")
	write(t, filepath.Join(entries, "arch-lts.conf"),
		"title Arch Linux (LTS)\nlinux /vmlinuz-lts\noptions root=UUID=dead-beef rw\n")
	// vmlinuz-lts deliberately missing would hide it; keep it healthy.
	write(t, filepath.Join(esp, "vmlinuz-lts"), "kernel")
	write(t, filepath.Join(entries, "fedora.conf"),
		"title Fedora Linux 42\nlinux /vmlinuz-fedora\noptions root=UUID=cafe rw\n")
	write(t, filepath.Join(entries, "old-kernel.conf"),
		"title Arch Linux (old)\nlinux /vmlinuz-gone\noptions root=UUID=dead-beef rw\n")
	write(t, filepath.Join(entries, "windows.conf"),
		"title Windows Boot Manager\nefi /EFI/Microsoft/Boot/bootmgfw.efi\n")
	write(t, filepath.Join(esp, "EFI", "Linux", "ubuntu-26.04.efi"), "uki")

	// A boot loader binary so version-aware diagnostics have something to
	// read; matches the packaged one when this machine has it, so `check` can
	// reach a clean state. bootctl install writes both the firmware entry and
	// the fallback, so the fixture does too.
	installLoaderBinary(t, esp)
	return esp
}

// packagedLoader is where the distro ships the boot loader binary that
// `bootctl update` would install.
const packagedLoader = "/usr/lib/systemd/boot/efi/systemd-bootx64.efi"

func installLoaderBinary(t *testing.T, esp string) {
	t.Helper()
	var data []byte
	if raw, err := os.ReadFile(packagedLoader); err == nil {
		data = raw
	} else {
		data = []byte("MZ#### LoaderInfo: systemd-boot 259.1 ####junk")
	}
	writeBytes(t, filepath.Join(esp, "EFI", "systemd", "systemd-bootx64.efi"), data)
	writeBytes(t, filepath.Join(esp, "EFI", "BOOT", "BOOTX64.EFI"), data)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	writeBytes(t, path, []byte(content))
}

func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// run drives Main with an isolated efivarfs, so the one-shot paths are
// exercised for real instead of being skipped as unavailable.
func run(t *testing.T, esp string, args ...string) (int, string, string) {
	t.Helper()
	vars := filepath.Join(t.TempDir(), "efivars")
	if err := os.MkdirAll(vars, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SDBCTL_EFIVARS", vars)

	var out, errb bytes.Buffer
	inv := Main(append([]string{"--esp", esp}, args...), &out, &errb)
	return inv.ExitCode, out.String(), errb.String()
}

func TestStatusTextAndJSON(t *testing.T) {
	esp := fakeESP(t)

	code, out, _ := run(t, esp, "status")
	if code != ExitOK {
		t.Fatalf("status exit=%d out=%s", code, out)
	}
	for _, want := range []string{"boot directory", "Arch Linux", "Fedora Linux 42"} {
		if !strings.Contains(out, want) {
			t.Errorf("status text missing %q:\n%s", want, out)
		}
	}

	code, out, _ = run(t, esp, "--json", "status")
	if code != ExitOK {
		t.Fatalf("json status exit=%d", code)
	}
	var st Status
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("status JSON does not decode: %v\n%s", err, out)
	}
	if len(st.Entries) != 6 {
		t.Errorf("expected 6 entries, got %d: %+v", len(st.Entries), st.Entries)
	}
	// old-kernel (missing linux) and windows (missing efi) are hidden;
	// the UKI is discovered but not analysed.
	if st.HiddenEntries != 2 {
		t.Errorf("expected 2 hidden entries, got %d", st.HiddenEntries)
	}
	if st.Default != "arch.conf" {
		t.Errorf("default: %q", st.Default)
	}
	if st.OK {
		t.Error("status should not be OK while entries are hidden")
	}
	for _, e := range st.Entries {
		if e.ID == "ubuntu-26.04" && !e.UkiImage {
			t.Error("UKI not flagged as an image entry")
		}
	}
}

func TestListIDsIsScriptable(t *testing.T) {
	esp := fakeESP(t)
	code, out, _ := run(t, esp, "list", "--ids")
	if code != ExitOK {
		t.Fatalf("list --ids exit=%d", code)
	}
	ids := strings.Fields(out)
	if len(ids) != 6 {
		t.Fatalf("expected 6 ids, got %v", ids)
	}
	// Full IDs, because that is what default= and one-shot need.
	for _, want := range []string{"arch.conf", "windows.conf", "ubuntu-26.04.efi"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %v", want, ids)
		}
	}
}

func TestDefaultWritesFullIDAndPreservesUnknownKeys(t *testing.T) {
	esp := fakeESP(t)

	code, out, errb := run(t, esp, "default", "fedora")
	if code != ExitOK {
		t.Fatalf("default failed: code=%d out=%s err=%s", code, out, errb)
	}

	data, err := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "default fedora.conf") {
		t.Errorf("default must carry the suffix or systemd-boot matches nothing:\n%s", s)
	}
	for _, want := range []string{"# managed by the installer", "keyfile", "secure-boot-enroll if-safe"} {
		if !strings.Contains(s, want) {
			t.Errorf("loader.conf lost %q:\n%s", want, s)
		}
	}
	if _, err := os.Stat(filepath.Join(esp, "loader", "sdbctl-backups")); err != nil {
		t.Errorf("no snapshot taken: %v", err)
	}
	if !strings.Contains(out, "sdbctl undo") {
		t.Errorf("should advertise undo:\n%s", out)
	}
}

func TestDefaultSavedIsVerbatim(t *testing.T) {
	esp := fakeESP(t)
	code, _, errb := run(t, esp, "default", "@saved")
	if code != ExitOK {
		t.Fatalf("default @saved failed: %s", errb)
	}
	data, _ := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))
	if !strings.Contains(string(data), "default @saved") {
		t.Errorf("@saved not stored:\n%s", data)
	}
}

func TestBootSetsOneShotWithSuffix(t *testing.T) {
	esp := fakeESP(t)

	code, out, errb := run(t, esp, "boot", "fedora")
	if code != ExitOK {
		t.Fatalf("boot failed: %d %s %s", code, out, errb)
	}
	got, err := oneShot(t)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fedora.conf" {
		t.Errorf("LoaderEntryOneShot = %q, want fedora.conf", got)
	}
}

func TestBootAcceptsUKI(t *testing.T) {
	esp := fakeESP(t)
	code, _, errb := run(t, esp, "boot", "ubuntu-26.04")
	if code != ExitOK {
		t.Fatalf("booting a UKI should work: %d %s", code, errb)
	}
	got, _ := oneShot(t)
	if got != "ubuntu-26.04.efi" {
		t.Errorf("one-shot = %q, want ubuntu-26.04.efi", got)
	}
}

func TestBootRefusesHiddenEntryWithoutForce(t *testing.T) {
	esp := fakeESP(t)

	code, _, errb := run(t, esp, "boot", "old-kernel")
	if code == ExitOK {
		t.Fatal("booting a hidden entry without --force should fail")
	}
	if !strings.Contains(errb, "--force") {
		t.Errorf("error should mention --force: %s", errb)
	}

	// With --force it is allowed, and says what is wrong with it.
	code, out, errb2 := run(t, esp, "--force", "boot", "old-kernel")
	if code != ExitOK {
		t.Fatalf("--force boot failed: %s", errb2)
	}
	got, _ := oneShot(t)
	if got != "old-kernel.conf" {
		t.Errorf("one-shot = %q", got)
	}
	_ = out
}

func TestSpecResolution(t *testing.T) {
	esp := fakeESP(t)

	// Exact ID, no suffix: unique.
	if code, _, errb := run(t, esp, "default", "fedora"); code != ExitOK {
		t.Errorf("bare id should resolve: %s", errb)
	}
	// Glob matching two entries: refused with the candidates listed.
	if code, _, errb := run(t, esp, "default", "arch*"); code != ExitFailure ||
		!strings.Contains(errb, "matches 2 entries") {
		t.Errorf("\"arch*\" should be ambiguous: code=%d err=%s", code, errb)
	}
	// Unknown: clear error.
	if code, _, errb := run(t, esp, "default", "nosuch"); code != ExitFailure ||
		!strings.Contains(errb, "no boot entry matches") {
		t.Errorf("unknown entry should be reported: code=%d err=%s", code, errb)
	}
}

func TestSetKeyValidation(t *testing.T) {
	esp := fakeESP(t)

	if code, _, errb := run(t, esp, "set", "console-mode", "banana"); code != ExitFailure ||
		!strings.Contains(errb, "console-mode") {
		t.Errorf("bad console-mode should be refused: %d %s", code, errb)
	}
	if code, _, errb := run(t, esp, "set", "editor", "no"); code != ExitOK {
		t.Errorf("editor=no failed: %d %s", code, errb)
	}
	data, _ := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))
	if !strings.Contains(string(data), "editor no") {
		t.Errorf("editor not written:\n%s", data)
	}
	if code, _, errb := run(t, esp, "set", "not-a-key", "x"); code != ExitFailure ||
		!strings.Contains(errb, "unknown loader.conf key") {
		t.Errorf("unknown key should be refused: %d %s", code, errb)
	}
}

func TestSetKeyClears(t *testing.T) {
	esp := fakeESP(t)
	if code, _, errb := run(t, esp, "--yes", "set", "timeout", ""); code != ExitOK {
		t.Fatalf("clearing timeout failed: %s", errb)
	}
	data, _ := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))
	if strings.Contains(string(data), "timeout ") {
		t.Errorf("timeout line should be gone:\n%s", data)
	}
}

func TestTimeoutVerb(t *testing.T) {
	esp := fakeESP(t)
	if code, _, errb := run(t, esp, "timeout", "menu-force"); code != ExitOK {
		t.Fatalf("timeout failed: %s", errb)
	}
	data, _ := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))
	if !strings.Contains(string(data), "timeout menu-force") {
		t.Errorf("timeout not written:\n%s", data)
	}
}

func TestRemoveThenUndo(t *testing.T) {
	esp := fakeESP(t)
	path := filepath.Join(esp, "loader", "entries", "old-kernel.conf")

	code, out, errb := run(t, esp, "--yes", "rm", "old-kernel")
	if code != ExitOK {
		t.Fatalf("rm failed: %d %s %s", code, out, errb)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("entry still exists")
	}

	code, out, errb = run(t, esp, "undo")
	if code != ExitOK {
		t.Fatalf("undo failed: %d %s %s", code, out, errb)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("undo did not restore the entry: %v", err)
	}
	if !strings.Contains(string(data), "title Arch Linux (old)") {
		t.Errorf("restored content wrong:\n%s", data)
	}
}

func TestRemoveRefusesUKI(t *testing.T) {
	esp := fakeESP(t)
	code, _, errb := run(t, esp, "--yes", "rm", "ubuntu-26.04")
	if code == ExitOK {
		t.Fatal("removing a UKI through this tool should be refused")
	}
	if !strings.Contains(errb, "package manager") {
		t.Errorf("should explain where UKIs belong: %s", errb)
	}
}

func TestSnapshotsListsThem(t *testing.T) {
	esp := fakeESP(t)
	if _, _, errb := run(t, esp, "--yes", "rm", "old-kernel"); errb != "" {
		t.Fatalf("rm failed: %s", errb)
	}
	code, out, _ := run(t, esp, "snapshots")
	if code != ExitOK || !strings.Contains(out, "loader/entries/old-kernel.conf") {
		t.Errorf("snapshots should list the deletion:\n%s", out)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	esp := fakeESP(t)
	before, _ := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))

	code, out, errb := run(t, esp, "--dry-run", "default", "fedora")
	if code != ExitOK {
		t.Fatalf("dry-run failed: %d %s", code, errb)
	}
	after, _ := os.ReadFile(filepath.Join(esp, "loader", "loader.conf"))
	if string(before) != string(after) {
		t.Errorf("dry run modified loader.conf:\n%s\n---\n%s", before, after)
	}
	if !strings.Contains(out, "dry run") {
		t.Errorf("dry run should say so:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(esp, "loader", "sdbctl-backups")); err == nil {
		t.Error("dry run should not create snapshots")
	}
}

func TestCheckExitCode(t *testing.T) {
	esp := fakeESP(t)
	if code, _, _ := run(t, esp, "check"); code != ExitFailure {
		t.Errorf("check should fail while entries are hidden, got %d", code)
	}

	// Repair the same ESP: the two hidden entries get their files back.
	write(t, filepath.Join(esp, "vmlinuz-gone"), "kernel")
	write(t, filepath.Join(esp, "EFI", "Microsoft", "Boot", "bootmgfw.efi"), "MZ")
	installLoaderBinary(t, esp)

	code, out, _ := run(t, esp, "check")
	if code != ExitOK {
		t.Errorf("a repaired ESP should pass check: %d\n%s", code, out)
	}
	if !strings.Contains(out, "no problems found") {
		t.Errorf("check should say so:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	esp := fakeESP(t)

	if code, _, errb := run(t, esp, "nonsense"); code != ExitUsage ||
		!strings.Contains(errb, "unknown verb") {
		t.Errorf("unknown verb: code=%d %s", code, errb)
	}
	if code, _, errb := run(t, esp, "boot"); code != ExitUsage ||
		!strings.Contains(errb, "argument") {
		t.Errorf("missing argument: code=%d %s", code, errb)
	}
	if code, _, errb := run(t, esp, "--wat"); code != ExitUsage ||
		!strings.Contains(errb, "unknown flag") {
		t.Errorf("unknown flag: code=%d %s", code, errb)
	}
	if code, _, errb := run(t, esp, "boot", "arch", "--nonsense"); code != ExitUsage {
		t.Errorf("unknown verb flag should fail: %d %s", code, errb)
	}
}

func TestCommandSurfaceIsCanonical(t *testing.T) {
	esp := fakeESP(t)

	// The verbs are the only interface: the old flag forms are gone, so
	// there is one obvious way to do each thing.
	if code, _, errb := run(t, esp, "--check"); code != ExitUsage ||
		!strings.Contains(errb, "unknown flag") {
		t.Errorf("--check should not be a flag any more: code=%d %s", code, errb)
	}
	if code, _, _ := run(t, esp, "check"); code != ExitFailure {
		t.Errorf("check should report the problems it finds: %d", code)
	}
	if code, out, _ := run(t, esp, "--version"); code != ExitOK || !strings.Contains(out, "sdbctl") {
		t.Errorf("--version: %d %q", code, out)
	}
}

func TestHelpAndCompletion(t *testing.T) {
	var out bytes.Buffer
	inv := Main([]string{"help"}, &out, &bytes.Buffer{})
	if inv.ExitCode != ExitOK || !strings.Contains(out.String(), "sdbctl boot") {
		t.Errorf("help text wrong:\n%s", out.String())
	}

	for _, sh := range []string{"bash", "zsh", "fish"} {
		var o bytes.Buffer
		inv := Main([]string{"completion", sh}, &o, &bytes.Buffer{})
		if inv.ExitCode != ExitOK || !strings.Contains(o.String(), "sdbctl") {
			t.Errorf("completion %s failed", sh)
		}
	}
	var o bytes.Buffer
	if inv := Main([]string{"completion", "csh"}, &o, &bytes.Buffer{}); inv.ExitCode != ExitUsage {
		t.Error("unsupported shell should fail usage")
	}
}

// oneShot reads back LoaderEntryOneShot through the same API the CLI used;
// run() already pointed SDBCTL_EFIVARS at the fixture directory.
func oneShot(t *testing.T) (string, error) {
	t.Helper()
	return boot.GetOneShot()
}

func TestRecommendVerb(t *testing.T) {
	esp := fakeESP(t)

	code, out, errb := run(t, esp, "recommend", "boot fedora once")
	if code != ExitOK {
		t.Fatalf("recommend failed: %d %s", code, errb)
	}
	if !strings.Contains(out, "sdbctl boot fedora.conf") {
		t.Errorf("recommendation should name the command:\n%s", out)
	}
	if !strings.Contains(out, "effect:") {
		t.Errorf("recommendation should say what it would change:\n%s", out)
	}

	// JSON form, for agents.
	code, out, _ = run(t, esp, "--json", "recommend", "make fedora the default")
	if code != ExitOK {
		t.Fatalf("json recommend failed: %d", code)
	}
	var payload struct {
		Recommendations []Recommendation `json:"recommendations"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("recommend JSON does not decode: %v\n%s", err, out)
	}
	if len(payload.Recommendations) == 0 {
		t.Error("no recommendations returned")
	}
	for _, r := range payload.Recommendations {
		if r.Command == "" {
			t.Errorf("recommendation without a command: %+v", r)
		}
	}
}
