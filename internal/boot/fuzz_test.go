package boot

import (
	"os"
	"path/filepath"
	"testing"
)

// These targets treat the parsers and their round-trip writers as the risky
// surface they are: sdboot-tui writes the files the firmware reads on the next
// boot, so a panic, a crash-on-garbage, or a writer that is not idempotent can
// turn an unrelated edit into an unbootable machine.
//
// Run the seed corpus (this is what CI executes):
//     go test ./internal/boot/
// Run the actual fuzzer locally, for as long as you like:
//     go test ./internal/boot/ -run '^$' -fuzz FuzzParseEntry        -fuzztime 60s
//     go test ./internal/boot/ -run '^$' -fuzz FuzzParseLoaderConf   -fuzztime 60s
//     go test ./internal/boot/ -run '^$' -fuzz FuzzMatchEntryID      -fuzztime 30s

func readOrEmpty(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func FuzzParseEntry(f *testing.F) {
	seeds := []string{
		"title Arch Linux\nlinux /vmlinuz\ninitrd /initrd\noptions root=/dev/sda1 rw\n",
		"# comment\ntitle X\n\nefi /EFI/Microsoft/Boot/bootmgfw.efi\n",
		"linux /a\nlinux /b\ntitle  double  space\n",
		"devicetree-overlay /a.dtbo /b.dtbo\narchitecture x64\n",
		"uki /img.efi\nprofile 1\nextra /foo.cred\nextra /bar.sysext.raw\n",
		"unknown-key whatever\nfoo\n",
		"\x00\n\r\r\n\ttitle tab\n",
		"machine-id 6a9857a393724b7a981ebb5b8495b9ea\n",
		"",
		"\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		in := filepath.Join(dir, "in.conf")
		if err := os.WriteFile(in, data, 0644); err != nil {
			t.Fatal(err)
		}

		first, err := ParseEntry(in)
		if err != nil {
			// Malformed input must be reported as an error, never a panic.
			return
		}

		// Whatever we accept has to survive being read back and rewritten:
		// the save path writes the file again after every later edit.
		out1 := filepath.Join(dir, "out1.conf")
		if err := first.Write(out1); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		second, err := ParseEntry(out1)
		if err != nil {
			t.Fatalf("our own output does not parse: %v\n---\n%s", err, readOrEmpty(t, out1))
		}
		out2 := filepath.Join(dir, "out2.conf")
		if err := second.Write(out2); err != nil {
			t.Fatalf("second write failed: %v", err)
		}

		a, b := readOrEmpty(t, out1), readOrEmpty(t, out2)
		if string(a) != string(b) {
			t.Fatalf("round-trip is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", a, b)
		}

		// An accepted entry must never lose the keys it recognised.
		if first.Linux != second.Linux || first.Efi != second.Efi ||
			first.UKI != second.UKI || first.UKIUrl != second.UKIUrl ||
			first.Title != second.Title || len(first.Initrds) != len(second.Initrds) ||
			len(first.Extras) != len(second.Extras) {
			t.Fatalf("fields changed across the round-trip:\n%+v\n%+v", first, second)
		}
	})
}

func FuzzParseLoaderConf(f *testing.F) {
	seeds := []string{
		"timeout 5\ndefault arch.conf\neditor yes\n",
		"# distro managed\nsecure-boot-enroll if-safe\nkeyfile /loader/keys/auto/KEYFILE.key\n",
		"timeout menu-disabled\ndefault @saved\n",
		"editor maybe\nbeep 1\nauto-reboot off\n",
		"garbage-line\n\n   \n",
		"preferred fedora-*.conf\nreboot-on-error auto\nlog-level debug\n",
		"\x00\n\r\nconsole-mode\tmax\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		in := filepath.Join(dir, "loader.conf")
		if err := os.WriteFile(in, data, 0644); err != nil {
			t.Fatal(err)
		}

		first, err := ParseLoaderConf(in)
		if err != nil {
			return // unreadable file: reported, not fatal
		}

		out1 := filepath.Join(dir, "out1.conf")
		if err := first.Write(out1); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		second, err := ParseLoaderConf(out1)
		if err != nil {
			t.Fatalf("our own output does not parse: %v\n---\n%s", err, readOrEmpty(t, out1))
		}
		out2 := filepath.Join(dir, "out2.conf")
		if err := second.Write(out2); err != nil {
			t.Fatalf("second write failed: %v", err)
		}

		a, b := readOrEmpty(t, out1), readOrEmpty(t, out2)
		if string(a) != string(b) {
			t.Fatalf("round-trip is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", a, b)
		}

		// Unknown keys must never be dropped: distro configs carry options
		// this tool does not know about.
		if len(first.Extras) != len(second.Extras) {
			t.Fatalf("unknown keys lost: %v vs %v", first.Extras, second.Extras)
		}
		for i := range first.Extras {
			if first.Extras[i] != second.Extras[i] {
				t.Fatalf("unknown key changed: %v vs %v", first.Extras[i], second.Extras[i])
			}
		}
	})
}

func FuzzMatchEntryID(f *testing.F) {
	seeds := [][2]string{
		{"arch.conf", "arch.conf"},
		{"*", "anything.conf"},
		{"linux-6.?0.efi", "linux-6.90.efi"},
		{"[a-z]*.conf", "arch.conf"},
		{"", "arch.conf"},
		{"\\*.conf", "x.conf"},
		{"a[", "a["},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}

	f.Fuzz(func(t *testing.T, pattern, id string) {
		// Must not panic; filepath.Match may legitimately report a bad
		// pattern, which MatchEntryID treats as "no match".
		_ = MatchEntryID(pattern, id)
	})
}

// TestRoundTripFixedPoint pins the invariant the fuzzer found broken: for any
// input the parser accepts, writing it must produce a file that, when parsed
// and written again, comes out byte-identical. Whitespace-only files used to
// grow a newline on every save.
func TestRoundTripFixedPoint(t *testing.T) {
	inputs := []string{
		"\n",
		"\n  ",
		"  \n\t\n",
		"\n\n\n",
		"title X\n\n\n",
		"title X\nlinux /vmlinuz\n\n   \n\n",
		"timeout 5\n\n  \n",
	}

	for _, in := range inputs {
		dir := t.TempDir()
		p := filepath.Join(dir, "e.conf")
		if err := os.WriteFile(p, []byte(in), 0644); err != nil {
			t.Fatal(err)
		}
		e, err := ParseEntry(p)
		if err != nil {
			t.Errorf("entry %q rejected: %v", in, err)
			continue
		}
		out1 := filepath.Join(dir, "e1.conf")
		if err := e.Write(out1); err != nil {
			t.Fatal(err)
		}
		e2, err := ParseEntry(out1)
		if err != nil {
			t.Errorf("reparse of %q output failed: %v", in, err)
			continue
		}
		out2 := filepath.Join(dir, "e2.conf")
		if err := e2.Write(out2); err != nil {
			t.Fatal(err)
		}
		if a, b := readOrEmpty(t, out1), readOrEmpty(t, out2); string(a) != string(b) {
			t.Errorf("entry round-trip of %q not a fixed point:\nfirst=%q\nsecond=%q", in, a, b)
		}

		// Same for loader.conf.
		lp := filepath.Join(dir, "l.conf")
		if err := os.WriteFile(lp, []byte(in), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := ParseLoaderConf(lp)
		if err != nil {
			t.Errorf("loader %q rejected: %v", in, err)
			continue
		}
		l1 := filepath.Join(dir, "l1.conf")
		if err := cfg.Write(l1); err != nil {
			t.Fatal(err)
		}
		cfg2, err := ParseLoaderConf(l1)
		if err != nil {
			t.Errorf("loader reparse of %q output failed: %v", in, err)
			continue
		}
		l2 := filepath.Join(dir, "l2.conf")
		if err := cfg2.Write(l2); err != nil {
			t.Fatal(err)
		}
		if a, b := readOrEmpty(t, l1), readOrEmpty(t, l2); string(a) != string(b) {
			t.Errorf("loader round-trip of %q not a fixed point:\nfirst=%q\nsecond=%q", in, a, b)
		}
	}
}
